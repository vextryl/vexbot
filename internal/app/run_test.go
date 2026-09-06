package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/vextryl/vexbot/internal/session"
)

func TestNewLoggerWritesHumanReadableText(t *testing.T) {
	var output bytes.Buffer
	newLogger(&output).Info("recording stopped", slog.String("guild_id", "42"))

	for _, value := range []string{"level=INFO", "msg=\"recording stopped\"", "guild_id=42"} {
		if !strings.Contains(output.String(), value) {
			t.Fatalf("logger output %q does not contain %q", output.String(), value)
		}
	}
}

func TestShutdownClosesCommandIntakeBeforeFinalizingRecordings(t *testing.T) {
	var output bytes.Buffer
	events := make([]string, 0, 3)
	intake := &testCommandIntake{events: &events}
	recordings := &testRecordingFinalizer{events: &events, result: session.FinalizationResult{FinalizedSessions: 2}}
	cancelled := false

	result := shutdown(context.Background(), intake, func() {
		events = append(events, "cancel")
		cancelled = true
	}, recordings, newLogger(&output))

	if !cancelled || result.FinalizedSessions != 2 {
		t.Fatalf("shutdown() = %#v, cancelled = %v", result, cancelled)
	}
	if got, want := strings.Join(events, ","), "intake,cancel,recordings"; got != want {
		t.Fatalf("shutdown order = %q, want %q", got, want)
	}
	for _, value := range []string{"msg=\"shutdown recording summary\"", "finalized_sessions=2", "failed_sessions=0"} {
		if !strings.Contains(output.String(), value) {
			t.Fatalf("shutdown log %q does not contain %q", output.String(), value)
		}
	}
}

func TestShutdownLogsRecordingFinalizationFailure(t *testing.T) {
	var output bytes.Buffer
	failure := errors.New("timeline write failed")
	shutdown(context.Background(), &testCommandIntake{}, nil, &testRecordingFinalizer{
		result: session.FinalizationResult{FailedSessions: 1, Err: failure},
	}, newLogger(&output))

	for _, value := range []string{"failed_sessions=1", "msg=\"finalizing recordings during shutdown\"", "timeline write failed"} {
		if !strings.Contains(output.String(), value) {
			t.Fatalf("shutdown log %q does not contain %q", output.String(), value)
		}
	}
}

type testCommandIntake struct {
	events *[]string
}

func (c *testCommandIntake) Close(context.Context) {
	if c.events != nil {
		*c.events = append(*c.events, "intake")
	}
}

type testRecordingFinalizer struct {
	events *[]string
	result session.FinalizationResult
}

func (f *testRecordingFinalizer) Finalize(context.Context) session.FinalizationResult {
	if f.events != nil {
		*f.events = append(*f.events, "recordings")
	}
	return f.result
}
