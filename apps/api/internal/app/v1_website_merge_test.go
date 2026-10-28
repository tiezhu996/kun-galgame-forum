package app

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// mergeSites builds a source (r18) and a survivor with a deliberate overlap
// shape: alice likes/favorites both, the source carries one single-group tag
// the survivor's group already holds and one ungrouped tag the survivor lacks.
func mergeSites(t *testing.T, f *writeFix) (source, target int, tagLoose int) {
	t.Helper()
	run := func(q string, args ...any) {
		t.Helper()
		if err := f.db.Exec(q, args...).Error; err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}

	createTag := func(slug string, group *int) int {
		body := map[string]any{"slug": slug, "label": "L " + slug, "level": 3}
		if group != nil {
			body["website_tag_group_id"] = fmt.Sprint(*group)
		}
		resp, got := f.ws(t, http.MethodPost, "/api/v1/admin/website-tags",
			"/admin/website-tags", "sess-staff", nil, body)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create tag %s: %d %+v", slug, resp.StatusCode, got)
		}
		return asInt(got["id"])
	}
	tagConflict := createTag("wt-mrg-c", ptr(wsGroupSingle))
	tagLoose = createTag("wt-mrg-n", nil)

	createSite := func(host string, tagIDs []int, nsfw bool) int {
		body := websiteBody(host, "MG "+host, map[string]any{
			"website_tag_ids": toDecimalIDs(tagIDs),
			"urls":            []string{"https://shared.example", "https://" + host},
			"is_nsfw":         nsfw,
		})
		resp, got := f.ws(t, http.MethodPost, "/api/v1/admin/websites",
			"/admin/websites", "sess-staff", nil, body)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create site %s: %d %+v", host, resp.StatusCode, got)
		}
		return asInt(got["id"])
	}
	target = createSite("mg-target.example", []int{wsTagA, wsTagMulti}, false)
	source = createSite("mg-source.example", []int{tagConflict, tagLoose}, true)

	run(`INSERT INTO galgame_website_like (user_id, website_id, created, updated) VALUES
		(?, ?, now(), now()), (?, ?, now(), now()), (?, ?, now(), now()), (?, ?, now(), now())`,
		w3UserAlice, source, w3UserBob, source, w3UserAlice, target, w3UserGrant, target)
	run(`INSERT INTO galgame_website_favorite (user_id, website_id, created, updated) VALUES
		(?, ?, now(), now()), (?, ?, now(), now()), (?, ?, now(), now())`,
		w3UserBob, source, w3UserAlice, target, w3UserGrant, target)
	run(`UPDATE galgame_website SET view = ?, comment_count = ? WHERE id = ?`, 7, 3, source)
	run(`UPDATE galgame_website SET view = ?, comment_count = ? WHERE id = ?`, 2, 2, target)

	now := time.Now()
	run(`INSERT INTO feed_activity (type, source_id, user_id, content, link, created) VALUES
		('GALGAME_WEBSITE_COMMENT_CREATION', 9_410_501, ?, 'c', ?, ?),
		('GALGAME_WEBSITE_COMMENT_CREATION', 9_410_502, ?, 'c', ?, ?)`,
		w3UserBob, "/website/mg-source.example", now,
		w3UserBob, "/website/mg-source.example?x=1", now)
	run(`INSERT INTO message (content, link, type, sender_id, receiver_id, updated, created) VALUES
		('c', ?, 'commented', ?, ?, ?, ?)`, "/website/mg-source.example#c", w3UserBob, w3UserAlice, now, now)
	t.Cleanup(func() {
		_ = f.db.Exec(`DELETE FROM feed_activity WHERE type = 'GALGAME_WEBSITE_COMMENT_CREATION' AND source_id IN (9410501, 9410502)`).Error
		_ = f.db.Exec(`DELETE FROM message WHERE sender_id = ? AND type = 'commented' AND link LIKE '/website/mg-%'`, w3UserBob).Error
		_ = f.db.Exec(`DELETE FROM galgame_website_merge WHERE old_host LIKE 'mg-%.example' OR new_website_id IN (?, ?)`, source, target).Error
	})
	return source, target, tagLoose
}

func toDecimalIDs(ids []int) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = fmt.Sprint(id)
	}
	return out
}

func mergeBody(source, target int) map[string]any {
	return map[string]any{
		"source_website_id": fmt.Sprint(source),
		"target_website_id": fmt.Sprint(target),
	}
}

