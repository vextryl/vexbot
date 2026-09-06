package dave

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

func TestDAVEDecryptLogHandlerRateLimitsOnlyDAVEFailures(t *testing.T) {
	var output bytes.Buffer
	handler := &daveDecryptLogHandler{
		next:    slog.NewTextHandler(&output, nil),
		tracker: &daveDecryptLogTracker{},
	}
	now := time.Date(2026, time.September, 1, 18, 0, 0, 0, time.UTC)

	for range 3 {
		record := slog.NewRecord(now, slog.LevelError, "error while reading packet", 0)
		record.AddAttrs(slog.Any("err", errors.New("failed to DAVE decrypt packet: failed to decrypt frame")))
		if err := handler.Handle(context.Background(), record); err != nil {
			t.Fatalf("Handle() error = %v", err)
		}
	}

	other := slog.NewRecord(now, slog.LevelError, "another voice error", 0)
	other.AddAttrs(slog.Any("err", errors.New("network disconnected")))
	if err := handler.Handle(context.Background(), other); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	got := output.String()
	if strings.Count(got, "DAVE decrypt failures dropped") != 1 ||
		!strings.Contains(got, "count=1") {
		t.Fatalf("summary output = %q", got)
	}
	if !strings.Contains(got, "another voice error") {
		t.Fatalf("non-DAVE error was not logged: %q", got)
	}
}

func TestDAVEDecryptLogHandlerReportsAccumulatedFailures(t *testing.T) {
	tracker := &daveDecryptLogTracker{}
	now := time.Now()
	if count, report := tracker.record(now); !report || count != 1 {
		t.Fatalf("first record = (%d, %v), want (1, true)", count, report)
	}
	if count, report := tracker.record(now.Add(time.Second)); report || count != 0 {
		t.Fatalf("second record = (%d, %v), want (0, false)", count, report)
	}
	if count, report := tracker.record(now.Add(daveDecryptLogInterval)); !report || count != 2 {
		t.Fatalf("third record = (%d, %v), want (2, true)", count, report)
	}
}

func TestDAVEDecryptLogHandlerCountsEveryRateLimitedFailure(t *testing.T) {
	counter := NewDecryptFailureCounter()
	const guildID = snowflake.ID(42)
	counter.Reset(guildID)
	handler := &daveDecryptLogHandler{
		next:    slog.NewTextHandler(&bytes.Buffer{}, nil),
		tracker: &daveDecryptLogTracker{},
		onFailure: func() {
			counter.Record(guildID)
		},
	}

	for range 3 {
		record := slog.NewRecord(time.Now(), slog.LevelError, "error while reading packet", 0)
		record.AddAttrs(slog.Any("err", errors.New("failed to DAVE decrypt packet: failed to decrypt frame")))
		if err := handler.Handle(context.Background(), record); err != nil {
			t.Fatalf("Handle() error = %v", err)
		}
	}

	if got := counter.Take(guildID); got != 3 {
		t.Fatalf("counted failures = %d, want 3", got)
	}
}
