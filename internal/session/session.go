// Package session manages active voice recording sessions and post-recording
// transcription work.
package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
	"github.com/vextryl/vexbot/internal/audio"
	"github.com/vextryl/vexbot/internal/transcript"
	"github.com/vextryl/vexbot/internal/wav"
	"github.com/vextryl/vexbot/internal/whisper"
)

type Session struct {
	mu                  sync.Mutex
	startedAt           time.Time
	guildID             snowflake.ID
	ownerID             snowflake.ID
	voiceChannelID      snowflake.ID
	transcriptChannelID snowflake.ID
	conn                voice.Conn
	buffer              *audio.SegmentBuffer
	recorder            recordingSink
	stopped             bool
}

type recordingSink interface {
	audio.ChunkSink
	Close() error
	Directory() string
	Files() []wav.File
}

func New(guildID, ownerID, voiceChannelID, transcriptChannelID snowflake.ID, conn voice.Conn, logger *slog.Logger) (*Session, error) {
	startedAt := time.Now()
	recorder, err := wav.NewRecorder(guildID, startedAt)
	if err != nil {
		return nil, err
	}

	return &Session{
		startedAt:           startedAt,
		guildID:             guildID,
		ownerID:             ownerID,
		voiceChannelID:      voiceChannelID,
		transcriptChannelID: transcriptChannelID,
		conn:                conn,
		recorder:            recorder,
		buffer:              audio.NewSegmentBuffer(recorder, logger),
	}, nil
}

func (s *Session) Timestamp() time.Duration {
	return time.Since(s.startedAt)
}

func (s *Session) OwnerID() snowflake.ID {
	return s.ownerID
}

func (s *Session) RecorderDirectory() string {
	return s.recorder.Directory()
}

func (s *Session) RecordingFiles() []wav.File {
	return s.recorder.Files()
}

func (s *Session) AudioBuffer() *audio.SegmentBuffer {
	return s.buffer
}

func (s *Session) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return nil
	}
	s.stopped = true
	s.mu.Unlock()

	if s.conn != nil {
		s.conn.Close(ctx)
	}
	s.buffer.Close()
	if err := s.recorder.Close(); err != nil {
		return fmt.Errorf("close recording: %w", err)
	}

	return nil
}

// Abort closes recording resources after joining the voice channel fails. The
// connection itself is owned and removed by the voice manager.
func (s *Session) Abort() error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return nil
	}
	s.stopped = true
	s.mu.Unlock()

	s.buffer.Close()
	if err := s.recorder.Close(); err != nil {
		return fmt.Errorf("close recording: %w", err)
	}
	return nil
}

type Manager struct {
	mu           sync.Mutex
	sessions     map[snowflake.ID]*Session
	starting     map[snowflake.ID]snowflake.ID
	transcriber  whisper.Transcriber
	transcribe   transcriptionRunner
	logger       *slog.Logger
	transcribing map[string]struct{}
	closing      bool
}

// RecordingState describes a guild's current recording lifecycle state.
type RecordingState string

const (
	RecordingStateIdle       RecordingState = "idle"
	RecordingStateConnecting RecordingState = "connecting"
	RecordingStateRecording  RecordingState = "recording"
)

// RecordingStatus is a read-only snapshot of a guild's recording state.
type RecordingStatus struct {
	State          RecordingState
	VoiceChannelID snowflake.ID
	Elapsed        time.Duration
	SpeakerCount   int
}

func NewManager(transcriber whisper.Transcriber, logger *slog.Logger) *Manager {
	return &Manager{
		sessions:     make(map[snowflake.ID]*Session),
		starting:     make(map[snowflake.ID]snowflake.ID),
		transcriber:  transcriber,
		transcribe:   newTranscriptionRunner(),
		logger:       logger,
		transcribing: make(map[string]struct{}),
	}
}

func (m *Manager) Reserve(guildID, voiceChannelID snowflake.ID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closing {
		return false
	}
	if _, ok := m.sessions[guildID]; ok {
		return false
	}
	if _, ok := m.starting[guildID]; ok {
		return false
	}

	m.starting[guildID] = voiceChannelID
	return true
}

// VoiceChannelID reports the channel VexBot is recording in or connecting to
// for a guild.
func (m *Manager) VoiceChannelID(guildID snowflake.ID) (snowflake.ID, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if active, ok := m.sessions[guildID]; ok {
		return active.voiceChannelID, true
	}
	channelID, ok := m.starting[guildID]
	return channelID, ok
}

