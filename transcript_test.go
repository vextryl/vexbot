package main

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestBuildTranscriptLinesOrdersSuccessfulTurns(t *testing.T) {
	lines := buildTranscriptLines([]TurnTranscription{
		{
			Turn: transcriptionTurn{UserID: "alex", SessionStart: 5 * time.Second, SessionEnd: 7 * time.Second},
			Transcription: Transcription{Tokens: []TranscriptionToken{
				{Text: " Again"},
				{Text: "."},
			}},
		},
		{
			Turn:          transcriptionTurn{UserID: "sam", SessionStart: time.Second, SessionEnd: 2 * time.Second},
			Transcription: Transcription{Tokens: []TranscriptionToken{{Text: " Hello."}}},
		},
		{
			Turn: transcriptionTurn{UserID: "alex", SessionStart: 3 * time.Second, SessionEnd: 4 * time.Second},
			Err:  errors.New("Whisper failed"),
		},
	})

	want := []transcriptLine{
		{UserID: "sam", SessionStart: time.Second, SessionEnd: 2 * time.Second, Text: "Hello."},
		{UserID: "alex", SessionStart: 5 * time.Second, SessionEnd: 7 * time.Second, Text: "Again."},
	}
	if len(lines) != len(want) {
		t.Fatalf("line count = %d, want %d", len(lines), len(want))
	}
	for index := range want {
		if lines[index] != want[index] {
			t.Fatalf("line %d = %#v, want %#v", index, lines[index], want[index])
		}
	}
}

func TestWriteCombinedTranscript(t *testing.T) {
	dir := t.TempDir()
	path, lineCount, err := writeCombinedTranscript(dir, map[string]string{"42": "Mörk 🐉"}, []TurnTranscription{
		{
			Turn:          transcriptionTurn{UserID: "42", SessionStart: 62 * time.Second, SessionEnd: 63 * time.Second},
			Transcription: Transcription{Tokens: []TranscriptionToken{{Text: " Hello."}}},
		},
	})
	if err != nil {
		t.Fatalf("writeCombinedTranscript() error = %v", err)
	}
	if lineCount != 1 {
		t.Fatalf("line count = %d, want 1", lineCount)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(transcript.txt) error = %v", err)
	}
	if got, want := string(contents), "[01:02] Mörk 🐉: Hello.\n"; got != want {
		t.Fatalf("transcript = %q, want %q", got, want)
	}
}

func TestBuildTranscriptLinesFiltersIsolatedBlankAudio(t *testing.T) {
	lines := buildTranscriptLines([]TurnTranscription{
		turnResult("alex", time.Second, " First."),
		turnResult("sam", 2*time.Second, "[BLANK_AUDIO]"),
		turnResult("alex", 3*time.Second, " Second."),
		turnResult("sam", 4*time.Second, "[BLANK_AUDIO]"),
		turnResult("alex", 5*time.Second, "[blank_audio]"),
		turnResult("sam", 6*time.Second, " Third."),
	})

	want := []transcriptLine{
		{UserID: "alex", SessionStart: time.Second, SessionEnd: 2 * time.Second, Text: "First."},
		{UserID: "alex", SessionStart: 3 * time.Second, SessionEnd: 4 * time.Second, Text: "Second."},
		{UserID: "sam", SessionStart: 4 * time.Second, SessionEnd: 5 * time.Second, Text: "[unintelligible audio]"},
		{UserID: "alex", SessionStart: 5 * time.Second, SessionEnd: 6 * time.Second, Text: "[unintelligible audio]"},
		{UserID: "sam", SessionStart: 6 * time.Second, SessionEnd: 7 * time.Second, Text: "Third."},
	}
	if len(lines) != len(want) {
		t.Fatalf("line count = %d, want %d", len(lines), len(want))
	}
	for index := range want {
		if lines[index] != want[index] {
			t.Fatalf("line %d = %#v, want %#v", index, lines[index], want[index])
		}
	}
}

func turnResult(userID string, start time.Duration, text string) TurnTranscription {
	return TurnTranscription{
		Turn: transcriptionTurn{UserID: userID, SessionStart: start, SessionEnd: start + time.Second},
		Transcription: Transcription{Tokens: []TranscriptionToken{
			{Text: text},
		}},
	}
}
