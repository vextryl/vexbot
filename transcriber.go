package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	whisperCLIPathEnv   = "WHISPER_CLI_PATH"
	whisperModelPathEnv = "WHISPER_MODEL_PATH"
	ffmpegPathEnv       = "FFMPEG_PATH"
	whisperLanguageEnv  = "WHISPER_LANGUAGE"
)

type Transcriber interface {
	Transcribe(context.Context, RecordingFile) (string, error)
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

func (t *whisperTranscriber) Transcribe(ctx context.Context, recording RecordingFile) (string, error) {
	temporaryDir, err := os.MkdirTemp("", "vexbot-whisper-*")
	if err != nil {
		return "", fmt.Errorf("create temporary audio directory: %w", err)
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
		return "", fmt.Errorf("prepare audio for user %v: %w", recording.UserID, err)
	}

	transcriptBase := strings.TrimSuffix(recording.Path, filepath.Ext(recording.Path))
	if err := runCommand(
		ctx,
		t.cliPath,
		"-m", t.modelPath,
		"-f", preparedAudio,
		"-l", t.language,
		"-otxt",
		"-of", transcriptBase,
	); err != nil {
		return "", fmt.Errorf("transcribe audio for user %v: %w", recording.UserID, err)
	}

	return transcriptBase + ".txt", nil
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
