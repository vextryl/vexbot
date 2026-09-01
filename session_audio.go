package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

const (
	audioChunkDuration  = time.Second
	audioFlushAfterIdle = 500 * time.Millisecond
)

type AudioChunk struct {
	UserID     snowflake.ID
	Samples    []int16
	SampleRate int
	Channels   int
	Timestamp  time.Duration
}

type SessionAudioBuffer struct {
	mu       sync.Mutex
	speakers map[snowflake.ID]*speakerBuffer
	sink     AudioChunkSink
	closed   bool
}

type speakerBuffer struct {
	samples       []int16
	startTime     time.Duration
	nextTimestamp time.Duration
	sampleRate    int
	channels      int
	timer         *time.Timer
}

type AudioChunkSink interface {
	ConsumeAudioChunk(AudioChunk)
}

type discardAudioChunkSink struct{}

func (discardAudioChunkSink) ConsumeAudioChunk(chunk AudioChunk) {
	fmt.Printf(
		"AUDIO: user=%v chunk=%.1fs timestamp=%.1fs\n",
		chunk.UserID,
		float64(len(chunk.Samples))/
			float64(chunk.SampleRate*chunk.Channels),
		chunk.Timestamp.Seconds(),
	)
}

func NewSessionAudioBuffer(sink AudioChunkSink) *SessionAudioBuffer {
	return &SessionAudioBuffer{
		speakers: make(map[snowflake.ID]*speakerBuffer),
		sink:     sink,
	}
}

func (b *SessionAudioBuffer) flushSpeaker(userID snowflake.ID) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	speaker, ok := b.speakers[userID]
	if !ok {
		return
	}

	b.flushSpeakerLocked(userID, speaker)
}

func (b *SessionAudioBuffer) ConsumeAudioFrame(frame AudioFrame) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	speaker, ok := b.speakers[frame.UserID]
	if !ok {
		speaker = &speakerBuffer{
			samples:    make([]int16, 0),
			sampleRate: frame.SampleRate,
			channels:   frame.Channels,
		}

		b.speakers[frame.UserID] = speaker
	}

	samplesPerChunk := frame.SampleRate * frame.Channels *
		int(audioChunkDuration/time.Second)
	frameTimestamp := frame.Timestamp

	for len(frame.Samples) > 0 {
		if len(speaker.samples) == 0 {
			speaker.startTime = frameTimestamp
		}

		remaining := samplesPerChunk - len(speaker.samples)
		count := min(remaining, len(frame.Samples))

		speaker.samples = append(
			speaker.samples,
			frame.Samples[:count]...,
		)

		frame.Samples = frame.Samples[count:]

		consumedDuration := time.Duration(
			float64(count) /
				float64(frame.SampleRate*frame.Channels) *
				float64(time.Second),
		)

		frameTimestamp += consumedDuration
		speaker.nextTimestamp = frameTimestamp

		if len(speaker.samples) == samplesPerChunk {
			b.flushSpeakerLocked(frame.UserID, speaker)
		}
	}

	if speaker.timer == nil {
		speaker.timer = time.AfterFunc(
			audioFlushAfterIdle,
			func() {
				b.flushSpeaker(frame.UserID)
			},
		)
	} else {
		speaker.timer.Reset(audioFlushAfterIdle)
	}
}

func (b *SessionAudioBuffer) CleanupUser(userID snowflake.ID) {
	b.mu.Lock()
	defer b.mu.Unlock()

	speaker, ok := b.speakers[userID]
	if ok && speaker.timer != nil {
		speaker.timer.Stop()
	}

	delete(b.speakers, userID)
}

func (b *SessionAudioBuffer) flushSpeakerLocked(
	userID snowflake.ID,
	speaker *speakerBuffer,
) {
	if len(speaker.samples) == 0 {
		return
	}

	b.sink.ConsumeAudioChunk(AudioChunk{
		UserID:     userID,
		Samples:    speaker.samples,
		SampleRate: speaker.sampleRate,
		Channels:   speaker.channels,
		Timestamp:  speaker.startTime,
	})

	speaker.samples = make([]int16, 0, speaker.sampleRate*speaker.channels)
}

func (b *SessionAudioBuffer) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	b.closed = true

	for userID, speaker := range b.speakers {
		if speaker.timer != nil {
			speaker.timer.Stop()
		}
		b.flushSpeakerLocked(userID, speaker)
	}

	b.speakers = make(map[snowflake.ID]*speakerBuffer)

	fmt.Println("AUDIO: session buffer closed")
}