// Status returns a read-only snapshot for a guild. A reservation represents a
// connection in progress; an active session represents a recording in progress.
func (m *Manager) Status(guildID snowflake.ID) RecordingStatus {
	m.mu.Lock()
	active, recording := m.sessions[guildID]
	connectingChannelID, connecting := m.starting[guildID]
	m.mu.Unlock()

	if recording {
		return RecordingStatus{
			State:          RecordingStateRecording,
			VoiceChannelID: active.voiceChannelID,
			Elapsed:        active.Timestamp(),
			SpeakerCount:   len(active.RecordingFiles()),
		}
	}
	if connecting {
		return RecordingStatus{
			State:          RecordingStateConnecting,
			VoiceChannelID: connectingChannelID,
		}
	}
	return RecordingStatus{State: RecordingStateIdle}
}

// Start registers a connected recording session. It returns false when the
// manager is shutting down, in which case the caller must abort the session.
func (m *Manager) Start(session *Session) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.starting, session.guildID)
	if m.closing {
		return false
	}
	m.sessions[session.guildID] = session
	return true
}

func (m *Manager) CancelReservation(guildID snowflake.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.starting, guildID)
}

// TranscribingDirectories returns the directories currently used by local
// transcription jobs. Recording maintenance must not remove these paths.
func (m *Manager) TranscribingDirectories() map[string]struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	directories := make(map[string]struct{}, len(m.transcribing))
	for directory := range m.transcribing {
		directories[directory] = struct{}{}
	}
	return directories
}

type StoppedRecording struct {
	GuildID             snowflake.ID
	VoiceChannelID      snowflake.ID
	TranscriptChannelID snowflake.ID
	Directory           string
	Files               []wav.File
	DisplayNames        map[string]string
	TranscriptMetadata  transcript.Metadata
}

func (m *Manager) Stop(ctx context.Context, guildID, userID snowflake.ID) (StoppedRecording, error) {
	m.mu.Lock()
	session, ok := m.sessions[guildID]
	m.mu.Unlock()

	if !ok {
		return StoppedRecording{}, fmt.Errorf("there is no active recording session")
	}
	if session.OwnerID() != userID {
		return StoppedRecording{}, fmt.Errorf("only the user who started the recording can stop it")
	}

	if err := session.Stop(ctx); err != nil {
		return StoppedRecording{}, err
	}

	m.mu.Lock()
	delete(m.sessions, guildID)
	m.mu.Unlock()

	return StoppedRecording{
		GuildID:             guildID,
		VoiceChannelID:      session.voiceChannelID,
		TranscriptChannelID: session.transcriptChannelID,
		Directory:           session.RecorderDirectory(),
		Files:               session.RecordingFiles(),
		TranscriptMetadata: transcript.Metadata{
			StartedAt: session.startedAt,
			EndedAt:   time.Now(),
		},
	}, nil
}

func (m *Manager) logInfo(message string, args ...any) {
	if m.logger != nil {
		m.logger.Info(message, args...)
	}
}

func (m *Manager) logError(message string, args ...any) {
	if m.logger != nil {
		m.logger.Error(message, args...)
	}
}

// FinalizationResult summarizes recording cleanup performed during shutdown.
type FinalizationResult struct {
	FinalizedSessions int
	FailedSessions    int
	Recordings        []FinalizedRecording
	Err               error
}

// FinalizedRecording identifies one recording handled during shutdown.
type FinalizedRecording struct {
	GuildID   snowflake.ID
	Directory string
}

// Finalize stops all active recordings without starting transcription. It also
// prevents late voice connections from becoming active recording sessions.
func (m *Manager) Finalize(ctx context.Context) FinalizationResult {
	m.mu.Lock()
	sessions := m.sessions
	m.sessions = make(map[snowflake.ID]*Session)
	m.starting = make(map[snowflake.ID]snowflake.ID)
	m.closing = true
	m.mu.Unlock()

	result := FinalizationResult{}
	for guildID, session := range sessions {
		recording := FinalizedRecording{
			GuildID:   guildID,
			Directory: session.RecorderDirectory(),
		}
		if err := session.Stop(ctx); err != nil {
			result.FailedSessions++
			result.Err = errors.Join(result.Err, fmt.Errorf("stop recording for guild %v: %w", guildID, err))
			result.Recordings = append(result.Recordings, recording)
			continue
		}
		result.FinalizedSessions++
		result.Recordings = append(result.Recordings, recording)
	}
	return result
}

// Close finalizes active recordings for callers that need only an error.
func (m *Manager) Close(ctx context.Context) error {
	return m.Finalize(ctx).Err
}
