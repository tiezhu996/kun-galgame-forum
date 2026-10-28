package app

import (
	"fmt"
	"net/http"
	"testing"
)

const draftSpec = "/me/topic-drafts/{draft_id}"

func (f *writeFix) createDraftFor(t *testing.T, session string, n int, content string) (string, map[string]any) {
	t.Helper()
	resp, body := f.postDraft(t, session, keyUUID(n), map[string]any{
		"title":            fmt.Sprintf("draft %d", n),
		"content_markdown": content,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create draft %d: %d %+v", n, resp.StatusCode, body)
	}
	return strID(body["id"]), body
}

func (f *writeFix) patchDraft(t *testing.T, session, id string, payload map[string]any) (*http.Response, map[string]any) {
	t.Helper()
	resp, body := f.doJSON(t, http.MethodPatch, draftsPath+"/"+id, session, draftSpec, "", nil, payload)
	return resp, problemMap(t, body)
}

func (f *writeFix) revisionList(t *testing.T, session, id string) (*http.Response, map[string]any) {
	t.Helper()
	path := draftsPath + "/" + id + "/revisions"
	resp, body := f.doJSON(t, http.MethodGet, path, session, draftSpec+"/revisions", "", nil, nil)
	return resp, problemMap(t, body)
}

func (f *writeFix) revisionGet(t *testing.T, session, draftID, revisionID string) (*http.Response, map[string]any) {
	t.Helper()
	path := draftsPath + "/" + draftID + "/revisions/" + revisionID
	resp, body := f.doJSON(t, http.MethodGet, path, session, draftSpec+"/revisions/{revision_id}", "", nil, nil)
	return resp, problemMap(t, body)
}

func (f *writeFix) revisionRestore(t *testing.T, session, draftID, revisionID string, payload any) (*http.Response, map[string]any) {
	t.Helper()
	path := draftsPath + "/" + draftID + "/revisions/" + revisionID + "/restore"
	resp, body := f.doJSON(t, http.MethodPost, path, session, draftSpec+"/revisions/{revision_id}/restore", "", nil, payload)
	return resp, problemMap(t, body)
}

func (f *writeFix) draftContent(t *testing.T, id string) string {
	t.Helper()
	var s string
	if err := f.db.Raw(`SELECT content FROM topic_draft WHERE id = ?`, id).Scan(&s).Error; err != nil {
		t.Fatalf("read draft content: %v", err)
	}
	return s
}

func revisionIDs(body map[string]any) []string {
	out := []string{}
	items, _ := body["items"].([]any)
	for _, item := range items {
		row, _ := item.(map[string]any)
		out = append(out, strID(row["id"]))
	}
	return out
}

// A new draft is born at revision 1 with one pinned revision.
func TestV1CreateTopicDraftBornsAtRevisionOne(t *testing.T) {
	f := newDraftFix(t)

	id, created := f.createDraftFor(t, "sess-alice", 1001, "first words")
	if asInt(created["revision"]) != 1 {
		t.Fatalf("revision %v", created["revision"])
	}
	resp, list := f.revisionList(t, "sess-alice", id)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list revisions %d %+v", resp.StatusCode, list)
	}
	items, _ := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("a new draft has one revision, got %d", len(items))
	}
	row, _ := items[0].(map[string]any)
	if row["kind"] != "manual" || asInt(row["base_revision"]) != 1 {
		t.Fatalf("revision 1 %+v", row)
	}
}

