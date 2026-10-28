package repository

import (
	"encoding/json"
	"errors"

	"gorm.io/gorm"
)

func (r *MessageRepository) FindNotificationSettings(userID int) (muted []string, quiet bool, err error) {
	var raw string
	row := r.db.Raw(
		`SELECT COALESCE(muted_notification_types::text, '[]'), COALESCE(quiet_hours_enabled, true)
FROM kungal_user_state WHERE user_id = ?`,
		userID,
	).Row()
	if scanErr := row.Scan(&raw, &quiet); scanErr != nil {
		if errors.Is(scanErr, gorm.ErrRecordNotFound) {
			return nil, true, nil
		}
		return nil, true, scanErr
	}
	if raw == "" {
		return nil, quiet, nil
	}
	if err := json.Unmarshal([]byte(raw), &muted); err != nil {
		return nil, true, err
	}
	return muted, quiet, nil
}
