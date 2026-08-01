package outreach

import (
	"context"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

// defaultBatchSize is how many emails the auto-send loop fires before pausing.
const defaultBatchSize = 5

// defaultBatchPause is how long the auto-send loop waits between batches.
const defaultBatchPause = 60 * time.Second

// BatchSize returns the configured outreach batch size (0 → default 5).
func BatchSize(cfg *config.Config) int {
	if cfg != nil && cfg.OutreachBatchSize > 0 {
		return cfg.OutreachBatchSize
	}
	return defaultBatchSize
}

// BatchPause returns the configured pause between outreach batches (0 → default 60s).
func BatchPause(cfg *config.Config) time.Duration {
	if cfg != nil && cfg.OutreachBatchPauseSec > 0 {
		return time.Duration(cfg.OutreachBatchPauseSec) * time.Second
	}
	return defaultBatchPause
}

// SendInBatches sends items through send in groups of BatchSize(cfg), pausing
// BatchPause(cfg) between groups whenever more work remains. pause lets callers
// inject the sleep (tests pass a no-op recorder); send is the per-item sender.
// ctx cancellation aborts between sends, and per-item errors are collected but
// never fatal — one bad item must not stop the batch. The returned sent count
// only counts items send completed without error.
func SendInBatches(ctx context.Context, cfg *config.Config, items []Item, send func(Item) error, pause func(time.Duration)) (sent int, errs []error) {
	size := BatchSize(cfg)
	for i := 0; i < len(items); i += size {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		end := i + size
		if end > len(items) {
			end = len(items)
		}
		for _, it := range items[i:end] {
			if err := ctx.Err(); err != nil {
				errs = append(errs, err)
				return sent, errs
			}
			if err := send(it); err != nil {
				errs = append(errs, err)
				continue
			}
			sent++
		}
		if end < len(items) && pause != nil {
			pause(BatchPause(cfg))
		}
	}
	return sent, errs
}
