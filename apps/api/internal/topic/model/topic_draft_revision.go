package model

import (
	"encoding/json"
	"time"
)

const (
	// DraftRevisionManual is a revision the author explicitly pinned. A new
	// draft is born with one (revision 1).
	DraftRevisionManual = "manual"
	// DraftRevisionCheckpoint is content preserved automatically: the other
	// window's working copy before a force-overwrite, or the working copy
	// immediately before a restore.
	DraftRevisionCheckpoint = "checkpoint"
)

type TopicDraftRevision struct {
	ID      int    `gorm:"primaryKey;autoIncrement" json:"id"`
	DraftID int    `gorm:"column:draft_id;not null" json:"draft_id"`
	UserID  int    `gorm:"column:user_id;not null" json:"user_id"`
	Kind    string `gorm:"type:varchar(16);not null;default:'manual'" json:"kind"`
	// BaseRevision is the working-copy revision whose content this row
	// captures. Ordering and identity come from id, not from this number.
	BaseRevision int `gorm:"column:base_revision;not null;default:1" json:"base_revision"`

	Title       string      `gorm:"type:varchar(233);not null;default:''" json:"title"`
	Content     string      `gorm:"type:text;not null;default:''" json:"content"`
	Category    string      `gorm:"not null;default:''" json:"category"`
	Sections    StringSlice `gorm:"column:sections;type:text;not null;default:''" json:"sections"`
	CoverImages ImageTokens `gorm:"column:cover_images;type:text;not null;default:''" json:"cover_images"`
	IsNSFW      bool        `gorm:"column:is_nsfw;default:false" json:"is_nsfw"`

	Polls            json.RawMessage `gorm:"column:polls;serializer:json;type:jsonb;not null;default:'[]'" json:"polls"`
	Lotteries        json.RawMessage `gorm:"column:lotteries;serializer:json;type:jsonb;not null;default:'[]'" json:"lotteries"`
	PrizeImageHashes []string        `gorm:"column:prize_image_hashes;serializer:json;type:jsonb;not null;default:'[]'" json:"prize_image_hashes"`

	CreatedAt time.Time `gorm:"column:created" json:"created"`
}

func (TopicDraftRevision) TableName() string { return "topic_draft_revision" }

// cloneJSONB never returns nil: GORM's json serializer turns a nil value on
// a NOT NULL jsonb column into an empty string, which Postgres rejects with
// 22P02 — append to a nil slice with zero elements keeps the nil.
func cloneJSONB(b json.RawMessage) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("[]")
	}
	out := make(json.RawMessage, len(b))
	copy(out, b)
	return out
}

func cloneStrings(s []string) []string {
	out := make([]string, len(s))
	copy(out, s)
	return out
}

// NewDraftRevision snapshots one draft's content as the given kind. The
// caller sets BaseRevision: the working revision the snapshot represents.
func NewDraftRevision(d *TopicDraft, kind string, baseRevision int) *TopicDraftRevision {
	return &TopicDraftRevision{
		DraftID:          d.ID,
		UserID:           d.UserID,
		Kind:             kind,
		BaseRevision:     baseRevision,
		Title:            d.Title,
		Content:          d.Content,
		Category:         d.Category,
		Sections:         append(StringSlice{}, d.Sections...),
		CoverImages:      append(ImageTokens{}, d.CoverImages...),
		IsNSFW:           d.IsNSFW,
		Polls:            cloneJSONB(d.Polls),
		Lotteries:        cloneJSONB(d.Lotteries),
		PrizeImageHashes: cloneStrings(d.PrizeImageHashes),
	}
}

// CopyRevisionToDraft overwrites every content field of dst with rev's
// content. Identity columns and timestamps are left untouched.
func CopyRevisionToDraft(rev *TopicDraftRevision, dst *TopicDraft) {
	dst.Title = rev.Title
	dst.Content = rev.Content
	dst.Category = rev.Category
	dst.Sections = append(StringSlice{}, rev.Sections...)
	dst.CoverImages = append(ImageTokens{}, rev.CoverImages...)
	dst.IsNSFW = rev.IsNSFW
	dst.Polls = cloneJSONB(rev.Polls)
	dst.Lotteries = cloneJSONB(rev.Lotteries)
	dst.PrizeImageHashes = cloneStrings(rev.PrizeImageHashes)
}
