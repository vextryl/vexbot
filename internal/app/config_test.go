package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vextryl/vexbot/internal/recording"
	"github.com/vextryl/vexbot/internal/whisper"
)

func TestValidateStartupConfigAcceptsEnabledLocalTranscription(t *testing.T) {
	modelPath := writeTestModel(t)
	config, err := validateStartupConfig(testEnvironment(map[string]string{
		discordTokenEnv:             "MTIz.NA.signature",
		discordGuildIDEnv:           "456",
		whisper.WhisperCLIPathEnv:   "whisper-cli",
		whisper.WhisperModelPathEnv: modelPath,
	}), testConfigDependencies(t, modelPath))
	if err != nil {
		t.Fatalf("validateStartupConfig() error = %v", err)
	}
	if config.botUserID.String() != "123" || config.retentionCount != recording.DefaultRetentionCount || config.transcriber == nil {
		t.Fatalf("startup config = %#v, want parsed Discord and Whisper configuration", config)
	}
}

func TestValidateStartupConfigAllowsRecordingOnlyMode(t *testing.T) {
	config, err := validateStartupConfig(testEnvironment(map[string]string{
		discordTokenEnv:   "MTIz.NA.signature",
		discordGuildIDEnv: "456",
	}), testConfigDependencies(t, ""))
	if err != nil {
		t.Fatalf("validateStartupConfig() error = %v", err)
	}
	if config.transcriber != nil {
		t.Fatalf("transcriber = %#v, want nil when Whisper is unset", config.transcriber)
	}
}

func TestValidateStartupConfigReportsPartialWhisperConfiguration(t *testing.T) {
	_, err := validateStartupConfig(testEnvironment(map[string]string{
		discordTokenEnv:           "MTIz.NA.signature",
		discordGuildIDEnv:         "456",
		whisper.WhisperCLIPathEnv: "whisper-cli",
	}), testConfigDependencies(t, ""))
	if err == nil || !strings.Contains(err.Error(), "WHISPER_CLI_PATH and WHISPER_MODEL_PATH must both be set") {
		t.Fatalf("validateStartupConfig() error = %v, want partial Whisper configuration error", err)
	}
}

func TestValidateStartupConfigReportsMissingModel(t *testing.T) {
	missingModel := filepath.Join(t.TempDir(), "missing.bin")
	_, err := validateStartupConfig(testEnvironment(map[string]string{
		discordTokenEnv:             "MTIz.NA.signature",
		discordGuildIDEnv:           "456",
		whisper.WhisperCLIPathEnv:   "whisper-cli",
		whisper.WhisperModelPathEnv: missingModel,
		whisper.WhisperDTWPresetEnv: "base.en",
	}), testConfigDependencies(t, missingModel))
	if err == nil || !strings.Contains(err.Error(), "inspect WHISPER_MODEL_PATH") {
		t.Fatalf("validateStartupConfig() error = %v, want missing-model error", err)
	}
}

func TestValidateStartupConfigRejectsModelDirectory(t *testing.T) {
	modelDirectory := t.TempDir()
	_, err := validateStartupConfig(testEnvironment(map[string]string{
		discordTokenEnv:             "MTIz.NA.signature",
		discordGuildIDEnv:           "456",
		whisper.WhisperCLIPathEnv:   "whisper-cli",
		whisper.WhisperModelPathEnv: modelDirectory,
		whisper.WhisperDTWPresetEnv: "base.en",
	}), testConfigDependencies(t, modelDirectory))
	if err == nil || !strings.Contains(err.Error(), "WHISPER_MODEL_PATH must be a regular file") {
		t.Fatalf("validateStartupConfig() error = %v, want model regular-file error", err)
	}
}

func TestValidateStartupConfigReportsInvalidExecutablePaths(t *testing.T) {
	modelPath := writeTestModel(t)
	dependencies := configDependencies{
		lookPath: func(path string) (string, error) { return "", errors.New(path + " not found") },
		stat:     os.Stat,
	}
	_, err := validateStartupConfig(testEnvironment(map[string]string{
		discordTokenEnv:             "MTIz.NA.signature",
		discordGuildIDEnv:           "456",
		whisper.WhisperCLIPathEnv:   "missing-whisper",
		whisper.WhisperModelPathEnv: modelPath,
		whisper.FFMPEGPathEnv:       "missing-ffmpeg",
	}), dependencies)
	if err == nil || !strings.Contains(err.Error(), "resolve WHISPER_CLI_PATH") || !strings.Contains(err.Error(), "resolve FFMPEG_PATH") {
		t.Fatalf("validateStartupConfig() error = %v, want executable path errors", err)
	}
}

func TestValidateStartupConfigReportsAllProblemsWithoutTokenValue(t *testing.T) {
	secret := "not-a-real-token"
	_, err := validateStartupConfig(testEnvironment(map[string]string{
		discordTokenEnv:             secret,
		discordGuildIDEnv:           "not-a-snowflake",
		recording.RetentionCountEnv: "0",
		whisper.WhisperCLIPathEnv:   "missing-whisper",
		whisper.WhisperModelPathEnv: "missing-model.bin",
		whisper.WhisperDTWPresetEnv: "base.en",
		whisper.FFMPEGPathEnv:       "missing-ffmpeg",
	}), configDependencies{
		lookPath: func(path string) (string, error) { return "", errors.New(path + " not found") },
		stat:     os.Stat,
	})
	if err == nil {
		t.Fatal("validateStartupConfig() error = nil, want aggregate configuration error")
	}
	for _, want := range []string{"DISCORD_TOKEN is invalid", "DISCORD_GUILD_ID must be a valid Discord ID", recording.RetentionCountEnv, "resolve WHISPER_CLI_PATH", "inspect WHISPER_MODEL_PATH", "resolve FFMPEG_PATH"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("configuration error %q does not contain %q", err, want)
		}
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("configuration error exposed token value: %q", err)
	}
}

func testEnvironment(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func testConfigDependencies(t *testing.T, modelPath string) configDependencies {
	t.Helper()
	return configDependencies{
		lookPath: func(path string) (string, error) { return path, nil },
		stat: func(path string) (os.FileInfo, error) {
			if path != modelPath {
				return nil, os.ErrNotExist
			}
			return os.Stat(path)
		},
	}
}

func writeTestModel(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ggml-base.en.bin")
	if err := os.WriteFile(path, []byte("model"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
