package main

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

func TestWAVRecorderWritesFinalizedPCMFile(t *testing.T) {
	dir := t.TempDir()
	recorder := newTestWAVRecorder(dir)
	userID := snowflake.ID(42)
	samples := []int16{-32768, -1, 0, 32767}

	recorder.ConsumeAudioChunk(AudioChunk{
		UserID:     userID,
		Samples:    samples,
		SampleRate: 48000,
		Channels:   2,
		Timestamp:  time.Second,
	})
	if err := recorder.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	contents, err := os.ReadFile(filepath.Join(dir, "42.wav"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(contents) != wavHeaderSize+len(samples)*wavSampleBytes {
		t.Fatalf("file length = %d, want %d", len(contents), wavHeaderSize+len(samples)*wavSampleBytes)
	}
	if string(contents[0:4]) != "RIFF" || string(contents[8:12]) != "WAVE" {
		t.Fatalf("file does not have a RIFF/WAVE header")
	}
	if got := binary.LittleEndian.Uint32(contents[40:44]); got != uint32(len(samples)*wavSampleBytes) {
		t.Fatalf("data size = %d, want %d", got, len(samples)*wavSampleBytes)
	}
	if got := int16(binary.LittleEndian.Uint16(contents[44:46])); got != samples[0] {
		t.Fatalf("first sample = %d, want %d", got, samples[0])
	}
}

func TestWAVRecorderWritesTimeline(t *testing.T) {
	dir := t.TempDir()
	recorder := newTestWAVRecorder(dir)

	recorder.ConsumeAudioChunk(AudioChunk{
		UserID: 42, Samples: []int16{1, 2}, SampleRate: 2, Channels: 1,
		Timestamp: time.Second,
	})
	recorder.ConsumeAudioChunk(AudioChunk{
		UserID: 7, Samples: []int16{3, 4}, SampleRate: 2, Channels: 1,
		Timestamp: 1500 * time.Millisecond,
	})
	recorder.ConsumeAudioChunk(AudioChunk{
		UserID: 42, Samples: []int16{5, 6}, SampleRate: 2, Channels: 1,
		Timestamp: 2 * time.Second,
	})
	if err := recorder.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	contents, err := os.ReadFile(filepath.Join(dir, timelineFileName))
	if err != nil {
		t.Fatalf("ReadFile(timeline.json) error = %v", err)
	}

	var timeline sessionTimeline
	if err := json.Unmarshal(contents, &timeline); err != nil {
		t.Fatalf("Unmarshal(timeline.json) error = %v", err)
	}
	if timeline.Version != 1 || len(timeline.Spans) != 2 {
		t.Fatalf("timeline = %#v, want version 1 with 2 spans", timeline)
	}
	if got := timeline.Spans[0]; got.UserID != "42" ||
		got.SessionStartMS != 1000 || got.SessionEndMS != 3000 ||
		got.WAVStartMS != 0 || got.WAVEndMS != 2000 {
		t.Fatalf("first timeline span = %#v", got)
	}
	if got := timeline.Spans[1]; got.UserID != "7" ||
		got.SessionStartMS != 1500 || got.SessionEndMS != 2500 ||
		got.WAVStartMS != 0 || got.WAVEndMS != 1000 {
		t.Fatalf("second timeline span = %#v", got)
	}
}

func newTestWAVRecorder(dir string) *wavRecorder {
	return &wavRecorder{
		dir:      dir,
		writers:  make(map[snowflake.ID]*wavWriter),
		lastSpan: make(map[snowflake.ID]int),
	}
}
