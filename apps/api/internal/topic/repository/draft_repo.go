package repository

import (
	"errors"
	"time"

	"kun-galgame-api/internal/topic/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MaxDraftRevisionsPerDraft caps how many pinned revisions one draft keeps.
// Autosave writes no revisions, so only explicit saves and safety
// checkpoints grow the list; older rows are pruned inside the write tx.
const MaxDraftRevisionsPerDraft = 50

// ErrDraftRevisionConflict is the repository-level signal that the stored
// revision moved between the service's read and its write. The service turns
// it into DRAFT_REVISION_CONFLICT and must not attribute it to an internal
// error.
var ErrDraftRevisionConflict = errors.New("topic draft revision conflict")

type TopicDraftRepository struct {
	db *gorm.DB
}

func NewTopicDraftRepository(db *gorm.DB) *TopicDraftRepository {
	return &TopicDraftRepository{db: db}
}

func (r *TopicDraftRepository) DB() *gorm.DB { return r.db }

func (r *TopicDraftRepository) CountByUser(userID int) (int64, error) {
	var n int64
	err := r.db.Model(&model.TopicDraft{}).Where("user_id = ?", userID).Count(&n).Error
	return n, err
}

func (r *TopicDraftRepository) GetByIDForUser(id, userID int) (*model.TopicDraft, error) {
	var d model.TopicDraft
	if err := r.db.Where("id = ? AND user_id = ?", id, userID).First(&d).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *TopicDraftRepository) DeleteForUser(id, userID int) (int64, error) {
	res := r.db.Where("id = ? AND user_id = ?", id, userID).Delete(&model.TopicDraft{})
	return res.RowsAffected, res.Error
}

// CreateWithRevision inserts a new draft and its revision-1 snapshot in one
// transaction: a crash between the two would leave a draft with no history.
func (r *TopicDraftRepository) CreateWithRevision(d *model.TopicDraft, rev *model.TopicDraftRevision) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(d).Error; err != nil {
			return err
		}
		rev.DraftID = d.ID
		rev.UserID = d.UserID
		return tx.Create(rev).Error
	})
}

// PatchDraft writes next over draft next.ID.
//
// The row is locked and re-read inside the transaction: if the stored
// revision differs from baseRevision the write is refused (the first return
// is the untouched current draft and err is ErrDraftRevisionConflict) unless
// force is set, in which case the stored content is pinned as a 'checkpoint'
// revision before the overwrite so the other window's work is never lost.
// snapshot additionally pins next's content as a 'manual' revision.
func (r *TopicDraftRepository) PatchDraft(
	next *model.TopicDraft,
	baseRevision int,
	force, snapshot bool,
) (*model.TopicDraft, []*model.TopicDraftRevision, error) {
	var inserted []*model.TopicDraftRevision
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var cur model.TopicDraft
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", next.ID, next.UserID).
			First(&cur).Error; err != nil {
			return err
		}
		if cur.Revision != baseRevision {
			if !force {
				return ErrDraftRevisionConflict
			}
			inserted = append(inserted,
				model.NewDraftRevision(&cur, model.DraftRevisionCheckpoint, cur.Revision))
		}

		next.Revision = cur.Revision + 1
		next.CreatedAt = cur.CreatedAt
		if next.UpdatedAt.IsZero() {
			next.UpdatedAt = time.Now().UTC()
		}
		if err := tx.Save(next).Error; err != nil {
			return err
		}
		if snapshot {
			inserted = append(inserted,
				model.NewDraftRevision(next, model.DraftRevisionManual, next.Revision))
		}
		for _, rev := range inserted {
			if err := tx.Create(rev).Error; err != nil {
				return err
			}
		}
		return pruneDraftRevisions(tx, next.ID)
	})
	if errors.Is(err, ErrDraftRevisionConflict) {
		var cur model.TopicDraft
		if reloadErr := r.db.Where("id = ? AND user_id = ?", next.ID, next.UserID).First(&cur).Error; reloadErr == nil {
			return &cur, nil, ErrDraftRevisionConflict
		}
		return nil, nil, ErrDraftRevisionConflict
	}
	if err != nil {
		return nil, nil, err
	}
	return next, inserted, nil
}

// RestoreRevision pins the current working copy as a 'checkpoint', then
// copies revision revisionID's content onto the draft and bumps its
// revision. A stale baseRevision is refused like PatchDraft: a restore must
// not land on top of another window's unseen write.
func (r *TopicDraftRepository) RestoreRevision(
	draftID, userID, revisionID, baseRevision int,
) (*model.TopicDraft, bool, error) {
	var out model.TopicDraft
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var cur model.TopicDraft
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", draftID, userID).First(&cur).Error; err != nil {
			return err
		}
		if cur.Revision != baseRevision {
			out = cur
			return ErrDraftRevisionConflict
		}
		var rev model.TopicDraftRevision
		if err := tx.
			Where("id = ? AND draft_id = ? AND user_id = ?", revisionID, draftID, userID).
			First(&rev).Error; err != nil {
			return err
		}

		checkpoint := model.NewDraftRevision(&cur, model.DraftRevisionCheckpoint, cur.Revision)
		next := cur
		model.CopyRevisionToDraft(&rev, &next)
		next.Revision = cur.Revision + 1
		next.UpdatedAt = time.Now().UTC()

		if err := tx.Save(&next).Error; err != nil {
			return err
		}
		if err := tx.Create(checkpoint).Error; err != nil {
			return err
		}
		if err := pruneDraftRevisions(tx, draftID); err != nil {
			return err
		}
		out = next
		return nil
	})
	if errors.Is(err, ErrDraftRevisionConflict) {
		return &out, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &out, false, nil
}

// Keep the newest MaxDraftRevisionsPerDraft rows of one draft.
func pruneDraftRevisions(tx *gorm.DB, draftID int) error {
	return tx.Exec(`
DELETE FROM topic_draft_revision
WHERE draft_id = ?
  AND id NOT IN (
    SELECT id FROM topic_draft_revision
    WHERE draft_id = ?
    ORDER BY id DESC
    LIMIT ?
  )`, draftID, draftID, MaxDraftRevisionsPerDraft).Error
}
