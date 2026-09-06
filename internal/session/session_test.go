package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/vextryl/vexbot/internal/audio"
	"github.com/vextryl/vexbot/internal/wav"
)

func TestManagerReservePreventsDuplicateReservations(t *testing.T) {
	manager := NewManager(nil, nil)
	guildID := snowflake.ID(42)
	voiceChannelID := snowflake.ID(43)

	if !manager.Reserve(guildID, voiceChannelID) {
		t.Fatal("first Reserve() = false, want true")
	}
	if got, ok := manager.VoiceChannelID(guildID); !ok || got != voiceChannelID {
		t.Fatalf("VoiceChannelID() = %v, %v, want %v, true", got, ok, voiceChannelID)
	}
	if manager.Reserve(guildID, voiceChannelID) {
		t.Fatal("second Reserve() = true, want false")
	}

	manager.CancelReservation(guildID)
	if !manager.Reserve(guildID, voiceChannelID) {
		t.Fatal("Reserve() after CancelReservation() = false, want true")
	}
}

func TestManagerStartConsumesReservationAndKeepsSessionActive(t *testing.T) {
	manager := NewManager(nil, nil)
	guildID := snowflake.ID(42)
	voiceChannelID := snowflake.ID(43)

	if !manager.Reserve(guildID, voiceChannelID) {
		t.Fatal("Reserve() = false, want true")
	}
	if !manager.Start(&Session{guildID: guildID, voiceChannelID: voiceChannelID}) {
		t.Fatal("Start() = false, want true")
	}

	if _, ok := manager.starting[guildID]; ok {
		t.Fatal("Start() left guild reservation in place")
	}
	if _, ok := manager.sessions[guildID]; !ok {
		t.Fatal("Start() did not register active session")
	}
	if got, ok := manager.VoiceChannelID(guildID); !ok || got != voiceChannelID {
		t.Fatalf("VoiceChannelID() = %v, %v, want %v, true", got, ok, voiceChannelID)
	}
	if manager.Reserve(guildID, voiceChannelID) {
		t.Fatal("Reserve() with active session = true, want false")
	}

	manager.CancelReservation(guildID)
	if manager.Reserve(guildID, voiceChannelID) {
		t.Fatal("CancelReservation() removed active session")
	}
}

func TestManagerStopPreservesJoinTranscriptChannel(t *testing.T) {
	const (
		guildID             = snowflake.ID(42)
		ownerID             = snowflake.ID(99)
		voiceChannelID      = snowflake.ID(100)
		transcriptChannelID = snowflake.ID(123)
	)

	recorder := &testRecordingSink{directory: "recordings/session"}
	startedAt := time.Date(2026, time.September, 6, 19, 30, 0, 0, time.UTC)
	active := &Session{
		startedAt:           startedAt,
		guildID:             guildID,
		ownerID:             ownerID,
		voiceChannelID:      voiceChannelID,
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
	if got := stopped.VoiceChannelID; got != voiceChannelID {
		t.Fatalf("VoiceChannelID = %v, want %v", got, voiceChannelID)
	}
	if got := stopped.TranscriptMetadata.StartedAt; !got.Equal(startedAt) {
		t.Fatalf("transcript metadata start = %v, want %v", got, startedAt)
	}
	if stopped.TranscriptMetadata.EndedAt.IsZero() {
		t.Fatal("transcript metadata end time is zero")
	}
	if !recorder.closed {
		t.Fatal("Stop() did not close recording sink")
	}
}

func TestManagerFinalizeStopsAllSessionsAndRejectsNewOnes(t *testing.T) {
	manager := NewManager(nil, nil)
	first := &testRecordingSink{directory: "recordings/first"}
	second := &testRecordingSink{directory: "recordings/second"}
	if !manager.Start(newTestSession(42, first)) || !manager.Start(newTestSession(43, second)) {
		t.Fatal("Start() = false, want active sessions")
	}

	result := manager.Finalize(context.Background())
	if result.FinalizedSessions != 2 || result.FailedSessions != 0 || result.Err != nil {
		t.Fatalf("Finalize() = %#v, want two successful finalizations", result)
	}
	if !first.closed || !second.closed {
		t.Fatal("Finalize() did not close every recording sink")
	}
	if manager.Reserve(44, 45) {
		t.Fatal("Reserve() succeeded after Finalize()")
	}
	if manager.Start(newTestSession(46, &testRecordingSink{})) {
		t.Fatal("Start() succeeded after Finalize()")
	}
}

func TestManagerFinalizeReportsRecordingFailure(t *testing.T) {
	manager := NewManager(nil, nil)
	failure := errors.New("write timeline")
	if !manager.Start(newTestSession(42, &testRecordingSink{closeErr: failure})) {
		t.Fatal("Start() = false, want true")
	}

	result := manager.Finalize(context.Background())
	if result.FinalizedSessions != 0 || result.FailedSessions != 1 || !errors.Is(result.Err, failure) {
		t.Fatalf("Finalize() = %#v, want one wrapped recording failure", result)
	}
}

func newTestSession(guildID snowflake.ID, recorder *testRecordingSink) *Session {
	return &Session{
		guildID:  guildID,
		recorder: recorder,
		buffer:   audio.NewSegmentBuffer(recorder, nil),
	}
}

type testRecordingSink struct {
	directory string
	closed    bool
	closeErr  error
}

func (s *testRecordingSink) ConsumeChunk(audio.Chunk) {}

func (s *testRecordingSink) Close() error {
	s.closed = true
	return s.closeErr
}

func (s *testRecordingSink) Directory() string { return s.directory }

func (s *testRecordingSink) Files() []wav.File { return nil }
