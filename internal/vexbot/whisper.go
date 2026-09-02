package vexbot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vextryl/vexbot/internal/wav"
)

const (
	whisperCLIPathEnv   = "WHISPER_CLI_PATH"
	whisperModelPathEnv = "WHISPER_MODEL_PATH"
	whisperDTWPresetEnv = "WHISPER_DTW_PRESET"
	ffmpegPathEnv       = "FFMPEG_PATH"
	whisperLanguageEnv  = "WHISPER_LANGUAGE"
	whisperEndOfTextID  = 50256
)

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

func newWhisperTranscriberFromEnv() (*whisperTranscriber, error) {
	return newWhisperTranscriber(
		os.Getenv(whisperCLIPathEnv),
		os.Getenv(whisperModelPathEnv),
		os.Getenv(whisperDTWPresetEnv),
		os.Getenv(ffmpegPathEnv),
		os.Getenv(whisperLanguageEnv),
	)
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
		return nil, fmt.Errorf("%s and %s must both be set", whisperCLIPathEnv, whisperModelPathEnv)
	}
	if dtwPreset = strings.TrimSpace(dtwPreset); dtwPreset == "" {
		dtwPreset = inferDTWPreset(modelPath)
	}
	if dtwPreset == "" {
		return nil, fmt.Errorf("set %s for Whisper model %q", whisperDTWPresetEnv, filepath.Base(modelPath))
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
