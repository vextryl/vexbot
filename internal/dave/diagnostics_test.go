package dave

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/disgoorg/snowflake/v2"
)

func TestDecryptFailureCounterReportsNoFailures(t *testing.T) {
	counter := NewDecryptFailureCounter()
	counter.Reset(42)

	var output bytes.Buffer
	LogRecordingSummary(slog.New(slog.NewTextHandler(&output, nil)), counter, 42, "recordings/first")

	for _, value := range []string{"guild_id=42", "directory=recordings/first", "dropped_failure_count=0", "failures_occurred=false"} {
		if !strings.Contains(output.String(), value) {
			t.Fatalf("summary %q does not contain %q", output.String(), value)
		}
	}
}

func TestDecryptFailureCounterCountsRepeatedFailuresForOneRecording(t *testing.T) {
	counter := NewDecryptFailureCounter()
	const guildID = snowflake.ID(42)
	counter.Reset(guildID)
	for range 3 {
		counter.Record(guildID)
	}

	if got := counter.Take(guildID); got != 3 {
		t.Fatalf("Take() = %d, want 3", got)
	}
}

func TestDecryptFailureCounterKeepsRecordingsSeparate(t *testing.T) {
	counter := NewDecryptFailureCounter()
	firstGuild := snowflake.ID(42)
	secondGuild := snowflake.ID(43)
	counter.Reset(firstGuild)
	counter.Reset(secondGuild)
	counter.Record(firstGuild)
	counter.Record(firstGuild)
	counter.Record(secondGuild)

	if got := counter.Take(firstGuild); got != 2 {
		t.Fatalf("first Take() = %d, want 2", got)
	}
	if got := counter.Take(secondGuild); got != 1 {
		t.Fatalf("second Take() = %d, want 1", got)
	}
}

func TestDecryptFailureCounterSummaryResetsRecording(t *testing.T) {
	counter := NewDecryptFailureCounter()
	const guildID = snowflake.ID(42)
	counter.Reset(guildID)
	counter.Record(guildID)

	if got := counter.Take(guildID); got != 1 {
		t.Fatalf("first Take() = %d, want 1", got)
	}
	counter.Reset(guildID)
	if got := counter.Take(guildID); got != 0 {
		t.Fatalf("second recording Take() = %d, want 0", got)
	}
}
