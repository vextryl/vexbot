package main

import (
	"time"
)

type VoiceSession struct {
	startedAt time.Time
}

func NewVoiceSession() *VoiceSession {
	return &VoiceSession{
		startedAt: time.Now(),
	}
}

func (s *VoiceSession) Timestamp() time.Duration {
	return time.Since(s.startedAt)
}