// Autosave rewrites one draft, bumps the revision and pins nothing.
func TestV1UpdateTopicDraftAutosaveContinues(t *testing.T) {
	f := newDraftFix(t)

	id, _ := f.createDraftFor(t, "sess-alice", 1002, "body one")
	draftsBefore := f.draftRowCount(t, `SELECT COUNT(*) FROM topic_draft WHERE user_id = ?`, w3UserAlice)
	revsBefore := f.draftRowCount(t, `SELECT COUNT(*) FROM topic_draft_revision WHERE draft_id = ?`, id)

	resp, body := f.patchDraft(t, "sess-alice", id, map[string]any{
		"title": "draft 1002", "content_markdown": "body two", "base_revision": 1,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("autosave %d %+v", resp.StatusCode, body)
	}
	if asInt(body["revision"]) != 2 || body["content_markdown"] != "body two" {
		t.Fatalf("autosave response %+v", body)
	}
	if n := f.draftRowCount(t, `SELECT COUNT(*) FROM topic_draft WHERE user_id = ?`, w3UserAlice); n != draftsBefore {
		t.Fatalf("autosave minted a draft: %d -> %d", draftsBefore, n)
	}
	if n := f.draftRowCount(t, `SELECT COUNT(*) FROM topic_draft_revision WHERE draft_id = ?`, id); n != revsBefore {
		t.Fatalf("autosave pinned a revision: %d -> %d", revsBefore, n)
	}

	resp, got := f.doJSONMap(t, http.MethodGet, draftsPath+"/"+id, "sess-alice", draftSpec)
	if resp.StatusCode != http.StatusOK || got["content_markdown"] != "body two" || asInt(got["revision"]) != 2 {
		t.Fatalf("read back %d %+v", resp.StatusCode, got)
	}
}

// An explicit save pins a manual revision the author can read back.
func TestV1UpdateTopicDraftSnapshotPinsRevision(t *testing.T) {
	f := newDraftFix(t)

	id, _ := f.createDraftFor(t, "sess-alice", 1003, "v1 body")
	resp, body := f.patchDraft(t, "sess-alice", id, map[string]any{
		"title": "draft 1003", "content_markdown": "v2 body",
		"base_revision": 1, "is_manual_save": true,
	})
	if resp.StatusCode != http.StatusOK || asInt(body["revision"]) != 2 {
		t.Fatalf("snapshot save %d %+v", resp.StatusCode, body)
	}

	resp, list := f.revisionList(t, "sess-alice", id)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list %d %+v", resp.StatusCode, list)
	}
	ids := revisionIDs(list)
	if len(ids) != 2 {
		t.Fatalf("revision 1 plus the manual pin, got %d: %v", len(ids), ids)
	}
	// Newest first: the pinned v2, then revision 1.
	if resp, pinned := f.revisionGet(t, "sess-alice", id, ids[0]); resp.StatusCode != http.StatusOK {
		t.Fatalf("get pinned %d %+v", resp.StatusCode, pinned)
	} else {
		if pinned["kind"] != "manual" || asInt(pinned["base_revision"]) != 2 || pinned["content_markdown"] != "v2 body" {
			t.Fatalf("pinned revision %+v", pinned)
		}
	}
	if resp, first := f.revisionGet(t, "sess-alice", id, ids[1]); resp.StatusCode != http.StatusOK {
		t.Fatalf("get first %d", resp.StatusCode)
	} else if first["content_markdown"] != "v1 body" || asInt(first["base_revision"]) != 1 {
		t.Fatalf("revision 1 %+v", first)
	}
}

