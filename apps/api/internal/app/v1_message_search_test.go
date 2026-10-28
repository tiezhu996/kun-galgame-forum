package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

const (
	sNotifMin = 960000400
	sNotifMax = 960000499
	sRoomNew  = 960000501
	sRoomMin  = 960000500
	sRoomMax  = 960000599
	sChatMin  = 960000600
	sChatMax  = 960000699

	sMarker = "zqmarker"
)

const messageSearchPath = "/api/v1/me/message-search"

type sNotif struct {
	id       int
	sender   int
	receiver int
	typ      string
	content  string
	at       time.Time
	rootKind string
	rootID   *int64
	link     string
	dropped  bool
}

type sDM struct {
	id       int
	room     int
	sender   int
	receiver int
	content  string
	at       time.Time
	recall   bool
	dropped  bool
}

func (f *writeFix) seedMessageSearch(t *testing.T) ([]sNotif, []sDM) {
	t.Helper()
	f.cleanupMessageSearch(t)
	t.Cleanup(func() { f.cleanupMessageSearch(t) })

	run := func(q string, args ...any) {
		t.Helper()
		if err := f.db.Exec(q, args...).Error; err != nil {
			t.Fatalf("seed search: %v\n%s", err, q)
		}
	}
	now := time.Now().UTC()
	d := func(n time.Duration) time.Time { return now.Add(-n) }

	nid := sNotifMin + 10
	nextNotifID := func() int { nid++; return nid }
	cid := sChatMin + 10
	nextChatID := func() int { cid++; return cid }

	topicRoot := int64(w3TopicPub)
	missingTopic := int64(w3TopicMax)
	workRoot := int64(501)

	notifs := []sNotif{
		{nextNotifID(), w3UserBob, w3UserAlice, "replied", "n01 ZQMARKER topic", d(3 * time.Minute), "topic", &topicRoot, "/topic/" + fmt.Sprint(w3TopicPub), false},
		{nextNotifID(), w3UserBob, w3UserAlice, "liked", "n02 zqmarker work", d(12 * time.Minute), "work", &workRoot, "/galgame/501", false},
		{nextNotifID(), w3UserBob, w3UserAlice, "replied", "n03 zqmarker missing topic", d(2 * 24 * time.Hour), "topic", &missingTopic, "/topic/" + fmt.Sprint(w3TopicMax), false},
		{nextNotifID(), w3UserBob, w3UserAlice, "commented", "n04 zqmarker old week", d(10 * 24 * time.Hour), "", nil, "", false},
		{nextNotifID(), w3UserBob, w3UserAlice, "replied", "n05 zqmarker old month", d(40 * 24 * time.Hour), "", nil, "", false},
		{nextNotifID(), w3UserAlice, w3UserAlice, "replied", "n06 zqmarker self sent", d(4 * 24 * time.Hour), "", nil, "", false},
		{nextNotifID(), w3UserBanned, w3UserAlice, "replied", "n07 zqmarker banned actor", d(30 * time.Second), "", nil, "", true},
		{nextNotifID(), w3UserBob, w3UserAlice, "admin", "n08 zqmarker unknown type", d(20 * time.Second), "", nil, "", true},
		{nextNotifID(), w3UserBob, w3UserAlice, "replied", "tieN zqmarker", d(6 * time.Minute), "", nil, "", false},
	}
	for i := 0; i < 12; i++ {
		notifs = append(notifs, sNotif{
			nextNotifID(), w3UserBob, w3UserAlice, "replied",
			fmt.Sprintf("fillN %d zqmarker", i), d(time.Duration(100+i) * time.Minute), "", nil, "", false,
		})
	}
	notifs = append(notifs,
		sNotif{nextNotifID(), w3UserAlice, w3UserBob, "replied", "bobonly zqmarker", d(5 * time.Minute), "topic", &topicRoot, "/topic/1", false},
		sNotif{nextNotifID(), w3UserBob, w3UserAlice, "replied", "no marker at all alice", d(time.Minute), "", nil, "", false},
	)

	for _, n := range notifs {
		run(`INSERT INTO message (id, content, link, status, type, sender_id, receiver_id, created, updated,
				item_count, actor_count, root_kind, root_id)
			VALUES (?, ?, ?, 'unread', ?, ?, ?, ?, ?, 1, 1, NULLIF(?, ''), ?)`,
			n.id, n.content, n.link, n.typ, n.sender, n.receiver, n.at, n.at, n.rootKind, n.rootID)
	}

	// bob <-> grant room: alice is not a participant.
	grantName := fmt.Sprintf("%d-%d", w3UserBob, w3UserGrant)
	run(`INSERT INTO chat_room (id, name, type, last_message_content, created, updated)
		VALUES (?, ?, 'private', '', ?, ?)`, sRoomNew, grantName, d(time.Minute), d(time.Minute))
	run(`INSERT INTO chat_room_participant (chat_room_id, user_id, created, updated) VALUES (?, ?, ?, ?), (?, ?, ?, ?)`,
		sRoomNew, w3UserBob, now, now, sRoomNew, w3UserGrant, now, now)

	insertDM := func(x sDM, recallTime *time.Time) {
		t.Helper()
		name := fmt.Sprintf("%d-%d", minInt(x.sender, x.receiver), maxInt(x.sender, x.receiver))
		run(`INSERT INTO chat_message (id, chat_room_id, chatroom_name, sender_id, receiver_id, content, is_recall, recall_time, created, updated)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			x.id, x.room, name, x.sender, x.receiver, x.content, x.recall, recallTime, x.at, x.at)
	}

	dms := []sDM{
		{nextChatID(), mRoomBob, w3UserBob, w3UserAlice, "d01 zqmarker", d(2 * 24 * time.Hour), false, false},
		{nextChatID(), mRoomBob, w3UserAlice, w3UserBob, "d02 zqmarker mine", d(5 * 24 * time.Hour), false, false},
		{nextChatID(), mRoomBob, w3UserBob, w3UserAlice, "d03 zqmarker ten days", d(10 * 24 * time.Hour), false, false},
		{nextChatID(), mRoomBob, w3UserBob, w3UserAlice, "d04 zqmarker forty days", d(40 * 24 * time.Hour), false, false},
		{nextChatID(), mRoomBob, w3UserBob, w3UserAlice, "d05 secret zqmarker recalled text", d(3 * 24 * time.Hour), true, false},
		{nextChatID(), mRoomBob, w3UserBob, w3UserAlice, "100%done zqmarker", d(24 * time.Hour), false, false},
		{nextChatID(), mRoomBob, w3UserBob, w3UserAlice, "a_b zqmarker", d(24*time.Hour + time.Minute), false, false},
		{nextChatID(), mRoomBob, w3UserBob, w3UserAlice, "tieD zqmarker", d(6 * time.Minute), false, false},
	}
	for i := 0; i < 10; i++ {
		dms = append(dms, sDM{
			nextChatID(), mRoomBob, w3UserBob, w3UserAlice,
			fmt.Sprintf("fillM %d zqmarker", i), d(time.Duration(200+i) * time.Minute), false, false,
		})
	}
	// Negative controls for ILIKE escaping: q=100% must not match 100Xdone,
	// q=a_b must not match aXb or aab.
	dms = append(dms,
		sDM{nextChatID(), mRoomBob, w3UserBob, w3UserAlice, "100Xdone zqmarker", d(23*time.Hour + 30*time.Minute), false, false},
		sDM{nextChatID(), mRoomBob, w3UserBob, w3UserAlice, "aXb zqmarker", d(23 * time.Hour), false, false},
		sDM{nextChatID(), mRoomBob, w3UserBob, w3UserAlice, "aab zqmarker", d(22 * time.Hour), false, false},
	)
	dms = append(dms,
		sDM{nextChatID(), mRoomBanned, w3UserBanned, w3UserAlice, "bannedpeer zqmarker", d(40 * time.Second), false, true},
		sDM{nextChatID(), sRoomNew, w3UserGrant, w3UserBob, "stranger zqmarker", d(time.Minute), false, false},
		sDM{nextChatID(), mRoomBob, w3UserBob, w3UserAlice, "no marker dm at all", d(2 * time.Minute), false, false},
	)

	recalledAt := d(3*24*time.Hour + time.Minute)
	for _, x := range dms {
		var recallTime *time.Time
		if x.recall {
			recallTime = &recalledAt
		}
		insertDM(x, recallTime)
	}

	return notifs, dms
}

func (f *writeFix) cleanupMessageSearch(t *testing.T) {
	t.Helper()
	_ = f.db.Exec(`DELETE FROM chat_message WHERE id BETWEEN ? AND ?`, sChatMin, sChatMax).Error
	_ = f.db.Exec(`DELETE FROM chat_room_participant WHERE chat_room_id BETWEEN ? AND ?`, sRoomMin, sRoomMax).Error
	_ = f.db.Exec(`DELETE FROM chat_room WHERE id BETWEEN ? AND ?`, sRoomMin, sRoomMax).Error
	_ = f.db.Exec(`DELETE FROM message WHERE id BETWEEN ? AND ?`, sNotifMin, sNotifMax).Error
}

func (f *writeFix) searchMsg(t *testing.T, session string, query url.Values) (*http.Response, map[string]any) {
	t.Helper()
	raw := messageSearchPath
	if len(query) > 0 {
		raw += "?" + query.Encode()
	}
	resp, body := f.doJSON(t, http.MethodGet, raw, session, "/me/message-search", "", nil, nil)
	return resp, problemMap(t, body)
}

func searchRows(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, _ := body["items"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		row, _ := item.(map[string]any)
		out = append(out, row)
	}
	return out
}

func searchIDSet(rows []map[string]any) map[string]bool {
	set := make(map[string]bool, len(rows))
	for _, row := range rows {
		id, _ := row["id"].(string)
		set[id] = true
	}
	return set
}

func notifID(n sNotif) string { return "notification:" + fmt.Sprint(n.id) }
func dmID(x sDM) string       { return "direct_message:" + fmt.Sprint(x.id) }

func walkSearch(t *testing.T, f *writeFix, session string, q url.Values) ([]map[string]any, *http.Response) {
	t.Helper()
	all := []map[string]any{}
	pages := 0
	for {
		resp, body := f.searchMsg(t, session, q)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("walk page %d: %d %+v", pages, resp.StatusCode, body)
		}
		rows := searchRows(t, body)
		all = append(all, rows...)
		next, _ := body["next_cursor"].(string)
		if next == "" {
			return all, resp
		}
		q.Set("cursor", next)
		pages++
		if pages > 200 {
			t.Fatal("search walk did not terminate")
		}
	}
}

func visibleAlice(notifs []sNotif, dms []sDM) map[string]bool {
	set := map[string]bool{}
	for _, n := range notifs {
		if n.receiver == w3UserAlice && !n.dropped && strings.Contains(strings.ToLower(n.content), sMarker) {
			set[notifID(n)] = true
		}
	}
	for _, x := range dms {
		if x.dropped {
			continue
		}
		if x.room == mRoomBob && strings.Contains(strings.ToLower(x.content), sMarker) {
			set[dmID(x)] = true
		}
	}
	return set
}

func TestV1MessageSearch_MergeOrderPrivacySource(t *testing.T) {
	f := newMessageFix(t)
	notifs, dms := f.seedMessageSearch(t)

	q := url.Values{"q": {sMarker}, "limit": {"100"}}
	resp, body := f.searchMsg(t, "sess-alice", q)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search %d %+v", resp.StatusCode, body)
	}
	rows := searchRows(t, body)
	if body["next_cursor"] != nil {
		t.Fatalf("all rows should fit one page of 100")
	}

	want := visibleAlice(notifs, dms)
	got := searchIDSet(rows)
	if len(got) != len(want) {
		t.Fatalf("visible hit count = %d, want %d", len(got), len(want))
	}
	for id := range want {
		if !got[id] {
			t.Fatalf("missing expected hit %s", id)
		}
	}
	for id := range got {
		if !want[id] {
			t.Fatalf("unexpected hit %s (privacy or control row leaked)", id)
		}
	}

	// Newest first; at the shared timestamp the direct message precedes the
	// notification (hit_ord tie-break).
	for i := 1; i < len(rows); i++ {
		a, _ := rows[i-1]["created_at"].(string)
		b, _ := rows[i]["created_at"].(string)
		if a < b {
			t.Fatalf("rows not newest-first at %d: %s before %s", i, a, b)
		}
	}
	var tieN, tieD int
	for i, row := range rows {
		if row["id"] == "direct_message:"+fmt.Sprint(sChatMin+18) {
			tieD = i
		}
		if row["id"] == "notification:"+fmt.Sprint(sNotifMin+19) {
			tieN = i
		}
	}
	if tieD+1 != tieN {
		t.Fatalf("tie order: dm index %d, notif index %d", tieD, tieN)
	}

	// Source fields: topic title resolves, work and missing topic stay null.
	var topicTitle string
	if err := f.db.Raw(`SELECT title FROM topic WHERE id = ?`, w3TopicPub).Scan(&topicTitle).Error; err != nil {
		t.Fatal(err)
	}
	byID := map[string]map[string]any{}
	for _, row := range rows {
		byID[row["id"].(string)] = row
	}
	n01 := byID["notification:"+fmt.Sprint(sNotifMin+11)]
	if n01["topic_title"] != topicTitle || n01["path"] != "/topic/"+fmt.Sprint(w3TopicPub) {
		t.Fatalf("topic notification source fields: %+v", n01)
	}
	if nt, _ := n01["notification_type"].(string); nt != "replied" {
		t.Fatalf("notification_type = %v", nt)
	}
	n02 := byID["notification:"+fmt.Sprint(sNotifMin+12)]
	if n02["topic_title"] != nil {
		t.Fatalf("work root topic_title must be null: %+v", n02)
	}
	n03 := byID["notification:"+fmt.Sprint(sNotifMin+13)]
	if n03["topic_title"] != nil {
		t.Fatalf("missing topic topic_title must be null: %+v", n03)
	}
	d01 := byID["direct_message:"+fmt.Sprint(sChatMin+11)]
	peer, _ := d01["peer"].(map[string]any)
	if peer["id"] != fmt.Sprint(w3UserBob) || d01["state"] != "sent" {
		t.Fatalf("dm source fields: %+v", d01)
	}

	// Bob's view: alice's notifications stay private; bob/grant room is visible.
	respB, bodyB := f.searchMsg(t, "sess-bob", url.Values{"q": {sMarker}, "limit": {"100"}})
	if respB.StatusCode != http.StatusOK {
		t.Fatalf("bob search %d %+v", respB.StatusCode, bodyB)
	}
	bobSet := searchIDSet(searchRows(t, bodyB))
	for _, n := range notifs {
		id := notifID(n)
		_, visible := bobSet[id]
		if visible != (n.receiver == w3UserBob) {
			t.Fatalf("bob visibility of %s = %v", id, visible)
		}
	}
	if !bobSet["direct_message:"+fmt.Sprint(sChatMin+33)] {
		t.Fatalf("bob must see the bob/grant room message")
	}
}

func TestV1MessageSearch_RecalledTombstone(t *testing.T) {
	f := newMessageFix(t)
	f.seedMessageSearch(t)

	resp, body := f.searchMsg(t, "sess-alice", url.Values{"q": {sMarker}, "limit": {"100"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search %d %+v", resp.StatusCode, body)
	}
	for _, row := range searchRows(t, body) {
		if row["id"] != "direct_message:"+fmt.Sprint(sChatMin+15) {
			continue
		}
		if row["state"] != "recalled" || row["snippet_markdown"] != "" || row["recalled_at"] == nil {
			t.Fatalf("recalled hit shape: %+v", row)
		}
		return
	}
	t.Fatal("recalled message missing from search results")
}

func TestV1MessageSearch_RecalledTextNotInBody(t *testing.T) {
	f := newMessageFix(t)
	f.seedMessageSearch(t)

	resp, raw := f.doJSON(t, http.MethodGet,
		messageSearchPath+"?q=secret&limit=100", "sess-alice", "/me/message-search", "", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", resp.StatusCode, raw)
	}
	if bytes.Contains(raw, []byte("d05 secret")) {
		t.Fatalf("recalled original text leaked into response: %s", raw)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range searchRows(t, body) {
		if row["id"] == "direct_message:"+fmt.Sprint(sChatMin+15) {
			found = true
			if row["snippet_markdown"] != "" || row["state"] != "recalled" {
				t.Fatalf("tombstone shape wrong: %+v", row)
			}
		}
	}
	if !found {
		t.Fatalf("searching a recalled-only keyword should still surface the tombstone")
	}
}

func TestV1MessageSearch_SenderFilter(t *testing.T) {
	f := newMessageFix(t)
	notifs, dms := f.seedMessageSearch(t)

	resp, body := f.searchMsg(t, "sess-alice",
		url.Values{"q": {sMarker}, "sender_id": {fmt.Sprint(w3UserBob)}, "limit": {"100"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%d %+v", resp.StatusCode, body)
	}
	for _, row := range searchRows(t, body) {
		sender, _ := row["sender"].(map[string]any)
		if sender["id"] != fmt.Sprint(w3UserBob) {
			t.Fatalf("sender filter leaked %d: %+v", w3UserBob, row)
		}
	}
	wantBob := map[string]bool{}
	for _, n := range notifs {
		if n.receiver == w3UserAlice && !n.dropped && n.sender == w3UserBob &&
			strings.Contains(strings.ToLower(n.content), sMarker) {
			wantBob[notifID(n)] = true
		}
	}
	for _, x := range dms {
		if !x.dropped && x.room == mRoomBob && x.sender == w3UserBob &&
			strings.Contains(strings.ToLower(x.content), sMarker) {
			wantBob[dmID(x)] = true
		}
	}
	got := searchIDSet(searchRows(t, body))
	if len(got) != len(wantBob) {
		t.Fatalf("bob-sender hits = %d, want %d", len(got), len(wantBob))
	}

	resp, body = f.searchMsg(t, "sess-alice",
		url.Values{"q": {sMarker}, "sender_id": {fmt.Sprint(w3UserAlice)}, "limit": {"100"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%d %+v", resp.StatusCode, body)
	}
	gotAlice := searchIDSet(searchRows(t, body))
	if !gotAlice[notifID(sNotif{id: sNotifMin + 16})] || !gotAlice[dmID(sDM{id: sChatMin + 12})] || len(gotAlice) != 2 {
		t.Fatalf("self sender hits = %v", gotAlice)
	}

	// grant is in no room with alice: the filter narrows to an empty set.
	resp, body = f.searchMsg(t, "sess-alice",
		url.Values{"q": {sMarker}, "sender_id": {fmt.Sprint(w3UserGrant)}, "limit": {"100"}})
	if resp.StatusCode != http.StatusOK || len(searchRows(t, body)) != 0 {
		t.Fatalf("grant as seen by alice: %d %+v", resp.StatusCode, body)
	}
}

func TestV1MessageSearch_TimeWindow(t *testing.T) {
	f := newMessageFix(t)
	notifs, dms := f.seedMessageSearch(t)

	fetch := func(within string) map[string]bool {
		t.Helper()
		q := url.Values{"q": {sMarker}, "limit": {"100"}}
		if within != "" {
			q.Set("within", within)
		}
		resp, body := f.searchMsg(t, "sess-alice", q)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("within=%s: %d %+v", within, resp.StatusCode, body)
		}
		return searchIDSet(searchRows(t, body))
	}
	all := fetch("")
	d7 := fetch("7d")
	d30 := fetch("30d")
	now := time.Now().UTC()

	for _, n := range notifs {
		id := notifID(n)
		if !all[id] {
			continue
		}
		age := now.Sub(n.at)
		if age <= 7*24*time.Hour && !d7[id] {
			t.Fatalf("%s inside 7d missing", id)
		}
		if age > 7*24*time.Hour && d7[id] {
			t.Fatalf("%s outside 7d present", id)
		}
		if age <= 30*24*time.Hour && !d30[id] {
			t.Fatalf("%s inside 30d missing", id)
		}
		if age > 30*24*time.Hour && d30[id] {
			t.Fatalf("%s outside 30d present", id)
		}
	}
	for _, x := range dms {
		id := dmID(x)
		if !all[id] {
			continue
		}
		age := now.Sub(x.at)
		if age <= 7*24*time.Hour && !d7[id] {
			t.Fatalf("%s inside 7d missing", id)
		}
		if age > 7*24*time.Hour && d7[id] {
			t.Fatalf("%s outside 7d present", id)
		}
		if age <= 30*24*time.Hour && !d30[id] {
			t.Fatalf("%s inside 30d missing", id)
		}
		if age > 30*24*time.Hour && d30[id] {
			t.Fatalf("%s outside 30d present", id)
		}
	}
	if len(d7) >= len(d30) || len(d30) >= len(all) {
		t.Fatalf("window sets not nested: 7d=%d 30d=%d all=%d", len(d7), len(d30), len(all))
	}
}

func TestV1MessageSearch_PaginationNoDupNoMiss(t *testing.T) {
	f := newMessageFix(t)
	notifs, dms := f.seedMessageSearch(t)

	base := url.Values{"q": {sMarker}, "limit": {"1"}}
	walked, _ := walkSearch(t, f, "sess-alice", base)
	if len(walked) == 0 {
		t.Fatal("no walked rows")
	}
	seen := map[string]int{}
	for _, row := range walked {
		seen[row["id"].(string)]++
	}
	for id, n := range seen {
		if n > 1 {
			t.Fatalf("hit %s appeared %d times across pages", id, n)
		}
	}
	want := visibleAlice(notifs, dms)
	if len(seen) != len(want) {
		t.Fatalf("walked %d rows, want %d", len(seen), len(want))
	}
	for id := range want {
		if seen[id] == 0 {
			t.Fatalf("walk missed %s", id)
		}
	}

	// Cursor is bound to q, sender_id and within.
	resp, body := f.searchMsg(t, "sess-alice", url.Values{"q": {sMarker}, "limit": {"2"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%d %+v", resp.StatusCode, body)
	}
	cur, _ := body["next_cursor"].(string)
	if cur == "" {
		t.Fatal("expected a cursor")
	}
	for _, query := range []url.Values{
		{"q": {"anotherphrase"}, "limit": {"2"}, "cursor": {cur}},
		{"q": {sMarker}, "limit": {"2"}, "cursor": {cur}, "sender_id": {fmt.Sprint(w3UserBob)}},
		{"q": {sMarker}, "limit": {"2"}, "cursor": {cur}, "within": {"7d"}},
	} {
		resp, body = f.searchMsg(t, "sess-alice", query)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("cursor reuse across params: %d %+v", resp.StatusCode, body)
		}
		if code, _ := body["code"].(string); code != "INVALID_CURSOR" {
			t.Fatalf("code = %v, want INVALID_CURSOR", body["code"])
		}
	}
}

func TestV1MessageSearch_WildcardsAreLiteral(t *testing.T) {
	f := newMessageFix(t)
	f.seedMessageSearch(t)

	resp, body := f.searchMsg(t, "sess-alice", url.Values{"q": {"100%"}, "limit": {"100"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%d %+v", resp.StatusCode, body)
	}
	got := searchIDSet(searchRows(t, body))
	if !got["direct_message:"+fmt.Sprint(sChatMin+16)] || len(got) != 1 {
		t.Fatalf("%% literal match set: %v", got)
	}

	resp, body = f.searchMsg(t, "sess-alice", url.Values{"q": {"a_b"}, "limit": {"100"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%d %+v", resp.StatusCode, body)
	}
	got = searchIDSet(searchRows(t, body))
	if !got["direct_message:"+fmt.Sprint(sChatMin+17)] {
		t.Fatalf("_ literal match set: %v", got)
	}
	// Negative controls prove % and _ are not wildcard metacharacters.
	if got["direct_message:"+fmt.Sprint(sChatMin+30)] ||
		got["direct_message:"+fmt.Sprint(sChatMin+31)] {
		t.Fatalf("underscore matched as wildcard: %v", got)
	}

	resp, body = f.searchMsg(t, "sess-alice", url.Values{"q": {"100%"}, "limit": {"100"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%d %+v", resp.StatusCode, body)
	}
	if searchIDSet(searchRows(t, body))["direct_message:"+fmt.Sprint(sChatMin+29)] {
		t.Fatalf("percent matched as wildcard: 100Xdone surfaced for q=100%%")
	}
}

func TestV1MessageSearch_Validation(t *testing.T) {
	f := newMessageFix(t)
	f.seedMessageSearch(t)

	cases := []struct {
		name  string
		query url.Values
		want  string
	}{
		{"blank q", url.Values{"q": {"   "}}, "INVALID_PARAMETER"},
		{"missing q", url.Values{}, "http400"},
		{"overlong q", url.Values{"q": {strings.Repeat("补", 51)}}, "http400"},
		{"sender zero", url.Values{"q": {sMarker}, "sender_id": {"0"}}, "http400"},
		{"sender letters", url.Values{"q": {sMarker}, "sender_id": {"abc"}}, "http400"},
		{"within unknown", url.Values{"q": {sMarker}, "within": {"9d"}}, "UNKNOWN_ENUM_VALUE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := f.searchMsg(t, "sess-alice", tc.query)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("%s: %d %+v", tc.name, resp.StatusCode, body)
			}
			if tc.want != "http400" {
				if code, _ := body["code"].(string); code != tc.want {
					t.Fatalf("%s: code %v want %s", tc.name, body["code"], tc.want)
				}
			}
		})
	}

	resp, raw := f.doJSON(t, http.MethodGet, messageSearchPath+"?q="+url.QueryEscape(sMarker), "", "/me/message-search", "", nil, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: %d %s", resp.StatusCode, raw)
	}
}

func TestV1MessageSearch_DroppedRowsRefill(t *testing.T) {
	f := newMessageFix(t)
	notifs, dms := f.seedMessageSearch(t)

	// The newest rows in both sources are dropped (banned actor, unknown type,
	// banned peer); the walk must still return every renderable row.
	walked, _ := walkSearch(t, f, "sess-alice", url.Values{"q": {sMarker}, "limit": {"3"}})
	want := visibleAlice(notifs, dms)
	got := searchIDSet(walked)
	if len(got) != len(want) {
		t.Fatalf("refill walked %d, want %d", len(got), len(want))
	}
	for _, banned := range []string{
		"notification:" + fmt.Sprint(sNotifMin+17),
		"notification:" + fmt.Sprint(sNotifMin+18),
		"direct_message:" + fmt.Sprint(sChatMin+32),
	} {
		if got[banned] {
			t.Fatalf("dropped row surfaced: %s", banned)
		}
	}

	// An account-service outage degrades to deleted-user refs, like the inbox
	// lists do; the request itself still succeeds.
	f.failOA.Store(true)
	resp, body := f.searchMsg(t, "sess-alice", url.Values{"q": {sMarker}, "limit": {"100"}})
	f.failOA.Store(false)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("oauth outage should degrade, not fail: %d %+v", resp.StatusCode, body)
	}
	if len(searchRows(t, body)) != len(want) {
		t.Fatalf("outage changed the visible set: %d want %d", len(searchRows(t, body)), len(want))
	}
}

func TestV1MessageSearch_CaseInsensitive(t *testing.T) {
	f := newMessageFix(t)
	f.seedMessageSearch(t)

	resp, body := f.searchMsg(t, "sess-alice", url.Values{"q": {"zQmArKeR"}, "limit": {"100"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%d %+v", resp.StatusCode, body)
	}
	if !searchIDSet(searchRows(t, body))["notification:"+fmt.Sprint(sNotifMin+11)] {
		t.Fatal("uppercase-stored row should match mixed-case phrase")
	}
}
