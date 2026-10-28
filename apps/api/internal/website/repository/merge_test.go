package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"kun-galgame-api/internal/testdb"

	"gorm.io/gorm"
)

const (
	mgCat      = 9_410_701
	mgSingle   = 9_410_711
	mgMulti    = 9_410_712
	mgTagA     = 9_410_721
	mgTagB     = 9_410_722
	mgTagM     = 9_410_723
	mgTagLoose = 9_410_724
	mgSiteA    = 9_410_801
	mgSiteB    = 9_410_802
	mgSiteC    = 9_410_803
	mgUser1    = 9_410_901
	mgUser2    = 9_410_902
	mgUser3    = 9_410_903
)

func mgHost(id int) string { return fmt.Sprintf("mg-site-%d.example", id) }

func mergeTestFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db := testdb.Open(t)
	sites := []any{mgSiteA, mgSiteB, mgSiteC}
	cleanup := func() {
		for _, c := range []struct {
			q    string
			args []any
		}{
			{`DELETE FROM galgame_website_merge WHERE old_website_id IN (?, ?, ?) OR new_website_id IN (?, ?, ?)`,
				append(append([]any{}, sites...), sites...)},
			{`DELETE FROM feed_activity WHERE link LIKE '/website/mg-site-%' OR source_id IN (9410501, 9410502)`, nil},
			{`DELETE FROM message WHERE link LIKE '/website/mg-site-%'`, nil},
			{`DELETE FROM galgame_website_like WHERE website_id IN (?, ?, ?)`, sites},
			{`DELETE FROM galgame_website_favorite WHERE website_id IN (?, ?, ?)`, sites},
			{`DELETE FROM galgame_website_tag_relation WHERE galgame_website_id IN (?, ?, ?)`, sites},
			{`DELETE FROM galgame_website WHERE id IN (?, ?, ?)`, sites},
			{`DELETE FROM galgame_website_tag WHERE id IN (?, ?, ?, ?)`, []any{mgTagA, mgTagB, mgTagM, mgTagLoose}},
			{`DELETE FROM galgame_website_tag_group WHERE id IN (?, ?)`, []any{mgSingle, mgMulti}},
			{`DELETE FROM galgame_website_category WHERE id = ?`, []any{mgCat}},
		} {
			_ = db.Exec(c.q, c.args...).Error
		}
	}
	cleanup()
	t.Cleanup(cleanup)
	run := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
	base := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)
	run(`INSERT INTO galgame_website_category (id, name, label, description, sort_order, created, updated)
		VALUES (?, 'mg-cat', 'MG', '', 1, ?, ?)`, mgCat, base, base)
	run(`INSERT INTO galgame_website_tag_group (id, name, label, description, sort_order, multi_select, created, updated)
		VALUES (?, 'mg-single', 'single', '', 1, false, ?, ?), (?, 'mg-multi', 'multi', '', 2, true, ?, ?)`,
		mgSingle, base, base, mgMulti, base, base)
	run(`INSERT INTO galgame_website_tag (id, level, name, label, description, group_id, created, updated) VALUES
		(?, 10, 'mg-a', 'A', '', ?, ?, ?), (?, 10, 'mg-b', 'B', '', ?, ?, ?),
		(?, 10, 'mg-m', 'M', '', ?, ?, ?), (?, 10, 'mg-loose', 'L', '', NULL, ?, ?)`,
		mgTagA, mgSingle, base, base, mgTagB, mgSingle, base, base,
		mgTagM, mgMulti, base, base, mgTagLoose, base, base)
	return db
}

