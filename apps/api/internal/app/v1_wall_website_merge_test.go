package app

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"kun-galgame-api/internal/community/anchor"
	websiteRepo "kun-galgame-api/internal/website/repository"
	"kun-galgame-api/pkg/communityclient"
)

const rcWebsiteOld = 950000602

// seedMergedWebsite folds a second listing into the fixture website straight
// in the database, the shape an admin merge leaves behind.
func seedMergedWebsite(t *testing.T, f *wallFix) {
	t.Helper()
	now := time.Now()
	if err := f.db.Exec(`INSERT INTO galgame_website (id, name, url, create_time, category_id, age_limit,
			comment_count, view, created, updated, status)
		VALUES (?, 'rc old', 'rc-old.example', '2019', ?, 'all', 5, 40, ?, ?, 'normal')`,
		rcWebsiteOld, rcCategory, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec(`UPDATE galgame_website SET comment_count = 2 WHERE id = ?`, rcWebsite).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := websiteRepo.NewStore(f.db).FoldWebsite(rcWebsiteOld, rcWebsite, rcStaff); err != nil {
		t.Fatalf("fold: %v", err)
	}
	t.Cleanup(func() {
		_ = f.db.Exec(`DELETE FROM galgame_website_merge WHERE new_website_id = ? OR old_website_id = ?`,
			rcWebsite, rcWebsiteOld).Error
	})
}

func TestV1WallMergedWebsiteReadsBothAnchors(t *testing.T) {
	f := newWallFix(t)
	seedMergedWebsite(t, f)

	now := time.Now()
	oldAnchor := resAnchor("website", rcWebsiteOld)
	newAnchor := resAnchor("website", rcWebsite)
	old1 := f.cm.seed(anchorRes, oldAnchor, rcBob, "old first", 0, 0, now.Add(-2*time.Hour))
	new1 := f.cm.seed(anchorRes, newAnchor, rcAlice, "new first", 0, 0, now.Add(-time.Hour))
	old2 := f.cm.seed(anchorRes, oldAnchor, rcBob, "old second", 0, 0, now.Add(-30*time.Minute))

	// First page cuts inside both threads; the per-anchor cursor keeps the
	// unconsumed old post for the next page.
	resp, out := f.list(t, "", "website", rcWebsite, "&limit=2")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list page 1 %d %+v", resp.StatusCode, out)
	}
	first := itemIDs(t, out)
	if fmt.Sprint(first) != fmt.Sprint([]string{pid(old1), pid(new1)}) {
		t.Fatalf("page 1 = %v", first)
	}
	next, _ := out["next_cursor"].(string)
	if next == "" {
		t.Fatal("merged page of 3 posts with limit 2 needs a cursor")
	}

	resp, out = f.list(t, "", "website", rcWebsite, "&limit=2&cursor="+next)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list page 2 %d %+v", resp.StatusCode, out)
	}
	if got := itemIDs(t, out); fmt.Sprint(got) != fmt.Sprint([]string{pid(old2)}) {
		t.Fatalf("page 2 = %v, want the leftover old post", got)
	}
	if out["next_cursor"] != nil && out["next_cursor"] != "" {
		t.Errorf("page 2 returned another cursor: %v", out["next_cursor"])
	}

	// The old subject id resolves to the same merged wall.
	resp, out = f.list(t, "", "website", rcWebsiteOld, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list via old id %d %+v", resp.StatusCode, out)
	}
	if got := itemIDs(t, out); fmt.Sprint(got) != fmt.Sprint([]string{pid(old1), pid(new1), pid(old2)}) {
		t.Fatalf("old-id wall = %v", got)
	}

	// A malformed cursor is refused.
	resp, out = f.list(t, "", "website", rcWebsite, "&cursor=cur_not-a-cursor")
	if resp.StatusCode != http.StatusBadRequest || wallCode(out) != "INVALID_CURSOR" {
		t.Errorf("garbage cursor = %d %+v", resp.StatusCode, out)
	}
}

func TestV1WallMergedWebsiteWrites(t *testing.T) {
	f := newWallFix(t)
	seedMergedWebsite(t, f)
	now := time.Now()
	oldPost := f.cm.seed(anchorRes, resAnchor("website", rcWebsiteOld), rcBob, "old thread", 0, 0, now.Add(-time.Hour))

	// Replies keep the old thread; new top-level comments open the survivor.
	body := map[string]any{
		"subject_type": "website", "subject_id": fmt.Sprint(rcWebsite),
		"content_markdown": "replying on the old wall", "parent_comment_id": pid(oldPost),
	}
	resp, out := f.create(t, "rc-alice", keyUUID(801), body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("reply to old post %d %+v", resp.StatusCode, out)
	}
	if f.cm.lastReq.AnchorID != resAnchor("website", rcWebsiteOld) {
		t.Errorf("reply anchor = %q, want the old thread", f.cm.lastReq.AnchorID)
	}

	top := map[string]any{
		"subject_type": "website", "subject_id": fmt.Sprint(rcWebsite),
		"content_markdown": "new top level",
	}
	resp, out = f.create(t, "rc-alice", keyUUID(802), top)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("top level %d %+v", resp.StatusCode, out)
	}
	if f.cm.lastReq.AnchorID != resAnchor("website", rcWebsite) {
		t.Errorf("top level anchor = %q, want the survivor", f.cm.lastReq.AnchorID)
	}

	// Permalink, edit-context and delete still resolve the old post, and every
	// counter move lands on the survivor.
	if n := f.count(t, `SELECT comment_count FROM galgame_website WHERE id = ?`, rcWebsite); n != 9 {
		t.Fatalf("comment_count after fold(7) + reply + top = %d, want 9", n)
	}
	resp, out = f.onComment(t, http.MethodGet, "rc-bob", oldPost, "", nil)
	if resp.StatusCode != http.StatusOK || out["id"] != pid(oldPost) {
		t.Fatalf("old permalink %d %+v", resp.StatusCode, out)
	}
	resp, _ = f.onComment(t, http.MethodDelete, "rc-bob", oldPost, "", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete old post %d", resp.StatusCode)
	}
	if n := f.count(t, `SELECT comment_count FROM galgame_website WHERE id = ?`, rcWebsite); n != 8 {
		t.Errorf("comment_count after deleting the old post = %d, want 8", n)
	}
}

func TestV1WallMergedWebsiteAnchorResolution(t *testing.T) {
	f := newWallFix(t)
	seedMergedWebsite(t, f)

	r := anchor.New(f.db, nil)
	oldRef := anchor.Ref{Kind: communityclient.AnchorSiteResource, ID: resAnchor("website", rcWebsiteOld)}
	targets := r.Resolve([]anchor.Ref{oldRef})
	got, ok := targets[oldRef]
	if !ok {
		t.Fatal("merged website anchor not resolved")
	}
	if got.Link != "/website/"+rcWebsiteSlug {
		t.Errorf("link = %q, want /website/%s", got.Link, rcWebsiteSlug)
	}
}
