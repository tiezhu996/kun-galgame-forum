package repository

import (
	"testing"
	"time"

	"kun-galgame-api/internal/message/quiet"
	"kun-galgame-api/internal/testdb"
)

const heldReceiver = 9_400_301
const heldReceiverOff = 9_400_302
const heldReceiverDigest = 9_400_303

func insertHeldRow(t *testing.T, repo *MessageRepository, id, receiver int, typ, status string, created time.Time) {
	t.Helper()
	if err := repo.DB().Exec(
		`INSERT INTO message (id, content, link, status, type, sender_id, receiver_id, created, updated)
VALUES (?, '', '/topic/1', ?, ?, 1, ?, ?, ?)`,
		id, status, typ, receiver, created, created,
	).Error; err != nil {
		t.Fatalf("insert message %d: %v", id, err)
	}
}

func heldHarness(t *testing.T) (*MessageRepository, *time.Location, time.Time, time.Time) {
	db := testdb.Open(t)
	repo := NewMessageRepository(db)
	loc, err := time.LoadLocation(quiet.ScheduleTZ)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 27, 23, 0, 0, 0, loc)
	end := start.Add(quiet.WindowHours * time.Hour)

	cleanup := `
DELETE FROM notification_digest_item WHERE digest_id IN (SELECT id FROM notification_digest WHERE receiver_id IN (9400301,9400302,9400303));
DELETE FROM notification_digest WHERE receiver_id IN (9400301,9400302,9400303);
DELETE FROM message WHERE receiver_id IN (9400301,9400302,9400303);
DELETE FROM kungal_user_state WHERE user_id IN (9400301,9400302,9400303);`
	if err := db.Exec(cleanup).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(cleanup) })
	if err := db.Exec(
		`INSERT INTO kungal_user_state (user_id, quiet_hours_enabled) VALUES (9400301, true), (9400302, false), (9400303, true)`,
	).Error; err != nil {
		t.Fatal(err)
	}
	return repo, loc, start, end
}

func rowIDs(rows []V1NotificationRow) map[int]bool {
	out := map[int]bool{}
	for _, r := range rows {
		out[r.ID] = true
	}
	return out
}

func TestHeldRowsHiddenInsideActiveWindow(t *testing.T) {
	repo, _, start, end := heldHarness(t)

	night := start.Add(3 * time.Hour)        // 02:00
	nightRead := start.Add(4 * time.Hour)    // 03:00, already read
	day := end.Add(4 * time.Hour)            // 12:00
	before := start.Add(-1 * time.Hour)      // 22:00 previous day
	atStart := start                         // 23:00 exactly, held
	atEnd := end                             // 08:00 exactly, not held
	insertHeldRow(t, repo, 9_401_101, heldReceiver, "replied", "unread", night)
	insertHeldRow(t, repo, 9_401_102, heldReceiver, "replied", "read", nightRead)
	insertHeldRow(t, repo, 9_401_103, heldReceiver, "liked", "unread", day)
	insertHeldRow(t, repo, 9_401_104, heldReceiver, "liked", "unread", before)
	insertHeldRow(t, repo, 9_401_105, heldReceiver, "mentioned", "unread", atStart)
	insertHeldRow(t, repo, 9_401_106, heldReceiver, "mentioned", "unread", atEnd)

	active := quiet.Hold{Quiet: true, Active: true, Start: start, End: end}

	count, err := repo.CountUnreadPartition(heldReceiver, false, nil, active)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 { // day, before, atEnd
		t.Fatalf("unread during window = %d, want 3", count)
	}

	rows, err := repo.FindNotificationsKeyset(heldReceiver, false, nil, "", nil, 50, active)
	if err != nil {
		t.Fatal(err)
	}
	ids := rowIDs(rows)
	if ids[9_401_101] || ids[9_401_105] {
		t.Fatalf("night unread rows must be hidden: %v", ids)
	}
	if !ids[9_401_102] {
		t.Fatalf("a read row inside the window stays visible: %v", ids)
	}
	for _, want := range []int{9_401_103, 9_401_104, 9_401_106} {
		if !ids[want] {
			t.Fatalf("row %d must show during the window: %v", want, ids)
		}
	}

	// Window inactive (next morning): in-window rows surface again unless
	// they belong to an unread digest.
	inactive := quiet.Hold{Quiet: true, Active: false, Start: end, End: end.Add(quiet.WindowHours * time.Hour)}
	count, err = repo.CountUnreadPartition(heldReceiver, false, nil, inactive)
	if err != nil {
		t.Fatal(err)
	}
	if count != 5 { // every unread row (read row excluded)
		t.Fatalf("unread outside window = %d, want 5", count)
	}

	// Quiet off: nothing hidden.
	count, err = repo.CountUnreadPartition(heldReceiver, false, nil, quiet.NoHold())
	if err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Fatalf("unread with quiet off = %d, want 5", count)
	}
}

