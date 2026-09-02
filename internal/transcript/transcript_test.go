package transcript

import (
	"errors"
	"os"
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
	path, count, err := Write(dir, map[string]string{"42": "Mörk 🐉"}, []turn.Result{{
		Turn:          turn.Turn{UserID: "42", SessionStart: 62 * time.Second},
		Transcription: whisper.Transcription{Tokens: []whisper.TranscriptionToken{{Text: " Hello."}}},
	}})
	if err != nil || count != 1 {
		t.Fatalf("Write() = %q, %d, %v", path, count, err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "[01:02] Mörk 🐉: Hello.\n" {
		t.Fatalf("contents = %q, err = %v", contents, err)
	}
}
