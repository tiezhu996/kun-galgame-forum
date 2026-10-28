package apiv1

import (
	"context"

	v1 "kun-galgame-api/internal/apiv1"
	"kun-galgame-api/internal/message/notifytype"
	"kun-galgame-api/pkg/problem"
)

type getNotificationPreferencesOutput struct {
	Body NotificationPreferences
}

type putNotificationPreferencesInput struct {
	Body putNotificationPreferencesBody
}

type putNotificationPreferencesBody struct {
	MutedTypes        []MutedType `json:"muted_types" maxItems:"25" uniqueItems:"true" doc:"Notification types to mute, replacing the stored set. Empty array mutes nothing."`
	IsQuietHoursEnabled *bool       `json:"is_quiet_hours_enabled,omitempty" doc:"Whether nightly quiet hours (23:00-08:00 Asia/Shanghai) are on. Optional: when the field is absent the stored value is kept, so older clients that only write muted_types keep working."`
}

type putNotificationPreferencesOutput struct {
	Body NotificationPreferences
}

func (s *Users) getNotificationPreferences(ctx context.Context, _ *struct{}) (*getNotificationPreferencesOutput, error) {
	if prob := s.readyUsers(); prob != nil {
		return nil, prob
	}
	user := v1.User(ctx)
	if user == nil {
		return nil, problem.New(problem.CodeInvalidCredential, "The credential is invalid, expired, or revoked.")
	}
	stored, err := s.users.MutedNotificationTypes(user.ID)
	if err != nil {
		return nil, problem.Internal(err)
	}
	quiet, err := s.users.QuietHoursEnabled(user.ID)
	if err != nil {
		return nil, problem.Internal(err)
	}
	return &getNotificationPreferencesOutput{Body: NotificationPreferences{
		Object:              "notification_preferences",
		MutedTypes:          mutedFromStored(stored),
		IsQuietHoursEnabled: quiet,
	}}, nil
}

func (s *Users) putNotificationPreferences(ctx context.Context, in *putNotificationPreferencesInput) (*putNotificationPreferencesOutput, error) {
	if prob := s.readyUsers(); prob != nil {
		return nil, prob
	}
	user := v1.User(ctx)
	if user == nil {
		return nil, problem.New(problem.CodeInvalidCredential, "The credential is invalid, expired, or revoked.")
	}
	stored := mutedToStored(in.Body.MutedTypes)
	quiet := in.Body.IsQuietHoursEnabled
	if quiet == nil {
		current, err := s.users.QuietHoursEnabled(user.ID)
		if err != nil {
			return nil, problem.Internal(err)
		}
		quiet = &current
	}
	if err := s.users.ReplaceNotificationPreferences(user.ID, stored, *quiet); err != nil {
		return nil, problem.Internal(err)
	}
	return &putNotificationPreferencesOutput{Body: NotificationPreferences{
		Object:              "notification_preferences",
		MutedTypes:          mutedFromStored(stored),
		IsQuietHoursEnabled: *quiet,
	}}, nil
}

type mutedTypeInput struct {
	MutedType MutedType `path:"muted_type"`
}

func (s *Users) muteNotificationType(ctx context.Context, in *mutedTypeInput) (*putNotificationPreferencesOutput, error) {
	return s.setMutedType(ctx, in.MutedType, true)
}

func (s *Users) unmuteNotificationType(ctx context.Context, in *mutedTypeInput) (*putNotificationPreferencesOutput, error) {
	return s.setMutedType(ctx, in.MutedType, false)
}

func (s *Users) setMutedType(ctx context.Context, tok MutedType, muted bool) (*putNotificationPreferencesOutput, error) {
	if prob := s.readyUsers(); prob != nil {
		return nil, prob
	}
	user := v1.User(ctx)
	if user == nil {
		return nil, problem.New(problem.CodeInvalidCredential, "The credential is invalid, expired, or revoked.")
	}
	stored, err := s.users.SetMutedNotificationType(user.ID, mutedToStored([]MutedType{tok})[0], muted)
	if err != nil {
		return nil, problem.Internal(err)
	}
	quiet, err := s.users.QuietHoursEnabled(user.ID)
	if err != nil {
		return nil, problem.Internal(err)
	}
	return &putNotificationPreferencesOutput{Body: NotificationPreferences{
		Object:              "notification_preferences",
		MutedTypes:          mutedFromStored(stored),
		IsQuietHoursEnabled: quiet,
	}}, nil
}

func mutedToStored(tokens []MutedType) []string {
	out := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		if string(tok) == notifytype.KeyChat {
			out = append(out, notifytype.KeyChat)
			continue
		}
		out = append(out, notifytype.ToDB(notifytype.Type(tok)))
	}
	return out
}

func mutedFromStored(dbKeys []string) []MutedType {
	out := make([]MutedType, 0, len(dbKeys))
	for _, db := range dbKeys {
		if db == notifytype.KeyChat {
			out = append(out, MutedType(notifytype.KeyChat))
			continue
		}
		t, ok := notifytype.FromDB(db)
		if !ok {
			continue
		}
		out = append(out, MutedType(t))
	}
	return out
}
