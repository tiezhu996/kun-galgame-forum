package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"kun-galgame-api/internal/website/model"

	"gorm.io/gorm"
)

var (
	ErrSameWebsite   = errors.New("website merge: source and target are the same row")
	ErrAlreadyMerged = errors.New("website merge: source was already merged into another website")
)

type MergeSide struct {
	ID            int      `json:"id"`
	Host          string   `json:"host"`
	Title         string   `json:"title"`
	Status        string   `json:"status"`
	IsNSFW        bool     `json:"is_nsfw"`
	CategoryID    int      `json:"category_id"`
	View          int      `json:"view"`
	LikeCount     int      `json:"like_count"`
	FavoriteCount int      `json:"favorite_count"`
	CommentCount  int      `json:"comment_count"`
	URLs          []string `json:"urls"`
}

type MergeTagDrop struct {
	TagID         int  `json:"tag_id"`
	GroupID       *int `json:"group_id"`
	SurvivorTagID *int `json:"survivor_tag_id"`
}

type WebsiteMergePlan struct {
	Source            MergeSide      `json:"source"`
	Target            MergeSide      `json:"target"`
	LikeSource        int            `json:"like_source"`
	LikeOverlap       int            `json:"like_overlap"`
	LikeMoved         int            `json:"like_moved"`
	LikeResulting     int            `json:"like_resulting"`
	FavoriteSource    int            `json:"favorite_source"`
	FavoriteOverlap   int            `json:"favorite_overlap"`
	FavoriteMoved     int            `json:"favorite_moved"`
	FavoriteResulting int            `json:"favorite_resulting"`
	TagsAdded         []int          `json:"tags_added"`
	TagsDropped       []MergeTagDrop `json:"tags_dropped"`
	URLsAdded         []string       `json:"urls_added"`
	CommentResulting  int            `json:"comment_resulting"`
	ViewResulting     int            `json:"view_resulting"`
	TargetBecomesNSFW bool           `json:"target_becomes_nsfw"`
}

type WebsiteMergeResult struct {
	LedgerID        int64     `json:"id"`
	OldID           int       `json:"old_website_id"`
	NewID           int       `json:"new_website_id"`
	OldHost         string    `json:"old_host"`
	NewHost         string    `json:"new_host"`
	MovedLike       int       `json:"moved_like"`
	DroppedLike     int       `json:"dropped_like"`
	MovedFavorite   int       `json:"moved_favorite"`
	DroppedFavorite int       `json:"dropped_favorite"`
	MovedTag        int       `json:"moved_tag"`
	DroppedTag      int       `json:"dropped_tag"`
	URLsAdded       []string  `json:"added_urls"`
	CommentCountOld int       `json:"comment_count_old"`
	ViewOld         int       `json:"view_old"`
	CreatedAt       time.Time `json:"created"`
}

func websiteURLs(raw json.RawMessage) []string {
	var urls []string
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &urls)
	}
	return urls
}

func loadMergeSide(q *gorm.DB, id int) (model.GalgameWebsite, []string, error) {
	var row model.GalgameWebsite
	err := q.Where("id = ?", id).Take(&row).Error
	if err != nil {
		return row, nil, err
	}
	return row, websiteURLs(row.Domain), nil
}

func mergeSideOf(row model.GalgameWebsite, urls []string) MergeSide {
	return MergeSide{
		ID: row.ID, Host: row.URL, Title: row.Name, Status: row.Status,
		IsNSFW: row.AgeLimit != "all", CategoryID: row.CategoryID,
		View: row.View, LikeCount: row.LikeCount, FavoriteCount: row.FavoriteCount,
		CommentCount: row.CommentCount, URLs: urls,
	}
}

