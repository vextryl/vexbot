package main

import (
	"testing"
	"time"
)

func TestNewWhisperTranscriberRequiresBothPaths(t *testing.T) {
	_, err := newWhisperTranscriber("whisper-cli", "", "", "", "")
	if err == nil {
		t.Fatal("newWhisperTranscriber() error = nil, want configuration error")
	}
}

func TestNewWhisperTranscriberUsesDefaults(t *testing.T) {
	transcriber, err := newWhisperTranscriber("whisper-cli", "ggml-base.en.bin", "", "", "")
	if err != nil {
		t.Fatalf("newWhisperTranscriber() error = %v", err)
	}
	if transcriber.ffmpeg != "ffmpeg" {
		t.Fatalf("ffmpeg = %q, want %q", transcriber.ffmpeg, "ffmpeg")
	}
	if transcriber.language != "en" {
		t.Fatalf("language = %q, want %q", transcriber.language, "en")
	}
	if transcriber.dtwPreset != "base.en" {
		t.Fatalf("dtwPreset = %q, want base.en", transcriber.dtwPreset)
	}
}

func TestParseWhisperJSON(t *testing.T) {
	segments, tokens, err := parseWhisperJSON([]byte(`{
  "transcription": [
    {
      "offsets": { "from": 640, "to": 11280 },
      "text": " A recorded phrase. ",
      "tokens": [
        { "offsets": { "from": 640, "to": 4120 }, "text": " A recorded" },
        { "offsets": { "from": 4120, "to": 11280 }, "text": " phrase." }
      ]
    }
  ]
}`))
	if err != nil {
		t.Fatalf("parseWhisperJSON() error = %v", err)
	}
	if len(segments) != 1 {
		t.Fatalf("segment count = %d, want 1", len(segments))
	}
	if got := segments[0]; got.WAVStart != 640*time.Millisecond ||
		got.WAVEnd != 11280*time.Millisecond || got.Text != "A recorded phrase." {
		t.Fatalf("segment = %#v", got)
	}
	if len(tokens) != 2 || tokens[1].WAVStart != 4120*time.Millisecond ||
		tokens[1].Text != " phrase." {
		t.Fatalf("tokens = %#v", tokens)
	}
}

func TestParseWhisperJSONRejectsInvalidOffsets(t *testing.T) {
	_, _, err := parseWhisperJSON([]byte(`{
  "transcription": [
    { "offsets": { "from": 100, "to": 99 }, "text": "bad" }
  ]
}`))
	if err == nil {
		t.Fatal("parseWhisperJSON() error = nil, want invalid-offset error")
	}
}

func TestNewWhisperTranscriberRequiresDTWPresetForUnknownModel(t *testing.T) {
	_, err := newWhisperTranscriber("whisper-cli", "custom-model.bin", "", "", "")
	if err == nil {
		t.Fatal("newWhisperTranscriber() error = nil, want DTW preset error")
	}
}