func TestV1WebsiteMergePreview(t *testing.T) {
	f := newWebsiteFix(t)
	source, target, tagLoose := mergeSites(t, f)
	ledgerBefore := f.scalar(t, `SELECT count(*) FROM galgame_website_merge`)

	resp, body := f.ws(t, http.MethodPost, "/api/v1/admin/website-merges/preview",
		"/admin/website-merges/preview", "sess-staff", nil, mergeBody(source, target))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview %d %+v", resp.StatusCode, body)
	}
	if body["object"] != "website_merge_preview" {
		t.Fatalf("object = %v", body["object"])
	}
	likes := body["likes"].(map[string]any)
	if asInt(likes["source_count"]) != 2 || asInt(likes["overlap"]) != 1 || asInt(likes["moved"]) != 1 || asInt(likes["resulting"]) != 3 {
		t.Errorf("likes = %+v", likes)
	}
	favs := body["favorites"].(map[string]any)
	if asInt(favs["source_count"]) != 1 || asInt(favs["overlap"]) != 0 || asInt(favs["moved"]) != 1 || asInt(favs["resulting"]) != 3 {
		t.Errorf("favorites = %+v", favs)
	}
	added := body["tags_added"].([]any)
	if len(added) != 1 || asInt(added[0].(map[string]any)["id"]) != tagLoose {
		t.Errorf("tags_added = %+v", added)
	}
	dropped := body["tags_dropped"].([]any)
	if len(dropped) != 1 {
		t.Fatalf("tags_dropped = %+v, want one single-group conflict", dropped)
	}
	d := dropped[0].(map[string]any)
	if d["drop_reason"] != "single_select_group" || asInt(d["survivor_tag"].(map[string]any)["id"]) != wsTagA {
		t.Errorf("drop = %+v", d)
	}
	if asInt(body["comments_resulting"]) != 5 || asInt(body["view_resulting"]) != 9 || body["is_target_upgraded_to_nsfw"] != true {
		t.Errorf("totals = comments %v view %v nsfw %v", body["comments_resulting"], body["view_resulting"], body["is_target_upgraded_to_nsfw"])
	}
	urls := body["urls_added"].([]any)
	if len(urls) != 1 || urls[0] != "https://mg-source.example" {
		t.Errorf("urls_added = %+v", urls)
	}
	if n := f.scalar(t, `SELECT count(*) FROM galgame_website_merge`); n != ledgerBefore {
		t.Errorf("preview wrote %d ledger rows", n)
	}
	if n := f.scalar(t, `SELECT view FROM galgame_website WHERE id = ?`, target); n != 2 {
		t.Errorf("preview changed the survivor view: %d", n)
	}
}

