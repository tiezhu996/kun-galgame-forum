package model

import "time"

type KungalUserState struct {
	UserID                  int       `gorm:"column:user_id;primaryKey" json:"user_id"`
	Moemoepoint             int       `gorm:"default:7" json:"moemoepoint"`
	DailyCheckIn            int       `gorm:"column:daily_check_in;default:0" json:"-"`
	DailyImageCount         int       `gorm:"column:daily_image_count;default:0" json:"-"`
	DailyToolsetUploadCount int       `gorm:"column:daily_toolset_upload_count;default:0" json:"-"`
	DailyToolsetUploadBytes int64     `gorm:"column:daily_toolset_upload_bytes;default:0" json:"-"`
	MutedNotificationTypes  []string  `gorm:"column:muted_notification_types;serializer:json;type:jsonb;default:'[]'" json:"muted_notification_types"`
	QuietHoursEnabled       bool      `gorm:"column:quiet_hours_enabled;not null;default:true" json:"quiet_hours_enabled"`
	CreatedAt               time.Time `gorm:"column:created" json:"created"`
	UpdatedAt               time.Time `gorm:"column:updated" json:"updated"`
}

func (KungalUserState) TableName() string { return "kungal_user_state" }