// A stale base_revision is refused; force keeps the other write as a
// checkpoint before overwriting it.
func TestV1UpdateTopicDraftTwoWindowsConflict(t *testing.T) {
	f := newDraftFix(t)

	id, _ := f.createDraftFor(t, "sess-alice", 1004, "shared start")

	// Window A saves first.
	resp, a := f.patchDraft(t, "sess-alice", id, map[string]any{
		"content_markdown": "window A edit", "base_revision": 1,
	})
	if resp.StatusCode != http.StatusOK || asInt(a["revision"]) != 2 {
		t.Fatalf("window A %d %+v", resp.StatusCode, a)
	}

	// Window B still holds revision 1: its write must not overwrite A.
	resp, b := f.patchDraft(t, "sess-alice", id, map[string]any{
		"content_markdown": "window B edit", "base_revision": 1,
	})
	if resp.StatusCode != http.StatusConflict || b["code"] != "DRAFT_REVISION_CONFLICT" {
		t.Fatalf("window B stale write %d %+v", resp.StatusCode, b)
	}
	if asInt(b["current_revision"]) != 2 {
		t.Fatalf("current_revision %v", b["current_revision"])
	}
	if got := f.draftContent(t, id); got != "window A edit" {
		t.Fatalf("the refused write touched the draft: %v", got)
	}

	// B forces its edit through: A's content survives as a checkpoint.
	resp, forced := f.patchDraft(t, "sess-alice", id, map[string]any{
		"content_markdown": "window B edit", "base_revision": 1, "is_force_overwrite": true,
	})
	if resp.StatusCode != http.StatusOK || asInt(forced["revision"]) != 3 {
		t.Fatalf("force %d %+v", resp.StatusCode, forced)
	}
	if got := f.draftContent(t, id); got != "window B edit" {
		t.Fatalf("working copy after force: %v", got)
	}
	_, list := f.revisionList(t, "sess-alice", id)
	var checkpoint map[string]any
	for _, item := range list["items"].([]any) {
		row, _ := item.(map[string]any)
		if row["kind"] == "checkpoint" {
			checkpoint = row
		}
	}
	if checkpoint == nil {
		t.Fatalf("no checkpoint kept for window A: %+v", list["items"])
	}
	if asInt(checkpoint["base_revision"]) != 2 {
		t.Fatalf("checkpoint pins revision 2, got %+v", checkpoint)
	}
	resp, full := f.revisionGet(t, "sess-alice", id, strID(checkpoint["id"]))
	if resp.StatusCode != http.StatusOK || full["content_markdown"] != "window A edit" {
		t.Fatalf("checkpoint content %d %+v", resp.StatusCode, full)
	}
}

// Restore swaps the old content in and keeps the current content as a
// checkpoint; a stale restore is refused.
func TestV1RestoreTopicDraftRevision(t *testing.T) {
	f := newDraftFix(t)

	id, _ := f.createDraftFor(t, "sess-alice", 1005, "v1 body")
	if _, body := f.patchDraft(t, "sess-alice", id, map[string]any{
		"content_markdown": "v2 body", "base_revision": 1, "is_manual_save": true,
	}); asInt(body["revision"]) != 2 {
		t.Fatalf("pin v2 %+v", body)
	}
	if _, body := f.patchDraft(t, "sess-alice", id, map[string]any{
		"content_markdown": "v3 autosaved", "base_revision": 2,
	}); asInt(body["revision"]) != 3 {
		t.Fatalf("autosave v3 %+v", body)
	}

	_, list := f.revisionList(t, "sess-alice", id)
	ids := revisionIDs(list)
	// ids[0] is the v2 manual pin; revision 1 (v1 body) is last.
	firstRevisionID := ids[len(ids)-1]

	// A stale base cannot restore.
	resp, stale := f.revisionRestore(t, "sess-alice", id, firstRevisionID, map[string]any{
		"base_revision": 2,
	})
	if resp.StatusCode != http.StatusConflict || stale["code"] != "DRAFT_REVISION_CONFLICT" {
		t.Fatalf("stale restore %d %+v", resp.StatusCode, stale)
	}

	// Current restore: v1 comes back, v3 is preserved as a checkpoint.
	resp, restored := f.revisionRestore(t, "sess-alice", id, firstRevisionID, map[string]any{
		"base_revision": 3,
	})
	if resp.StatusCode != http.StatusOK || asInt(restored["revision"]) != 4 {
		t.Fatalf("restore %d %+v", resp.StatusCode, restored)
	}
	if restored["content_markdown"] != "v1 body" {
		t.Fatalf("restored content %v", restored["content_markdown"])
	}
	_, after := f.revisionList(t, "sess-alice", id)
	items, _ := after["items"].([]any)
	latest, _ := items[0].(map[string]any)
	if latest["kind"] != "checkpoint" || asInt(latest["base_revision"]) != 3 {
		t.Fatalf("the pre-restore working copy must be a checkpoint at revision 3: %+v", latest)
	}
	resp, full := f.revisionGet(t, "sess-alice", id, strID(latest["id"]))
	if resp.StatusCode != http.StatusOK || full["content_markdown"] != "v3 autosaved" {
		t.Fatalf("checkpoint holds v3: %d %+v", resp.StatusCode, full)
	}
}

