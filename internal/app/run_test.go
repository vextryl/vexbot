package app

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
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
