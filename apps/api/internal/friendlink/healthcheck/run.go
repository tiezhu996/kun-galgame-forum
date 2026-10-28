// Package healthcheck runs one friend-link probe sweep: every link is probed
// concurrently and each verdict is fed to the repository state machine.
package healthcheck

import (
	"context"
	"log/slog"
	"sync"

	"kun-galgame-api/internal/friendlink/repository"
	"kun-galgame-api/internal/friendlink/siteprobe"

	"gorm.io/gorm"
)

const DefaultConcurrency = 6

type Stats struct {
	Checked   int
	Failed    int
	Marked    int
	Recovered int
	Errored   int
}

// Run performs one sweep. A single probe or write failing only drops that row
// from the totals; the sweep itself returns an error only when the candidate
// list cannot be read.
func Run(ctx context.Context, db *gorm.DB, prober siteprobe.Prober, concurrency int) (Stats, error) {
	repo := repository.NewFriendLinkRepository(db)
	candidates, err := repo.HealthCandidates(ctx)
	if err != nil {
		return Stats{}, err
	}
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}

	var (
		wg    sync.WaitGroup
		sem   = make(chan struct{}, concurrency)
		mu    sync.Mutex
		stats Stats
	)

	for _, c := range candidates {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			probeCtx, cancel := context.WithTimeout(ctx, siteprobe.DefaultTimeout)
			ok, reason := prober.Probe(probeCtx, c.Link)
			cancel()

			transition, applyErr := repo.ApplyCheckResult(ctx, c.ID, repository.CheckResult{OK: ok, Reason: reason})
			mu.Lock()
			defer mu.Unlock()
			stats.Checked++
			switch {
			case applyErr != nil:
				stats.Errored++
				slog.Error("友链检查结果写入失败", "id", c.ID, "error", applyErr)
			case transition == repository.TransitionMarked:
				stats.Failed++
				stats.Marked++
			case transition == repository.TransitionRecovered:
				stats.Recovered++
			case !ok:
				stats.Failed++
			}
		}()
	}
	wg.Wait()
	return stats, ctx.Err()
}