// classifyTags partitions the source's tag links into ones the survivor can
// take and ones a single-select group rule drops: a tag moves unless the
// survivor already carries it or another tag of the same single-select group.
func classifyTags(q *gorm.DB, oldID, newID int) (added []int, dropped []MergeTagDrop, err error) {
	type tagRow struct {
		ID      int  `gorm:"column:id"`
		GroupID *int `gorm:"column:group_id"`
	}
	var sourceTags []tagRow
	err = q.Raw(`SELECT t.id, t.group_id FROM galgame_website_tag_relation r
		JOIN galgame_website_tag t ON t.id = r.galgame_website_tag_id
		WHERE r.galgame_website_id = ? ORDER BY t.id`, oldID).Scan(&sourceTags).Error
	if err != nil {
		return nil, nil, err
	}
	var targetTags []tagRow
	err = q.Raw(`SELECT t.id, t.group_id FROM galgame_website_tag_relation r
		JOIN galgame_website_tag t ON t.id = r.galgame_website_tag_id
		WHERE r.galgame_website_id = ? ORDER BY t.id`, newID).Scan(&targetTags).Error
	if err != nil {
		return nil, nil, err
	}
	groupIDs := map[int]struct{}{}
	for _, t := range sourceTags {
		if t.GroupID != nil {
			groupIDs[*t.GroupID] = struct{}{}
		}
	}
	multi := map[int]bool{}
	if len(groupIDs) > 0 {
		ids := make([]int, 0, len(groupIDs))
		for id := range groupIDs {
			ids = append(ids, id)
		}
		var groups []struct {
			ID          int  `gorm:"column:id"`
			MultiSelect bool `gorm:"column:multi_select"`
		}
		if err = q.Table("galgame_website_tag_group").Select("id, multi_select").Where("id IN ?", ids).Scan(&groups).Error; err != nil {
			return nil, nil, err
		}
		for _, g := range groups {
			multi[g.ID] = g.MultiSelect
		}
	}
	held := map[int]struct{}{}
	heldGroup := map[int]int{}
	for _, t := range targetTags {
		held[t.ID] = struct{}{}
		if t.GroupID != nil {
			heldGroup[*t.GroupID] = t.ID
		}
	}
	// A group that was flipped from multi-select to single-select may already
	// hold several tags on the source itself; only the first one of such a
	// group can move, mirroring what the tag editor guarantees going forward.
	sourceGroupWinner := map[int]int{}
	for _, t := range sourceTags {
		if t.GroupID == nil || multi[*t.GroupID] {
			continue
		}
		if _, exists := sourceGroupWinner[*t.GroupID]; !exists {
			sourceGroupWinner[*t.GroupID] = t.ID
		}
	}
	for _, t := range sourceTags {
		if _, dup := held[t.ID]; dup {
			dropped = append(dropped, MergeTagDrop{TagID: t.ID, GroupID: t.GroupID})
			continue
		}
		if t.GroupID != nil && !multi[*t.GroupID] {
			if survivor, blocked := heldGroup[*t.GroupID]; blocked {
				s := survivor
				dropped = append(dropped, MergeTagDrop{TagID: t.ID, GroupID: t.GroupID, SurvivorTagID: &s})
				continue
			}
			if winner, isExtra := sourceGroupWinner[*t.GroupID]; isExtra && winner != t.ID {
				w := winner
				dropped = append(dropped, MergeTagDrop{TagID: t.ID, GroupID: t.GroupID, SurvivorTagID: &w})
				continue
			}
		}
		added = append(added, t.ID)
	}
	return added, dropped, nil
}

func slotCounts(q *gorm.DB, table string, oldID, newID int) (oldTotal, newTotal, overlap int, err error) {
	var n struct{ Old, New, Overlap int }
	err = q.Raw(fmt.Sprintf(`SELECT
		(SELECT count(*) FROM %[1]s WHERE website_id = ?) AS old,
		(SELECT count(*) FROM %[1]s WHERE website_id = ?) AS new,
		(SELECT count(*) FROM %[1]s a JOIN %[1]s b ON b.user_id = a.user_id
			WHERE a.website_id = ? AND b.website_id = ?) AS overlap`, table),
		oldID, newID, oldID, newID).Scan(&n).Error
	return n.Old, n.New, n.Overlap, err
}

func (s *Store) WebsiteMergePlan(oldID, newID int) (*WebsiteMergePlan, error) {
	if oldID <= 0 || newID <= 0 || oldID == newID {
		return nil, ErrSameWebsite
	}
	oldRow, oldURLs, err := loadMergeSide(s.db, oldID)
	if err != nil {
		return nil, notFound(err)
	}
	newRow, newURLs, err := loadMergeSide(s.db, newID)
	if err != nil {
		return nil, notFound(err)
	}
	added, dropped, err := classifyTags(s.db, oldID, newID)
	if err != nil {
		return nil, err
	}
	likeOld, likeNew, likeOverlap, err := slotCounts(s.db, "galgame_website_like", oldID, newID)
	if err != nil {
		return nil, err
	}
	favOld, favNew, favOverlap, err := slotCounts(s.db, "galgame_website_favorite", oldID, newID)
	if err != nil {
		return nil, err
	}
	_, addedURLs := appendURLsCopy(newURLs, oldURLs)
	plan := &WebsiteMergePlan{
		Source:            mergeSideOf(oldRow, oldURLs),
		Target:            mergeSideOf(newRow, newURLs),
		LikeSource:        likeOld,
		LikeOverlap:       likeOverlap,
		LikeMoved:         likeOld - likeOverlap,
		LikeResulting:     likeNew + likeOld - likeOverlap,
		FavoriteSource:    favOld,
		FavoriteOverlap:   favOverlap,
		FavoriteMoved:     favOld - favOverlap,
		FavoriteResulting: favNew + favOld - favOverlap,
		TagsAdded:         added,
		TagsDropped:       dropped,
		URLsAdded:         addedURLs,
		CommentResulting:  newRow.CommentCount + oldRow.CommentCount,
		ViewResulting:     newRow.View + oldRow.View,
		TargetBecomesNSFW: newRow.AgeLimit == "all" && oldRow.AgeLimit != "all",
	}
	return plan, nil
}

