package apiv1

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	v1 "kun-galgame-api/internal/apiv1"
	"kun-galgame-api/internal/apiv1/collect"
	"kun-galgame-api/internal/apiv1/repr"
	"kun-galgame-api/internal/infrastructure/markdown"
	"kun-galgame-api/internal/topic/model"
	"kun-galgame-api/internal/topic/repository"
	"kun-galgame-api/pkg/problem"

	"gorm.io/gorm"
)

const MaxTopicDraftsPerUser = 30

type Drafts struct {
	drafts *repository.TopicDraftRepository
}

func NewDrafts(drafts *repository.TopicDraftRepository) *Drafts {
	return &Drafts{drafts: drafts}
}

func (d *Drafts) ready() *problem.Problem {
	if d == nil || d.drafts == nil || d.drafts.DB() == nil {
		return problem.Internal(errUnconfigured)
	}
	return nil
}

func draftLimitReached(limit int) *problem.Problem {
	p := problem.New(problem.CodeDraftLimitReached,
		"The caller already holds the maximum number of drafts. Continued saves update one draft, so the only way to hit the cap is to start that many separate posts; delete one to make room.")
	p.SetExtension("limit", limit)
	return p
}

func draftRevisionConflict(currentRevision int) *problem.Problem {
	p := problem.New(problem.CodeDraftRevisionConflict,
		"The draft was written by another request after this request loaded it.")
	p.SetExtension("current_revision", currentRevision)
	return p
}

type listTopicDraftsInput struct {
	collect.Page
}

type listTopicDraftsOutput struct {
	Body repr.List[TopicDraftSummary]
}

type topicDraftInput struct {
	DraftID string `path:"draft_id" pattern:"^[1-9][0-9]{0,18}$" maxLength:"19" doc:"Draft id."`
}

type topicDraftOutput struct {
	Body TopicDraft
}

type createTopicDraftInput struct {
	Body TopicDraftCreate
}

type createTopicDraftOutput struct {
	Location string `header:"Location" format:"uri-reference" maxLength:"64" doc:"Absolute path of the new draft, such as /api/v1/me/topic-drafts/412."`
	Body     TopicDraft
}

type updateTopicDraftInput struct {
	DraftID string `path:"draft_id" pattern:"^[1-9][0-9]{0,18}$" maxLength:"19" doc:"Draft id."`
	Body    TopicDraftUpdate
}

type updateTopicDraftOutput struct {
	Body TopicDraft
}

type deleteTopicDraftOutput struct{}

const draftSort = "updated_desc"

func draftFingerprint(userID int) string {
	return collect.Fingerprint("topic_drafts", strconv.Itoa(userID))
}

func parseDraftPos(keys []string) (*repository.DraftKeysetPos, *problem.Problem) {
	if keys == nil {
		return nil, nil
	}
	if len(keys) != 2 {
		return nil, invalidCursor()
	}
	updated, err := time.Parse(time.RFC3339Nano, keys[0])
	if err != nil {
		return nil, invalidCursor()
	}
	id, err := strconv.Atoi(keys[1])
	if err != nil || id <= 0 {
		return nil, invalidCursor()
	}
	return &repository.DraftKeysetPos{Updated: updated, ID: id}, nil
}

func (d *Drafts) listTopicDrafts(ctx context.Context, in *listTopicDraftsInput) (*listTopicDraftsOutput, error) {
	if prob := d.ready(); prob != nil {
		return nil, prob
	}
	user := v1.User(ctx)
	if user == nil {
		return nil, notFound()
	}
	fp := draftFingerprint(user.ID)
	keys, curErr := collect.DecodeCursor(in.Cursor, draftSort, fp)
	if curErr != nil {
		return nil, curErr
	}
	pos, posErr := parseDraftPos(keys)
	if posErr != nil {
		return nil, posErr
	}
	limit := in.Limit
	if limit <= 0 {
		limit = collect.DefaultLimit
	}

	rows, err := d.drafts.FindKeysetForUser(user.ID, limit+1, pos)
	if err != nil {
		return nil, problem.Internal(err)
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	items := make([]TopicDraftSummary, len(rows))
	for i, row := range rows {
		items[i] = TopicDraftSummary{
			Object:    "topic_draft_summary",
			ID:        repr.ID(row.ID),
			Title:     row.Title,
			Summary:   row.Summary,
			Revision:  row.Revision,
			CreatedAt: repr.Timestamp(row.CreatedAt),
			UpdatedAt: repr.Timestamp(row.UpdatedAt),
		}
	}
	var next *string
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		cur := collect.EncodeCursor(draftSort, fp,
			last.UpdatedAt.UTC().Format(time.RFC3339Nano), strconv.Itoa(last.ID))
		next = &cur
	}
	return &listTopicDraftsOutput{Body: repr.NewList(items, next)}, nil
}

