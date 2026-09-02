// Package audio receives decoded Discord audio and groups it into contiguous
// per-speaker PCM segments for recording sinks.
package audio

import (
	"log/slog"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

type Frame struct {
	UserID     snowflake.ID
	Samples    []int16
	SampleRate int
	Channels   int
	Timestamp  time.Duration
}

type FrameSink interface {
	ConsumeFrame(Frame)
}

type discardAudioSink struct {
	mu       sync.Mutex
	duration map[snowflake.ID]time.Duration
	reported map[snowflake.ID]time.Duration
	logger   *slog.Logger
}

func (s *discardAudioSink) ConsumeFrame(frame Frame) {
	s.mu.Lock()
	defer s.mu.Unlock()

	duration := time.Duration(
		float64(len(frame.Samples)) /
			float64(frame.Channels) /
			float64(frame.SampleRate) *
			float64(time.Second),
	)

	s.duration[frame.UserID] += duration

	if s.duration[frame.UserID]-s.reported[frame.UserID] >= 5*time.Second {
		if s.logger != nil {
			s.logger.Debug(
				"decoded speaker audio",
				slog.String("user_id", frame.UserID.String()),
				slog.Duration("decoded", s.duration[frame.UserID]),
				slog.Duration("timestamp", frame.Timestamp),
			)
		}

		s.reported[frame.UserID] = s.duration[frame.UserID]
	}
}
