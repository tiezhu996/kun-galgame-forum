package repository

import (
	"context"
	"testing"
	"time"

	"kun-galgame-api/internal/testdb"
)

const healthSeedID = 932100901

func seedHealthRow(t *testing.T, r *FriendLinkRepository, status string, fail, success int, reason, description string) {
	t.Helper()
	db := r.db
	if err := db.Exec(`DELETE FROM friend_link WHERE id = ?`, healthSeedID).Error; err != nil {
		t.Fatal(err)
	}
	checked := time.Now().Add(-time.Hour)
	if err := db.Exec(`INSERT INTO friend_link
		(id, category, name, link, description, banner, banner_image_hash, status, sort_order,
		 health_fail_count, health_success_count, last_checked_at, last_fail_reason, created, updated)
		VALUES (?, 'others', 'probe', 'https://probe.example', ?, '', '', ?, 0, ?, ?, ?, ?, now(), now())`,
		healthSeedID, description, status, fail, success, checked, reason).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(`DELETE FROM friend_link WHERE id = ?`, healthSeedID).Error })
}

func healthRow(t *testing.T, r *FriendLinkRepository) healthState {
	t.Helper()
	var s healthState
	if err := r.db.Table("friend_link").
		Select("status, health_fail_count, health_success_count, last_fail_reason").
		Where("id = ?", healthSeedID).Take(&s).Error; err != nil {
		t.Fatal(err)
	}
	return s
}

func checkedFreshness(t *testing.T, r *FriendLinkRepository) time.Time {
	t.Helper()
	var at *time.Time
	if err := r.db.Raw(`SELECT last_checked_at FROM friend_link WHERE id = ?`, healthSeedID).
		Scan(&at).Error; err != nil {
		t.Fatal(err)
	}
	if at == nil {
		t.Fatal("last_checked_at not written")
	}
	if time.Since(*at) > time.Minute {
		t.Fatalf("last_checked_at stale: %v", at)
	}
	return *at
}

func TestApplyCheckResultThreeStrikesHide(t *testing.T) {
	db := testdb.Open(t)
	r := NewFriendLinkRepository(db)
	ctx := context.Background()
	seedHealthRow(t, r, StatusNormal, 0, 0, "", "站点说明")

	if tr, err := r.ApplyCheckResult(ctx, healthSeedID, CheckResult{OK: false, Reason: "HTTP 502"}); err != nil {
		t.Fatal(err)
	} else if tr != TransitionNone {
		t.Fatalf("strike 1 transition = %q", tr)
	}
	if s := healthRow(t, r); s.Status != StatusNormal || s.FailCount != 1 || s.FailReason != "HTTP 502" {
		t.Fatalf("strike 1: %+v", s)
	}
	if tr, err := r.ApplyCheckResult(ctx, healthSeedID, CheckResult{OK: false, Reason: ""}); err != nil {
		t.Fatal(err)
	} else if tr != TransitionNone {
		t.Fatalf("strike 2 transition = %q", tr)
	}
	if s := healthRow(t, r); s.Status != StatusNormal || s.FailCount != 2 || s.FailReason != "HTTP 502" {
		t.Fatalf("strike 2 must keep the previous reason: %+v", s)
	}
	if tr, err := r.ApplyCheckResult(ctx, healthSeedID, CheckResult{OK: false, Reason: "请求超时（10s）"}); err != nil {
		t.Fatal(err)
	} else if tr != TransitionMarked {
		t.Fatalf("strike 3 transition = %q, want marked", tr)
	}
	if s := healthRow(t, r); s.Status != StatusAbnormal || s.FailCount != 3 || s.SuccessCount != 0 || s.FailReason != "请求超时（10s）" {
		t.Fatalf("strike 3 must hide: %+v", s)
	}
	checkedFreshness(t, r)

	var description string
	if err := db.Raw(`SELECT description FROM friend_link WHERE id = ?`, healthSeedID).Scan(&description).Error; err != nil {
		t.Fatal(err)
	}
	if description != "站点说明" {
		t.Fatalf("checker touched description: %q", description)
	}
}

func TestApplyCheckResultTwoSuccessesRestore(t *testing.T) {
	db := testdb.Open(t)
	r := NewFriendLinkRepository(db)
	ctx := context.Background()
	seedHealthRow(t, r, StatusAbnormal, 3, 0, "HTTP 500", "")

	if tr, err := r.ApplyCheckResult(ctx, healthSeedID, CheckResult{OK: true}); err != nil {
		t.Fatal(err)
	} else if tr != TransitionNone {
		t.Fatalf("success 1 transition = %q", tr)
	}
	if s := healthRow(t, r); s.Status != StatusAbnormal || s.SuccessCount != 1 || s.FailReason != "HTTP 500" {
		t.Fatalf("success 1 must keep hidden state and reason: %+v", s)
	}
	if tr, err := r.ApplyCheckResult(ctx, healthSeedID, CheckResult{OK: true}); err != nil {
		t.Fatal(err)
	} else if tr != TransitionRecovered {
		t.Fatalf("success 2 transition = %q, want recovered", tr)
	}
	if s := healthRow(t, r); s.Status != StatusNormal || s.FailCount != 0 || s.SuccessCount != 0 || s.FailReason != "" {
		t.Fatalf("success 2 must restore fully: %+v", s)
	}
}

func TestApplyCheckResultNormalSuccessClearsStrike(t *testing.T) {
	db := testdb.Open(t)
	r := NewFriendLinkRepository(db)
	ctx := context.Background()
	seedHealthRow(t, r, StatusNormal, 2, 0, "HTTP 502", "")

	if _, err := r.ApplyCheckResult(ctx, healthSeedID, CheckResult{OK: true}); err != nil {
		t.Fatal(err)
	}
	if s := healthRow(t, r); s.Status != StatusNormal || s.FailCount != 0 || s.FailReason != "" {
		t.Fatalf("one success before the threshold clears the strikes: %+v", s)
	}
}

