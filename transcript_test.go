package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildTranscriptLinesRestoresSessionOrder(t *testing.T) {
	timeline := sessionTimeline{
		Version: 1,
		Spans: []timelineSpan{
			{UserID: "alex", SessionStartMS: 1000, SessionEndMS: 2000, WAVStartMS: 0, WAVEndMS: 1000},
			{UserID: "sam", SessionStartMS: 2500, SessionEndMS: 3500, WAVStartMS: 0, WAVEndMS: 1000},
			{UserID: "alex", SessionStartMS: 5000, SessionEndMS: 6000, WAVStartMS: 1000, WAVEndMS: 2000},
		},
	}
	transcriptions := map[string]Transcription{
		"alex": {Tokens: []TranscriptionToken{
			{WAVStart: 0, WAVEnd: 400 * time.Millisecond, Text: " Hello"},
			{WAVStart: 400 * time.Millisecond, WAVEnd: 900 * time.Millisecond, Text: " there."},
			{WAVStart: time.Second, WAVEnd: 1500 * time.Millisecond, Text: " Again."},
		}},
		"sam": {Tokens: []TranscriptionToken{
			{WAVStart: 0, WAVEnd: 600 * time.Millisecond, Text: " Hi."},
		}},
	}

	lines, err := buildTranscriptLines(timeline, transcriptions)
	if err != nil {
		t.Fatalf("buildTranscriptLines() error = %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("line count = %d, want 3", len(lines))
	}
	want := []transcriptLine{
		{UserID: "alex", SessionStart: time.Second, SessionEnd: 2 * time.Second, Text: "Hello there."},
		{UserID: "sam", SessionStart: 2500 * time.Millisecond, SessionEnd: 3500 * time.Millisecond, Text: "Hi."},
		{UserID: "alex", SessionStart: 5 * time.Second, SessionEnd: 6 * time.Second, Text: "Again."},
	}
	for index := range want {
		if lines[index] != want[index] {
			t.Fatalf("line %d = %#v, want %#v", index, lines[index], want[index])
		}
	}
}

func TestMergeTranscriptContinuations(t *testing.T) {
	lines := mergeTranscriptContinuations([]transcriptLine{
		{UserID: "alex", SessionStart: time.Second, SessionEnd: 2 * time.Second, Text: "Hate that that"},
		{UserID: "sam", SessionStart: 1500 * time.Millisecond, SessionEnd: 2 * time.Second, Text: "Overlapping reply."},
		{UserID: "alex", SessionStart: 3 * time.Second, SessionEnd: 4 * time.Second, Text: "'s on the test"},
		{UserID: "alex", SessionStart: 4500 * time.Millisecond, SessionEnd: 5 * time.Second, Text: "."},
		{UserID: "alex", SessionStart: 6 * time.Second, SessionEnd: 7 * time.Second, Text: "A new sentence."},
	})

	want := []transcriptLine{
		{UserID: "alex", SessionStart: time.Second, SessionEnd: 5 * time.Second, Text: "Hate that that's on the test."},
		{UserID: "sam", SessionStart: 1500 * time.Millisecond, SessionEnd: 2 * time.Second, Text: "Overlapping reply."},
		{UserID: "alex", SessionStart: 6 * time.Second, SessionEnd: 7 * time.Second, Text: "A new sentence."},
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

func TestMergeTranscriptContinuationsMovesLeadingPunctuation(t *testing.T) {
	lines := mergeTranscriptContinuations([]transcriptLine{
		{UserID: "alex", SessionStart: time.Second, SessionEnd: 2 * time.Second, Text: "Hello"},
		{UserID: "alex", SessionStart: 10 * time.Second, SessionEnd: 11 * time.Second, Text: "? How are you?"},
		{UserID: "alex", SessionStart: 20 * time.Second, SessionEnd: 21 * time.Second, Text: "The door"},
		{UserID: "alex", SessionStart: 22 * time.Second, SessionEnd: 23 * time.Second, Text: "."},
	})

	want := []transcriptLine{
		{UserID: "alex", SessionStart: time.Second, SessionEnd: 11 * time.Second, Text: "Hello?"},
		{UserID: "alex", SessionStart: 10 * time.Second, SessionEnd: 11 * time.Second, Text: "How are you?"},
		{UserID: "alex", SessionStart: 20 * time.Second, SessionEnd: 23 * time.Second, Text: "The door."},
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
	if err := os.WriteFile(filepath.Join(dir, timelineFileName), []byte(`{
  "version": 1,
  "spans": [
    {"user_id":"42","session_start_ms":62000,"session_end_ms":63000,"wav_start_ms":0,"wav_end_ms":1000}
  ]
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(timeline.json) error = %v", err)
	}

	path, lineCount, err := writeCombinedTranscript(dir, map[string]Transcription{
		"42": {Tokens: []TranscriptionToken{{Text: " Hello.", WAVStart: 0, WAVEnd: time.Second}}},
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
	if got, want := string(contents), "[01:02] 42: Hello.\n"; got != want {
		t.Fatalf("transcript = %q, want %q", got, want)
	}
}
