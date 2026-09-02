package vexbot

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const daveDecryptLogInterval = 30 * time.Second

type daveDecryptLogHandler struct {
	next    slog.Handler
	tracker *daveDecryptLogTracker
}

type daveDecryptLogTracker struct {
	mu           sync.Mutex
	lastReported time.Time
	pending      int
}

func newDAVEDecryptRateLimitedLogger(logger *slog.Logger) *slog.Logger {
	return slog.New(&daveDecryptLogHandler{
		next:    logger.Handler(),
		tracker: &daveDecryptLogTracker{},
	})
}

func (h *daveDecryptLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *daveDecryptLogHandler) Handle(ctx context.Context, record slog.Record) error {
	if !isDAVEDecryptFailure(record) {
		return h.next.Handle(ctx, record)
	}

	if count, report := h.tracker.record(record.Time); report {
		summary := slog.NewRecord(
			record.Time,
			slog.LevelWarn,
			"DAVE decrypt failures dropped",
			0,
		)
		summary.AddAttrs(
			slog.Int("count", count),
			slog.Duration("window", daveDecryptLogInterval),
		)
		return h.next.Handle(ctx, summary)
	}

	return nil
}

func (h *daveDecryptLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &daveDecryptLogHandler{
		next:    h.next.WithAttrs(attrs),
		tracker: h.tracker,
	}
}

func (h *daveDecryptLogHandler) WithGroup(name string) slog.Handler {
	return &daveDecryptLogHandler{
		next:    h.next.WithGroup(name),
		tracker: h.tracker,
	}
}

func (t *daveDecryptLogTracker) record(now time.Time) (int, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.pending++
	if !t.lastReported.IsZero() && now.Sub(t.lastReported) < daveDecryptLogInterval {
		return 0, false
	}

	count := t.pending
	t.pending = 0
	t.lastReported = now
	return count, true
}

func isDAVEDecryptFailure(record slog.Record) bool {
	matched := false
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key != "err" {
			return true
		}

		err, ok := attr.Value.Any().(error)
		matched = ok && strings.Contains(
			err.Error(),
			"failed to DAVE decrypt packet",
		)
		return !matched
	})

	return matched
}