func TestMutedPartitionIgnoresHold(t *testing.T) {
	repo, _, start, end := heldHarness(t)
	insertHeldRow(t, repo, 9_402_101, heldReceiver, "liked", "unread", start.Add(2*time.Hour))
	muted := []string{"liked"}
	active := quiet.Hold{Quiet: true, Active: true, Start: start, End: end}

	count, err := repo.CountUnreadPartition(heldReceiver, true, muted, active)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("muted partition count = %d, want 1 (hold never applies)", count)
	}
	rows, err := repo.FindNotificationsKeyset(heldReceiver, true, muted, "", nil, 50, active)
	if err != nil {
		t.Fatal(err)
	}
	if !rowIDs(rows)[9_402_101] {
		t.Fatalf("muted row must list in its partition even at night")
	}
}

func TestMembersOfUnreadDigestStayHiddenAfterWindow(t *testing.T) {
	repo, _, start, end := heldHarness(t)
	digestAt := end
	insertHeldRow(t, repo, 9_403_100, heldReceiverDigest, "digest", "unread", digestAt)
	insertHeldRow(t, repo, 9_403_101, heldReceiverDigest, "replied", "unread", start.Add(2*time.Hour))
	insertHeldRow(t, repo, 9_403_102, heldReceiverDigest, "liked", "unread", start.Add(3*time.Hour))
	if err := repo.DB().Exec(
		`INSERT INTO notification_digest (id, receiver_id, message_id, window_start, window_end, total_count)
VALUES (940030301, 9400303, 9403100, ?, ?, 2)
ON CONFLICT DO NOTHING`,
		start, end,
	).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.DB().Exec(
		`INSERT INTO notification_digest_item (digest_id, message_id) VALUES (940030301, 9403101), (940030301, 9403102) ON CONFLICT DO NOTHING`,
	).Error; err != nil {
		t.Fatal(err)
	}

	inactive := quiet.Hold{Quiet: true, Active: false, Start: end, End: end.Add(quiet.WindowHours * time.Hour)}
	count, err := repo.CountUnreadPartition(heldReceiverDigest, false, nil, inactive)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 { // only the digest card itself
		t.Fatalf("count with unread digest = %d, want 1", count)
	}
	rows, err := repo.FindNotificationsKeyset(heldReceiverDigest, false, nil, "", nil, 50, inactive)
	if err != nil {
		t.Fatal(err)
	}
	ids := rowIDs(rows)
	if !ids[9_403_100] || ids[9_403_101] || ids[9_403_102] {
		t.Fatalf("only the digest card shows, got %v", ids)
	}

	// The up-to marker must not swallow either the card or its members, even
	// when a newer ordinary row (higher id) pushes upTo past them.
	insertHeldRow(t, repo, 9_403_199, heldReceiverDigest, "liked", "unread", end.Add(5*time.Hour))
	marked, _, unread, err := repo.MarkReadUpTo(heldReceiverDigest, 9_403_199, false, nil, inactive)
	if err != nil {
		t.Fatal(err)
	}
	if marked != 1 || unread != 1 {
		t.Fatalf("marker marked %d, left %d, want 1 and 1 (digest unread)", marked, unread)
	}
	var status string
	repo.DB().Raw(`SELECT status FROM message WHERE id = 9403101`).Scan(&status)
	if status != "unread" {
		t.Fatalf("digest member status = %s, want unread", status)
	}
}

func TestNotificationSettingsRead(t *testing.T) {
	repo, _, _, _ := heldHarness(t)
	muted, quietOn, err := repo.FindNotificationSettings(heldReceiver)
	if err != nil || !quietOn || len(muted) != 0 {
		t.Fatalf("enabled user settings = %v %v %v", muted, quietOn, err)
	}
	muted, quietOn, err = repo.FindNotificationSettings(heldReceiverOff)
	if err != nil || quietOn || len(muted) != 0 {
		t.Fatalf("disabled user settings = %v %v %v", muted, quietOn, err)
	}
}