func appendURLsCopy(target, source []string) ([]string, []string) {
	out := append([]string{}, target...)
	added := []string{}
	seen := make(map[string]struct{}, len(target))
	for _, u := range target {
		seen[u] = struct{}{}
	}
	for _, u := range source {
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		added = append(added, u)
		out = append(out, u)
	}
	return out, added
}

type websiteMergeLedger struct {
	ID              int64     `gorm:"column:id"`
	OldWebsiteID    int       `gorm:"column:old_website_id"`
	NewWebsiteID    int       `gorm:"column:new_website_id"`
	OldHost         string    `gorm:"column:old_host"`
	MovedLike       int       `gorm:"column:moved_like"`
	DroppedLike     int       `gorm:"column:dropped_like"`
	MovedFavorite   int       `gorm:"column:moved_favorite"`
	DroppedFavorite int       `gorm:"column:dropped_favorite"`
	MovedTag        int       `gorm:"column:moved_tag"`
	DroppedTag      int       `gorm:"column:dropped_tag"`
	CommentCountOld int       `gorm:"column:comment_count_old"`
	ViewOld         int       `gorm:"column:view_old"`
	Created         time.Time `gorm:"column:created"`
}

func (s *Store) replayMerge(tx *gorm.DB, oldID, newID int) (WebsiteMergeResult, bool, error) {
	var led websiteMergeLedger
	res := tx.Table("galgame_website_merge").Where("old_website_id = ?", oldID).Take(&led)
	if errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return WebsiteMergeResult{}, false, nil
	}
	if res.Error != nil {
		return WebsiteMergeResult{}, false, res.Error
	}
	if led.NewWebsiteID != newID {
		return WebsiteMergeResult{}, false, ErrAlreadyMerged
	}
	var hostRow struct {
		URL string `gorm:"column:url"`
	}
	survivor := tx.Table("galgame_website").Select("url").Where("id = ?", newID).Take(&hostRow)
	if errors.Is(survivor.Error, gorm.ErrRecordNotFound) {
		return WebsiteMergeResult{}, false, ErrNotFound
	}
	if survivor.Error != nil {
		return WebsiteMergeResult{}, false, survivor.Error
	}
	return WebsiteMergeResult{
		LedgerID: led.ID, OldID: oldID, NewID: newID, OldHost: led.OldHost, NewHost: hostRow.URL,
		MovedLike: led.MovedLike, DroppedLike: led.DroppedLike,
		MovedFavorite: led.MovedFavorite, DroppedFavorite: led.DroppedFavorite,
		MovedTag: led.MovedTag, DroppedTag: led.DroppedTag,
		CommentCountOld: led.CommentCountOld, ViewOld: led.ViewOld, CreatedAt: led.Created,
	}, true, nil
}

func (s *Store) FoldWebsite(oldID, newID, actorID int) (WebsiteMergeResult, error) {
	var out WebsiteMergeResult
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		out, err = s.FoldWebsiteTx(tx, oldID, newID, actorID)
		return err
	})
	if err != nil {
		return WebsiteMergeResult{}, err
	}
	return out, nil
}

