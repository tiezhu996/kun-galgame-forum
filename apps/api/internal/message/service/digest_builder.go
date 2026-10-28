package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"kun-galgame-api/internal/message/quiet"
	msgRepo "kun-galgame-api/internal/message/repository"
)

type DigestBuilder struct {
	repo *msgRepo.MessageRepository
}

func NewDigestBuilder(repo *msgRepo.MessageRepository) *DigestBuilder {
	return &DigestBuilder{repo: repo}
}

type DigestRunStats struct {
	UsersScanned int
	Digests      int
	NewDigests   int
}

// BuildWindow builds digests for every quiet-hours-enabled user for the
// window starting at windowStart. Per-user errors are logged and skipped.
func (b *DigestBuilder) BuildWindow(ctx context.Context, windowStart time.Time) (DigestRunStats, error) {
	end := windowStart.Add(quiet.WindowHours * time.Hour)
	var stats DigestRunStats
	after := 0
	for {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		ids, err := b.repo.ListQuietUserIDs(after, 0)
		if err != nil {
			return stats, err
		}
		if len(ids) == 0 {
			return stats, nil
		}
		for _, id := range ids {
			build, err := b.repo.BuildUserDigest(id, windowStart, end)
			if err != nil {
				slog.Error("notification digest: build failed for user", "user_id", id, "error", err)
				continue
			}
			stats.UsersScanned++
			if build == nil {
				continue
			}
			stats.Digests++
			if build.Created {
				stats.NewDigests++
			}
			content := digestContentLine(build.TopicGroups, build.TopicItems, build.OtherItems)
			if err := b.repo.UpdateDigestContent(build.DigestMessageID, content); err != nil {
				slog.Error("notification digest: content update failed", "message_id", build.DigestMessageID, "error", err)
			}
		}
		after = ids[len(ids)-1]
	}
}

func digestContentLine(topicGroups, topicItems, otherItems int) string {
	switch {
	case topicGroups > 0 && otherItems > 0:
		return fmt.Sprintf("夜间通知摘要：%d 个话题有 %d 条新回复，另有 %d 条其他互动", topicGroups, topicItems, otherItems)
	case topicGroups > 0:
		return fmt.Sprintf("夜间通知摘要：%d 个话题有 %d 条新回复", topicGroups, topicItems)
	default:
		return fmt.Sprintf("夜间通知摘要：%d 条其他互动", otherItems)
	}
}
