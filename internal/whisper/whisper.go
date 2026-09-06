// Package whisper runs the local Whisper CLI for WAV recordings and parses
// its timestamped transcription output.
package whisper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vextryl/vexbot/internal/wav"
)

const (
	WhisperCLIPathEnv   = "WHISPER_CLI_PATH"
	WhisperModelPathEnv = "WHISPER_MODEL_PATH"
	WhisperDTWPresetEnv = "WHISPER_DTW_PRESET"
	FFMPEGPathEnv       = "FFMPEG_PATH"
	WhisperLanguageEnv  = "WHISPER_LANGUAGE"
	whisperEndOfTextID  = 50256
)

// Config contains the local executable and model settings needed to run
// Whisper transcription.
type Config struct {
	CLIPath    string
	ModelPath  string
	DTWPreset  string
	FFmpegPath string
	Language   string
}

// ConfigFromEnvironment reads Whisper configuration without validating paths.
func ConfigFromEnvironment(getenv func(string) string) Config {
	return Config{
		CLIPath:    getenv(WhisperCLIPathEnv),
		ModelPath:  getenv(WhisperModelPathEnv),
		DTWPreset:  getenv(WhisperDTWPresetEnv),
		FFmpegPath: getenv(FFMPEGPathEnv),
		Language:   getenv(WhisperLanguageEnv),
	}
}

// ValidateConfig normalizes optional values and validates enabled local
// transcription dependencies. It reports whether transcription is enabled.
func ValidateConfig(config Config, lookPath func(string) (string, error), stat func(string) (os.FileInfo, error)) (Config, bool, error) {
	config.CLIPath = strings.TrimSpace(config.CLIPath)
	config.ModelPath = strings.TrimSpace(config.ModelPath)
	config.DTWPreset = strings.TrimSpace(config.DTWPreset)
	config.FFmpegPath = strings.TrimSpace(config.FFmpegPath)
	config.Language = strings.TrimSpace(config.Language)

	if config.CLIPath == "" && config.ModelPath == "" {
		return config, false, nil
	}

	var validationErrors []error
	if config.CLIPath == "" || config.ModelPath == "" {
		validationErrors = append(validationErrors, fmt.Errorf("%s and %s must both be set", WhisperCLIPathEnv, WhisperModelPathEnv))
	}
	if config.DTWPreset == "" {
		config.DTWPreset = inferDTWPreset(config.ModelPath)
		if config.DTWPreset == "" {
			validationErrors = append(validationErrors, fmt.Errorf("set %s for Whisper model %q", WhisperDTWPresetEnv, filepath.Base(config.ModelPath)))
		}
	}
	if config.FFmpegPath == "" {
		config.FFmpegPath = "ffmpeg"
	}
	if config.Language == "" {
		config.Language = "en"
	}

	if config.CLIPath != "" {
		if _, err := lookPath(config.CLIPath); err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("resolve %s: %w", WhisperCLIPathEnv, err))
		}
	}
	if config.ModelPath != "" {
		info, err := stat(config.ModelPath)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("inspect %s: %w", WhisperModelPathEnv, err))
		} else if !info.Mode().IsRegular() {
			validationErrors = append(validationErrors, fmt.Errorf("%s must be a regular file", WhisperModelPathEnv))
		}
	}
	if _, err := lookPath(config.FFmpegPath); err != nil {
		validationErrors = append(validationErrors, fmt.Errorf("resolve %s: %w", FFMPEGPathEnv, err))
	}

	return config, true, errors.Join(validationErrors...)
}

type Transcriber interface {
	Transcribe(context.Context, wav.File) (Transcription, error)
}

type Transcription struct {
	TextPath string
	JSONPath string
	Segments []TranscriptionSegment
	Tokens   []TranscriptionToken
}

type TranscriptionSegment struct {
	WAVStart time.Duration
	WAVEnd   time.Duration
	Text     string
}

type TranscriptionToken struct {
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
	Tokens  []whisperJSONToken `json:"tokens"`
}

type whisperJSONOffsets struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
}

type whisperJSONToken struct {
	Offsets *whisperJSONOffsets `json:"offsets"`
	ID      int                 `json:"id"`
	Text    string              `json:"text"`
}

type whisperTranscriber struct {
	cliPath   string
	modelPath string
	dtwPreset string
	ffmpeg    string
	language  string
}

func NewWhisperTranscriberFromEnv() (*whisperTranscriber, error) {
	config, enabled, err := ValidateConfig(ConfigFromEnvironment(os.Getenv), exec.LookPath, os.Stat)
	if err != nil || !enabled {
		return nil, err
	}
	return NewWhisperTranscriber(config)
}