// mergeSeedSites builds A (source, r18) and B (survivor) with the overlap
// shape the assertions use; foldSites additionally creates C.
func mergeSeedSites(t *testing.T, db *gorm.DB, foldSites bool) {
	t.Helper()
	run := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
	site := func(id int, age string, view, comments int, domain string) {
		run(`INSERT INTO galgame_website (id, name, url, create_time, description, language, age_limit, domain,
			category_id, user_id, view, like_count, favorite_count, comment_count, created, updated, status)
			VALUES (?, ?, ?, '2020-01-01', 'desc long enough', 'ja', ?, ?::jsonb, ?, ?, ?, ?, ?, ?, now(), now(), 'normal')`,
			id, fmt.Sprintf("MG Site %d", id), mgHost(id), age, domain, mgCat, mgUser1, view, 0, 0, comments)
	}
	site(mgSiteA, "r18", 10, 4, `["https://a.example","https://extra-a.example"]`)
	site(mgSiteB, "all", 20, 6, `["https://b.example","https://a.example"]`)
	if foldSites {
		site(mgSiteC, "all", 30, 8, `["https://c.example"]`)
	}
	for _, rel := range [][2]int{{mgSiteA, mgTagA}, {mgSiteA, mgTagM}, {mgSiteA, mgTagLoose}, {mgSiteB, mgTagB}, {mgSiteB, mgTagM}} {
		run(`INSERT INTO galgame_website_tag_relation (galgame_website_id, galgame_website_tag_id, created, updated) VALUES (?, ?, now(), now())`,
			rel[0], rel[1])
	}
	for _, u := range []int{mgUser1, mgUser2} {
		run(`INSERT INTO galgame_website_like (user_id, website_id, created, updated) VALUES (?, ?, now(), now())`, u, mgSiteA)
	}
	for _, u := range []int{mgUser1, mgUser3} {
		run(`INSERT INTO galgame_website_like (user_id, website_id, created, updated) VALUES (?, ?, now(), now())`, u, mgSiteB)
	}
	run(`INSERT INTO galgame_website_favorite (user_id, website_id, created, updated) VALUES (?, ?, now(), now())`, mgUser1, mgSiteA)
	for _, u := range []int{mgUser2, mgUser3} {
		run(`INSERT INTO galgame_website_favorite (user_id, website_id, created, updated) VALUES (?, ?, now(), now())`, u, mgSiteB)
	}
	now := time.Now()
	run(`INSERT INTO feed_activity (type, source_id, user_id, content, link, created) VALUES
		('GALGAME_WEBSITE_COMMENT_CREATION', 9_410_501, ?, 'c', ?, ?),
		('GALGAME_WEBSITE_COMMENT_CREATION', 9_410_502, ?, 'c', ?, ?)`,
		mgUser1, "/website/"+mgHost(mgSiteA), now, mgUser1, "/website/"+mgHost(mgSiteA)+"?x=1", now)
	run(`INSERT INTO message (content, link, type, sender_id, receiver_id, updated, created) VALUES
		('c', ?, 'commented', ?, ?, ?, ?)`, "/website/"+mgHost(mgSiteA)+"#c", mgUser2, mgUser3, now, now)
}

