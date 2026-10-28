package repository

import (
	"time"

	"kun-galgame-api/internal/topic/model"
)

const draftSummaryChars = 120

type DraftKeysetRow struct {
	ID        int
	Title     string
	Summary   string
	Revision  int
	CreatedAt time.Time `gorm:"column:created"`
	UpdatedAt time.Time `gorm:"column:updated"`
}

type DraftKeysetPos struct {
	Updated time.Time
	ID      int
}

// LEFT() counts characters, not bytes, so the summary is at most 120 code
// points and the schema's maxLength is in the same unit.
func (r *TopicDraftRepository) FindKeysetForUser(userID, limit int, pos *DraftKeysetPos) ([]DraftKeysetRow, error) {
	query := r.db.Model(&model.TopicDraft{}).
		Select("id, title, LEFT(content, ?) AS summary, revision, created, updated", draftSummaryChars).
		Where("user_id = ?", userID)
	if pos != nil {
		query = query.Where("(updated, id) < (?, ?)", pos.Updated, pos.ID)
	}
	var rows []DraftKeysetRow
	err := query.Order("updated DESC, id DESC").Limit(limit).Scan(&rows).Error
	return rows, err
}
