package quiet

import "time"

// ScheduleTZ mirrors cron.ScheduleTZ: the night window is one fixed local
// schedule, not a per-user offset.
const ScheduleTZ = "Asia/Shanghai"

const (
	StartHour   = 23
	WindowHours = 9
)

func location() *time.Location {
	loc, err := time.LoadLocation(ScheduleTZ)
	if err != nil {
		return time.Local
	}
	return loc
}

// Hold is the caller's quiet-hours state at query time. When Quiet is false
// every row surfaces normally. Active additionally hides unattached rows
// created inside the ongoing window; members of an unread digest stay hidden
// regardless of whether a window is active right now.
type Hold struct {
	Quiet  bool
	Active bool
	Start  time.Time
	End    time.Time
}

func NoHold() Hold { return Hold{} }

// At builds the hold state for one instant.
func At(now time.Time, quiet bool) Hold {
	if !quiet {
		return Hold{Quiet: false}
	}
	start, end, active := WindowAt(now)
	return Hold{Quiet: true, Active: active, Start: start, End: end}
}

// WindowAt returns the half-open window [start, end) relevant at t.
// Inside 23:00-08:00 local it is the ongoing window (start that night at
// 23:00, end the same calendar morning at 08:00) and active is true.
// Outside the window it is the upcoming one and active is false; the read
// side then only keeps rows already attached to an unread digest hidden.
func WindowAt(t time.Time) (start, end time.Time, active bool) {
	local := t.In(location())
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	tonight := day.Add(StartHour * time.Hour)
	if local.Hour() >= StartHour {
		return tonight, tonight.Add(WindowHours * time.Hour), true
	}
	lastNight := tonight.Add(-24 * time.Hour)
	if local.Before(lastNight.Add(WindowHours * time.Hour)) {
		return lastNight, lastNight.Add(WindowHours * time.Hour), true
	}
	return tonight, tonight.Add(WindowHours * time.Hour), false
}

// LastEndedWindow returns the window whose 08:00 end boundary most recently
// passed at or before t. The 08:00 build job uses it, so running a couple of
// minutes late still digests the canonical night instead of a shifted range.
func LastEndedWindow(t time.Time) (start, end time.Time) {
	local := t.In(location())
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	end = day.Add(8 * time.Hour)
	if local.Before(end) {
		end = end.Add(-24 * time.Hour)
	}
	return end.Add(-WindowHours * time.Hour), end
}

// HeldNotInSQL is ANDed to unmuted-partition queries over the message table.
// idColumn qualifies message.id for the correlated membership lookups.
// The branches must stay in sync with membership written by the digest
// builder and the read marker that skips members of unread digests.
func HeldNotInSQL(hold Hold, idColumn string) (string, []any) {
	if !hold.Quiet {
		return "", nil
	}
	// No leading AND: gorm wraps each Where in parentheses and joins them
	// with AND itself. Raw callers concatenating this manually prepend AND.
	sql := `NOT (
		status = 'unread'
		AND type <> 'digest'
		AND (
			EXISTS (
				SELECT 1 FROM notification_digest_item hdi
				JOIN notification_digest hd ON hd.id = hdi.digest_id
				WHERE hdi.message_id = ` + idColumn + ` AND hd.status = 'unread'
			)`
	args := []any{}
	if hold.Active {
		sql += `
			OR (
				created >= ? AND created < ?
				AND NOT EXISTS (SELECT 1 FROM notification_digest_item hdi2 WHERE hdi2.message_id = ` + idColumn + `)
			)`
		args = append(args, hold.Start, hold.End)
	}
	sql += `
		)
	)`
	return sql, args
}
