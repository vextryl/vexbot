// Package session manages active voice recording sessions and post-recording
// transcription work.
package session

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
	"github.com/vextryl/vexbot/internal/audio"
	"github.com/vextryl/vexbot/internal/transcript"
	"github.com/vextryl/vexbot/internal/turn"
	"github.com/vextryl/vexbot/internal/wav"
	"github.com/vextryl/vexbot/internal/whisper"
)

type Session struct {
	mu                  sync.Mutex
	startedAt           time.Time
	guildID             snowflake.ID
	ownerID             snowflake.ID
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

func New(guildID, ownerID, transcriptChannelID snowflake.ID, conn voice.Conn, logger *slog.Logger) (*Session, error) {
	startedAt := time.Now()
	recorder, err := wav.NewRecorder(guildID, startedAt)
	if err != nil {
		return nil, err
	}

	return &Session{
		startedAt:           startedAt,
		guildID:             guildID,
		ownerID:             ownerID,
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
	mu          sync.Mutex
	sessions    map[snowflake.ID]*Session
	starting    map[snowflake.ID]struct{}
	transcriber whisper.Transcriber
	transcribe  transcriptionRunner
	logger      *slog.Logger
}

func NewManager(transcriber whisper.Transcriber, logger *slog.Logger) *Manager {
	return &Manager{
		sessions:    make(map[snowflake.ID]*Session),
		starting:    make(map[snowflake.ID]struct{}),
		transcriber: transcriber,
		transcribe:  turn.Transcribe,
		logger:      logger,
	}
}

func (m *Manager) Reserve(guildID snowflake.ID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[guildID]; ok {
		return false
	}
	if _, ok := m.starting[guildID]; ok {
		return false
	}

	m.starting[guildID] = struct{}{}
	return true
}

func (m *Manager) Start(session *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.starting, session.guildID)
	m.sessions[session.guildID] = session
}

func (m *Manager) CancelReservation(guildID snowflake.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.starting, guildID)
}

type StoppedRecording struct {
	GuildID             snowflake.ID
	TranscriptChannelID snowflake.ID
	Directory           string
	Files               []wav.File
	DisplayNames        map[string]string
}

// TranscriptionResult reports the outcome of asynchronous local
// transcription. TranscriptPath and LineCount are set only on success.
type TranscriptionResult struct {
	TranscriptPath string
	LineCount      int
	Err            error
}

type transcriptionRunner func(
	context.Context,
	string,
	[]wav.File,
	whisper.Transcriber,
	func(int),
	func(int, int, turn.Result),
) ([]turn.Result, error)

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
		TranscriptChannelID: session.transcriptChannelID,
		Directory:           session.RecorderDirectory(),
		Files:               session.RecordingFiles(),
	}, nil
}

// StartTranscription begins asynchronous local transcription. It returns a
// channel that receives exactly one completion result, plus false when local
// transcription is unavailable for the recording.
func (m *Manager) StartTranscription(session StoppedRecording) (<-chan TranscriptionResult, bool) {
	if m.transcriber == nil || len(session.Files) == 0 {
		return nil, false
	}

	completion := make(chan TranscriptionResult, 1)
	go func() {
		defer close(completion)
		nextProgressPercent := 10
		results, err := m.transcribe(
			context.Background(),
			session.Directory,
			session.Files,
			m.transcriber,
			func(total int) {
				m.logInfo("starting local transcription",
					slog.String("guild_id", session.GuildID.String()),
					slog.String("directory", session.Directory),
					slog.Int("total_turns", total),
				)
			},
			func(completed, total int, result turn.Result) {
				if result.Err != nil {
					m.logError("transcribing turn",
						"err", result.Err,
						slog.String("guild_id", session.GuildID.String()),
						slog.String("user_id", result.Turn.UserID),
						slog.Int("completed_turns", completed),
						slog.Int("total_turns", total),
					)
				}
				percent := completed * 100 / total
				if percent >= nextProgressPercent || completed == total {
					m.logInfo("local transcription progress",
						slog.String("guild_id", session.GuildID.String()),
						slog.Int("percent", percent),
						slog.Int("completed_turns", completed),
						slog.Int("total_turns", total),
					)
					for nextProgressPercent <= percent {
						nextProgressPercent += 10
					}
				}
			},
		)
		if err != nil {
			m.logError("preparing local transcription",
				"err", err,
				slog.String("guild_id", session.GuildID.String()),
				slog.String("directory", session.Directory),
			)
			completion <- TranscriptionResult{Err: fmt.Errorf("prepare local transcription: %w", err)}
			return
		}

		failed := 0
		for _, result := range results {
			if result.Err != nil {
				failed++
			}
		}
		m.logInfo("local transcription finished",
			slog.String("guild_id", session.GuildID.String()),
			slog.Int("succeeded_turns", len(results)-failed),
			slog.Int("failed_turns", failed),
		)

		transcriptPath, lineCount, err := transcript.Write(session.Directory, session.DisplayNames, results)
		if err != nil {
			m.logError("writing combined transcript",
				"err", err,
				slog.String("guild_id", session.GuildID.String()),
				slog.String("directory", session.Directory),
			)
			completion <- TranscriptionResult{Err: fmt.Errorf("write combined transcript: %w", err)}
			return
		}
		m.logInfo("combined transcript saved",
			slog.String("guild_id", session.GuildID.String()),
			slog.String("path", transcriptPath),
			slog.Int("line_count", lineCount),
		)
		completion <- TranscriptionResult{
			TranscriptPath: transcriptPath,
			LineCount:      lineCount,
		}
	}()

	return completion, true
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

func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	sessions := m.sessions
	m.sessions = make(map[snowflake.ID]*Session)
	m.starting = make(map[snowflake.ID]struct{})
	m.mu.Unlock()

	var closeErr error
	for guildID, session := range sessions {
		if err := session.Stop(ctx); err != nil {
			closeErr = fmt.Errorf("stop recording for guild %v: %w", guildID, err)
		}
	}

	return closeErr
}
