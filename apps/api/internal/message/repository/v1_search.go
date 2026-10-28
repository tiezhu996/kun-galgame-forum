package repository

import (
	"strings"
	"time"
)

const (
	SearchHitNotification  = 0
	SearchHitDirectMessage = 1
)

type V1MessageSearchFilter struct {
	CallerID int
	Phrase   string
	SenderID int
	Since    *time.Time
}

type V1MessageSearchPos struct {
	Created time.Time
	HitOrd  int
	HitID   int
}

type V1MessageSearchRow struct {
	HitOrd     int        `gorm:"column:hit_ord"`
	HitID      int        `gorm:"column:hit_id"`
	Created    time.Time  `gorm:"column:created"`
	SenderID   int        `gorm:"column:sender_id"`
	Snippet    string     `gorm:"column:snippet"`
	NotifType  string     `gorm:"column:notif_type"`
	Link       string     `gorm:"column:link"`
	RootKind   string     `gorm:"column:root_kind"`
	RootID     *int64     `gorm:"column:root_id"`
	PeerID     *int       `gorm:"column:peer_id"`
	IsRecall   bool       `gorm:"column:is_recall"`
	RecallTime *time.Time `gorm:"column:recall_time"`
}

// EscapeILike neutralises the three ILIKE metacharacters so a user phrase is a
// plain substring even when it contains %, _ or backslash.
func EscapeILike(phrase string) string {
	phrase = strings.ReplaceAll(phrase, `\`, `\\`)
	phrase = strings.ReplaceAll(phrase, `%`, `\%`)
	phrase = strings.ReplaceAll(phrase, `_`, `\_`)
	return phrase
}

// SearchInboxKeyset merges the caller's notifications and direct messages that
// contain the phrase into one time-ordered stream. Both branches are scoped to
// the caller in SQL: notifications by receiver_id, direct messages by private
// room participation. Recalled messages still match on their stored text; the
// caller maps them to tombstones and never emits that text. Sender and since
// are pushed into each branch so the receiver/participant indexes stay usable.
func (r *MessageRepository) SearchInboxKeyset(
	f V1MessageSearchFilter,
	pos *V1MessageSearchPos,
	limit int,
) ([]V1MessageSearchRow, error) {
	like := "%" + EscapeILike(f.Phrase) + "%"

	var q strings.Builder
	args := make([]any, 0, 16)

	q.WriteString(`SELECT s.* FROM (
  SELECT
    0 AS hit_ord, m.id AS hit_id, m.created, m.sender_id,
    m.content AS snippet, m.type AS notif_type, m.link,
    m.root_kind, m.root_id,
    NULL::int AS peer_id, FALSE AS is_recall, NULL::timestamptz AS recall_time
  FROM message m
  WHERE m.receiver_id = ? AND m.content ILIKE ? ESCAPE '\'`)
	args = append(args, f.CallerID, like)
	if f.SenderID > 0 {
		q.WriteString(" AND m.sender_id = ?")
		args = append(args, f.SenderID)
	}
	if f.Since != nil {
		q.WriteString(" AND m.created >= ?")
		args = append(args, *f.Since)
	}

	q.WriteString(`
  UNION ALL
  SELECT
    1 AS hit_ord, cm.id AS hit_id, cm.created, cm.sender_id,
    cm.content AS snippet, NULL::text AS notif_type, NULL::varchar(100) AS link,
    NULL::text AS root_kind, NULL::bigint AS root_id,
    peer.uid AS peer_id, cm.is_recall, cm.recall_time
  FROM chat_message cm
  JOIN chat_room cr ON cr.id = cm.chat_room_id AND cr.type = 'private'
  JOIN chat_room_participant me ON me.chat_room_id = cr.id AND me.user_id = ?
  JOIN LATERAL (
    SELECT p.user_id AS uid
    FROM chat_room_participant p
    WHERE p.chat_room_id = cr.id AND p.user_id <> ?
    ORDER BY p.user_id LIMIT 1
  ) peer ON TRUE
  WHERE cm.content ILIKE ? ESCAPE '\'`)
	args = append(args, f.CallerID, f.CallerID, like)
	if f.SenderID > 0 {
		q.WriteString(" AND cm.sender_id = ?")
		args = append(args, f.SenderID)
	}
	if f.Since != nil {
		q.WriteString(" AND cm.created >= ?")
		args = append(args, *f.Since)
	}

	q.WriteString(`
) s`)
	if pos != nil {
		q.WriteString(" WHERE (s.created, s.hit_ord, s.hit_id) < (?, ?, ?)")
		args = append(args, pos.Created, pos.HitOrd, pos.HitID)
	}
	q.WriteString(" ORDER BY s.created DESC, s.hit_ord DESC, s.hit_id DESC LIMIT ?")
	args = append(args, limit)

	var rows []V1MessageSearchRow
	err := r.db.Raw(q.String(), args...).Scan(&rows).Error
	if rows == nil {
		rows = []V1MessageSearchRow{}
	}
	return rows, err
}

type topicTitleRow struct {
	ID    int64  `gorm:"column:id"`
	Title string `gorm:"column:title"`
}

func (r *MessageRepository) TopicTitles(ids []int64) (map[int64]string, error) {
	titles := make(map[int64]string)
	if len(ids) == 0 {
		return titles, nil
	}
	var rows []topicTitleRow
	if err := r.db.Table("topic").
		Select("id, title").
		Where("id IN ?", ids).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		titles[row.ID] = row.Title
	}
	return titles, nil
}
