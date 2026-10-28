package model

import "time"

const DigestStatusUnread = "unread"
const DigestStatusRead = "read"

type NotificationDigest struct {
	ID          int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	ReceiverID  int        `gorm:"column:receiver_id;not null" json:"receiver_id"`
	MessageID   int        `gorm:"column:message_id;not null" json:"message_id"`
	WindowStart time.Time  `gorm:"column:window_start;not null" json:"window_start"`
	WindowEnd   time.Time  `gorm:"column:window_end;not null" json:"window_end"`
	Status      string     `gorm:"not null;default:'unread'" json:"status"`
	TotalCount  int        `gorm:"column:total_count;not null;default:0" json:"total_count"`
	ReadAt      *time.Time `gorm:"column:read_at" json:"read_at"`
	CreatedAt   time.Time  `gorm:"column:created" json:"created"`
}

func (NotificationDigest) TableName() string { return "notification_digest" }

type NotificationDigestItem struct {
	DigestID  int64 `gorm:"primaryKey;column:digest_id" json:"digest_id"`
	MessageID int   `gorm:"primaryKey;column:message_id" json:"message_id"`
}

func (NotificationDigestItem) TableName() string { return "notification_digest_item" }
