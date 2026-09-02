package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
)

type VoiceSession struct {
	mu        sync.Mutex
	startedAt time.Time
	guildID   snowflake.ID
	ownerID   snowflake.ID
	conn      voice.Conn
	buffer    *SessionAudioBuffer
	recorder  *wavRecorder
	stopped   bool
}

func NewVoiceSession(guildID, ownerID snowflake.ID, conn voice.Conn) (*VoiceSession, error) {
	startedAt := time.Now()
	recorder, err := newWAVRecorder(guildID, startedAt)
	if err != nil {
		return nil, err
	}

	return &VoiceSession{
		startedAt: startedAt,
		guildID:   guildID,
		ownerID:   ownerID,
		conn:      conn,
		recorder:  recorder,
		buffer:    NewSessionAudioBuffer(recorder),
	}, nil
}

func (s *VoiceSession) Timestamp() time.Duration {
	return time.Since(s.startedAt)
}

func (s *VoiceSession) OwnerID() snowflake.ID {
	return s.ownerID
}

func (s *VoiceSession) RecorderDirectory() string {
	return s.recorder.Directory()
}

func (s *VoiceSession) RecordingFiles() []RecordingFile {
	return s.recorder.Files()
}

func (s *VoiceSession) AudioBuffer() *SessionAudioBuffer {
	return s.buffer
}

func (s *VoiceSession) Stop(ctx context.Context) error {
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

type SessionManager struct {
	mu          sync.Mutex
	sessions    map[snowflake.ID]*VoiceSession
	starting    map[snowflake.ID]struct{}
	transcriber Transcriber
}

func NewSessionManager(transcriber Transcriber) *SessionManager {
	return &SessionManager{
		sessions:    make(map[snowflake.ID]*VoiceSession),
		starting:    make(map[snowflake.ID]struct{}),
		transcriber: transcriber,
	}
}

func (m *SessionManager) Reserve(guildID snowflake.ID) bool {
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

func (m *SessionManager) Start(session *VoiceSession) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.starting, session.guildID)
	m.sessions[session.guildID] = session
}

func (m *SessionManager) CancelReservation(guildID snowflake.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.starting, guildID)
}

type StoppedSession struct {
	Directory string
	Files     []RecordingFile
}

func (m *SessionManager) Stop(ctx context.Context, guildID, userID snowflake.ID) (StoppedSession, error) {
	m.mu.Lock()
	session, ok := m.sessions[guildID]
	m.mu.Unlock()

	if !ok {
		return StoppedSession{}, fmt.Errorf("there is no active recording session")
	}
	if session.OwnerID() != userID {
		return StoppedSession{}, fmt.Errorf("only the user who started the recording can stop it")
	}

	if err := session.Stop(ctx); err != nil {
		return StoppedSession{}, err
	}

	m.mu.Lock()
	delete(m.sessions, guildID)
	m.mu.Unlock()

	return StoppedSession{
		Directory: session.RecorderDirectory(),
		Files:     session.RecordingFiles(),
	}, nil
}

func (m *SessionManager) StartTranscription(session StoppedSession) bool {
	if m.transcriber == nil || len(session.Files) == 0 {
		return false
	}

	go func() {
		fmt.Println("Starting local transcription for detected speaking turns")
		results, err := transcribeRecordingTurns(
			context.Background(),
			session.Directory,
			session.Files,
			m.transcriber,
		)
		if err != nil {
			fmt.Printf("Error preparing local transcription: %v\n", err)
			return
		}

		fmt.Printf("Local transcription finished for %d turn(s)\n", len(results))
		for index, result := range results {
			if result.Err != nil {
				fmt.Printf("Error transcribing turn %d for user %s: %v\n", index+1, result.Turn.UserID, result.Err)
				continue
			}
			fmt.Printf(
				"Transcribed turn %d for user %s (%d timestamped token(s))\n",
				index+1,
				result.Turn.UserID,
				len(result.Transcription.Tokens),
			)
		}

		transcriptPath, lineCount, err := writeCombinedTranscript(session.Directory, results)
		if err != nil {
			fmt.Printf("Error writing combined transcript: %v\n", err)
			return
		}
		fmt.Printf("Combined transcript saved to %s (%d line(s))\n", transcriptPath, lineCount)
	}()

	return true
}

func (m *SessionManager) Close(ctx context.Context) error {
	m.mu.Lock()
	sessions := m.sessions
	m.sessions = make(map[snowflake.ID]*VoiceSession)
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
