package apiv1

import (
	"context"
	"errors"
	"strconv"

	"kun-galgame-api/internal/apiv1/repr"
	"kun-galgame-api/internal/website/model"
	"kun-galgame-api/internal/website/repository"
	"kun-galgame-api/pkg/problem"
)

type WebsiteMergeRequest struct {
	SourceWebsiteID repr.DecimalID `json:"source_website_id" doc:"Id of the website folded away. Its host starts answering ENTITY_MERGED and its row is deleted."`
	TargetWebsiteID repr.DecimalID `json:"target_website_id" doc:"Id of the website that survives and receives every row."`
}

type WebsiteMergeSide struct {
	Object          string             `json:"object" enum:"website_merge_side" maxLength:"18" doc:"Type discriminant. Always website_merge_side."`
	ID              repr.DecimalID     `json:"id" doc:"Website id."`
	Host            string             `json:"host" pattern:"^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$" maxLength:"253" doc:"Main host name, lower case."`
	Title           string             `json:"title" maxLength:"233" doc:"Name of the site. Free text; never use it as a decision input."`
	State           string             `json:"state" enum:"normal,unreachable,closed" maxLength:"11" doc:"normal: the site is up. unreachable: temporarily down. closed: shut down for good."`
	IsNSFW          bool               `json:"is_nsfw" doc:"Whether the site is adult-oriented."`
	WebsiteCategory WebsiteCategoryRef `json:"website_category" doc:"Category the site is listed under."`
	ViewCount       int                `json:"view_count" minimum:"0" doc:"Lifetime views before the fold."`
	LikeCount       int                `json:"like_count" minimum:"0" doc:"Likes before the fold."`
	FavoriteCount   int                `json:"favorite_count" minimum:"0" doc:"Favorites before the fold."`
	CommentCount    int                `json:"comment_count" minimum:"0" doc:"Cached comment count before the fold."`
	URLs            []SiteURL          `json:"urls" doc:"Known addresses of the site."`
}

type WebsiteMergeTagDrop struct {
	Tag         WebsiteTag       `json:"tag" doc:"The source tag that does not move."`
	DropReason  string           `json:"drop_reason" enum:"duplicate,single_select_group" maxLength:"19" doc:"duplicate: the survivor already carries this tag; single_select_group: the survivor carries another tag of the tag's single-select group."`
	Group       *WebsiteTagGroup `json:"group,omitempty" doc:"The single-select group that caused the drop. null for a plain duplicate."`
	SurvivorTag *WebsiteTag      `json:"survivor_tag,omitempty" doc:"The survivor tag that wins the group. null for a plain duplicate."`
}

type WebsiteMergeSlotStats struct {
	SourceCount int `json:"source_count" minimum:"0" doc:"Rows the source held."`
	Overlap     int `json:"overlap" minimum:"0" doc:"Rows the same users already held on the survivor; they count once."`
	Moved       int `json:"moved" minimum:"0" doc:"Rows the fold adds to the survivor."`
	Resulting   int `json:"resulting" minimum:"0" doc:"Survivor rows after the fold."`
}

type WebsiteMergePreview struct {
	Object                 string                `json:"object" enum:"website_merge_preview" maxLength:"22" doc:"Type discriminant. Always website_merge_preview."`
	SourceWebsite          WebsiteMergeSide      `json:"source_website" doc:"The website that would be folded away."`
	TargetWebsite          WebsiteMergeSide      `json:"target_website" doc:"The website that would survive."`
	Likes                  WebsiteMergeSlotStats `json:"likes" doc:"Like transfer counts."`
	Favorites              WebsiteMergeSlotStats `json:"favorites" doc:"Favorite transfer counts."`
	TagsAdded              []WebsiteTag          `json:"tags_added" doc:"Tags the survivor gains."`
	TagsDropped            []WebsiteMergeTagDrop `json:"tags_dropped" doc:"Source tags that do not move, with the reason."`
	URLsAdded              []SiteURL             `json:"urls_added" doc:"Source-only addresses appended to the survivor's urls."`
	CommentsResulting      int                   `json:"comments_resulting" minimum:"0" doc:"Sum of the two cached comment counts; the survivor page reads both walls."`
	ViewResulting          int                   `json:"view_resulting" minimum:"0" doc:"Sum of the two view counters."`
	IsTargetUpgradedToNSFW bool                  `json:"is_target_upgraded_to_nsfw" doc:"True when the survivor would flip to r18 because the source is r18. NSFW only ever moves up."`
}

