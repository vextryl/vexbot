package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	whisperCLIPathEnv   = "WHISPER_CLI_PATH"
	whisperModelPathEnv = "WHISPER_MODEL_PATH"
	ffmpegPathEnv       = "FFMPEG_PATH"
	whisperLanguageEnv  = "WHISPER_LANGUAGE"
)

type Transcriber interface {
	Transcribe(context.Context, RecordingFile) (Transcription, error)
}

type Transcription struct {
	TextPath string
	JSONPath string
	Segments []TranscriptionSegment
}

type TranscriptionSegment struct {
	WAVStart time.Duration
	WAVEnd   time.Duration
	Text     string
}

type whisperJSONOutput struct {
	Transcription []whisperJSONSegment `json:"transcription"`
}

type whisperJSONSegment struct {
	Offsets whisperJSONOffsets `json:"offsets"`
	Text    string             `json:"text"`
}

type whisperJSONOffsets struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
}

type whisperTranscriber struct {
	cliPath   string
	modelPath string
	ffmpeg    string
	language  string
}

func newWhisperTranscriberFromEnv() (*whisperTranscriber, error) {
	return newWhisperTranscriber(
		os.Getenv(whisperCLIPathEnv),
		os.Getenv(whisperModelPathEnv),
		os.Getenv(ffmpegPathEnv),
		os.Getenv(whisperLanguageEnv),
	)
}

func newWhisperTranscriber(
	cliPath string,
	modelPath string,
	ffmpeg string,
	language string,
) (*whisperTranscriber, error) {
	cliPath = strings.TrimSpace(cliPath)
	modelPath = strings.TrimSpace(modelPath)

	if cliPath == "" && modelPath == "" {
		return nil, nil
	}
	if cliPath == "" || modelPath == "" {
		return nil, fmt.Errorf("%s and %s must both be set", whisperCLIPathEnv, whisperModelPathEnv)
	}

	if ffmpeg = strings.TrimSpace(ffmpeg); ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	if language = strings.TrimSpace(language); language == "" {
		language = "en"
	}

	return &whisperTranscriber{
		cliPath:   cliPath,
		modelPath: modelPath,
		ffmpeg:    ffmpeg,
		language:  language,
	}, nil
}

func (t *whisperTranscriber) Transcribe(ctx context.Context, recording RecordingFile) (Transcription, error) {
	temporaryDir, err := os.MkdirTemp("", "vexbot-whisper-*")
	if err != nil {
		return Transcription{}, fmt.Errorf("create temporary audio directory: %w", err)
	}
	defer os.RemoveAll(temporaryDir)

	preparedAudio := filepath.Join(temporaryDir, "prepared.wav")
	if err := runCommand(
		ctx,
		t.ffmpeg,
		"-y",
		"-i", recording.Path,
		"-ar", "16000",
		"-ac", "1",
		"-c:a", "pcm_s16le",
		preparedAudio,
	); err != nil {
		return Transcription{}, fmt.Errorf("prepare audio for user %v: %w", recording.UserID, err)
	}

	transcriptBase := strings.TrimSuffix(recording.Path, filepath.Ext(recording.Path))
	if err := runCommand(
		ctx,
		t.cliPath,
		"-m", t.modelPath,
		"-f", preparedAudio,
		"-l", t.language,
		"-otxt",
		"-oj",
		"-of", transcriptBase,
	); err != nil {
		return Transcription{}, fmt.Errorf("transcribe audio for user %v: %w", recording.UserID, err)
	}

	jsonPath := transcriptBase + ".json"
	contents, err := os.ReadFile(jsonPath)
	if err != nil {
		return Transcription{}, fmt.Errorf("read transcription JSON for user %v: %w", recording.UserID, err)
	}
	segments, err := parseWhisperJSON(contents)
	if err != nil {
		return Transcription{}, fmt.Errorf("parse transcription JSON for user %v: %w", recording.UserID, err)
	}

	return Transcription{
		TextPath: transcriptBase + ".txt",
		JSONPath: jsonPath,
		Segments: segments,
	}, nil
}

func parseWhisperJSON(contents []byte) ([]TranscriptionSegment, error) {
	var output whisperJSONOutput
	if err := json.Unmarshal(contents, &output); err != nil {
		return nil, err
	}

	segments := make([]TranscriptionSegment, 0, len(output.Transcription))
	for index, segment := range output.Transcription {
		if segment.Offsets.From < 0 || segment.Offsets.To < segment.Offsets.From {
			return nil, fmt.Errorf("segment %d has invalid offsets", index)
		}

		segments = append(segments, TranscriptionSegment{
			WAVStart: time.Duration(segment.Offsets.From) * time.Millisecond,
			WAVEnd:   time.Duration(segment.Offsets.To) * time.Millisecond,
			Text:     strings.TrimSpace(segment.Text),
		})
	}

	return segments, nil
}

func runCommand(ctx context.Context, name string, args ...string) error {
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err == nil {
		return nil
	}

	message := strings.TrimSpace(string(output))
	if message == "" {
		return err
	}

	return fmt.Errorf("%w: %s", err, message)
}
