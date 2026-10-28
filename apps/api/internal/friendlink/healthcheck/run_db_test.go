package healthcheck

import (
	"context"
	"testing"
	"time"

	"kun-galgame-api/internal/friendlink/repository"
	"kun-galgame-api/internal/friendlink/siteprobe"
	"kun-galgame-api/internal/testdb"
)

type sweepRow struct {
	id      int
	status  string
	fail    int
	success int
	link    string
	probeOK bool
}

type sweepDBRow struct {
	ID            int        `gorm:"column:id"`
	Status        string     `gorm:"column:status"`
	Fail          int        `gorm:"column:health_fail_count"`
	Success       int        `gorm:"column:health_success_count"`
	Reason        string     `gorm:"column:last_fail_reason"`
	LastCheckedAt *time.Time `gorm:"column:last_checked_at"`
}

func TestRunOneSweep(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()

	if err := db.Exec(`DELETE FROM friend_link`).Error; err != nil {
		t.Fatal(err)
	}

	rows := []sweepRow{
		{932100951, repository.StatusNormal, 2, 0, "https://fail-mark.example", false},
		{932100952, repository.StatusAbnormal, 3, 1, "https://ok-recover.example", true},
		{932100953, repository.StatusDown, 0, 0, "https://fail-down.example", false},
		{932100954, repository.StatusNormal, 0, 0, "https://fail-fresh.example", false},
		{932100955, repository.StatusNormal, 0, 0, "https://ok-normal.example", true},
	}
	for _, r := range rows {
		if err := db.Exec(`INSERT INTO friend_link
			(id, category, name, link, description, banner, banner_image_hash, status, sort_order,
			 health_fail_count, health_success_count, last_checked_at, last_fail_reason, created, updated)
			VALUES (?, 'others', ?, ?, '', '', '', ?, 0, ?, ?, ?, 'old reason', now(), now())`,
			r.id, r.link, r.link, r.status, r.fail, r.success, time.Now().Add(-time.Hour)).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM friend_link WHERE id BETWEEN 932100951 AND 932100955`).Error
	})

	prober := siteprobe.Func(func(_ context.Context, rawURL string) (bool, string) {
		for _, r := range rows {
			if r.link == rawURL {
				if r.probeOK {
					return true, ""
				}
				return false, "HTTP 500"
			}
		}
		return false, "未知链接"
	})

	stats, err := Run(ctx, db, prober, 3)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Checked != 5 || stats.Marked != 1 || stats.Recovered != 1 || stats.Failed != 3 || stats.Errored != 0 {
		t.Fatalf("stats = %+v", stats)
	}

	var fetched []sweepDBRow
	if err := db.Table("friend_link").
		Select("id, status, health_fail_count, health_success_count, last_fail_reason, last_checked_at").
		Where("id BETWEEN 932100951 AND 932100955").Scan(&fetched).Error; err != nil {
		t.Fatal(err)
	}
	got := make(map[int]sweepDBRow, len(fetched))
	for _, f := range fetched {
		got[f.ID] = f
	}

	want := map[int]sweepDBRow{
		932100951: {ID: 932100951, Status: repository.StatusAbnormal, Fail: 3, Success: 0, Reason: "HTTP 500"},
		932100952: {ID: 932100952, Status: repository.StatusNormal, Fail: 0, Success: 0, Reason: ""},
		932100953: {ID: 932100953, Status: repository.StatusDown, Fail: 0, Success: 0, Reason: "HTTP 500"},
		932100954: {ID: 932100954, Status: repository.StatusNormal, Fail: 1, Success: 0, Reason: "HTTP 500"},
		932100955: {ID: 932100955, Status: repository.StatusNormal, Fail: 0, Success: 0, Reason: ""},
	}
	for id, w := range want {
		g, ok := got[id]
		if !ok {
			t.Fatalf("row %d missing", id)
		}
		if g.Status != w.Status || g.Fail != w.Fail || g.Success != w.Success || g.Reason != w.Reason {
			t.Fatalf("row %d = %+v, want %+v", id, g, w)
		}
		if g.LastCheckedAt == nil || time.Since(*g.LastCheckedAt) > time.Minute {
			t.Fatalf("row %d last_checked_at not refreshed: %v", id, g.LastCheckedAt)
		}
	}
}
