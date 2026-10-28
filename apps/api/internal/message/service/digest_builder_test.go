package service

import (
	"context"
	"strings"
	"testing"
	"time"

	msgRepo "kun-galgame-api/internal/message/repository"
	"kun-galgame-api/internal/message/quiet"
	"kun-galgame-api/internal/testdb"
)

const (
	dbUserOn    = 9_410_001
	dbUserOff   = 9_410_002
	dbUserEmpty = 9_410_003
)

func setupDigestBuilderDB(t *testing.T) (*msgRepo.MessageRepository, time.Time, time.Time) {
	db := testdb.Open(t)
	loc, err := time.LoadLocation(quiet.ScheduleTZ)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 27, 23, 0, 0, 0, loc)
	end := start.Add(quiet.WindowHours * time.Hour)

	cleanup := `
DELETE FROM notification_digest_item WHERE digest_id IN (SELECT id FROM notification_digest WHERE receiver_id BETWEEN 9410001 AND 9410003);
DELETE FROM notification_digest WHERE receiver_id BETWEEN 9410001 AND 9410003;
DELETE FROM message WHERE receiver_id BETWEEN 9410001 AND 9410003;
DELETE FROM kungal_user_state WHERE user_id BETWEEN 9410001 AND 9410003;`
	if err := db.Exec(cleanup).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(cleanup) })
	if err := db.Exec(
		`INSERT INTO kungal_user_state (user_id, quiet_hours_enabled) VALUES (9410001, true), (9410002, false), (9410003, true)`,
	).Error; err != nil {
		t.Fatal(err)
	}

	insert := func(id, receiver int, typ, status string, rootKind *string, rootID *int64, itemCount int, created time.Time) {
		if err := db.Exec(
			`INSERT INTO message (id, content, link, status, type, sender_id, receiver_id, root_kind, root_id, item_count, created, updated)
VALUES (?, '', '/x', ?, ?, 1, ?, ?, ?, ?, ?, ?)`,
			id, status, typ, receiver, rootKind, rootID, itemCount, created, created,
		).Error; err != nil {
			t.Fatalf("insert %d: %v", id, err)
		}
	}
	topic := "topic"
	work := "work"
	var t1, t2 int64 = 5_001, 5_002
	var w1 int64 = 7_001
	// User on: two topics (a fold of 3 replies on t2), a work like, a muted
	// type, a read row, and a row outside the window.
	insert(9_411_101, dbUserOn, "replied", "unread", &topic, &t1, 1, start.Add(time.Hour))
	insert(9_411_102, dbUserOn, "subscribed-topic", "unread", &topic, &t2, 3, start.Add(2*time.Hour))
	insert(9_411_103, dbUserOn, "liked", "unread", &work, &w1, 1, start.Add(3*time.Hour))
	insert(9_411_104, dbUserOn, "notice", "unread", nil, nil, 1, start.Add(4*time.Hour))
	insert(9_411_105, dbUserOn, "replied", "read", &topic, &t1, 1, start.Add(5*time.Hour))
	insert(9_411_106, dbUserOn, "replied", "unread", &topic, &t1, 1, end.Add(2*time.Hour))
	if err := db.Exec(`UPDATE kungal_user_state SET muted_notification_types = '["liked"]' WHERE user_id = ?`, dbUserOn).Error; err != nil {
		t.Fatal(err)
	}
	// Off user: same shape, must never be digested.
	insert(9_412_101, dbUserOff, "replied", "unread", &topic, &t1, 1, start.Add(time.Hour))

	return msgRepo.NewMessageRepository(db), start, end
}

