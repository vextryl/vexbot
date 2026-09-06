package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/vextryl/vexbot/internal/turn"
	"github.com/vextryl/vexbot/internal/wav"
	"github.com/vextryl/vexbot/internal/whisper"
)

func TestStartTranscriptionReportsOrderedProgressAndCompletion(t *testing.T) {
	directory := t.TempDir()
	manager := NewManager(testTranscriber{}, nil)
	manager.transcribe = func(
		_ context.Context,
		_ string,
		_ []wav.File,
		_ whisper.Transcriber,
		onStart func(int),
		onProgress func(int, int, turn.Result),
	) ([]turn.Result, error) {
		onStart(3)
		results := make([]turn.Result, 0, 3)
		for completed := 1; completed <= 3; completed++ {
			result := turn.Result{
				Turn: turn.Turn{UserID: "42"},
				Transcription: whisper.Transcription{Tokens: []whisper.TranscriptionToken{{
					Text: " Hello.",
				}}},
			}
			results = append(results, result)
			onProgress(completed, 3, result)
		}
		return results, nil
	}

	job, started := manager.StartTranscription(StoppedRecording{
		Directory:    directory,
		Files:        []wav.File{{}},
		DisplayNames: map[string]string{"42": "Alex"},
	})
	if !started {
		t.Fatal("StartTranscription() started = false, want true")
	}

	result := <-job.Completion
	if result.Err != nil {
		t.Fatalf("completion error = %v", result.Err)
	}
	if want := filepath.Join(directory, "transcript.txt"); result.TranscriptPath != want {
		t.Fatalf("TranscriptPath = %q, want %q", result.TranscriptPath, want)
	}
	if result.LineCount != 3 {
		t.Fatalf("LineCount = %d, want 3", result.LineCount)
	}
	if contents, err := os.ReadFile(result.TranscriptPath); err != nil || string(contents) != "[00:00] Alex: Hello.\n[00:00] Alex: Hello.\n[00:00] Alex: Hello.\n" {
		t.Fatalf("transcript contents = %q, error = %v", contents, err)
	}

	var progress []TranscriptionProgress
	for update := range job.Progress {
		progress = append(progress, update)
	}
	want := []TranscriptionProgress{
		{Phase: TranscriptionPhasePreparing},
		{Phase: TranscriptionPhaseTranscribing, TotalTurns: 3},
		{Phase: TranscriptionPhaseTranscribing, CompletedTurns: 1, TotalTurns: 3, Percent: 30},
		{Phase: TranscriptionPhaseTranscribing, CompletedTurns: 2, TotalTurns: 3, Percent: 60},
		{Phase: TranscriptionPhaseTranscribing, CompletedTurns: 3, TotalTurns: 3, Percent: 90},
		{Phase: TranscriptionPhaseCombining, CompletedTurns: 3, TotalTurns: 3, Percent: 95},
		{Phase: TranscriptionPhaseComplete, CompletedTurns: 3, TotalTurns: 3, Percent: 100},
	}
	if len(progress) != len(want) {
		t.Fatalf("progress updates = %#v, want %#v", progress, want)
	}
	for index := range want {
		if progress[index] != want[index] {
			t.Fatalf("progress[%d] = %#v, want %#v", index, progress[index], want[index])
		}
	}
}

func TestStartTranscriptionReportsFailure(t *testing.T) {
	want := errors.New("Whisper failed")
	manager := NewManager(testTranscriber{}, nil)
	manager.transcribe = func(
		_ context.Context,
		_ string,
		_ []wav.File,
		_ whisper.Transcriber,
		_ func(int),
		_ func(int, int, turn.Result),
	) ([]turn.Result, error) {
		return nil, want
	}

	job, started := manager.StartTranscription(StoppedRecording{Directory: t.TempDir(), Files: []wav.File{{}}})
	if !started {
		t.Fatal("StartTranscription() started = false, want true")
	}

	result := <-job.Completion
	if !errors.Is(result.Err, want) {
		t.Fatalf("completion error = %v, want wrapped %v", result.Err, want)
	}
	if result.TranscriptPath != "" || result.LineCount != 0 {
		t.Fatalf("failure result = %#v, want no transcript metadata", result)
	}
	progress := drainProgress(job.Progress)
	if got := progress[len(progress)-1].Phase; got != TranscriptionPhaseFailed {
		t.Fatalf("final progress phase = %q, want %q", got, TranscriptionPhaseFailed)
	}
}