// The revision history is capped: old pins are pruned, the newest 50 stay.
func TestV1TopicDraftRevisionsAreCapped(t *testing.T) {
	f := newDraftFix(t)

	id, created := f.createDraftFor(t, "sess-alice", 1006, "rev 0")
	rev := asInt(created["revision"])
	// One manual row exists already; 50 more snapshot saves must leave 50.
	for i := 0; i < 50; i++ {
		resp, body := f.patchDraft(t, "sess-alice", id, map[string]any{
			"content_markdown": fmt.Sprintf("rev %d", i+1),
			"base_revision":    rev + i,
			"is_manual_save":   true,
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("snapshot %d: %d %+v", i, resp.StatusCode, body)
		}
	}
	if n := f.draftRowCount(t, `SELECT COUNT(*) FROM topic_draft_revision WHERE draft_id = ?`, id); n != 50 {
		t.Fatalf("revision cap leaves 50 rows, got %d", n)
	}
	_, list := f.revisionList(t, "sess-alice", id)
	items, _ := list["items"].([]any)
	newest, _ := items[0].(map[string]any)
	if newest["summary"] != "rev 50" {
		t.Fatalf("newest revision %+v", newest)
	}
	oldest, _ := items[len(items)-1].(map[string]any)
	if oldest["summary"] != "rev 1" {
		t.Fatalf("the oldest kept row is rev 1 (rev 0 and nothing earlier pruned), got %+v", oldest)
	}
}

// Deleting the draft cascades to its revisions; another author sees nothing.
func TestV1TopicDraftRevisionPrivacyAndCascade(t *testing.T) {
	f := newDraftFix(t)

	id, _ := f.createDraftFor(t, "sess-alice", 1007, "alice secret")
	resp, body := f.revisionList(t, "sess-bob", id)
	if resp.StatusCode != http.StatusNotFound || body["code"] != "NOT_FOUND" {
		t.Fatalf("bob lists alice's revisions %d %+v", resp.StatusCode, body)
	}

	resp, deleted := f.doJSONMap(t, http.MethodDelete, draftsPath+"/"+id, "sess-alice", draftSpec)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete draft %d %+v", resp.StatusCode, deleted)
	}
	if n := f.draftRowCount(t, `SELECT COUNT(*) FROM topic_draft_revision WHERE draft_id = ?`, id); n != 0 {
		t.Fatalf("revisions survived the draft: %d rows", n)
	}
}

// A blank PATCH is refused before the revision check.
func TestV1UpdateTopicDraftNeedsSomething(t *testing.T) {
	f := newDraftFix(t)

	id, _ := f.createDraftFor(t, "sess-alice", 1008, "keep me")
	resp, body := f.patchDraft(t, "sess-alice", id, map[string]any{
		"title": " ", "content_markdown": "\t", "base_revision": 1,
	})
	if resp.StatusCode != http.StatusUnprocessableEntity || body["code"] != "VALIDATION_FAILED" {
		t.Fatalf("blank patch %d %+v", resp.StatusCode, body)
	}
	if n := f.scalar(t, `SELECT revision FROM topic_draft WHERE id = ?`, id); n != 1 {
		t.Fatalf("the refused patch bumped the revision: %v", n)
	}
}
