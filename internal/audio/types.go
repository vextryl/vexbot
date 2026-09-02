package audio

import (
	"fmt"
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
		fmt.Printf(
			"AUDIO: user=%v decoded=%.1fs timestamp=%.1fs\n",
			frame.UserID,
			s.duration[frame.UserID].Seconds(),
			frame.Timestamp.Seconds(),
		)

		s.reported[frame.UserID] = s.duration[frame.UserID]
	}
}
