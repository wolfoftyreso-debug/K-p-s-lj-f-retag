package lifecycle

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// ProcessNext owns the durable claim, consume, and acknowledgement boundary.
// Its bool reports whether a row was claimed; delivery is allowed to repeat.
type ProcessNext func(context.Context) (bool, error)

// RunWorker polls persistent work until cancellation. Store failures are
// observable and back off; successful work is drained without an idle delay.
// Retry scheduling and dead-letter state belong to the durable store.
func RunWorker(ctx context.Context, process ProcessNext, idle time.Duration, logger *slog.Logger) error {
	if ctx == nil || process == nil || idle <= 0 || logger == nil {
		return errors.New("invalid worker dependencies or interval")
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		worked, err := process(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			logger.ErrorContext(ctx, "worker_processing_failed", "code", "processing_failed")
		} else if worked {
			logger.InfoContext(ctx, "worker_event_processed")
			continue
		}
		timer := time.NewTimer(idle)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

// ObserveWorker samples durable queue state independently of processing, so a
// backlog or dead-letter row remains visible even when no healthy work remains.
func ObserveWorker(ctx context.Context, sample func(context.Context) error, interval time.Duration, logger *slog.Logger) error {
	if ctx == nil || sample == nil || interval <= 0 || logger == nil {
		return errors.New("invalid worker observer dependencies or interval")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return nil
		}
		if err := sample(ctx); err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "worker_observation_failed", "code", "queue_status_unavailable")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