func TestV1WebsiteMergeExecute(t *testing.T) {
	f := newWebsiteFix(t)
	source, target, _ := mergeSites(t, f)
	sourceHost, targetHost := "mg-source.example", "mg-target.example"

	key := keyUUID(701)
	hdr := http.Header{"Idempotency-Key": {key}}
	resp, body := f.ws(t, http.MethodPost, "/api/v1/admin/website-merges",
		"/admin/website-merges", "sess-staff", hdr, mergeBody(source, target))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("execute %d %+v", resp.StatusCode, body)
	}
	if body["object"] != "website_merge" || body["source_host"] != sourceHost || body["target_host"] != targetHost {
		t.Errorf("merge body = %+v", body)
	}
	likes := body["likes"].(map[string]any)
	if asInt(likes["resulting"]) != 3 || asInt(likes["moved"]) != 1 || asInt(likes["overlap"]) != 1 {
		t.Errorf("result likes = %+v", likes)
	}
	if asInt(body["tags_moved_count"]) != 1 || asInt(body["tags_dropped_count"]) != 1 || asInt(body["comments_resulting"]) != 5 {
		t.Errorf("result tags/comments = %+v", body)
	}

	if n := f.scalar(t, `SELECT count(*) FROM galgame_website WHERE id = ?`, source); n != 0 {
		t.Errorf("source row survived")
	}
	for _, c := range []struct {
		table string
		want  int
	}{
		{"galgame_website_like", 3}, {"galgame_website_favorite", 3},
	} {
		if n := f.scalar(t, fmt.Sprintf(`SELECT count(*) FROM %s WHERE website_id = ?`, c.table), target); n != c.want {
			t.Errorf("%s on survivor = %d, want %d", c.table, n, c.want)
		}
		if n := f.scalar(t, fmt.Sprintf(`SELECT count(DISTINCT user_id) FROM %s WHERE website_id = ?`, c.table), target); n != c.want {
			t.Errorf("%s distinct users on survivor = %d, want %d", c.table, n, c.want)
		}
	}
	var counts struct {
		Like     int    `gorm:"column:like_count"`
		Fav      int    `gorm:"column:favorite_count"`
		Comments int    `gorm:"column:comment_count"`
		View     int    `gorm:"column:view"`
		Age      string `gorm:"column:age_limit"`
	}
	f.db.Raw(`SELECT like_count, favorite_count, comment_count, view, age_limit FROM galgame_website WHERE id = ?`,
		target).Scan(&counts)
	if counts.Like != 3 || counts.Fav != 3 || counts.Comments != 5 || counts.View != 9 || counts.Age != "r18" {
		t.Errorf("survivor counts = %+v", counts)
	}
	if n := f.scalar(t, `SELECT dropped_like FROM galgame_website_merge WHERE old_website_id = ?`, source); n != 1 {
		t.Errorf("dropped_like ledger = %d", n)
	}

	// The old host now names the survivor on every host-keyed read.
	for _, c := range []struct {
		method, url, spec string
	}{
		{http.MethodGet, "/api/v1/websites/" + sourceHost, "/websites/{website_host}"},
		{http.MethodPut, "/api/v1/websites/" + sourceHost + "/like", "/websites/{website_host}/like"},
		{http.MethodDelete, "/api/v1/websites/" + sourceHost + "/favorite", "/websites/{website_host}/favorite"},
	} {
		resp, got := f.ws(t, c.method, c.url, c.spec, "sess-alice", nil, nil)
		if resp.StatusCode != http.StatusNotFound || got["code"] != "ENTITY_MERGED" ||
			got["current_id"] != fmt.Sprint(target) || got["current_host"] != targetHost {
			t.Errorf("%s %s -> %d %+v, want ENTITY_MERGED -> %d", c.method, c.url, resp.StatusCode, got, target)
		}
	}

	links := f.sqlIDs(t, `SELECT link FROM feed_activity
		WHERE type = 'GALGAME_WEBSITE_COMMENT_CREATION' AND source_id IN (9410501, 9410502) ORDER BY source_id`)
	if fmt.Sprint(links) != fmt.Sprint([]string{"/website/" + targetHost, "/website/" + targetHost + "?x=1"}) {
		t.Errorf("feed links = %v", links)
	}
	msgs := f.sqlIDs(t, `SELECT link FROM message WHERE type = 'commented'`)
	if fmt.Sprint(msgs) != fmt.Sprint([]string{"/website/" + targetHost + "#c"}) {
		t.Errorf("message links = %v", msgs)
	}
	if n := f.scalar(t, `SELECT count(*) FROM feed_activity WHERE type = 'GALGAME_WEBSITE_CREATION' AND source_id = ?`, source); n != 0 {
		t.Errorf("source creation card survived")
	}
}

func TestV1WebsiteMergeIdempotency(t *testing.T) {
	f := newWebsiteFix(t)
	source, target, _ := mergeSites(t, f)
	third := 930000802

	key := keyUUID(702)
	hdr := http.Header{"Idempotency-Key": {key}}
	first, firstBody := f.ws(t, http.MethodPost, "/api/v1/admin/website-merges",
		"/admin/website-merges", "sess-staff", hdr, mergeBody(source, target))
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first %d %+v", first.StatusCode, firstBody)
	}
	replay, replayBody := f.ws(t, http.MethodPost, "/api/v1/admin/website-merges",
		"/admin/website-merges", "sess-staff", hdr, mergeBody(source, target))
	if replay.StatusCode != http.StatusOK || replay.Header.Get("Idempotency-Replayed") != "true" {
		t.Fatalf("replay %d replayed=%q", replay.StatusCode, replay.Header.Get("Idempotency-Replayed"))
	}
	if replayBody["id"] != firstBody["id"] {
		t.Errorf("replay ledger id = %v, want %v", replayBody["id"], firstBody["id"])
	}

	// A different key (or none at all) after the Redis record expires still has
	// to be a no-op replay rather than a second fold.
	natural, naturalBody := f.ws(t, http.MethodPost, "/api/v1/admin/website-merges",
		"/admin/website-merges", "sess-staff", http.Header{"Idempotency-Key": {keyUUID(703)}},
		mergeBody(source, target))
	if natural.StatusCode != http.StatusOK || naturalBody["id"] != firstBody["id"] {
		t.Errorf("natural replay %d %+v", natural.StatusCode, naturalBody)
	}
	if n := f.scalar(t, `SELECT count(*) FROM galgame_website_merge WHERE old_website_id = ?`, source); n != 1 {
		t.Errorf("ledger rows = %d, want 1", n)
	}
	if n := f.scalar(t, `SELECT like_count FROM galgame_website WHERE id = ?`, target); n != 3 {
		t.Errorf("like_count after replays = %d, want 3", n)
	}

	resp, body := f.ws(t, http.MethodPost, "/api/v1/admin/website-merges",
		"/admin/website-merges", "sess-staff", http.Header{"Idempotency-Key": {keyUUID(704)}},
		mergeBody(source, third))
	if resp.StatusCode != http.StatusConflict || body["code"] != "ALREADY_EXISTS" {
		t.Errorf("different survivor = %d %+v", resp.StatusCode, body)
	}
	resp, body = f.ws(t, http.MethodPost, "/api/v1/admin/website-merges",
		"/admin/website-merges", "sess-staff", http.Header{"Idempotency-Key": {keyUUID(705)}},
		mergeBody(target, target))
	if resp.StatusCode != http.StatusUnprocessableEntity || body["code"] != "VALIDATION_FAILED" {
		t.Errorf("same ids = %d %+v", resp.StatusCode, body)
	}
	resp, body = f.ws(t, http.MethodPost, "/api/v1/admin/website-merges",
		"/admin/website-merges", "sess-staff", nil, mergeBody(source, target))
	if resp.StatusCode != http.StatusBadRequest || body["code"] != "INVALID_PARAMETER" {
		t.Errorf("missing key = %d %+v", resp.StatusCode, body)
	}
}

