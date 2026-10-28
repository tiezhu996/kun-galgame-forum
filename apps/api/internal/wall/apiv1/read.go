package apiv1

import (
	"context"
	"encoding/json"
	"strconv"

	v1 "kun-galgame-api/internal/apiv1"
	"kun-galgame-api/internal/apiv1/collect"
	"kun-galgame-api/internal/apiv1/repr"
	"kun-galgame-api/internal/middleware"
	"kun-galgame-api/pkg/communityclient"
	"kun-galgame-api/pkg/problem"
)

const (
	listSort = "post_number_asc"
	// The community service answers at most 100 posts; above that it silently
	// falls back to its default page size.
	upstreamMaxLimit = 100
)

func (s *Service) listWallComments(ctx context.Context, in *listInput) (*listOutput, error) {
	if !s.ready() {
		return nil, problem.Internal(errUnconfigured)
	}
	spec, ok := subjects[in.SubjectType]
	if !ok {
		return nil, problem.New(problem.CodeUnknownEnumValue, "subject_type is not in this collection's vocabulary.",
			problem.AtParameter("subject_type", problem.ReasonUnknownValue, "use a declared subject type", nil))
	}
	sid, ok := repr.ParseID(repr.DecimalID(in.SubjectID))
	if !ok {
		return nil, problem.New(problem.CodeInvalidParameter, "subject_id is not a positive decimal integer.",
			problem.AtParameter("subject_id", problem.ReasonInvalidFormat, "must be a positive decimal integer", nil))
	}
	fp := collect.Fingerprint(string(spec.typ), strconv.Itoa(sid))
	viewer := v1.User(ctx)
	sub, p := s.resolveSubject(ctx, spec, sid, viewer)
	if p != nil {
		return nil, p
	}

	if spec.typ == "website" {
		return s.listWebsiteWall(ctx, in, sub, viewer, fp)
	}

	keys, curErr := collect.DecodeCursor(in.Cursor, listSort, fp)
	if curErr != nil {
		return nil, curErr
	}
	after := ""
	if keys != nil {
		n, err := strconv.Atoi(keys[0])
		if len(keys) != 1 || err != nil || n < 1 {
			return nil, invalidCursor()
		}
		after = keys[0]
	}

	fetch := min(in.Limit+1, upstreamMaxLimit)
	page, err := s.community.GetComments(ctx, spec.anchorKind, spec.anchorID(sid), after, strconv.Itoa(fetch))
	if err != nil {
		return nil, upstreamProblem(err)
	}
	posts := page.Posts
	more := false
	if len(posts) > in.Limit {
		posts, more = posts[:in.Limit], true
	} else if fetch == in.Limit {
		more = page.NextCursor != ""
	}
	rendered, p := s.render(ctx, sub, posts, viewer)
	if p != nil {
		return nil, p
	}
	items := make([]WallComment, 0, len(rendered))
	for _, item := range rendered {
		if item != nil {
			items = append(items, *item)
		}
	}
	var next *string
	if more && len(posts) > 0 {
		cur := collect.EncodeCursor(listSort, fp, strconv.Itoa(int(posts[len(posts)-1].PostNumber)))
		next = &cur
	}
	return &listOutput{Body: repr.NewList(items, next)}, nil
}