// FoldWebsiteTx folds one website row onto the survivor. A website that was
// already folded into the same survivor replays the stored result and writes
// nothing; one folded somewhere else is ErrAlreadyMerged.
func (s *Store) FoldWebsiteTx(tx *gorm.DB, oldID, newID, actorID int) (WebsiteMergeResult, error) {
	if oldID <= 0 || newID <= 0 || oldID == newID {
		return WebsiteMergeResult{}, ErrSameWebsite
	}
	if out, ok, err := s.replayMerge(tx, oldID, newID); err != nil {
		return WebsiteMergeResult{}, err
	} else if ok {
		return out, nil
	}

	var ids []int
	if err := tx.Raw(`SELECT id FROM galgame_website WHERE id IN (?, ?) ORDER BY id FOR UPDATE`,
		oldID, newID).Scan(&ids).Error; err != nil {
		return WebsiteMergeResult{}, err
	}
	if len(ids) != 2 {
		// A concurrent fold may have committed while this transaction waited
		// for the row locks: re-check the ledger instead of answering 404.
		if out, ok, err := s.replayMerge(tx, oldID, newID); err != nil {
			return WebsiteMergeResult{}, err
		} else if ok {
			return out, nil
		}
		return WebsiteMergeResult{}, ErrNotFound
	}

	oldRow, oldURLs, err := loadMergeSide(tx, oldID)
	if err != nil {
		return WebsiteMergeResult{}, notFound(err)
	}
	newRow, newURLs, err := loadMergeSide(tx, newID)
	if err != nil {
		return WebsiteMergeResult{}, notFound(err)
	}

	addedTags, droppedTags, err := classifyTags(tx, oldID, newID)
	if err != nil {
		return WebsiteMergeResult{}, err
	}
	mergedURLs, addedURLs := appendURLsCopy(newURLs, oldURLs)
	droppedLikeUsers, err := slotUserIDs(tx, "galgame_website_like", oldID, newID)
	if err != nil {
		return WebsiteMergeResult{}, err
	}
	droppedFavUsers, err := slotUserIDs(tx, "galgame_website_favorite", oldID, newID)
	if err != nil {
		return WebsiteMergeResult{}, err
	}
	likeOverlap, favOverlap := len(droppedLikeUsers), len(droppedFavUsers)

	var ledgerID int64
	if err = tx.Raw(`INSERT INTO galgame_website_merge
			(old_website_id, new_website_id, old_host, old_row, comment_count_old, view_old, created_by)
		VALUES (?, ?, ?, (SELECT to_jsonb(w) FROM galgame_website w WHERE w.id = ?), ?, ?, ?)
		RETURNING id`,
		oldID, newID, oldRow.URL, oldID, oldRow.CommentCount, oldRow.View, actorID).Scan(&ledgerID).Error; err != nil {
		return WebsiteMergeResult{}, err
	}

	movedLike, err := moveSlot(tx, "galgame_website_like", oldID, newID)
	if err != nil {
		return WebsiteMergeResult{}, err
	}
	movedFav, err := moveSlot(tx, "galgame_website_favorite", oldID, newID)
	if err != nil {
		return WebsiteMergeResult{}, err
	}
	movedTag := 0
	if len(addedTags) > 0 {
		res := tx.Exec(`INSERT INTO galgame_website_tag_relation (galgame_website_id, galgame_website_tag_id, created, updated)
			SELECT ?, id, now(), now() FROM galgame_website_tag WHERE id IN ?
			ON CONFLICT DO NOTHING`, newID, addedTags)
		if res.Error != nil {
			return WebsiteMergeResult{}, res.Error
		}
		movedTag = int(res.RowsAffected)
	}
	if err = tx.Exec(`DELETE FROM galgame_website_like WHERE website_id = ?`, oldID).Error; err != nil {
		return WebsiteMergeResult{}, err
	}
	if err = tx.Exec(`DELETE FROM galgame_website_favorite WHERE website_id = ?`, oldID).Error; err != nil {
		return WebsiteMergeResult{}, err
	}

	domainJSON, _ := json.Marshal(mergedURLs)
	if err = tx.Exec(`UPDATE galgame_website SET
			like_count = (SELECT count(*) FROM galgame_website_like WHERE website_id = ?),
			favorite_count = (SELECT count(*) FROM galgame_website_favorite WHERE website_id = ?),
			comment_count = comment_count + ?,
			view = view + ?,
			age_limit = CASE WHEN ? = 'r18' THEN 'r18' ELSE age_limit END,
			domain = ?::jsonb,
			updated = now()
		WHERE id = ?`,
		newID, newID, oldRow.CommentCount, oldRow.View, oldRow.AgeLimit, string(domainJSON), newID).Error; err != nil {
		return WebsiteMergeResult{}, err
	}

	// B→C while A→B stands: A's redirect and absorbed anchor must land on C.
	if err = tx.Exec(`UPDATE galgame_website_merge SET new_website_id = ? WHERE new_website_id = ?`,
		newID, oldID).Error; err != nil {
		return WebsiteMergeResult{}, err
	}
	if err = rewriteHostLinks(tx, oldRow.URL, newRow.URL); err != nil {
		return WebsiteMergeResult{}, err
	}
	if err = tx.Exec(`DELETE FROM galgame_website WHERE id = ?`, oldID).Error; err != nil {
		return WebsiteMergeResult{}, err
	}

	likeUsersJSON, _ := json.Marshal(droppedLikeUsers)
	favUsersJSON, _ := json.Marshal(droppedFavUsers)
	droppedTagJSON, _ := json.Marshal(droppedTags)
	addedURLsJSON, _ := json.Marshal(addedURLs)
	if err = tx.Exec(`UPDATE galgame_website_merge SET
			moved_like = ?, dropped_like = ?, moved_favorite = ?, dropped_favorite = ?,
			moved_tag = ?, dropped_tag = ?, added_urls = ?::jsonb,
			dropped_like_users = ?::jsonb, dropped_favorite_users = ?::jsonb, dropped_tags = ?::jsonb
		WHERE id = ?`,
		movedLike, likeOverlap, movedFav, favOverlap, movedTag, len(droppedTags),
		string(addedURLsJSON), string(likeUsersJSON), string(favUsersJSON), string(droppedTagJSON), ledgerID).Error; err != nil {
		return WebsiteMergeResult{}, err
	}

	return WebsiteMergeResult{
		LedgerID: ledgerID, OldID: oldID, NewID: newID, OldHost: oldRow.URL, NewHost: newRow.URL,
		MovedLike: movedLike, DroppedLike: likeOverlap, MovedFavorite: movedFav, DroppedFavorite: favOverlap,
		MovedTag: movedTag, DroppedTag: len(droppedTags), URLsAdded: addedURLs,
		CommentCountOld: oldRow.CommentCount, ViewOld: oldRow.View,
	}, nil
}

