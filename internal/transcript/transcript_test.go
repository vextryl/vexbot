package transcript

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vextryl/vexbot/internal/turn"
	"github.com/vextryl/vexbot/internal/whisper"
)

func TestBuildLinesOrdersAndFilters(t *testing.T) {
	lines := buildLines([]turn.Result{
		{Turn: turn.Turn{UserID: "alex", SessionStart: 5 * time.Second}, Transcription: whisper.Transcription{Tokens: []whisper.TranscriptionToken{{Text: " Again."}}}},
		{Turn: turn.Turn{UserID: "sam", SessionStart: time.Second}, Transcription: whisper.Transcription{Tokens: []whisper.TranscriptionToken{{Text: " Hello."}}}},
		{Turn: turn.Turn{UserID: "alex", SessionStart: 3 * time.Second}, Err: errors.New("failed")},
		{Turn: turn.Turn{UserID: "sam", SessionStart: 2 * time.Second}, Transcription: whisper.Transcription{Tokens: []whisper.TranscriptionToken{{Text: "[BLANK_AUDIO]"}}}},
	})
	if len(lines) != 2 || lines[0].UserID != "sam" || lines[1].Text != "Again." {
		t.Fatalf("lines = %#v", lines)
	}
}

func TestWriteUsesDisplayName(t *testing.T) {
	dir := t.TempDir()
	path, count, err := Write(dir, Metadata{}, map[string]string{"42": "Mörk 🐉"}, []turn.Result{{
		Turn:          turn.Turn{UserID: "42", SessionStart: 62 * time.Second},
		Transcription: whisper.Transcription{Tokens: []whisper.TranscriptionToken{{Text: " Hello."}}},
	}})
	if err != nil || count != 1 {
		t.Fatalf("Write() = %q, %d, %v", path, count, err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || !strings.HasSuffix(string(contents), "[01:02] Mörk 🐉: Hello.\n") {
		t.Fatalf("contents = %q, err = %v", contents, err)
	}
}

func TestWriteIncludesCompleteMetadataHeader(t *testing.T) {
	dir := t.TempDir()
	zone := time.FixedZone("CDT", -5*60*60)
	metadata := Metadata{
		StartedAt:    time.Date(2026, time.September, 6, 19, 30, 0, 0, zone),
		EndedAt:      time.Date(2026, time.September, 6, 22, 45, 2, 0, zone),
		VoiceChannel: "  D&D\nTable  ",
		Participants: []string{"Morgan", "Alex", "Morgan", " Bob "},
	}
	path, _, err := Write(dir, metadata, map[string]string{"42": "Alex"}, []turn.Result{{
		Turn:          turn.Turn{UserID: "42", SessionStart: time.Second},
		Transcription: whisper.Transcription{Tokens: []whisper.TranscriptionToken{{Text: " Hello."}}},
	}})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := "VexBot transcript\n" +
		"Recording started: 2026-09-06 19:30:00 CDT\n" +
		"Recording ended: 2026-09-06 22:45:02 CDT\n" +
		"Voice channel: D&D Table\n" +
		"Participants: Alex, Bob, Morgan\n\n" +
		"[00:01] Alex: Hello.\n"
	if got := string(contents); got != want {
		t.Fatalf("transcript = %q, want %q", got, want)
	}
}

func TestWriteUsesUnknownMetadataWhenDetailsAreUnavailable(t *testing.T) {
	path, _, err := Write(t.TempDir(), Metadata{}, nil, nil)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := "VexBot transcript\nRecording started: Unknown\nRecording ended: Unknown\nVoice channel: Unknown\nParticipants: Unknown\n\n"
	if got := string(contents); got != want {
		t.Fatalf("transcript = %q, want %q", got, want)
	}
}