type WebsiteMerge struct {
	Object                 string                `json:"object" enum:"website_merge" maxLength:"13" doc:"Type discriminant. Always website_merge."`
	ID                     repr.DecimalID        `json:"id" doc:"Merge ledger id."`
	SourceWebsiteID        repr.DecimalID        `json:"source_website_id" doc:"Folded-away website id."`
	TargetWebsiteID        repr.DecimalID        `json:"target_website_id" doc:"Survivor website id."`
	SourceHost             string                `json:"source_host" pattern:"^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$" maxLength:"253" doc:"Host that now answers ENTITY_MERGED."`
	TargetHost             string                `json:"target_host" pattern:"^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$" maxLength:"253" doc:"Host of the surviving page."`
	Likes                  WebsiteMergeSlotStats `json:"likes" doc:"Like transfer counts as executed."`
	Favorites              WebsiteMergeSlotStats `json:"favorites" doc:"Favorite transfer counts as executed."`
	TagsMovedCount         int                   `json:"tags_moved_count" minimum:"0" doc:"Tag links added to the survivor."`
	TagsDroppedCount       int                   `json:"tags_dropped_count" minimum:"0" doc:"Source tag links not moved."`
	URLsAdded              []SiteURL             `json:"urls_added" doc:"Addresses appended to the survivor."`
	CommentsResulting      int                   `json:"comments_resulting" minimum:"0" doc:"Survivor comment count after the fold."`
	ViewResulting          int                   `json:"view_resulting" minimum:"0" doc:"Survivor view counter after the fold."`
	IsTargetUpgradedToNSFW bool                  `json:"is_target_upgraded_to_nsfw" doc:"Whether the survivor flipped to r18."`
}

type mergeRequestInput struct {
	Body WebsiteMergeRequest
}

type mergePreviewOutput struct {
	Body WebsiteMergePreview
}

type mergeOutput struct {
	Body WebsiteMerge
}

func (s *Service) parseMergeIDs(body WebsiteMergeRequest) (int, int, *problem.Problem) {
	source, ok1 := parseID(string(body.SourceWebsiteID))
	target, ok2 := parseID(string(body.TargetWebsiteID))
	var errs []problem.FieldError
	if !ok1 {
		errs = append(errs, problem.AtPointer("/source_website_id", problem.ReasonInvalidFormat, "must be a positive decimal integer", nil))
	}
	if !ok2 {
		errs = append(errs, problem.AtPointer("/target_website_id", problem.ReasonInvalidFormat, "must be a positive decimal integer", nil))
	}
	if ok1 && ok2 && source == target {
		errs = append(errs, problem.AtPointer("/target_website_id", problem.ReasonInvalidFormat, "must differ from source_website_id", nil))
	}
	if len(errs) > 0 {
		return 0, 0, validationFailed(errs...)
	}
	return source, target, nil
}

func mergeProblem(err error) *problem.Problem {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return notFound()
	case errors.Is(err, repository.ErrAlreadyMerged):
		return problem.New(problem.CodeAlreadyExists, "The source website was already merged into another website.",
			problem.AtPointer("/source_website_id", problem.ReasonNotAllowedValue, "already merged; a website can only be folded once", nil))
	default:
		return problem.Internal(err)
	}
}

func siteURLs(urls []string) []SiteURL {
	out := make([]SiteURL, len(urls))
	for i, u := range urls {
		out[i] = SiteURL(u)
	}
	return out
}

func (s *Service) mergeSideView(side repository.MergeSide) (WebsiteMergeSide, *problem.Problem) {
	cats, err := s.store.CategoriesByIDs([]int{side.CategoryID})
	if err != nil {
		return WebsiteMergeSide{}, problem.Internal(err)
	}
	return WebsiteMergeSide{
		Object: "website_merge_side", ID: repr.ID(side.ID), Host: side.Host, Title: side.Title,
		State: side.Status, IsNSFW: side.IsNSFW, WebsiteCategory: categoryRef(cats[side.CategoryID]),
		ViewCount: side.View, LikeCount: side.LikeCount, FavoriteCount: side.FavoriteCount,
		CommentCount: side.CommentCount, URLs: siteURLs(side.URLs),
	}, nil
}

