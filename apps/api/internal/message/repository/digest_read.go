package repository

import (
	"database/sql"
	"errors"
	"time"

	msgModel "kun-galgame-api/internal/message/model"
	"kun-galgame-api/internal/message/quiet"

	"gorm.io/gorm"
)

// digestOtherItemLimit caps how many non-topic members the detail face
// returns. The read endpoint marks every member regardless, so the cap only
// bounds rendering.
const digestOtherItemLimit = 200

// FindDigestByMessageID resolves a digest through its synthetic notification
// row, scoped to the receiver.
func (r *MessageRepository) FindDigestByMessageID(receiverID, messageID int) (*msgModel.NotificationDigest, error) {
	var d msgModel.NotificationDigest
	err := r.db.Where("receiver_id = ? AND message_id = ?", receiverID, messageID).Take(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

type DigestTopicGroupRow struct {
	TopicID int64 `gorm:"column:topic_id"`
	Count   int   `gorm:"column:item_count"`
	Latest  time.Time `gorm:"column:latest"`
}

// DigestTopicGroups aggregates topic-rooted members the caller has not muted
// since the digest was built.
func (r *MessageRepository) DigestTopicGroups(digestID int64, mutedTypes []string) ([]DigestTopicGroupRow, error) {
	q := r.db.Table("notification_digest_item").
		Select(`m.root_id AS topic_id,
			COALESCE(SUM(GREATEST(m.item_count, 1)), 0) AS item_count,
			MAX(m.created) AS latest`).
		Joins("JOIN message m ON m.id = notification_digest_item.message_id").
		Where("notification_digest_item.digest_id = ?", digestID).
		Where("m.root_kind = 'topic' AND m.root_id IS NOT NULL")
	if len(mutedTypes) > 0 {
		q = q.Where("m.type NOT IN ?", mutedTypes)
	}
	var rows []DigestTopicGroupRow
	err := q.Group("m.root_id").Order("item_count DESC, topic_id DESC").Scan(&rows).Error
	return rows, err
}

// FindTopicTitles resolves titles for a batch of topic ids.
func (r *MessageRepository) FindTopicTitles(ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out, nil
	}
	type row struct {
		ID    int64
		Title string
	}
	var rows []row
	if err := r.db.Table("topic").Select("id, title").Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, x := range rows {
		out[x.ID] = x.Title
	}
	return out, nil
}

// DigestOtherItems returns non-topic members the caller has not muted,
// newest first.
func (r *MessageRepository) DigestOtherItems(digestID int64, mutedTypes []string) ([]V1NotificationRow, error) {
	q := r.db.Table("message AS m").
		Select("DISTINCT m.id, m.sender_id, m.link, m.content, m.status, m.type, m.created, m.item_count, m.actor_count, m.community_notification_id").
		Joins("JOIN notification_digest_item di ON di.message_id = m.id").
		Where("di.digest_id = ?", digestID).
		Where("NOT (m.root_kind = 'topic' AND m.root_id IS NOT NULL)")
	if len(mutedTypes) > 0 {
		q = q.Where("m.type NOT IN ?", mutedTypes)
	}
	var rows []V1NotificationRow
	err := q.Order("m.created DESC, m.id DESC").Limit(digestOtherItemLimit).Scan(&rows).Error
	return rows, err
}

// MarkDigestRead marks the digest notification and its currently unmuted
// members read in one transaction, returning the mirrored community ids to
// forward. Replaying on an already-read digest marks nothing.
func (r *MessageRepository) MarkDigestRead(receiverID, messageID int, mutedTypes []string, hold quiet.Hold) (marked int, mirrorIDs []int64, unread int, found bool, err error) {
	mirrorIDs = []int64{}
	err = r.db.Transaction(func(tx *gorm.DB) error {
		var digest msgModel.NotificationDigest
		if dErr := tx.Where("receiver_id = ? AND message_id = ?", receiverID, messageID).Take(&digest).Error; dErr != nil {
			if errors.Is(dErr, gorm.ErrRecordNotFound) {
				return nil
			}
			return dErr
		}
		found = true

		if digest.Status != msgModel.DigestStatusRead {
			if uErr := tx.Model(&msgModel.NotificationDigest{}).Where("id = ?", digest.ID).
				Updates(map[string]any{"status": msgModel.DigestStatusRead, "read_at": gorm.Expr("now()")}).Error; uErr != nil {
				return uErr
			}
			if uErr := tx.Model(&msgModel.Message{}).Where("id = ?", digest.MessageID).
				Updates(map[string]any{"status": "read", "updated": gorm.Expr("now()")}).Error; uErr != nil {
				return uErr
			}

			q := `UPDATE message m SET status = 'read', updated = now()
FROM notification_digest_item di
WHERE di.digest_id = ? AND di.message_id = m.id AND m.status = 'unread'`
			args := []any{digest.ID}
			if len(mutedTypes) > 0 {
				q += ` AND m.type NOT IN ?`
				args = append(args, mutedTypes)
			}
			q += ` RETURNING m.community_notification_id`
			rows, qErr := tx.Raw(q, args...).Rows()
			if qErr != nil {
				return qErr
			}
			defer rows.Close()
			n := 0
			ids := []int64{}
			for rows.Next() {
				var id sql.NullInt64
				if sErr := rows.Scan(&id); sErr != nil {
					return sErr
				}
				n++
				if id.Valid && id.Int64 > 0 {
					ids = append(ids, id.Int64)
				}
			}
			if rErr := rows.Err(); rErr != nil {
				return rErr
			}
			marked = n
			mirrorIDs = ids
		}

		cq := tx.Table("message").Where("receiver_id = ? AND status = 'unread'", receiverID)
		if len(mutedTypes) > 0 {
			cq = cq.Not("type IN ?", mutedTypes)
		}
		if frag, fragArgs := quiet.HeldNotInSQL(hold, "message.id"); frag != "" {
			cq = cq.Where(frag, fragArgs...)
		}
		var remaining int64
		if cErr := cq.Count(&remaining).Error; cErr != nil {
			return cErr
		}
		unread = int(remaining)
		return nil
	})
	return marked, mirrorIDs, unread, found, err
}
