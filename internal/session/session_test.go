package session

import (
	"context"
	"testing"

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
	manager.Start(&Session{guildID: guildID, voiceChannelID: voiceChannelID})

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
	active := &Session{
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
	if !recorder.closed {
		t.Fatal("Stop() did not close recording sink")
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
