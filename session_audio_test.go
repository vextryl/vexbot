package main

import (
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

type collectedAudioChunks []AudioChunk

func (c *collectedAudioChunks) ConsumeAudioChunk(chunk AudioChunk) {
	*c = append(*c, chunk)
}

func TestSessionAudioBufferSplitsTimestampGaps(t *testing.T) {
	var chunks collectedAudioChunks
	buffer := NewSessionAudioBuffer(&chunks)
	userID := snowflake.ID(42)

	buffer.ConsumeAudioFrame(AudioFrame{
		UserID: userID, Samples: []int16{1, 2}, SampleRate: 2, Channels: 1,
		Timestamp: time.Second,
	})
	buffer.ConsumeAudioFrame(AudioFrame{
		UserID: userID, Samples: []int16{3, 4}, SampleRate: 2, Channels: 1,
		Timestamp: 3 * time.Second,
	})
	buffer.Close()

	if len(chunks) != 2 {
		t.Fatalf("chunk count = %d, want 2", len(chunks))
	}
	if chunks[0].Timestamp != time.Second || chunks[1].Timestamp != 3*time.Second {
		t.Fatalf("chunk timestamps = %v, %v", chunks[0].Timestamp, chunks[1].Timestamp)
	}
}
