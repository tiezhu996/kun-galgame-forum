package app

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

const (
	dgCardID = 960000400
	dgTopicA = 960000401
	dgTopicB = 960000402
	dgLiked  = 960000403
	dgNotice = 960000404
	dgMirror = 960000405
	dgTopicID = 960000450
	dgDigestID = int64(960000490)
)

func seedDigestFix(t *testing.T, f *writeFix) {
	t.Helper()
	run := func(q string, args ...any) {
		t.Helper()
		if err := f.db.Exec(q, args...).Error; err != nil {
			t.Fatalf("seed digest: %v\n%s", err, q)
		}
	}
	cleanup := `
DELETE FROM notification_digest_item;
DELETE FROM notification_digest WHERE id = 960000490;
DELETE FROM message WHERE id BETWEEN 960000400 AND 960000409;
DELETE FROM topic WHERE id = 960000450;`
	run(cleanup)
	t.Cleanup(func() { run(cleanup) })

	now := time.Now().Add(-time.Hour)
	run(`INSERT INTO topic (id, title, content, category, user_id, created, updated)
VALUES (?, '夜间的那个话题', 'x', 'test', ?, now(), now())`, dgTopicID, w3UserBob)

	insert := func(id, sender int, typ, link string, rootKind *string, rootID *int64, itemCount int, cid *int64) {
		run(`INSERT INTO message (id, content, link, status, type, sender_id, receiver_id, root_kind, root_id, item_count, community_notification_id, created, updated)
VALUES (?, 'preview', ?, 'unread', ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, link, typ, sender, w3UserAlice, rootKind, rootID, itemCount, cid, now, now)
	}
	topic := "topic"
	work := "work"
	var topicRoot int64 = dgTopicID
	var workRoot int64 = 70001
	var mirrorCID int64 = 960000009
	insert(dgTopicA, w3UserBob, "replied", "/topic/960000450", &topic, &topicRoot, 1, nil)
	insert(dgTopicB, w3UserBob, "subscribed-topic", "/topic/960000450", &topic, &topicRoot, 2, nil)
	insert(dgLiked, w3UserBob, "liked", "/galgame/70001", &work, &workRoot, 1, nil)
	insert(dgNotice, 0, "notice", "/shop", nil, nil, 1, nil)
	insert(dgMirror, w3UserBob, "replied", "/topic/960000450", &topic, &topicRoot, 1, &mirrorCID)

	run(`INSERT INTO message (id, content, link, status, type, sender_id, receiver_id, created, updated, item_count, actor_count)
VALUES (?, '夜间通知摘要：1 个话题有 4 条新回复，另有 2 条其他互动', '/message/digest/960000400', 'unread', 'digest', 0, ?, now(), now(), 1, 1)`,
		dgCardID, w3UserAlice)
	run(`INSERT INTO notification_digest (id, receiver_id, message_id, window_start, window_end, status, total_count)
VALUES (?, ?, ?, now() - interval '9 hour', now(), 'unread', 6)`,
		dgDigestID, w3UserAlice, dgCardID)
	run(`INSERT INTO notification_digest_item (digest_id, message_id) VALUES
(?, 960000401), (?, 960000402), (?, 960000403), (?, 960000404), (?, 960000405)`,
		dgDigestID, dgDigestID, dgDigestID, dgDigestID, dgDigestID)
}

func digestCall(t *testing.T, f *writeFix, method, path, spec, session string) (*http.Response, map[string]any) {
	t.Helper()
	resp, raw := f.doJSON(t, method, path, session, spec, "", nil, nil)
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil && resp.StatusCode < 400 {
		t.Fatalf("decode %s: %v: %s", path, err, raw)
	}
	return resp, body
}

func TestV1NotificationDigestDetail(t *testing.T) {
	f := newMessageFix(t)
	seedDigestFix(t, f)

	resp, body := digestCall(t, f, http.MethodGet, notificationsPath+"/digests/960000400",
		"/me/notifications/digests/{digest_id}", "sess-alice")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get digest %d %+v", resp.StatusCode, body)
	}
	if body["object"] != "notification_digest" || body["is_read"] != false {
		t.Fatalf("digest body %+v", body)
	}
	groups, _ := body["topic_groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("topic_groups %+v", groups)
	}
	g0 := groups[0].(map[string]any)
	if g0["topic_title"] != "夜间的那个话题" || g0["path"] != "/topic/960000450" || g0["count"].(float64) != 4 {
		t.Fatalf("group %+v", g0)
	}
	others, _ := body["other_items"].([]any)
	if len(others) != 2 { // liked + notice; mirror is topic-rooted
		t.Fatalf("other_items %+v", others)
	}

	// Members do not surface in the list while the digest is unread: only
	// the digest card from this batch appears on top of the fixture's own
	// unread rows.
	resp, raw := f.doJSON(t, http.MethodGet, notificationsPath, "sess-alice", "/me/notifications", "", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list %d %s", resp.StatusCode, raw)
	}
	var listBody map[string]any
	if err := json.Unmarshal(raw, &listBody); err != nil {
		t.Fatal(err)
	}
	listIDs := map[string]bool{}
	for _, it := range listBody["items"].([]any) {
		listIDs[it.(map[string]any)["id"].(string)] = true
	}
	if !listIDs["960000400"] {
		t.Fatalf("digest card missing from list: %v", listIDs)
	}
	for _, id := range []string{"960000401", "960000402", "960000403", "960000404", "960000405"} {
		if listIDs[id] {
			t.Fatalf("member %s leaked into list while digest unread: %v", id, listIDs)
		}
	}

	// 404 for another user.
	resp, body = digestCall(t, f, http.MethodGet, notificationsPath+"/digests/960000400",
		"/me/notifications/digests/{digest_id}", "sess-bob")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("bob get digest %d %+v", resp.StatusCode, body)
	}
}

func TestV1NotificationDigestReadCascade(t *testing.T) {
	f := newMessageFix(t)
	seedDigestFix(t, f)

	put := func(session string) (*http.Response, map[string]any) {
		return digestCall(t, f, http.MethodPut, notificationsPath+"/digests/960000400/read",
			"/me/notifications/digests/{digest_id}/read", session)
	}
	resp, body := put("sess-alice")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put read %d %+v", resp.StatusCode, body)
	}
	if body["marked_count"].(float64) != 5 {
		t.Fatalf("marked_count %v, want 5", body["marked_count"])
	}
	// The shared message fixture seeds 17 other unread rows for alice; the
	// digest batch leaves all of them untouched.
	if body["unread_count"].(float64) != 17 {
		t.Fatalf("unread_count %v, want 17 fixture rows", body["unread_count"])
	}

	// Members now appear in the ordinary list, read.
	resp, raw := f.doJSON(t, http.MethodGet, notificationsPath, "sess-alice", "/me/notifications", "", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list %d %s", resp.StatusCode, raw)
	}
	var list map[string]any
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, it := range list["items"].([]any) {
		ids[it.(map[string]any)["id"].(string)] = true
	}
	for _, id := range []string{"960000400", "960000401", "960000404", "960000405"} {
		if !ids[id] {
			t.Fatalf("read member %s missing from list: %v", id, ids)
		}
	}

	resp, body = put("sess-alice")
	if resp.StatusCode != http.StatusOK || body["marked_count"].(float64) != 0 {
		t.Fatalf("replay %d %+v", resp.StatusCode, body)
	}

	resp, body = put("sess-bob")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("bob put read %d %+v", resp.StatusCode, body)
	}
}

func TestV1NotificationDigestHidesMutedAfterBuild(t *testing.T) {
	f := newMessageFix(t)
	seedDigestFix(t, f)
	if err := f.db.Exec(`UPDATE kungal_user_state SET muted_notification_types = '["liked"]' WHERE user_id = ?`, w3UserAlice).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		f.db.Exec(`UPDATE kungal_user_state SET muted_notification_types = '[]' WHERE user_id = ?`, w3UserAlice)
	})

	resp, body := digestCall(t, f, http.MethodGet, notificationsPath+"/digests/960000400",
		"/me/notifications/digests/{digest_id}", "sess-alice")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get digest %d %+v", resp.StatusCode, body)
	}
	for _, it := range body["other_items"].([]any) {
		if it.(map[string]any)["notification_type"] == "liked" {
			t.Fatalf("muted-after-build liked leaked into digest detail: %+v", body["other_items"])
		}
	}

	resp, marker := digestCall(t, f, http.MethodPut, notificationsPath+"/digests/960000400/read",
		"/me/notifications/digests/{digest_id}/read", "sess-alice")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put read %d %+v", resp.StatusCode, marker)
	}
	if marker["marked_count"].(float64) != 4 { // liked member not touched
		t.Fatalf("marked_count %v, want 4", marker["marked_count"])
	}
	var likedStatus string
	f.db.Raw(`SELECT status FROM message WHERE id = ?`, dgLiked).Scan(&likedStatus)
	if likedStatus != "unread" {
		t.Fatalf("muted member status %s, want unread", likedStatus)
	}
}