func moveSlot(tx *gorm.DB, table string, oldID, newID int) (int, error) {
	res := tx.Exec(fmt.Sprintf(`INSERT INTO %[1]s (user_id, website_id, created, updated)
		SELECT user_id, ?, now(), now() FROM %[1]s WHERE website_id = ?
		ON CONFLICT (user_id, website_id) DO NOTHING`, table), newID, oldID)
	return int(res.RowsAffected), res.Error
}

func slotUserIDs(tx *gorm.DB, table string, oldID, newID int) ([]int, error) {
	var ids []int
	err := tx.Raw(fmt.Sprintf(`SELECT a.user_id FROM %[1]s a JOIN %[1]s b ON b.user_id = a.user_id
		WHERE a.website_id = ? AND b.website_id = ? ORDER BY a.user_id`, table), oldID, newID).Scan(&ids).Error
	return ids, err
}

func (s *Store) WebsiteRedirectByHost(host string) (newID int, newHost string, ok bool, err error) {
	var row struct {
		NewID int    `gorm:"column:new_website_id"`
		URL   string `gorm:"column:url"`
	}
	res := s.db.Table("galgame_website_merge AS m").
		Select("m.new_website_id, w.url").
		Joins("JOIN galgame_website w ON w.id = m.new_website_id").
		Where("m.old_host = ?", host).Take(&row)
	if errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return 0, "", false, nil
	}
	if res.Error != nil {
		return 0, "", false, res.Error
	}
	return row.NewID, row.URL, true, nil
}

func (s *Store) AbsorbedWebsiteIDs(targetID int) ([]int, error) {
	var ids []int
	err := s.db.Table("galgame_website_merge").
		Where("new_website_id = ?", targetID).Order("old_website_id").
		Pluck("old_website_id", &ids).Error
	return ids, err
}

// ResolveMergedWebsite maps a deleted website id to its survivor. ok is false
// for a live website and for an id that was never folded.
func (s *Store) ResolveMergedWebsite(oldID int) (newID int, ok bool, err error) {
	var n int
	res := s.db.Table("galgame_website_merge").Select("new_website_id").
		Where("old_website_id = ?", oldID).Take(&n)
	if errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if res.Error != nil {
		return 0, false, res.Error
	}
	return n, true, nil
}

func (s *Store) MergedWebsiteDomains(ids []int) (map[int]string, error) {
	out := map[int]string{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		OldID int    `gorm:"column:old_website_id"`
		URL   string `gorm:"column:url"`
	}
	err := s.db.Table("galgame_website_merge AS m").
		Select("m.old_website_id, w.url").
		Joins("JOIN galgame_website w ON w.id = m.new_website_id").
		Where("m.old_website_id IN ?", ids).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.OldID] = r.URL
	}
	return out, nil
}
