package repository

import (
	"time"

	"kun-galgame-api/internal/topic/model"
)

type DraftRevisionSummaryRow struct {
	ID           int
	Kind         string
	BaseRevision int `gorm:"column:base_revision"`
	Title        string
	Summary      string
	CreatedAt    time.Time `gorm:"column:created"`
}

// LEFT() counts characters, not bytes — same cut as the draft list summary
// (see FindKeysetForUser).
func (r *TopicDraftRepository) ListRevisions(draftID int) ([]DraftRevisionSummaryRow, error) {
	var rows []DraftRevisionSummaryRow
	err := r.db.Model(&model.TopicDraftRevision{}).
		Select("id, kind, base_revision, title, LEFT(content, ?) AS summary, created", draftSummaryChars).
		Where("draft_id = ?", draftID).
		Order("id DESC").
		Limit(MaxDraftRevisionsPerDraft).
		Scan(&rows).Error
	return rows, err
}

func (r *TopicDraftRepository) GetRevision(id, draftID, userID int) (*model.TopicDraftRevision, error) {
	var rev model.TopicDraftRevision
	err := r.db.
		Where("id = ? AND draft_id = ? AND user_id = ?", id, draftID, userID).
		First(&rev).Error
	if err != nil {
		return nil, err
	}
	return &rev, nil
}

func (r *TopicDraftRepository) CountRevisions(draftID int) (int64, error) {
	var n int64
	err := r.db.Model(&model.TopicDraftRevision{}).
		Where("draft_id = ?", draftID).Count(&n).Error
	return n, err
}
