package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

type recordingTranscriber struct {
	paths        []string
	previousPath string
}

func (t *recordingTranscriber) Transcribe(_ context.Context, recording RecordingFile) (Transcription, error) {
	if t.previousPath != "" {
		if _, err := os.Stat(t.previousPath); !os.IsNotExist(err) {
			return Transcription{}, fmt.Errorf("previous temporary WAV still exists: %w", err)
		}
	}
	contents, err := os.ReadFile(recording.Path)
	if err != nil {
		return Transcription{}, err
	}
	t.paths = append(t.paths, recording.Path)
	t.previousPath = recording.Path
	return Transcription{
		TextPath: recording.Path + ".txt",
		JSONPath: recording.Path + ".json",
		Tokens: []TranscriptionToken{{
			Text:     string(contents[wavHeaderSize:]),
			WAVStart: 0,
			WAVEnd:   time.Millisecond,
		}},
	}, nil
}

func TestTranscribeRecordingTurnsUsesTemporaryTurnWAVs(t *testing.T) {
	dir := t.TempDir()
	writeTestPCM16WAV(t, filepath.Join(dir, "42.wav"), 1000, 1, []int16{10, 20, 30, 40, 50, 60})
	timelineContents, err := json.Marshal(sessionTimeline{
		Version: 1,
		Spans: []timelineSpan{
			{UserID: "42", SessionStartMS: 100, SessionEndMS: 102, WAVStartMS: 0, WAVEndMS: 2},
			{UserID: "42", SessionStartMS: 3000, SessionEndMS: 3003, WAVStartMS: 2, WAVEndMS: 5},
		},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, timelineFileName), timelineContents, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	transcriber := &recordingTranscriber{}
	started := 0
	progress := make([]int, 0)
	results, err := transcribeRecordingTurns(context.Background(), dir, []RecordingFile{{
		UserID: snowflake.ID(42),
		Path:   filepath.Join(dir, "42.wav"),
	}}, transcriber, func(total int) {
		started = total
	}, func(completed, _ int, _ TurnTranscription) {
		progress = append(progress, completed)
	})
	if err != nil {
		t.Fatalf("transcribeRecordingTurns() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("result count = %d, want 2", len(results))
	}
	if started != 2 {
		t.Fatalf("started total = %d, want 2", started)
	}
	if got, want := fmt.Sprint(progress), "[1 2]"; got != want {
		t.Fatalf("progress = %s, want %s", got, want)
	}
	if got := results[0].Turn; got.SessionStart != 100*time.Millisecond ||
		got.SessionEnd != 102*time.Millisecond || got.WAVStart != 0 || got.WAVEnd != 2*time.Millisecond {
		t.Fatalf("turn = %#v", got)
	}
	if got := len(transcriber.paths); got != 2 {
		t.Fatalf("transcriber call count = %d, want 2", got)
	}
	for _, path := range transcriber.paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("temporary WAV still exists after transcription: %v", err)
		}
	}
	if matches, err := filepath.Glob(filepath.Join(dir, ".transcription-tmp-*")); err != nil || len(matches) != 0 {
		t.Fatalf("temporary directory matches = %v, error = %v", matches, err)
	}
	for _, result := range results {
		if result.Transcription.TextPath != "" || result.Transcription.JSONPath != "" {
			t.Fatalf("temporary output paths = %#v", result.Transcription)
		}
	}
}
