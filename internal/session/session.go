// Package session manages active voice recording sessions and post-recording
// transcription work.
package session

import (
	"context"
	"fmt"
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
	mu        sync.Mutex
	startedAt time.Time
	guildID   snowflake.ID
	ownerID   snowflake.ID
	conn      voice.Conn
	buffer    *audio.SegmentBuffer
	recorder  *wav.Recorder
	stopped   bool
}

func New(guildID, ownerID snowflake.ID, conn voice.Conn) (*Session, error) {
	startedAt := time.Now()
	recorder, err := wav.NewRecorder(guildID, startedAt)
	if err != nil {
		return nil, err
	}

	return &Session{
		startedAt: startedAt,
		guildID:   guildID,
		ownerID:   ownerID,
		conn:      conn,
		recorder:  recorder,
		buffer:    audio.NewSegmentBuffer(recorder),
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

	s.conn.Close(ctx)
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
}

func NewManager(transcriber whisper.Transcriber) *Manager {
	return &Manager{
		sessions:    make(map[snowflake.ID]*Session),
		starting:    make(map[snowflake.ID]struct{}),
		transcriber: transcriber,
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
	Directory    string
	Files        []wav.File
	DisplayNames map[string]string
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
		Directory: session.RecorderDirectory(),
		Files:     session.RecordingFiles(),
	}, nil
}

func (m *Manager) StartTranscription(session StoppedRecording) bool {
	if m.transcriber == nil || len(session.Files) == 0 {
		return false
	}

	go func() {
		nextProgressPercent := 10
		results, err := turn.Transcribe(
			context.Background(),
			session.Directory,
			session.Files,
			m.transcriber,
			func(total int) {
				fmt.Printf("Starting local transcription for %d turn(s)\n", total)
			},
			func(completed, total int, result turn.Result) {
				if result.Err != nil {
					fmt.Printf("Error transcribing turn %d for user %s: %v\n", completed, result.Turn.UserID, result.Err)
				}
				percent := completed * 100 / total
				if percent >= nextProgressPercent || completed == total {
					fmt.Printf("Local transcription progress: %d%% (%d/%d turns)\n", percent, completed, total)
					for nextProgressPercent <= percent {
						nextProgressPercent += 10
					}
				}
			},
		)
		if err != nil {
			fmt.Printf("Error preparing local transcription: %v\n", err)
			return
		}

		failed := 0
		for _, result := range results {
			if result.Err != nil {
				failed++
			}
		}
		fmt.Printf("Local transcription finished: %d succeeded, %d failed\n", len(results)-failed, failed)

		transcriptPath, lineCount, err := transcript.Write(session.Directory, session.DisplayNames, results)
		if err != nil {
			fmt.Printf("Error writing combined transcript: %v\n", err)
			return
		}
		fmt.Printf("Combined transcript saved to %s (%d line(s))\n", transcriptPath, lineCount)
	}()

	return true
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
