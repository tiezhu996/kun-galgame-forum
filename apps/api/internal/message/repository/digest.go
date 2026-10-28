package repository

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	msgModel "kun-galgame-api/internal/message/model"
	"kun-galgame-api/internal/message/notifytype"

	"gorm.io/gorm"
)

const quietUsersBatch = 500

// DigestBuild is the per-user outcome of one window build.
type DigestBuild struct {
	DigestMessageID int
	Created         bool
	Total           int
	TopicGroups     int
	TopicItems      int
	OtherItems      int
}

// ListQuietUserIDs pages users with quiet hours on, ascending id.
func (r *MessageRepository) ListQuietUserIDs(afterID, limit int) ([]int, error) {
	if limit <= 0 || limit > quietUsersBatch {
		limit = quietUsersBatch
	}
	var ids []int
	err := r.db.Table("kungal_user_state").
		Select("user_id").
		Where("user_id > ? AND quiet_hours_enabled = true", afterID).
		Order("user_id ASC").Limit(limit).Scan(&ids).Error
	return ids, err
}

// BuildUserDigest groups one user's unattached unread rows of one window into
// a digest. It is idempotent: rerunning the same window reuses the digest and
// only attaches late-arriving rows. Nothing is created when the user has
// neither an existing digest for the window nor new candidates.
func (r *MessageRepository) BuildUserDigest(userID int, start, end time.Time) (*DigestBuild, error) {
	var build DigestBuild
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var digest msgModel.NotificationDigest
		dbErr := tx.Where("receiver_id = ? AND window_start = ?", userID, start).Take(&digest).Error
		if dbErr != nil && !errors.Is(dbErr, gorm.ErrRecordNotFound) {
			return dbErr
		}
		exists := dbErr == nil

		var rawMuted string
		if err := tx.Raw(
			`SELECT COALESCE(muted_notification_types::text, '[]') FROM kungal_user_state WHERE user_id = ?`,
			userID,
		).Scan(&rawMuted).Error; err != nil {
			return err
		}
		var mutedKeys []string
		if rawMuted != "" {
			if err := json.Unmarshal([]byte(rawMuted), &mutedKeys); err != nil {
				return err
			}
		}

		candidateSQL := `
SELECT m.id FROM message m
WHERE m.receiver_id = ?
  AND m.status = 'unread'
  AND m.type <> 'digest'
  AND m.created >= ? AND m.created < ?
  AND NOT EXISTS (SELECT 1 FROM notification_digest_item di WHERE di.message_id = m.id)`
		args := []any{userID, start, end}
		if len(mutedKeys) > 0 {
			candidateSQL += ` AND m.type NOT IN ?`
			args = append(args, mutedKeys)
		}
		var candidateIDs []int
		if err := tx.Raw(candidateSQL, args...).Scan(&candidateIDs).Error; err != nil {
			return err
		}

		if !exists && len(candidateIDs) == 0 {
			return nil
		}

		if !exists {
			msgRow := &msgModel.Message{
				Type:       notifytype.ToDB(notifytype.DigestNotification),
				Status:     "unread",
				SenderID:   0,
				ReceiverID: userID,
			}
			if err := tx.Create(msgRow).Error; err != nil {
				return err
			}
			link := "/message/digest/" + strconv.Itoa(msgRow.ID)
			if err := tx.Model(&msgModel.Message{}).Where("id = ?", msgRow.ID).Update("link", link).Error; err != nil {
				return err
			}
			msgRow.Link = link
			digest = msgModel.NotificationDigest{
				ReceiverID:  userID,
				MessageID:   msgRow.ID,
				WindowStart: start,
				WindowEnd:   end,
				Status:      msgModel.DigestStatusUnread,
			}
			if err := tx.Create(&digest).Error; err != nil {
				return err
			}
			build.Created = true
		}

		for _, id := range candidateIDs {
			if err := tx.Exec(
				`INSERT INTO notification_digest_item (digest_id, message_id) VALUES (?, ?) ON CONFLICT DO NOTHING`,
				digest.ID, id,
			).Error; err != nil {
				return err
			}
		}

		var stats DigestBuild
		if err := tx.Raw(`
SELECT
	COALESCE(SUM(GREATEST(m.item_count, 1)), 0) AS total,
	COUNT(DISTINCT m.root_id) FILTER (WHERE m.root_kind = 'topic' AND m.root_id IS NOT NULL) AS topic_groups,
	COALESCE(SUM(GREATEST(m.item_count, 1)) FILTER (WHERE m.root_kind = 'topic' AND m.root_id IS NOT NULL), 0) AS topic_items,
	COALESCE(SUM(GREATEST(m.item_count, 1)) FILTER (WHERE NOT (m.root_kind = 'topic' AND m.root_id IS NOT NULL)), 0) AS other_items
FROM notification_digest_item di
JOIN message m ON m.id = di.message_id
WHERE di.digest_id = ?`, digest.ID).Scan(&stats).Error; err != nil {
			return err
		}
		if err := tx.Model(&msgModel.NotificationDigest{}).
			Where("id = ?", digest.ID).
			Update("total_count", stats.Total).Error; err != nil {
			return err
		}

		build.DigestMessageID = digest.MessageID
		build.Total = stats.Total
		build.TopicGroups = stats.TopicGroups
		build.TopicItems = stats.TopicItems
		build.OtherItems = stats.OtherItems
		return nil
	})
	if err != nil {
		return nil, err
	}
	if build.DigestMessageID == 0 {
		return nil, nil
	}
	return &build, nil
}

// UpdateDigestContent rewrites the synthetic notification's summary line.
func (r *MessageRepository) UpdateDigestContent(messageID int, content string) error {
	return r.db.Model(&msgModel.Message{}).Where("id = ?", messageID).Update("content", content).Error
}