func (d *Drafts) getTopicDraft(ctx context.Context, in *topicDraftInput) (*topicDraftOutput, error) {
	draft, prob := d.ownDraft(ctx, in.DraftID)
	if prob != nil {
		return nil, prob
	}
	body, err := mapDraft(draft)
	if err != nil {
		return nil, problem.Internal(err)
	}
	return &topicDraftOutput{Body: body}, nil
}

// draftFields is the normalized content shared by create and update; the two
// request bodies differ only in the optimistic-lock fields.
type draftFields struct {
	title    string
	content  string
	category string
	sections model.StringSlice
	covers   model.ImageTokens
	isNSFW   bool
	miniApps draftMiniApps
}

func normalizeDraftFields(
	title, contentMD string,
	category *string,
	sectionSlugs []SectionSlug,
	isNSFW bool,
	hashes []ImageHash,
	polls []TopicDraftPoll,
	lotteries []TopicDraftLottery,
) (draftFields, error) {
	cat := ""
	if category != nil {
		cat = *category
	}
	sections := model.StringSlice{}
	if len(sectionSlugs) > 0 {
		if key, ok := NormalizeSectionKey(string(sectionSlugs[0])); ok {
			sections = append(sections, key)
		}
	}
	miniApps, err := encodeDraftMiniApps(polls, lotteries)
	if err != nil {
		return draftFields{}, err
	}
	return draftFields{
		title:    title,
		content:  markdown.NormalizeStoredContent(contentMD),
		category: cat,
		sections: sections,
		covers:   coversFromHashes(hashes),
		isNSFW:   isNSFW,
		miniApps: miniApps,
	}, nil
}

func (f draftFields) blank() bool {
	return strings.TrimSpace(f.title) == "" && strings.TrimSpace(f.content) == ""
}

func (f draftFields) toModel(userID int) *model.TopicDraft {
	return &model.TopicDraft{
		UserID:           userID,
		Title:            f.title,
		Content:          f.content,
		Category:         f.category,
		Sections:         f.sections,
		CoverImages:      f.covers,
		IsNSFW:           f.isNSFW,
		Polls:            f.miniApps.polls,
		Lotteries:        f.miniApps.lotteries,
		PrizeImageHashes: f.miniApps.prizeImages,
	}
}

func (d *Drafts) createTopicDraft(ctx context.Context, in *createTopicDraftInput) (*createTopicDraftOutput, error) {
	if prob := d.ready(); prob != nil {
		return nil, prob
	}
	user := v1.User(ctx)
	if user == nil {
		return nil, notFound()
	}

	fields, err := normalizeDraftFields(
		in.Body.Title, in.Body.ContentMarkdown, in.Body.Category,
		in.Body.Sections, in.Body.IsNSFW, in.Body.CoverImageHashes,
		in.Body.Polls, in.Body.Lotteries)
	if err != nil {
		return nil, problem.Internal(err)
	}
	if fields.blank() {
		min := 1
		return nil, validationFailed(problem.AtPointer("/content_markdown", problem.ReasonRequired,
			"a draft needs a title or a body; both were blank after trimming whitespace",
			&problem.FieldParams{MinLength: &min}))
	}

	count, err := d.drafts.CountByUser(user.ID)
	if err != nil {
		return nil, problem.Internal(err)
	}
	if count >= MaxTopicDraftsPerUser {
		return nil, draftLimitReached(MaxTopicDraftsPerUser)
	}

	draft := fields.toModel(user.ID)
	draft.Revision = 1
	rev := model.NewDraftRevision(draft, model.DraftRevisionManual, 1)
	if err := d.drafts.CreateWithRevision(draft, rev); err != nil {
		return nil, problem.Internal(err)
	}
	body, err := mapDraft(draft)
	if err != nil {
		return nil, problem.Internal(err)
	}
	return &createTopicDraftOutput{
		Location: "/api/v1/me/topic-drafts/" + strconv.Itoa(draft.ID),
		Body:     body,
	}, nil
}

