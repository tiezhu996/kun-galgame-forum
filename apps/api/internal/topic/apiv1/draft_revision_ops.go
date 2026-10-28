package apiv1

import (
	"context"
	"errors"

	v1 "kun-galgame-api/internal/apiv1"
	"kun-galgame-api/internal/apiv1/repr"
	"kun-galgame-api/internal/topic/model"
	"kun-galgame-api/pkg/problem"

	"gorm.io/gorm"
)

type listTopicDraftRevisionsInput struct {
	DraftID string `path:"draft_id" pattern:"^[1-9][0-9]{0,18}$" maxLength:"19" doc:"Draft id."`
}

type listTopicDraftRevisionsOutput struct {
	Body repr.List[TopicDraftRevisionSummary]
}

type topicDraftRevisionInput struct {
	DraftID    string `path:"draft_id" pattern:"^[1-9][0-9]{0,18}$" maxLength:"19" doc:"Draft id."`
	RevisionID string `path:"revision_id" pattern:"^[1-9][0-9]{0,18}$" maxLength:"19" doc:"Revision id."`
}

type topicDraftRevisionOutput struct {
	Body TopicDraftRevision
}

type restoreTopicDraftRevisionInput struct {
	DraftID    string `path:"draft_id" pattern:"^[1-9][0-9]{0,18}$" maxLength:"19" doc:"Draft id."`
	RevisionID string `path:"revision_id" pattern:"^[1-9][0-9]{0,18}$" maxLength:"19" doc:"Revision id."`
	Body       TopicDraftRevisionRestore
}

type restoreTopicDraftRevisionOutput struct {
	Body TopicDraft
}

func (d *Drafts) listTopicDraftRevisions(ctx context.Context, in *listTopicDraftRevisionsInput) (*listTopicDraftRevisionsOutput, error) {
	draft, prob := d.ownDraft(ctx, in.DraftID)
	if prob != nil {
		return nil, prob
	}
	rows, err := d.drafts.ListRevisions(draft.ID)
	if err != nil {
		return nil, problem.Internal(err)
	}
	items := make([]TopicDraftRevisionSummary, len(rows))
	for i, row := range rows {
		items[i] = TopicDraftRevisionSummary{
			Object:       "topic_draft_revision_summary",
			ID:           repr.ID(row.ID),
			Kind:         row.Kind,
			BaseRevision: row.BaseRevision,
			Title:        row.Title,
			Summary:      row.Summary,
			CreatedAt:    repr.Timestamp(row.CreatedAt),
		}
	}
	return &listTopicDraftRevisionsOutput{Body: repr.NewList(items, nil)}, nil
}

func (d *Drafts) ownRevision(ctx context.Context, draftIDStr, revisionIDStr string) (*model.TopicDraft, *model.TopicDraftRevision, *problem.Problem) {
	draft, prob := d.ownDraft(ctx, draftIDStr)
	if prob != nil {
		return nil, nil, prob
	}
	revisionID, ok := parsePositiveID(revisionIDStr)
	if !ok {
		return nil, nil, notFound()
	}
	rev, err := d.drafts.GetRevision(revisionID, draft.ID, v1.User(ctx).ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, notFound()
		}
		return nil, nil, problem.Internal(err)
	}
	return draft, rev, nil
}

func (d *Drafts) getTopicDraftRevision(ctx context.Context, in *topicDraftRevisionInput) (*topicDraftRevisionOutput, error) {
	_, rev, prob := d.ownRevision(ctx, in.DraftID, in.RevisionID)
	if prob != nil {
		return nil, prob
	}
	body, err := mapRevision(rev)
	if err != nil {
		return nil, problem.Internal(err)
	}
	return &topicDraftRevisionOutput{Body: body}, nil
}

func (d *Drafts) restoreTopicDraftRevision(ctx context.Context, in *restoreTopicDraftRevisionInput) (*restoreTopicDraftRevisionOutput, error) {
	draft, prob := d.ownDraft(ctx, in.DraftID)
	if prob != nil {
		return nil, prob
	}
	revisionID, ok := parsePositiveID(in.RevisionID)
	if !ok {
		return nil, notFound()
	}
	user := v1.User(ctx)
	updated, conflict, err := d.drafts.RestoreRevision(draft.ID, user.ID, revisionID, in.Body.BaseRevision)
	if conflict {
		return nil, draftRevisionConflict(updated.Revision)
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, notFound()
		}
		return nil, problem.Internal(err)
	}
	body, err := mapDraft(updated)
	if err != nil {
		return nil, problem.Internal(err)
	}
	return &restoreTopicDraftRevisionOutput{Body: body}, nil
}

func mapRevision(r *model.TopicDraftRevision) (TopicDraftRevision, error) {
	polls, lotteries, err := decodeMiniApps(r.Polls, r.Lotteries)
	if err != nil {
		return TopicDraftRevision{}, err
	}
	return TopicDraftRevision{
		Object:           "topic_draft_revision",
		ID:               repr.ID(r.ID),
		Kind:             r.Kind,
		BaseRevision:     r.BaseRevision,
		Title:            r.Title,
		ContentMarkdown:  r.Content,
		Category:         draftCategoryPtr(r.Category),
		Sections:         draftSectionList(r.Sections),
		IsNSFW:           r.IsNSFW,
		CoverImageHashes: draftCoverHashes(r.CoverImages),
		Polls:            polls,
		Lotteries:        lotteries,
		CreatedAt:        repr.Timestamp(r.CreatedAt),
	}, nil
}