func (s *Service) previewView(plan *repository.WebsiteMergePlan) (*WebsiteMergePreview, *problem.Problem) {
	source, p := s.mergeSideView(plan.Source)
	if p != nil {
		return nil, p
	}
	target, p := s.mergeSideView(plan.Target)
	if p != nil {
		return nil, p
	}
	tagByID := map[int]model.GalgameWebsiteTag{}
	for _, id := range plan.TagsAdded {
		tagByID[id] = model.GalgameWebsiteTag{ID: id}
	}
	for _, d := range plan.TagsDropped {
		tagByID[d.TagID] = model.GalgameWebsiteTag{ID: d.TagID}
		if d.SurvivorTagID != nil {
			tagByID[*d.SurvivorTagID] = model.GalgameWebsiteTag{ID: *d.SurvivorTagID}
		}
	}
	ids := make([]int, 0, len(tagByID))
	for id := range tagByID {
		ids = append(ids, id)
	}
	tags, err := s.store.TagsByIDs(ids)
	if err != nil {
		return nil, problem.Internal(err)
	}
	groupIDs := map[int]struct{}{}
	for _, t := range tags {
		if t.GroupID != nil {
			groupIDs[*t.GroupID] = struct{}{}
		}
	}
	gids := make([]int, 0, len(groupIDs))
	for id := range groupIDs {
		gids = append(gids, id)
	}
	groups, err := s.store.GroupsByIDs(gids)
	if err != nil {
		return nil, problem.Internal(err)
	}
	groupByID := map[int]model.GalgameWebsiteTagGroup{}
	for _, g := range groups {
		groupByID[g.ID] = g
	}
	for _, t := range tags {
		tagByID[t.ID] = t
	}
	added := make([]WebsiteTag, 0, len(plan.TagsAdded))
	for _, id := range plan.TagsAdded {
		added = append(added, renderTag(tagByID[id]))
	}
	dropped := make([]WebsiteMergeTagDrop, 0, len(plan.TagsDropped))
	for _, d := range plan.TagsDropped {
		item := WebsiteMergeTagDrop{Tag: renderTag(tagByID[d.TagID]), DropReason: "duplicate"}
		if d.GroupID != nil && d.SurvivorTagID != nil {
			item.DropReason = "single_select_group"
			if g, ok := groupByID[*d.GroupID]; ok {
				gg := renderTagGroup(g)
				item.Group = &gg
			}
			if t, ok := tagByID[*d.SurvivorTagID]; ok {
				tt := renderTag(t)
				item.SurvivorTag = &tt
			}
		}
		dropped = append(dropped, item)
	}
	return &WebsiteMergePreview{
		Object: "website_merge_preview", SourceWebsite: source, TargetWebsite: target,
		Likes:     WebsiteMergeSlotStats{SourceCount: plan.LikeSource, Overlap: plan.LikeOverlap, Moved: plan.LikeMoved, Resulting: plan.LikeResulting},
		Favorites: WebsiteMergeSlotStats{SourceCount: plan.FavoriteSource, Overlap: plan.FavoriteOverlap, Moved: plan.FavoriteMoved, Resulting: plan.FavoriteResulting},
		TagsAdded: added, TagsDropped: dropped, URLsAdded: siteURLs(plan.URLsAdded),
		CommentsResulting: plan.CommentResulting, ViewResulting: plan.ViewResulting,
		IsTargetUpgradedToNSFW: plan.TargetBecomesNSFW,
	}, nil
}

func (s *Service) previewAdminWebsiteMerge(ctx context.Context, in *mergeRequestInput) (*mergePreviewOutput, error) {
	if p := s.ready(); p != nil {
		return nil, p
	}
	if _, p := s.staff(ctx, permDelete); p != nil {
		return nil, p
	}
	source, target, p := s.parseMergeIDs(in.Body)
	if p != nil {
		return nil, p
	}
	plan, err := s.store.WebsiteMergePlan(source, target)
	if err != nil {
		if errors.Is(err, repository.ErrSameWebsite) {
			return nil, validationFailed(problem.AtPointer("/target_website_id", problem.ReasonInvalidFormat, "must differ from source_website_id", nil))
		}
		return nil, mergeProblem(err)
	}
	view, p := s.previewView(plan)
	if p != nil {
		return nil, p
	}
	return &mergePreviewOutput{Body: *view}, nil
}

func (s *Service) createAdminWebsiteMerge(ctx context.Context, in *mergeRequestInput) (*mergeOutput, error) {
	if p := s.ready(); p != nil {
		return nil, p
	}
	user, p := s.staff(ctx, permDelete)
	if p != nil {
		return nil, p
	}
	source, target, p := s.parseMergeIDs(in.Body)
	if p != nil {
		return nil, p
	}
	res, err := s.store.FoldWebsite(source, target, user.ID)
	if err != nil {
		if errors.Is(err, repository.ErrSameWebsite) {
			return nil, validationFailed(problem.AtPointer("/target_website_id", problem.ReasonInvalidFormat, "must differ from source_website_id", nil))
		}
		return nil, mergeProblem(err)
	}
	row, perr := s.store.FindWebsite(target)
	if perr != nil {
		return nil, storeProblem(perr)
	}
	out := WebsiteMerge{
		Object: "website_merge", ID: repr.DecimalID(strconv.FormatInt(res.LedgerID, 10)),
		SourceWebsiteID: repr.ID(source), TargetWebsiteID: repr.ID(target),
		SourceHost: res.OldHost, TargetHost: res.NewHost,
		Likes: WebsiteMergeSlotStats{
			SourceCount: res.MovedLike + res.DroppedLike, Overlap: res.DroppedLike,
			Moved: res.MovedLike, Resulting: row.LikeCount,
		},
		Favorites: WebsiteMergeSlotStats{
			SourceCount: res.MovedFavorite + res.DroppedFavorite, Overlap: res.DroppedFavorite,
			Moved: res.MovedFavorite, Resulting: row.FavoriteCount,
		},
		TagsMovedCount: res.MovedTag, TagsDroppedCount: res.DroppedTag, URLsAdded: siteURLs(res.URLsAdded),
		CommentsResulting: row.CommentCount, ViewResulting: row.View,
		IsTargetUpgradedToNSFW: row.AgeLimit != "all",
	}
	return &mergeOutput{Body: out}, nil
}