func (d *Drafts) updateTopicDraft(ctx context.Context, in *updateTopicDraftInput) (*updateTopicDraftOutput, error) {
	draft, prob := d.ownDraft(ctx, in.DraftID)
	if prob != nil {
		return nil, prob
	}
	user := v1.User(ctx)

	fields, err := normalizeDraftFields(
		in.Body.Title, in.Body.ContentMarkdown, in.Body.Category,
		in.Body.Sections, in.Body.IsNSFW, in.Body.CoverImageHashes,
		in.Body.Polls, in.Body.Lotteries)
	if err != nil {
		return nil, problem.Internal(err)
	}
	if fields.blank() {
		min := 1
		return nil, validationFailed(problem.AtPointer("/content_markdown", problem.ReasonRequired,
			"a draft needs a title or a body; both were blank after trimming whitespace",
			&problem.FieldParams{MinLength: &min}))
	}

	next := fields.toModel(user.ID)
	next.ID = draft.ID
	updated, _, err := d.drafts.PatchDraft(next, in.Body.BaseRevision, in.Body.IsForceOverwrite, in.Body.IsManualSave)
	if errors.Is(err, repository.ErrDraftRevisionConflict) {
		return nil, draftRevisionConflict(updated.Revision)
	}
	if err != nil {
		return nil, problem.Internal(err)
	}
	body, err := mapDraft(updated)
	if err != nil {
		return nil, problem.Internal(err)
	}
	return &updateTopicDraftOutput{Body: body}, nil
}

func (d *Drafts) deleteTopicDraft(ctx context.Context, in *topicDraftInput) (*deleteTopicDraftOutput, error) {
	if prob := d.ready(); prob != nil {
		return nil, prob
	}
	user := v1.User(ctx)
	if user == nil {
		return nil, notFound()
	}
	id, ok := parsePositiveID(in.DraftID)
	if !ok {
		return nil, notFound()
	}
	n, err := d.drafts.DeleteForUser(id, user.ID)
	if err != nil {
		return nil, problem.Internal(err)
	}
	if n == 0 {
		return nil, notFound()
	}
	return &deleteTopicDraftOutput{}, nil
}

// Another user's draft is NOT_FOUND, not PERMISSION_REQUIRED: a private
// resource must not tell a stranger that its id exists.
func (d *Drafts) ownDraft(ctx context.Context, idStr string) (*model.TopicDraft, *problem.Problem) {
	if prob := d.ready(); prob != nil {
		return nil, prob
	}
	user := v1.User(ctx)
	if user == nil {
		return nil, notFound()
	}
	id, ok := parsePositiveID(idStr)
	if !ok {
		return nil, notFound()
	}
	draft, err := d.drafts.GetByIDForUser(id, user.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, notFound()
		}
		return nil, problem.Internal(err)
	}
	return draft, nil
}

func draftCategoryPtr(category string) *string {
	if category == "" {
		return nil
	}
	c := category
	return &c
}

func draftSectionList(sections model.StringSlice) []SectionSlug {
	out := make([]SectionSlug, 0, len(sections))
	for _, s := range sections {
		out = append(out, SectionSlug(s))
	}
	return out
}

func draftCoverHashes(tokens model.ImageTokens) []ImageHash {
	out := make([]ImageHash, 0, len(tokens))
	for _, token := range tokens {
		hash, _, ok := markdown.ParseContentImageRef(token)
		if !ok {
			continue
		}
		out = append(out, ImageHash(hash))
	}
	return out
}

func mapDraft(d *model.TopicDraft) (TopicDraft, error) {
	polls, lotteries, err := decodeDraftMiniApps(d)
	if err != nil {
		return TopicDraft{}, err
	}
	return TopicDraft{
		Object:           "topic_draft",
		ID:               repr.ID(d.ID),
		Title:            d.Title,
		ContentMarkdown:  d.Content,
		Category:         draftCategoryPtr(d.Category),
		Sections:         draftSectionList(d.Sections),
		IsNSFW:           d.IsNSFW,
		CoverImageHashes: draftCoverHashes(d.CoverImages),
		Polls:            polls,
		Lotteries:        lotteries,
		Revision:         d.Revision,
		CreatedAt:        repr.Timestamp(d.CreatedAt),
		UpdatedAt:        repr.Timestamp(d.UpdatedAt),
	}, nil
}
