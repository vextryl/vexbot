package main

import "testing"

func TestNewWhisperTranscriberRequiresBothPaths(t *testing.T) {
	_, err := newWhisperTranscriber("whisper-cli", "", "", "")
	if err == nil {
		t.Fatal("newWhisperTranscriber() error = nil, want configuration error")
	}
}

func TestNewWhisperTranscriberUsesDefaults(t *testing.T) {
	transcriber, err := newWhisperTranscriber("whisper-cli", "model.bin", "", "")
	if err != nil {
		t.Fatalf("newWhisperTranscriber() error = %v", err)
	}
	if transcriber.ffmpeg != "ffmpeg" {
		t.Fatalf("ffmpeg = %q, want %q", transcriber.ffmpeg, "ffmpeg")
	}
	if transcriber.language != "en" {
		t.Fatalf("language = %q, want %q", transcriber.language, "en")
	}
}