func TestDigestBuilderBuildsAndIsIdempotent(t *testing.T) {
	repo, start, _ := setupDigestBuilderDB(t)
	builder := NewDigestBuilder(repo)
	db := repo.DB()

	stats, err := builder.BuildWindow(context.Background(), start)
	if err != nil {
		t.Fatal(err)
	}
	if stats.NewDigests != 1 {
		t.Fatalf("stats %+v, want exactly one new digest", stats)
	}

	var digestCount, messageCount int
	db.Raw(`SELECT count(*) FROM notification_digest WHERE receiver_id = ?`, dbUserOn).Scan(&digestCount)
	db.Raw(`SELECT count(*) FROM message WHERE receiver_id = ? AND type = 'digest'`, dbUserOn).Scan(&messageCount)
	if digestCount != 1 || messageCount != 1 {
		t.Fatalf("digest rows %d, digest messages %d", digestCount, messageCount)
	}
	db.Raw(`SELECT count(*) FROM notification_digest WHERE receiver_id IN (?, ?)`, dbUserOff, dbUserEmpty).Scan(&digestCount)
	if digestCount != 0 {
		t.Fatalf("off/empty users got %d digests", digestCount)
	}

	var itemCount int
	db.Raw(`SELECT count(*) FROM notification_digest_item di
JOIN notification_digest d ON d.id = di.digest_id WHERE d.receiver_id = ?`, dbUserOn).Scan(&itemCount)
	if itemCount != 3 { // t1 replied, t2 fold, notice; liked/read/out-of-window excluded
		t.Fatalf("members %d, want 3", itemCount)
	}

	var total, groups, topicItems, others int
	db.Raw(`SELECT d.total_count FROM notification_digest d WHERE d.receiver_id = ?`, dbUserOn).Scan(&total)
	if total != 5 { // 1 + 3 + 1
		t.Fatalf("total_count %d, want 5", total)
	}
	db.Raw(`
SELECT COUNT(DISTINCT m.root_id) FILTER (WHERE m.root_kind='topic'),
       COALESCE(SUM(GREATEST(m.item_count,1)) FILTER (WHERE m.root_kind='topic'),0),
       COALESCE(SUM(GREATEST(m.item_count,1)) FILTER (WHERE m.root_kind IS DISTINCT FROM 'topic'),0)
FROM notification_digest_item di JOIN notification_digest d ON d.id=di.digest_id JOIN message m ON m.id=di.message_id
WHERE d.receiver_id = ?`, dbUserOn).Row().Scan(&groups, &topicItems, &others)
	if groups != 2 || topicItems != 4 || others != 1 {
		t.Fatalf("groups %d topicItems %d others %d", groups, topicItems, others)
	}

	var content string
	db.Raw(`SELECT m.content FROM message m JOIN notification_digest d ON d.message_id = m.id WHERE d.receiver_id = ?`, dbUserOn).Scan(&content)
	if content == "" || !strings.Contains(content, "2") {
		t.Fatalf("digest content %q", content)
	}

	// Rerun: no new digest or members.
	stats2, err := builder.BuildWindow(context.Background(), start)
	if err != nil || stats2.NewDigests != 0 {
		t.Fatalf("rerun stats %+v err %v", stats2, err)
	}
	db.Raw(`SELECT count(*) FROM notification_digest_item di
JOIN notification_digest d ON d.id = di.digest_id WHERE d.receiver_id = ?`, dbUserOn).Scan(&itemCount)
	if itemCount != 3 {
		t.Fatalf("members after rerun %d, want 3", itemCount)
	}

	// Late-arriving row inside the window gets attached and counts corrected.
	if err := db.Exec(
		`INSERT INTO message (id, content, link, status, type, sender_id, receiver_id, root_kind, root_id, item_count, created, updated)
VALUES (9411199, '', '/x', 'unread', 'replied', 1, ?, 'topic', 5001, 1, ?, ?)`,
		dbUserOn, start.Add(7*time.Hour), start.Add(7*time.Hour),
	).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := builder.BuildWindow(context.Background(), start); err != nil {
		t.Fatal(err)
	}
	db.Raw(`SELECT count(*) FROM notification_digest_item di
JOIN notification_digest d ON d.id = di.digest_id WHERE d.receiver_id = ?`, dbUserOn).Scan(&itemCount)
	if itemCount != 4 {
		t.Fatalf("members after late attach %d, want 4", itemCount)
	}
	db.Raw(`SELECT d.total_count FROM notification_digest d WHERE d.receiver_id = ?`, dbUserOn).Scan(&total)
	if total != 6 {
		t.Fatalf("total after late attach %d, want 6", total)
	}
}