// listWebsiteWall merges the survivor anchor with every anchor a folded
// website left behind. Each anchor keeps its own after-post_number cursor; the
// one cursor the client holds is that map as JSON. Only post numbers actually
// returned on the merged page advance an anchor, so a post the page cut leaves
// out is fetched again rather than skipped.
func (s *Service) listWebsiteWall(ctx context.Context, in *listInput, sub *subject, viewer *middleware.UserInfo, fp string) (*listOutput, error) {
	keys, curErr := collect.DecodeCursor(in.Cursor, listSort, fp)
	if curErr != nil {
		return nil, curErr
	}
	cursor := map[string]string{}
	if keys != nil {
		if len(keys) != 1 {
			return nil, invalidCursor()
		}
		if err := json.Unmarshal([]byte(keys[0]), &cursor); err != nil || len(keys[0]) == 0 {
			return nil, invalidCursor()
		}
		allowed := map[string]bool{}
		for _, id := range sub.websiteAnchorIDs {
			allowed[strconv.Itoa(id)] = true
		}
		for anchor, after := range cursor {
			n, err := strconv.Atoi(after)
			if !allowed[anchor] || err != nil || n < 1 {
				return nil, invalidCursor()
			}
		}
	}

	fetch := min(in.Limit+1, upstreamMaxLimit)
	type fetched struct {
		anchorID int
		posts    []communityclient.PostView
		more     bool
	}
	pages := make([]fetched, 0, len(sub.websiteAnchorIDs))
	rawTotal := 0
	for _, anchorID := range sub.websiteAnchorIDs {
		page, err := s.community.GetComments(ctx, sub.spec.anchorKind, sub.spec.anchorID(anchorID),
			cursor[strconv.Itoa(anchorID)], strconv.Itoa(fetch))
		if err != nil {
			return nil, upstreamProblem(err)
		}
		posts := page.Posts
		more := false
		if len(posts) > in.Limit {
			posts, more = posts[:in.Limit], true
		} else if fetch == in.Limit {
			more = page.NextCursor != ""
		}
		pages = append(pages, fetched{anchorID: anchorID, posts: posts, more: more})
		rawTotal += len(posts)
	}

	idx := make([]int, len(pages))
	posts := make([]communityclient.PostView, 0, min(rawTotal, in.Limit))
	consumed := map[int]int32{}
	for range in.Limit {
		pick := -1
		var pickAt communityclient.PostView
		for i := range pages {
			if idx[i] >= len(pages[i].posts) {
				continue
			}
			cand := pages[i].posts[idx[i]]
			if pick < 0 || postBefore(cand, pickAt) {
				pick, pickAt = i, cand
			}
		}
		if pick < 0 {
			break
		}
		posts = append(posts, pickAt)
		consumed[pages[pick].anchorID] = pickAt.PostNumber
		idx[pick]++
	}

	anchorMore := false
	for _, pg := range pages {
		if pg.more {
			anchorMore = true
		}
	}
	more := anchorMore || rawTotal > len(posts)

	rendered, p := s.render(ctx, sub, posts, viewer)
	if p != nil {
		return nil, p
	}
	items := make([]WallComment, 0, len(rendered))
	for _, item := range rendered {
		if item != nil {
			items = append(items, *item)
		}
	}
	var next *string
	if more {
		for anchor, after := range consumed {
			cursor[strconv.Itoa(anchor)] = strconv.Itoa(int(after))
		}
		raw, err := json.Marshal(cursor)
		if err != nil {
			return nil, problem.Internal(err)
		}
		cur := collect.EncodeCursor(listSort, fp, string(raw))
		next = &cur
	}
	return &listOutput{Body: repr.NewList(items, next)}, nil
}

func postBefore(a, b communityclient.PostView) bool {
	at, aerr := parseUpstreamTime(a.CreatedAt)
	bt, berr := parseUpstreamTime(b.CreatedAt)
	if aerr != nil || berr != nil || at.Equal(bt) {
		return a.ID < b.ID
	}
	return at.Before(bt)
}

func invalidCursor() *problem.Problem {
	return problem.New(
		problem.CodeInvalidCursor,
		"The cursor cannot be parsed or is no longer valid.",
		problem.AtParameter("cursor", problem.ReasonInvalidFormat, "pass the next_cursor from a previous page of this collection", nil),
	)
}

func (s *Service) getWallComment(ctx context.Context, in *commentInput) (*commentOutput, error) {
	if !s.ready() {
		return nil, problem.Internal(errUnconfigured)
	}
	viewer := v1.User(ctx)
	loc, p := s.locate(ctx, in.WallCommentID, viewer)
	if p != nil {
		return nil, p
	}
	item, p := s.renderOne(ctx, loc.sub, loc.post, viewer)
	if p != nil {
		return nil, p
	}
	return &commentOutput{Body: *item}, nil
}

func (s *Service) getWallCommentSource(ctx context.Context, in *commentInput) (*sourceOutput, error) {
	if !s.ready() {
		return nil, problem.Internal(errUnconfigured)
	}
	viewer := v1.User(ctx)
	loc, p := s.locate(ctx, in.WallCommentID, viewer)
	if p != nil {
		return nil, p
	}
	if !caps(loc.sub, loc.post, viewer).CanEdit {
		return nil, permissionRequired()
	}
	return &sourceOutput{Body: WallCommentSource{
		Object:          "wall_comment_source",
		WallCommentID:   repr.DecimalID(in.WallCommentID),
		ContentMarkdown: loc.post.ContentRaw,
	}}, nil
}
