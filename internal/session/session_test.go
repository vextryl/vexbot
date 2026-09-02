package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/disgoorg/snowflake/v2"
	"github.com/vextryl/vexbot/internal/audio"
	"github.com/vextryl/vexbot/internal/turn"
	"github.com/vextryl/vexbot/internal/wav"
	"github.com/vextryl/vexbot/internal/whisper"
)

func TestManagerReservePreventsDuplicateReservations(t *testing.T) {
	manager := NewManager(nil, nil)
	guildID := snowflake.ID(42)

	if !manager.Reserve(guildID) {
		t.Fatal("first Reserve() = false, want true")
	}
	if manager.Reserve(guildID) {
		t.Fatal("second Reserve() = true, want false")
	}

	manager.CancelReservation(guildID)
	if !manager.Reserve(guildID) {
		t.Fatal("Reserve() after CancelReservation() = false, want true")
	}
}

func TestManagerStartConsumesReservationAndKeepsSessionActive(t *testing.T) {
	manager := NewManager(nil, nil)
	guildID := snowflake.ID(42)

	if !manager.Reserve(guildID) {
		t.Fatal("Reserve() = false, want true")
	}
	manager.Start(&Session{guildID: guildID})

	if _, ok := manager.starting[guildID]; ok {
		t.Fatal("Start() left guild reservation in place")
	}
	if _, ok := manager.sessions[guildID]; !ok {
		t.Fatal("Start() did not register active session")
	}
	if manager.Reserve(guildID) {
		t.Fatal("Reserve() with active session = true, want false")
	}

	manager.CancelReservation(guildID)
	if manager.Reserve(guildID) {
		t.Fatal("CancelReservation() removed active session")
	}
}

func TestManagerStopPreservesJoinTranscriptChannel(t *testing.T) {
	const (
		guildID             = snowflake.ID(42)
		ownerID             = snowflake.ID(99)
		transcriptChannelID = snowflake.ID(123)
	)

	recorder := &testRecordingSink{directory: "recordings/session"}
	active := &Session{
		guildID:             guildID,
		ownerID:             ownerID,
		transcriptChannelID: transcriptChannelID,
		recorder:            recorder,
		buffer:              audio.NewSegmentBuffer(recorder, nil),
	}
	manager := NewManager(nil, nil)
	manager.Start(active)

	stopped, err := manager.Stop(context.Background(), guildID, ownerID)
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if got := stopped.TranscriptChannelID; got != transcriptChannelID {
		t.Fatalf("TranscriptChannelID = %v, want %v", got, transcriptChannelID)
	}
	if !recorder.closed {
		t.Fatal("Stop() did not close recording sink")
	}
}

func TestStartTranscriptionReportsSuccess(t *testing.T) {
	directory := t.TempDir()
	manager := NewManager(testTranscriber{}, nil)
	manager.transcribe = func(
		_ context.Context,
		_ string,
		_ []wav.File,
		_ whisper.Transcriber,
		_ func(int),
		_ func(int, int, turn.Result),
	) ([]turn.Result, error) {
		return []turn.Result{{
			Turn: turn.Turn{UserID: "42"},
			Transcription: whisper.Transcription{Tokens: []whisper.TranscriptionToken{{
				Text: " Hello.",
			}}},
		}}, nil
	}

	completion, started := manager.StartTranscription(StoppedRecording{
		Directory:    directory,
		Files:        []wav.File{{}},
		DisplayNames: map[string]string{"42": "Alex"},
	})
	if !started {
		t.Fatal("StartTranscription() started = false, want true")
	}

	result := <-completion
	if result.Err != nil {
		t.Fatalf("completion error = %v", result.Err)
	}
	if want := filepath.Join(directory, "transcript.txt"); result.TranscriptPath != want {
		t.Fatalf("TranscriptPath = %q, want %q", result.TranscriptPath, want)
	}
	if result.LineCount != 1 {
		t.Fatalf("LineCount = %d, want 1", result.LineCount)
	}
	if contents, err := os.ReadFile(result.TranscriptPath); err != nil || string(contents) != "[00:00] Alex: Hello.\n" {
		t.Fatalf("transcript contents = %q, error = %v", contents, err)
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

	completion, started := manager.StartTranscription(StoppedRecording{
		Directory: t.TempDir(),
		Files:     []wav.File{{}},
	})
	if !started {
		t.Fatal("StartTranscription() started = false, want true")
	}

	result := <-completion
	if !errors.Is(result.Err, want) {
		t.Fatalf("completion error = %v, want wrapped %v", result.Err, want)
	}
	if result.TranscriptPath != "" || result.LineCount != 0 {
		t.Fatalf("failure result = %#v, want no transcript metadata", result)
	}
}

func TestStartTranscriptionRejectsUnavailableTranscriber(t *testing.T) {
	completion, started := NewManager(nil, nil).StartTranscription(StoppedRecording{
		Files: []wav.File{{}},
	})
	if started || completion != nil {
		t.Fatalf("StartTranscription() = %v, %v, want nil, false", completion, started)
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

	completion, started := manager.StartTranscription(StoppedRecording{
		Directory: t.TempDir(),
		Files:     []wav.File{{}},
	})
	if !started {
		t.Fatal("StartTranscription() started = false, want true")
	}
	<-runnerStarted
	select {
	case result := <-completion:
		t.Fatalf("completion arrived before runner released: %#v", result)
	default:
	}

	close(release)
	if result := <-completion; result.Err != nil {
		t.Fatalf("completion error = %v", result.Err)
	}
}

type testRecordingSink struct {
	directory string
	closed    bool
}

func (s *testRecordingSink) ConsumeChunk(audio.Chunk) {}

func (s *testRecordingSink) Close() error {
	s.closed = true
	return nil
}

func (s *testRecordingSink) Directory() string { return s.directory }

func (s *testRecordingSink) Files() []wav.File { return nil }

type testTranscriber struct{}

func (testTranscriber) Transcribe(context.Context, wav.File) (whisper.Transcription, error) {
	return whisper.Transcription{}, nil
}
