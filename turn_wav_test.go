package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExtractTurnWAV(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "speaker.wav")
	writeTestPCM16WAV(t, sourcePath, 1000, 1, []int16{10, 20, 30, 40, 50, 60})

	temporaryDir := temporaryTurnWAVDirectory(dir)
	if err := os.Mkdir(temporaryDir, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	path, err := extractTurnWAV(temporaryDir, sourcePath, transcriptionTurn{
		UserID:   "42",
		WAVStart: 2 * time.Millisecond,
		WAVEnd:   5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("extractTurnWAV() error = %v", err)
	}
	if filepath.Dir(path) != temporaryDir {
		t.Fatalf("temporary path directory = %q, want %q", filepath.Dir(path), temporaryDir)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := binary.LittleEndian.Uint32(contents[40:44]), uint32(6); got != want {
		t.Fatalf("data size = %d, want %d", got, want)
	}
	for index, want := range []int16{30, 40, 50} {
		got := int16(binary.LittleEndian.Uint16(contents[wavHeaderSize+index*wavSampleBytes:]))
		if got != want {
			t.Fatalf("sample %d = %d, want %d", index, got, want)
		}
	}
}

func TestExtractTurnWAVRejectsOutOfRangeAudio(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "speaker.wav")
	writeTestPCM16WAV(t, sourcePath, 1000, 1, []int16{10, 20})

	if _, err := extractTurnWAV(dir, sourcePath, transcriptionTurn{
		UserID:   "42",
		WAVStart: 1 * time.Millisecond,
		WAVEnd:   3 * time.Millisecond,
	}); err == nil {
		t.Fatal("extractTurnWAV() error = nil, want range error")
	}
}

func writeTestPCM16WAV(t *testing.T, path string, sampleRate uint32, channels uint16, samples []int16) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer file.Close()

	if err := writePCM16WAVHeader(file, sampleRate, channels, uint32(len(samples)*wavSampleBytes)); err != nil {
		t.Fatalf("writePCM16WAVHeader() error = %v", err)
	}
	for _, sample := range samples {
		var bytes [wavSampleBytes]byte
		binary.LittleEndian.PutUint16(bytes[:], uint16(sample))
		if _, err := file.Write(bytes[:]); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}
}
