package apiv1

import (
	"context"
	"fmt"

	"kun-galgame-api/internal/apiv1/repr"
	"kun-galgame-api/internal/message/repository"
	"kun-galgame-api/pkg/problem"
	"kun-galgame-api/pkg/userclient"
)

type DigestTopicGroup struct {
	Object     string         `json:"object" enum:"notification_digest_topic_group" maxLength:"31" doc:"Type discriminant. Always notification_digest_topic_group."`
	TopicID    repr.DecimalID `json:"topic_id" doc:"Topic id. JSON string of a decimal integer."`
	TopicTitle string         `json:"topic_title" maxLength:"233" doc:"Title of that topic. Free text; never use it as a decision input. Empty string when the topic no longer exists; the client renders its own fallback while keeping the path."`
	Path       string         `json:"path" maxLength:"100" pattern:"^/" doc:"In-site path /topic/{topic_id}."`
	Count      int            `json:"count" minimum:"0" doc:"New replies folded into this group, summed item_count across its member rows."`
}

type NotificationDigestDetail struct {
	Object        string             `json:"object" enum:"notification_digest" maxLength:"20" doc:"Type discriminant. Always notification_digest."`
	ID            repr.DecimalID     `json:"id" doc:"The digest notification's id; it is also a listable notification of notification_type notification_digest."`
	IsRead        bool               `json:"is_read" doc:"Whether the digest (and the members it owned at mark time) has been marked read."`
	WindowStartAt repr.DateTime      `json:"window_start_at" doc:"Start of the covered quiet window (23:00 Asia/Shanghai), inclusive."`
	WindowEndAt   repr.DateTime      `json:"window_end_at" doc:"End of the covered quiet window (08:00 Asia/Shanghai), exclusive."`
	TotalCount    int                `json:"total_count" minimum:"0" doc:"Snapshot of every member's folded item count at build time, including types the caller muted afterwards. topic_groups and other_items only render the types currently unmuted."`
	TopicGroups   []DigestTopicGroup `json:"topic_groups" doc:"One group per topic with new replies, most new replies first."`
	OtherItems    []Notification     `json:"other_items" doc:"Member notifications not rooted in a topic (likes, follows, system notices and similar), newest first. Rendered with the same notification shape as the list."`
}

type DigestReadMarker struct {
	Object      string `json:"object" enum:"notification_digest_read_marker" maxLength:"31" doc:"Type discriminant. Always notification_digest_read_marker."`
	MarkedCount int    `json:"marked_count" minimum:"0" doc:"Member rows this request marked read. Zero on a replay."`
	UnreadCount int    `json:"unread_count" minimum:"0" doc:"Unread rows remaining in the unmuted partition after the mark, counted in SQL with the quiet-hours hold applied."`
}

type getDigestInput struct {
	DigestID string `path:"digest_id" pattern:"^[1-9][0-9]{0,18}$" maxLength:"19" doc:"The digest notification's id."`
}

type getDigestOutput struct {
	Body NotificationDigestDetail
}

type markDigestReadInput struct {
	DigestID string `path:"digest_id" pattern:"^[1-9][0-9]{0,18}$" maxLength:"19" doc:"The digest notification's id."`
}

type markDigestReadOutput struct {
	Body DigestReadMarker
}

func (s *Service) getNotificationDigest(ctx context.Context, in *getDigestInput) (*getDigestOutput, error) {
	if p := s.ready(); p != nil {
		return nil, p
	}
	user, p := mustUser(ctx)
	if p != nil {
		return nil, p
	}
	id, ok := parsePositiveID(in.DigestID)
	if !ok {
		return nil, notFound()
	}
	mutedTypes, _, _, p := s.notificationSettings(user.ID)
	if p != nil {
		return nil, p
	}
	digest, err := s.messages.FindDigestByMessageID(user.ID, id)
	if err != nil {
		return nil, problem.Internal(err)
	}
	if digest == nil {
		return nil, notFound()
	}

	groupRows, err := s.messages.DigestTopicGroups(digest.ID, mutedTypes)
	if err != nil {
		return nil, problem.Internal(err)
	}
	topicIDs := make([]int64, 0, len(groupRows))
	for _, g := range groupRows {
		topicIDs = append(topicIDs, g.TopicID)
	}
	titles, err := s.messages.FindTopicTitles(topicIDs)
	if err != nil {
		return nil, problem.Internal(err)
	}
	groups := make([]DigestTopicGroup, 0, len(groupRows))
	for _, g := range groupRows {
		groups = append(groups, DigestTopicGroup{
			Object:     "notification_digest_topic_group",
			TopicID:    repr.ID(int(g.TopicID)),
			TopicTitle: titles[g.TopicID],
			Path:       fmt.Sprintf("/topic/%d", g.TopicID),
			Count:      g.Count,
		})
	}

	otherRows, err := s.messages.DigestOtherItems(digest.ID, mutedTypes)
	if err != nil {
		return nil, problem.Internal(err)
	}
	users := s.hydrateUsers(ctx, userclient.CollectIDs(otherRows, func(r repository.V1NotificationRow) int { return r.SenderID }))
	others := make([]Notification, 0, len(otherRows))
	for _, row := range otherRows {
		n, ok := s.mapNotification(row, users)
		if ok {
			others = append(others, n)
		}
	}

	return &getDigestOutput{Body: NotificationDigestDetail{
		Object:        "notification_digest",
		ID:            repr.ID(digest.MessageID),
		IsRead:        digest.Status == "read",
		WindowStartAt: repr.Timestamp(digest.WindowStart),
		WindowEndAt:   repr.Timestamp(digest.WindowEnd),
		TotalCount:    digest.TotalCount,
		TopicGroups:   groups,
		OtherItems:    others,
	}}, nil
}

func (s *Service) markNotificationDigestRead(ctx context.Context, in *markDigestReadInput) (*markDigestReadOutput, error) {
	if p := s.ready(); p != nil {
		return nil, p
	}
	user, p := mustUser(ctx)
	if p != nil {
		return nil, p
	}
	id, ok := parsePositiveID(in.DigestID)
	if !ok {
		return nil, notFound()
	}
	mutedTypes, _, hold, p := s.notificationSettings(user.ID)
	if p != nil {
		return nil, p
	}
	marked, ids, unread, found, err := s.messages.MarkDigestRead(user.ID, id, mutedTypes, hold)
	if err != nil {
		return nil, problem.Internal(err)
	}
	if !found {
		return nil, notFound()
	}
	s.forwardRead(user.ID, ids)
	return &markDigestReadOutput{Body: DigestReadMarker{
		Object:      "notification_digest_read_marker",
		MarkedCount: marked,
		UnreadCount: unread,
	}}, nil
}