func mgInt(t *testing.T, db *gorm.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.Raw(q, args...).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func mgTagIDs(t *testing.T, db *gorm.DB, siteID int) []int {
	t.Helper()
	var ids []int
	if err := db.Table("galgame_website_tag_relation").Where("galgame_website_id = ?", siteID).
		Order("galgame_website_tag_id").Pluck("galgame_website_tag_id", &ids).Error; err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestWebsiteMergePlanNumbers(t *testing.T) {
	db := mergeTestFixture(t)
	mergeSeedSites(t, db, false)

	plan, err := NewStore(db).WebsiteMergePlan(mgSiteA, mgSiteB)
	if err != nil {
		t.Fatal(err)
	}
	if plan.LikeSource != 2 || plan.LikeOverlap != 1 || plan.LikeMoved != 1 || plan.LikeResulting != 3 {
		t.Errorf("like plan = +%d overlap %d moved %d result %d, want 2/1/1/3",
			plan.LikeSource, plan.LikeOverlap, plan.LikeMoved, plan.LikeResulting)
	}
	if plan.FavoriteSource != 1 || plan.FavoriteOverlap != 0 || plan.FavoriteMoved != 1 || plan.FavoriteResulting != 3 {
		t.Errorf("favorite plan = +%d overlap %d moved %d result %d, want 1/0/1/3",
			plan.FavoriteSource, plan.FavoriteOverlap, plan.FavoriteMoved, plan.FavoriteResulting)
	}
	if fmt.Sprint(plan.TagsAdded) != fmt.Sprint([]int{mgTagLoose}) {
		t.Errorf("tags added = %v, want [%d]", plan.TagsAdded, mgTagLoose)
	}
	if len(plan.TagsDropped) != 2 {
		t.Fatalf("tags dropped = %+v, want tA (single group) and tM (dup)", plan.TagsDropped)
	}
	dropByTag := map[int]MergeTagDrop{}
	for _, d := range plan.TagsDropped {
		dropByTag[d.TagID] = d
	}
	if d := dropByTag[mgTagA]; d.SurvivorTagID == nil || *d.SurvivorTagID != mgTagB {
		t.Errorf("tA drop = %+v, want blocked by survivor tB", d)
	}
	if d := dropByTag[mgTagM]; d.SurvivorTagID != nil {
		t.Errorf("tM is a plain duplicate, got survivor pointer %+v", d)
	}
	if plan.ViewResulting != 30 || plan.CommentResulting != 10 {
		t.Errorf("view/comment = %d/%d, want 30/10", plan.ViewResulting, plan.CommentResulting)
	}
	if fmt.Sprint(plan.URLsAdded) != fmt.Sprint([]string{"https://extra-a.example"}) {
		t.Errorf("urls added = %v", plan.URLsAdded)
	}
	if !plan.TargetBecomesNSFW {
		t.Errorf("target should become nsfw when the source is r18")
	}
}

type foldSnapshot struct {
	websiteCount, likeA, likeB, favA, favB, tagA, tagB, ledger, feedA, feedB, msgA, msgB int
}

func snapshotFold(t *testing.T, db *gorm.DB) foldSnapshot {
	t.Helper()
	s := foldSnapshot{
		websiteCount: mgInt(t, db, `SELECT count(*) FROM galgame_website WHERE id IN (?, ?)`, mgSiteA, mgSiteB),
		likeA:        mgInt(t, db, `SELECT count(*) FROM galgame_website_like WHERE website_id = ?`, mgSiteA),
		likeB:        mgInt(t, db, `SELECT count(*) FROM galgame_website_like WHERE website_id = ?`, mgSiteB),
		favA:         mgInt(t, db, `SELECT count(*) FROM galgame_website_favorite WHERE website_id = ?`, mgSiteA),
		favB:         mgInt(t, db, `SELECT count(*) FROM galgame_website_favorite WHERE website_id = ?`, mgSiteB),
		tagA:         mgInt(t, db, `SELECT count(*) FROM galgame_website_tag_relation WHERE galgame_website_id = ?`, mgSiteA),
		tagB:         mgInt(t, db, `SELECT count(*) FROM galgame_website_tag_relation WHERE galgame_website_id = ?`, mgSiteB),
		ledger:       mgInt(t, db, `SELECT count(*) FROM galgame_website_merge WHERE old_website_id = ?`, mgSiteA),
		feedA:        mgInt(t, db, `SELECT count(*) FROM feed_activity WHERE link LIKE ?`, "/website/"+mgHost(mgSiteA)+"%"),
		feedB:        mgInt(t, db, `SELECT count(*) FROM feed_activity WHERE link LIKE ?`, "/website/"+mgHost(mgSiteB)+"%"),
		msgA:         mgInt(t, db, `SELECT count(*) FROM message WHERE link LIKE ?`, "/website/"+mgHost(mgSiteA)+"%"),
		msgB:         mgInt(t, db, `SELECT count(*) FROM message WHERE link LIKE ?`, "/website/"+mgHost(mgSiteB)+"%"),
	}
	return s
}

func TestFoldWebsiteMovesEverythingOnce(t *testing.T) {
	db := mergeTestFixture(t)
	mergeSeedSites(t, db, false)

	res, err := NewStore(db).FoldWebsite(mgSiteA, mgSiteB, 99)
	if err != nil {
		t.Fatalf("fold: %v", err)
	}
	if res.MovedLike != 1 || res.DroppedLike != 1 || res.MovedFavorite != 1 || res.DroppedFavorite != 0 ||
		res.MovedTag != 1 || res.DroppedTag != 2 || res.OldHost != mgHost(mgSiteA) || res.NewHost != mgHost(mgSiteB) {
		t.Errorf("fold result = %+v", res)
	}

	if n := mgInt(t, db, `SELECT count(*) FROM galgame_website WHERE id = ?`, mgSiteA); n != 0 {
		t.Errorf("source row still exists")
	}
	if n := mgInt(t, db, `SELECT count(*) FROM galgame_website_like WHERE website_id = ?`, mgSiteB); n != 3 {
		t.Errorf("survivor likes = %d, want 3", n)
	}
	if n := mgInt(t, db, `SELECT count(DISTINCT user_id) FROM galgame_website_like WHERE website_id = ?`, mgSiteB); n != 3 {
		t.Errorf("survivor likes by distinct users = %d, want 3", n)
	}
	if n := mgInt(t, db, `SELECT count(*) FROM galgame_website_favorite WHERE website_id = ?`, mgSiteB); n != 3 {
		t.Errorf("survivor favorites = %d, want 3", n)
	}
	var row struct {
		LikeCount     int    `gorm:"column:like_count"`
		FavoriteCount int    `gorm:"column:favorite_count"`
		CommentCount  int    `gorm:"column:comment_count"`
		View          int    `gorm:"column:view"`
		AgeLimit      string `gorm:"column:age_limit"`
		Domain        string `gorm:"column:domain"`
	}
	if err := db.Raw(`SELECT like_count, favorite_count, comment_count, view, age_limit, domain::text FROM galgame_website WHERE id = ?`,
		mgSiteB).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.LikeCount != 3 || row.FavoriteCount != 3 || row.CommentCount != 10 || row.View != 30 || row.AgeLimit != "r18" {
		t.Errorf("survivor row = %+v", row)
	}
	var gotDomain []string
	if err := json.Unmarshal([]byte(row.Domain), &gotDomain); err != nil {
		t.Fatalf("domain json: %v (%s)", err, row.Domain)
	}
	if fmt.Sprint(gotDomain) != fmt.Sprint([]string{"https://b.example", "https://a.example", "https://extra-a.example"}) {
		t.Errorf("domain = %v", gotDomain)
	}
	if ids := mgTagIDs(t, db, mgSiteB); fmt.Sprint(ids) != fmt.Sprint([]int{mgTagB, mgTagM, mgTagLoose}) {
		t.Errorf("survivor tags = %v, want b/m/loose union", ids)
	}

	var led struct {
		MovedLike, DroppedLike, MovedFavorite, DroppedFavorite, MovedTag, DroppedTag int
		OldRow                                                                       []byte
		DroppedLikeUsers, DroppedTags, AddedURLs                                     []byte
		CreatedBy                                                                    int
	}
	if err := db.Raw(`SELECT moved_like, dropped_like, moved_favorite, dropped_favorite, moved_tag, dropped_tag,
			old_row, dropped_like_users, dropped_tags, added_urls, created_by
			FROM galgame_website_merge WHERE old_website_id = ?`, mgSiteA).Scan(&led).Error; err != nil {
		t.Fatal(err)
	}
	if led.MovedLike != 1 || led.DroppedLike != 1 || led.MovedFavorite != 1 || led.DroppedFavorite != 0 ||
		led.MovedTag != 1 || led.DroppedTag != 2 || led.CreatedBy != 99 {
		t.Errorf("ledger counts = %+v", led)
	}
	if string(led.DroppedLikeUsers) != fmt.Sprintf("[%d]", mgUser1) {
		t.Errorf("dropped like users = %s", led.DroppedLikeUsers)
	}
	if string(led.AddedURLs) != `["https://extra-a.example"]` {
		t.Errorf("added urls ledger = %s", led.AddedURLs)
	}
	if len(led.OldRow) == 0 || string(led.DroppedTags) == "[]" {
		t.Errorf("ledger missing old row / dropped tags: %+v", led)
	}

	// The creation card of the deleted site goes with its row (site trigger).
	if n := mgInt(t, db, `SELECT count(*) FROM feed_activity WHERE type = 'GALGAME_WEBSITE_CREATION' AND source_id = ?`, mgSiteA); n != 0 {
		t.Errorf("source creation feed card survived the fold")
	}
	links := func(table string) []string {
		var out []string
		var col string
		if table == "feed_activity" {
			col = "source_id"
		} else {
			col = "id"
		}
		q := db.Table(table)
		if table == "feed_activity" {
			q = q.Where("type = 'GALGAME_WEBSITE_COMMENT_CREATION' AND source_id IN (9410501, 9410502)")
		} else {
			q = q.Where("type = 'commented'").Where(col + " IS NOT NULL")
		}
		if err := q.Order("link").Pluck("link", &out).Error; err != nil {
			t.Fatal(err)
		}
		sort.Strings(out)
		return out
	}
	got := append(links("feed_activity"), links("message")...)
	want := []string{
		"/website/" + mgHost(mgSiteB),
		"/website/" + mgHost(mgSiteB) + "?x=1",
		"/website/" + mgHost(mgSiteB) + "#c",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("rewritten links = %v, want %v", got, want)
	}

	newID, newHost, ok, err := NewStore(db).WebsiteRedirectByHost(mgHost(mgSiteA))
	if err != nil || !ok || newID != mgSiteB || newHost != mgHost(mgSiteB) {
		t.Errorf("redirect = (%d,%q,%v,%v)", newID, newHost, ok, err)
	}
}

func TestFoldWebsiteRollsBackOnFailure(t *testing.T) {
	db := mergeTestFixture(t)
	mergeSeedSites(t, db, false)
	before := snapshotFold(t, db)

	outer := db.Begin()
	if outer.Error != nil {
		t.Fatal(outer.Error)
	}
	if err := outer.Exec(`CREATE FUNCTION kungal_5220_merge_fail() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'injected fold failure'; END $$;`).Error; err != nil {
		t.Fatal(err)
	}
	if err := outer.Exec(`CREATE TRIGGER trg_kungal_5220_merge_fail BEFORE DELETE ON galgame_website
		FOR EACH ROW EXECUTE FUNCTION kungal_5220_merge_fail();`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(outer).FoldWebsiteTx(outer, mgSiteA, mgSiteB, 1); err == nil {
		t.Fatal("fold succeeded with the failing trigger in place")
	}
	if err := outer.Rollback().Error; err != nil {
		t.Fatal(err)
	}

	after := snapshotFold(t, db)
	if after != before {
		t.Errorf("rollback changed data:\nbefore = %+v\nafter  = %+v", before, after)
	}
}

func TestFoldWebsiteReplayAndConflict(t *testing.T) {
	db := mergeTestFixture(t)
	mergeSeedSites(t, db, true)

	if _, err := NewStore(db).FoldWebsite(mgSiteA, mgSiteB, 1); err != nil {
		t.Fatalf("first fold: %v", err)
	}
	afterFirst := snapshotFold(t, db)

	replay, err := NewStore(db).FoldWebsite(mgSiteA, mgSiteB, 1)
	if err != nil {
		t.Fatalf("replay same direction: %v", err)
	}
	if replay.MovedLike != 1 || replay.LedgerID == 0 {
		t.Errorf("replay result = %+v, want stored ledger summary", replay)
	}
	if got := snapshotFold(t, db); got != afterFirst {
		t.Errorf("replay wrote data:\nfirst = %+v\nagain = %+v", afterFirst, got)
	}
	if n := mgInt(t, db, `SELECT count(*) FROM galgame_website_merge WHERE old_website_id = ?`, mgSiteA); n != 1 {
		t.Errorf("replay created a second ledger row: %d", n)
	}

	if _, err := NewStore(db).FoldWebsite(mgSiteA, mgSiteC, 1); !errors.Is(err, ErrAlreadyMerged) {
		t.Errorf("fold into a different survivor = %v, want ErrAlreadyMerged", err)
	}
	if _, err := NewStore(db).FoldWebsite(mgSiteB, mgSiteB, 1); !errors.Is(err, ErrSameWebsite) {
		t.Errorf("same source/target = %v, want ErrSameWebsite", err)
	}
}

func TestFoldWebsiteChain(t *testing.T) {
	db := mergeTestFixture(t)
	mergeSeedSites(t, db, true)

	if _, err := NewStore(db).FoldWebsite(mgSiteA, mgSiteB, 1); err != nil {
		t.Fatalf("A->B: %v", err)
	}
	if _, err := NewStore(db).FoldWebsite(mgSiteB, mgSiteC, 1); err != nil {
		t.Fatalf("B->C: %v", err)
	}

	for host, want := range map[string]int{mgHost(mgSiteA): mgSiteC, mgHost(mgSiteB): mgSiteC} {
		id, newHost, ok, err := NewStore(db).WebsiteRedirectByHost(host)
		if err != nil || !ok || id != want || newHost != mgHost(mgSiteC) {
			t.Errorf("redirect %s = (%d,%q,%v,%v), want %d", host, id, newHost, ok, err, want)
		}
	}
	ids, err := NewStore(db).AbsorbedWebsiteIDs(mgSiteC)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(ids) != fmt.Sprint([]int{mgSiteA, mgSiteB}) {
		t.Errorf("absorbed ids = %v, want A and B", ids)
	}
	if n := mgInt(t, db, `SELECT count(*) FROM galgame_website_like WHERE website_id = ?`, mgSiteC); n != 3 {
		t.Errorf("C likes = %d, want 3 unique users after the chain", n)
	}
	if n := mgInt(t, db, `SELECT like_count FROM galgame_website WHERE id = ?`, mgSiteC); n != 3 {
		t.Errorf("C like_count = %d, want 3", n)
	}
	if n := mgInt(t, db, `SELECT view FROM galgame_website WHERE id = ?`, mgSiteC); n != 60 {
		t.Errorf("C view = %d, want 60", n)
	}
	if n := mgInt(t, db, `SELECT comment_count FROM galgame_website WHERE id = ?`, mgSiteC); n != 18 {
		t.Errorf("C comment_count = %d, want 18", n)
	}
	domains, err := NewStore(db).MergedWebsiteDomains([]int{mgSiteA, mgSiteB})
	if err != nil {
		t.Fatal(err)
	}
	if domains[mgSiteA] != mgHost(mgSiteC) || domains[mgSiteB] != mgHost(mgSiteC) {
		t.Errorf("merged domains = %v", domains)
	}
}
