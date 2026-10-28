package repository

import (
	"testing"
	"time"

	msgModel "kun-galgame-api/internal/message/model"
	"kun-galgame-api/internal/message/quiet"
	"kun-galgame-api/internal/testdb"
)

func setupDigestRead(t *testing.T) (*MessageRepository, msgModel.NotificationDigest) {
	db := testdb.Open(t)
	repo := NewMessageRepository(db)
	loc, _ := time.LoadLocation(quiet.ScheduleTZ)
	start := time.Date(2026, 10, 27, 23, 0, 0, 0, loc)
	end := start.Add(quiet.WindowHours * time.Hour)

	cleanup := `
DELETE FROM notification_digest_item;
DELETE FROM notification_digest WHERE id = 94209001;
DELETE FROM message WHERE receiver_id = 9420900 OR id IN (94209000, 94209101, 94209102, 94209103, 94209104);
DELETE FROM topic WHERE id IN (5101, 5102);`
	if err := db.Exec(cleanup).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(cleanup) })

	for _, q := range []string{
		`INSERT INTO topic (id, title, content, category, user_id, created, updated) VALUES (5101, 'Night topic one', 'x', 'test', 9001, now(), now())`,
		`INSERT INTO topic (id, title, content, category, user_id, created, updated) VALUES (5102, 'Night topic two', 'x', 'test', 9001, now(), now())`,
	} {
		if err := db.Exec(q).Error; err != nil {
			t.Fatalf("seed topic: %v", err)
		}
	}

	insert := func(id int, typ string, rootKind *string, rootID *int64, itemCount int, cid *int64) {
		if err := db.Exec(
			`INSERT INTO message (id, content, link, status, type, sender_id, receiver_id, root_kind, root_id, item_count, community_notification_id, created, updated)
VALUES (?, '', '/x', 'unread', ?, 0, 9420900, ?, ?, ?, ?, ?, ?)`,
			id, typ, rootKind, rootID, itemCount, cid, start.Add(2*time.Hour), start.Add(2*time.Hour),
		).Error; err != nil {
			t.Fatalf("insert %d: %v", id, err)
		}
	}
	topic := "topic"
	work := "work"
	var t1, t2, w1 int64 = 5101, 5102, 7101
	var cid int64 = 8_800_001
	insert(9_420_9101, "replied", &topic, &t1, 1, nil)
	insert(9_420_9102, "subscribed-topic", &topic, &t2, 3, nil)
	insert(9_420_9103, "liked", &work, &w1, 1, nil)
	insert(9_420_9104, "notice", nil, nil, 1, &cid)

	insertDigest := func(id int, status string) {
		if err := db.Exec(
			`INSERT INTO message (id, content, link, status, type, sender_id, receiver_id, created, updated)
VALUES (?, 'digest line', '/message/digest/94209000', ?, 'digest', 0, 9420900, ?, ?)`,
			id, status, end, end,
		).Error; err != nil {
			t.Fatalf("digest msg: %v", err)
		}
	}
	insertDigest(9_420_9000, "unread")
	if err := db.Exec(
		`INSERT INTO notification_digest (id, receiver_id, message_id, window_start, window_end, status, total_count)
VALUES (94209001, 9420900, 94209000, ?, ?, 'unread', 6)`,
		start, end,
	).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO notification_digest_item (digest_id, message_id) VALUES
(94209001, 94209101), (94209001, 94209102), (94209001, 94209103), (94209001, 94209104)`).Error; err != nil {
		t.Fatal(err)
	}

	var digest msgModel.NotificationDigest
	if err := db.Where("id = 94209001").Take(&digest).Error; err != nil {
		t.Fatal(err)
	}
	return repo, digest
}

func TestDigestDetailQueries(t *testing.T) {
	repo, digest := setupDigestRead(t)

	groups, err := repo.DigestTopicGroups(digest.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups %d, want 2", len(groups))
	}
	if groups[0].TopicID != 5102 || groups[0].Count != 3 || groups[1].TopicID != 5101 || groups[1].Count != 1 {
		t.Fatalf("group order/counts %+v", groups)
	}
	titles, err := repo.FindTopicTitles([]int64{5101, 5102, 999999})
	if err != nil {
		t.Fatal(err)
	}
	if titles[5101] != "Night topic one" || titles[999999] != "" {
		t.Fatalf("titles %v", titles)
	}

	others, err := repo.DigestOtherItems(digest.ID, []string{"liked"})
	if err != nil {
		t.Fatal(err)
	}
	if len(others) != 1 || others[0].Type != "notice" {
		t.Fatalf("muted filtering for others: %+v", others)
	}
	groups, err = repo.DigestTopicGroups(digest.ID, []string{"subscribed-topic"})
	if err != nil || len(groups) != 1 || groups[0].TopicID != 5101 {
		t.Fatalf("muted filtering for groups: %+v %v", groups, err)
	}
}

func TestMarkDigestReadCascade(t *testing.T) {
	repo, digest := setupDigestRead(t)
	db := repo.DB()

	hold := quiet.Hold{Quiet: true, Active: false}
	marked, ids, unread, found, err := repo.MarkDigestRead(9_420_900, digest.MessageID, []string{"liked"}, hold)
	if err != nil || !found {
		t.Fatalf("mark read: %v found=%v", err, found)
	}
	// replied(1) + subscribed(3) + notice(1) marked; liked is muted and left.
	if marked != 3 || unread != 0 || len(ids) != 1 || ids[0] != 8_800_001 {
		t.Fatalf("marked=%d unread=%d ids=%v", marked, unread, ids)
	}
	var status string
	db.Raw(`SELECT status FROM notification_digest WHERE id = 94209001`).Scan(&status)
	if status != "read" {
		t.Fatalf("digest status %s", status)
	}
	db.Raw(`SELECT status FROM message WHERE id = 94209000`).Scan(&status)
	if status != "read" {
		t.Fatalf("digest message status %s", status)
	}
	var likedStatus string
	db.Raw(`SELECT status FROM message WHERE id = 94209103`).Scan(&likedStatus)
	if likedStatus != "unread" {
		t.Fatalf("muted member status %s, want unread", likedStatus)
	}

	// Replay marks nothing.
	marked, _, _, found, err = repo.MarkDigestRead(9_420_900, digest.MessageID, []string{"liked"}, hold)
	if err != nil || !found || marked != 0 {
		t.Fatalf("replay marked=%d found=%v err=%v", marked, found, err)
	}

	// Missing digest.
	_, _, _, found, err = repo.MarkDigestRead(9_420_900, 1, nil, quiet.NoHold())
	if err != nil || found {
		t.Fatalf("missing digest found=%v err=%v", found, err)
	}
}
