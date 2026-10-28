package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	StatusNormal   = "normal"
	StatusDown     = "down"
	StatusAbnormal = "abnormal"

	HealthFailThreshold    = 3
	HealthRecoverThreshold = 2
	maxFailReasonLen       = 200
)

type HealthCandidate struct {
	ID   int    `gorm:"column:id"`
	Link string `gorm:"column:link"`
}

type CheckResult struct {
	OK     bool
	Reason string
}

// Transition reports a status flip caused by one probe outcome, so the runner
// can log mark/recover totals without rescanning rows.
type Transition string

const (
	TransitionNone      Transition = ""
	TransitionMarked    Transition = "marked"
	TransitionRecovered Transition = "recovered"
)

// HealthCandidates lists every link the cron probes, including down rows:
// staff wants to see whether a manually offline site has recovered.
func (r *FriendLinkRepository) HealthCandidates(ctx context.Context) ([]HealthCandidate, error) {
	var rows []HealthCandidate
	err := r.db.WithContext(ctx).Table("friend_link").
		Select("id, link").Order("id").Scan(&rows).Error
	return rows, err
}

type healthState struct {
	Status       string `gorm:"column:status"`
	FailCount    int    `gorm:"column:health_fail_count"`
	SuccessCount int    `gorm:"column:health_success_count"`
	FailReason   string `gorm:"column:last_fail_reason"`
}

// ApplyCheckResult advances the health state machine for one probe outcome.
// down is staff-owned state: the metadata refreshes but status and counters
// never move. A failure carrying an empty reason keeps the previous one.
func (r *FriendLinkRepository) ApplyCheckResult(ctx context.Context, id int, res CheckResult) (Transition, error) {
	var transition Transition
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cur healthState
		err := tx.Table("friend_link").
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("status, health_fail_count, health_success_count, last_fail_reason").
			Where("id = ?", id).Take(&cur).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}

		updates := map[string]any{"last_checked_at": time.Now()}
		reason := truncateReason(res.Reason)

		switch cur.Status {
		case StatusDown:
			if res.OK {
				updates["last_fail_reason"] = ""
			} else if reason != "" {
				updates["last_fail_reason"] = reason
			}
		case StatusNormal:
			if res.OK {
				updates["health_fail_count"] = 0
				updates["health_success_count"] = 0
				updates["last_fail_reason"] = ""
			} else {
				fail := cur.FailCount + 1
				updates["health_fail_count"] = fail
				updates["health_success_count"] = 0
				if reason != "" {
					updates["last_fail_reason"] = reason
				}
				if fail >= HealthFailThreshold {
					updates["status"] = StatusAbnormal
					transition = TransitionMarked
				}
			}
		case StatusAbnormal:
			if res.OK {
				success := cur.SuccessCount + 1
				if success >= HealthRecoverThreshold {
					updates["status"] = StatusNormal
					updates["health_fail_count"] = 0
					updates["health_success_count"] = 0
					updates["last_fail_reason"] = ""
					transition = TransitionRecovered
				} else {
					updates["health_success_count"] = success
				}
			} else {
				updates["health_success_count"] = 0
				if reason != "" {
					updates["last_fail_reason"] = reason
				}
			}
		default:
			return fmt.Errorf("friend_link %d has status %q outside the closed enum", id, cur.Status)
		}

		return tx.Table("friend_link").Where("id = ?", id).UpdateColumns(updates).Error
	})
	return transition, err
}

// truncateReason cuts on a rune boundary: byte-slicing a multibyte error
// message (Chinese net errors on some hosts) made the UPDATE fail.
func truncateReason(s string) string {
	if len(s) <= maxFailReasonLen {
		return s
	}
	r := []rune(s)
	if len(r) <= maxFailReasonLen {
		return s
	}
	return string(r[:maxFailReasonLen])
}