// NewWhisperTranscriber creates a transcriber from validated configuration.
func NewWhisperTranscriber(config Config) (*whisperTranscriber, error) {
	return newWhisperTranscriber(config.CLIPath, config.ModelPath, config.DTWPreset, config.FFmpegPath, config.Language)
}

func newWhisperTranscriber(
	cliPath string,
	modelPath string,
	dtwPreset string,
	ffmpeg string,
	language string,
) (*whisperTranscriber, error) {
	cliPath = strings.TrimSpace(cliPath)
	modelPath = strings.TrimSpace(modelPath)

	if cliPath == "" && modelPath == "" {
		return nil, nil
	}
	if cliPath == "" || modelPath == "" {
		return nil, fmt.Errorf("%s and %s must both be set", WhisperCLIPathEnv, WhisperModelPathEnv)
	}
	if dtwPreset = strings.TrimSpace(dtwPreset); dtwPreset == "" {
		dtwPreset = inferDTWPreset(modelPath)
	}
	if dtwPreset == "" {
		return nil, fmt.Errorf("set %s for Whisper model %q", WhisperDTWPresetEnv, filepath.Base(modelPath))
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
		dtwPreset: dtwPreset,
		ffmpeg:    ffmpeg,
		language:  language,
	}, nil
}

func (t *whisperTranscriber) Transcribe(ctx context.Context, recording wav.File) (Transcription, error) {
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
		"-ojf",
		"-dtw", t.dtwPreset,
		"-of", transcriptBase,
	); err != nil {
		return Transcription{}, fmt.Errorf("transcribe audio for user %v: %w", recording.UserID, err)
	}

	jsonPath := transcriptBase + ".json"
	contents, err := os.ReadFile(jsonPath)
	if err != nil {
		return Transcription{}, fmt.Errorf("read transcription JSON for user %v: %w", recording.UserID, err)
	}
	segments, tokens, err := parseWhisperJSON(contents)
	if err != nil {
		return Transcription{}, fmt.Errorf("parse transcription JSON for user %v: %w", recording.UserID, err)
	}

	return Transcription{
		TextPath: transcriptBase + ".txt",
		JSONPath: jsonPath,
		Segments: segments,
		Tokens:   tokens,
	}, nil
}

func parseWhisperJSON(contents []byte) ([]TranscriptionSegment, []TranscriptionToken, error) {
	var output whisperJSONOutput
	if err := json.Unmarshal(contents, &output); err != nil {
		return nil, nil, err
	}

	segments := make([]TranscriptionSegment, 0, len(output.Transcription))
	tokens := make([]TranscriptionToken, 0)
	for index, segment := range output.Transcription {
		if segment.Offsets.From < 0 || segment.Offsets.To < segment.Offsets.From {
			return nil, nil, fmt.Errorf("segment %d has invalid offsets", index)
		}

		segments = append(segments, TranscriptionSegment{
			WAVStart: time.Duration(segment.Offsets.From) * time.Millisecond,
			WAVEnd:   time.Duration(segment.Offsets.To) * time.Millisecond,
			Text:     strings.TrimSpace(segment.Text),
		})

		for tokenIndex, token := range segment.Tokens {
			if token.ID >= whisperEndOfTextID {
				continue
			}
			if token.Offsets == nil {
				if strings.TrimSpace(token.Text) == "" {
					continue
				}
				return nil, nil, fmt.Errorf("segment %d token %d has no timestamps", index, tokenIndex)
			}
			if token.Offsets.From < 0 || token.Offsets.To < token.Offsets.From {
				return nil, nil, fmt.Errorf("segment %d token %d has invalid offsets", index, tokenIndex)
			}

			tokens = append(tokens, TranscriptionToken{
				WAVStart: time.Duration(token.Offsets.From) * time.Millisecond,
				WAVEnd:   time.Duration(token.Offsets.To) * time.Millisecond,
				Text:     token.Text,
			})
		}
	}

	if len(output.Transcription) > 0 && len(tokens) == 0 {
		return nil, nil, fmt.Errorf("Whisper did not produce token timestamps")
	}

	return segments, tokens, nil
}

func inferDTWPreset(modelPath string) string {
	name := strings.TrimSuffix(filepath.Base(modelPath), filepath.Ext(modelPath))
	name = strings.TrimPrefix(name, "ggml-")
	for _, preset := range []string{
		"tiny.en", "tiny", "base.en", "base", "small.en", "small",
		"medium.en", "medium", "large-v1", "large-v2", "large-v3",
	} {
		if name == preset || strings.HasPrefix(name, preset+"-") {
			return preset
		}
	}

	return ""
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