func TestV1WebsiteMergePermissions(t *testing.T) {
	f := newWebsiteFix(t)
	source, target, _ := mergeSites(t, f)
	hdr := http.Header{"Idempotency-Key": {keyUUID(706)}}
	for _, c := range []struct {
		name, path, session string
		header              http.Header
		want                string
	}{
		{"anonymous preview", "/admin/website-merges/preview", "", nil, "MISSING_CREDENTIAL"},
		{"bearer preview", "/admin/website-merges/preview", "",
			http.Header{"Authorization": {"Bearer staff-token"}}, "PERMISSION_REQUIRED"},
		{"banned preview", "/admin/website-merges/preview", "sess-banned", nil, "ACCOUNT_BANNED"},
		{"anonymous execute", "/admin/website-merges", "", hdr, "MISSING_CREDENTIAL"},
		{"bearer execute", "/admin/website-merges", "",
			http.Header{"Authorization": {"Bearer staff-token"}, "Idempotency-Key": {keyUUID(707)}}, "PERMISSION_REQUIRED"},
		{"banned execute", "/admin/website-merges", "sess-banned", hdr, "ACCOUNT_BANNED"},
	} {
		t.Run(c.name, func(t *testing.T) {
			resp, body := f.ws(t, http.MethodPost, "/api/v1"+c.path, c.path, c.session, c.header, mergeBody(source, target))
			if body["code"] != c.want {
				t.Errorf("%s: %d %+v, want %s", c.name, resp.StatusCode, body, c.want)
			}
		})
	}
	if n := f.scalar(t, `SELECT count(*) FROM galgame_website_merge`); n != 0 {
		t.Errorf("permission-refused writes left a ledger: %d", n)
	}
}

func TestV1WebsiteMergeChain(t *testing.T) {
	f := newWebsiteFix(t)
	create := func(host string) int {
		resp, body := f.ws(t, http.MethodPost, "/api/v1/admin/websites", "/admin/websites", "sess-staff", nil,
			websiteBody(host, "Chain "+host, nil))
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create %s: %d %+v", host, resp.StatusCode, body)
		}
		return asInt(body["id"])
	}
	a, b, c := create("mg-a.example"), create("mg-b.example"), create("mg-c.example")
	t.Cleanup(func() {
		_ = f.db.Exec(`DELETE FROM galgame_website_merge WHERE old_host IN ('mg-a.example', 'mg-b.example')`).Error
	})

	for _, step := range []struct{ src, dst int }{
		{a, b}, {b, c},
	} {
		resp, body := f.ws(t, http.MethodPost, "/api/v1/admin/website-merges", "/admin/website-merges",
			"sess-staff", http.Header{"Idempotency-Key": {keyUUID(710 + step.src)}},
			mergeBody(step.src, step.dst))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("fold %d->%d: %d %+v", step.src, step.dst, resp.StatusCode, body)
		}
	}
	for _, host := range []string{"mg-a.example", "mg-b.example"} {
		resp, body := f.ws(t, http.MethodGet, "/api/v1/websites/"+host,
			"/websites/{website_host}", "", nil, nil)
		if resp.StatusCode != http.StatusNotFound || body["code"] != "ENTITY_MERGED" || body["current_host"] != "mg-c.example" {
			t.Errorf("%s -> %d %+v", host, resp.StatusCode, body)
		}
	}
	if n := f.scalar(t, `SELECT count(*) FROM galgame_website_merge WHERE new_website_id = ?`, c); n != 2 {
		t.Errorf("ledger rows pointing at C = %d, want 2", n)
	}
}
