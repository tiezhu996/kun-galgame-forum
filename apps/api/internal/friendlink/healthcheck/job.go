package healthcheck

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"

	"kun-galgame-api/internal/friendlink/siteprobe"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const lockKey = "kungal:cron:friend-link-check"

// A sweep against ~40 links at six workers with a 10s timeout finishes well
// inside two minutes; the TTL only needs to outlast a pathological run and to
// release a crashed one before the next beat.
const lockTTL = 15 * time.Minute

// deleteOwnerLock removes the key only when it still holds our token, so a
// sweep that outlives the TTL cannot delete a successor instance's lock.
const deleteOwnerLock = `if redis.call('get', KEYS[1]) == ARGV[1] then
	return redis.call('del', KEYS[1])
else
	return 0
end`

// NewJob returns the cron entrypoint: one sweep per instance at a time, even
// when several API replicas fire the same 30-minute beat. A nil client (local
// dev) skips the lock.
func NewJob(db *gorm.DB, rdb *redis.Client, prober siteprobe.Prober) func() {
	return func() {
		ctx := context.Background()
		if rdb != nil {
			token := lockToken()
			locked, err := rdb.SetNX(ctx, lockKey, token, lockTTL).Result()
			if err != nil {
				slog.Error("友链健康检查抢锁失败, 本轮跳过", "error", err)
				return
			}
			if !locked {
				slog.Info("友链健康检查已在其他实例执行, 本轮跳过")
				return
			}
			defer func() { _ = rdb.Eval(ctx, deleteOwnerLock, []string{lockKey}, token).Err() }()
		}

		started := time.Now()
		stats, err := Run(ctx, db, prober, DefaultConcurrency)
		if err != nil {
			slog.Error("友链健康检查失败", "error", err)
			return
		}
		slog.Info("友链健康检查完成",
			"checked", stats.Checked, "failed", stats.Failed,
			"marked", stats.Marked, "recovered", stats.Recovered,
			"errored", stats.Errored, "elapsed", time.Since(started))
	}
}

func lockToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(b[:])
}