func TestApplyCheckResultAbnormalFailureResetsSuccessStreak(t *testing.T) {
	db := testdb.Open(t)
	r := NewFriendLinkRepository(db)
	ctx := context.Background()
	seedHealthRow(t, r, StatusAbnormal, 3, 1, "", "")

	if _, err := r.ApplyCheckResult(ctx, healthSeedID, CheckResult{OK: false, Reason: "连接被拒绝"}); err != nil {
		t.Fatal(err)
	}
	if s := healthRow(t, r); s.Status != StatusAbnormal || s.SuccessCount != 0 || s.FailCount != 3 || s.FailReason != "连接被拒绝" {
		t.Fatalf("a failure breaks the recovery streak: %+v", s)
	}
}

func TestApplyCheckResultDownIsImmutable(t *testing.T) {
	db := testdb.Open(t)
	r := NewFriendLinkRepository(db)
	ctx := context.Background()
	seedHealthRow(t, r, StatusDown, 0, 0, "", "")

	before := checkedFreshnessOrNil(t, r)
	for _, res := range []CheckResult{
		{OK: false, Reason: "HTTP 503"},
		{OK: false, Reason: ""},
		{OK: true},
		{OK: false, Reason: "DNS 解析失败"},
	} {
		if _, err := r.ApplyCheckResult(ctx, healthSeedID, res); err != nil {
			t.Fatal(err)
		}
	}
	if s := healthRow(t, r); s.Status != StatusDown || s.FailCount != 0 || s.SuccessCount != 0 {
		t.Fatalf("down row counters/status must never move: %+v", s)
	}
	if s := healthRow(t, r); s.FailReason != "DNS 解析失败" {
		t.Fatalf("down row still records the latest non-empty reason, got %q", s.FailReason)
	}
	after := checkedFreshnessOrNil(t, r)
	if !after.After(before) {
		t.Fatalf("down row check time must refresh: before %v after %v", before, after)
	}

	if _, err := r.ApplyCheckResult(ctx, healthSeedID, CheckResult{OK: true}); err != nil {
		t.Fatal(err)
	}
	if s := healthRow(t, r); s.Status != StatusDown || s.FailReason != "" {
		t.Fatalf("down row success clears reason only, status stays: %+v", s)
	}
}

func checkedFreshnessOrNil(t *testing.T, r *FriendLinkRepository) time.Time {
	t.Helper()
	var at time.Time
	if err := r.db.Raw(`SELECT last_checked_at FROM friend_link WHERE id = ?`, healthSeedID).
		Scan(&at).Error; err != nil {
		t.Fatal(err)
	}
	return at
}

func TestApplyCheckResultMissingRowIsNoop(t *testing.T) {
	db := testdb.Open(t)
	r := NewFriendLinkRepository(db)
	if _, err := r.ApplyCheckResult(context.Background(), 932100999, CheckResult{OK: false, Reason: "x"}); err != nil {
		t.Fatalf("a deleted row must not error the run: %v", err)
	}
}

func TestUpdateLinkResetsHealthOnURLEdit(t *testing.T) {
	db := testdb.Open(t)
	r := NewFriendLinkRepository(db)
	seedHealthRow(t, r, StatusNormal, 2, 0, "HTTP 502", "保留说明")

	if err := r.UpdateLink(healthSeedID, LinkChanges{
		Link: strPtr("https://new-target.example"),
	}); err != nil {
		t.Fatal(err)
	}
	var s healthState
	if err := db.Table("friend_link").
		Select("status, health_fail_count, health_success_count, last_fail_reason").
		Where("id = ?", healthSeedID).Take(&s).Error; err != nil {
		t.Fatal(err)
	}
	var extra struct {
		LastCheckedAt *time.Time `gorm:"column:last_checked_at"`
		Description   string     `gorm:"column:description"`
	}
	if err := db.Table("friend_link").
		Select("last_checked_at, description").
		Where("id = ?", healthSeedID).Take(&extra).Error; err != nil {
		t.Fatal(err)
	}
	at, description := extra.LastCheckedAt, extra.Description
	if s.FailCount != 0 || s.SuccessCount != 0 || s.FailReason != "" || at != nil {
		t.Fatalf("URL edit must reset health metadata: %+v %v", s, at)
	}
	if description != "保留说明" {
		t.Fatalf("description must survive: %q", description)
	}

	seedHealthRow(t, r, StatusNormal, 2, 0, "HTTP 502", "保留说明")
	desc := "改了说明"
	if err := r.UpdateLink(healthSeedID, LinkChanges{Description: &desc}); err != nil {
		t.Fatal(err)
	}
	if s = healthRow(t, r); s.FailCount != 2 || s.FailReason != "HTTP 502" {
		t.Fatalf("description-only edit must keep health data: %+v", s)
	}
}

func TestUpdateLinkManualStatusResetsCounters(t *testing.T) {
	db := testdb.Open(t)
	r := NewFriendLinkRepository(db)
	seedHealthRow(t, r, StatusAbnormal, 3, 0, "HTTP 502", "")

	if err := r.UpdateLink(healthSeedID, LinkChanges{Status: strPtr(StatusNormal)}); err != nil {
		t.Fatal(err)
	}
	if s := healthRow(t, r); s.Status != StatusNormal || s.FailCount != 0 || s.SuccessCount != 0 || s.FailReason != "" {
		t.Fatalf("manual normal is a rescue: %+v", s)
	}
}

func strPtr(s string) *string { return &s }