func TestStartTranscriptionReportsZeroTurnCompletion(t *testing.T) {
	manager := NewManager(testTranscriber{}, nil)
	manager.transcribe = func(
		_ context.Context,
		_ string,
		_ []wav.File,
		_ whisper.Transcriber,
		onStart func(int),
		_ func(int, int, turn.Result),
	) ([]turn.Result, error) {
		onStart(0)
		return nil, nil
	}

	job, started := manager.StartTranscription(StoppedRecording{Directory: t.TempDir(), Files: []wav.File{{}}})
	if !started {
		t.Fatal("StartTranscription() started = false, want true")
	}
	if result := <-job.Completion; result.Err != nil || result.LineCount != 0 {
		t.Fatalf("completion = %#v, want empty successful transcript", result)
	}
	progress := drainProgress(job.Progress)
	if got := progress[len(progress)-1]; got.Phase != TranscriptionPhaseComplete || got.Percent != 100 {
		t.Fatalf("final progress = %#v, want complete at 100%%", got)
	}
}

func TestStartTranscriptionRejectsUnavailableTranscriber(t *testing.T) {
	job, started := NewManager(nil, nil).StartTranscription(StoppedRecording{Files: []wav.File{{}}})
	if started || job.Progress != nil || job.Completion != nil {
		t.Fatalf("StartTranscription() = %#v, %v, want empty job, false", job, started)
	}
}

func TestStartTranscriptionRunsAsynchronously(t *testing.T) {
	release := make(chan struct{})
	runnerStarted := make(chan struct{})
	manager := NewManager(testTranscriber{}, nil)
	manager.transcribe = func(
		_ context.Context,
		_ string,
		_ []wav.File,
		_ whisper.Transcriber,
		_ func(int),
		_ func(int, int, turn.Result),
	) ([]turn.Result, error) {
		close(runnerStarted)
		<-release
		return nil, nil
	}

	job, started := manager.StartTranscription(StoppedRecording{Directory: t.TempDir(), Files: []wav.File{{}}})
	if !started {
		t.Fatal("StartTranscription() started = false, want true")
	}
	<-runnerStarted
	select {
	case result := <-job.Completion:
		t.Fatalf("completion arrived before runner released: %#v", result)
	default:
	}

	close(release)
	if result := <-job.Completion; result.Err != nil {
		t.Fatalf("completion error = %v", result.Err)
	}
}

func TestStartTranscriptionProtectsDirectoryUntilCompletion(t *testing.T) {
	directory := t.TempDir()
	release := make(chan struct{})
	runnerStarted := make(chan struct{})
	manager := NewManager(testTranscriber{}, nil)
	manager.transcribe = func(
		_ context.Context,
		_ string,
		_ []wav.File,
		_ whisper.Transcriber,
		_ func(int),
		_ func(int, int, turn.Result),
	) ([]turn.Result, error) {
		close(runnerStarted)
		<-release
		return nil, nil
	}

	job, started := manager.StartTranscription(StoppedRecording{Directory: directory, Files: []wav.File{{}}})
	if !started {
		t.Fatal("StartTranscription() started = false, want true")
	}
	<-runnerStarted
	if _, ok := manager.TranscribingDirectories()[directory]; !ok {
		t.Fatal("TranscribingDirectories() did not include active transcription")
	}

	close(release)
	if result := <-job.Completion; result.Err != nil {
		t.Fatalf("completion error = %v", result.Err)
	}
	if _, ok := manager.TranscribingDirectories()[directory]; ok {
		t.Fatal("TranscribingDirectories() retained completed transcription")
	}
}

func drainProgress(progress <-chan TranscriptionProgress) []TranscriptionProgress {
	var updates []TranscriptionProgress
	for update := range progress {
		updates = append(updates, update)
	}
	return updates
}

type testTranscriber struct{}

func (testTranscriber) Transcribe(context.Context, wav.File) (whisper.Transcription, error) {
	return whisper.Transcription{}, nil
}
