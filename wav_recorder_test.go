package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

func TestWAVRecorderWritesFinalizedPCMFile(t *testing.T) {
	dir := t.TempDir()
	recorder := &wavRecorder{
		dir:     dir,
		writers: make(map[snowflake.ID]*wavWriter),
	}
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
