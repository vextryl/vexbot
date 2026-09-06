package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"

	"github.com/disgoorg/snowflake/v2"
	discordbot "github.com/vextryl/vexbot/internal/discord"
	"github.com/vextryl/vexbot/internal/recording"
	"github.com/vextryl/vexbot/internal/whisper"
)

const (
	discordTokenEnv   = "DISCORD_TOKEN"
	discordGuildIDEnv = "DISCORD_GUILD_ID"
)

type startupConfig struct {
	token          string
	guildID        string
	botUserID      snowflake.ID
	retentionCount int
	transcriber    whisper.Transcriber
}

type configDependencies struct {
	lookPath func(string) (string, error)
	stat     func(string) (fs.FileInfo, error)
}

func loadStartupConfig(getenv func(string) string) (startupConfig, error) {
	return validateStartupConfig(getenv, configDependencies{
		lookPath: exec.LookPath,
		stat:     os.Stat,
	})
}

func validateStartupConfig(getenv func(string) string, dependencies configDependencies) (startupConfig, error) {
	config := startupConfig{
		token:   strings.TrimSpace(getenv(discordTokenEnv)),
		guildID: strings.TrimSpace(getenv(discordGuildIDEnv)),
	}
	var validationErrors []error

	if config.token == "" {
		validationErrors = append(validationErrors, fmt.Errorf("%s must be set", discordTokenEnv))
	} else if botUserID, err := discordbot.BotUserIDFromToken(config.token); err != nil {
		validationErrors = append(validationErrors, fmt.Errorf("%s is invalid", discordTokenEnv))
	} else {
		config.botUserID = botUserID
	}
	if config.guildID == "" {
		validationErrors = append(validationErrors, fmt.Errorf("%s must be set", discordGuildIDEnv))
	} else if _, err := snowflake.Parse(config.guildID); err != nil {
		validationErrors = append(validationErrors, fmt.Errorf("%s must be a valid Discord ID", discordGuildIDEnv))
	}

	retentionCount, err := recording.RetentionCount(getenv(recording.RetentionCountEnv))
	if err != nil {
		validationErrors = append(validationErrors, err)
	} else {
		config.retentionCount = retentionCount
	}

	whisperConfig, transcriptionEnabled, err := whisper.ValidateConfig(
		whisper.ConfigFromEnvironment(getenv),
		dependencies.lookPath,
		dependencies.stat,
	)
	if err != nil {
		validationErrors = append(validationErrors, err)
	} else if transcriptionEnabled {
		transcriber, err := whisper.NewWhisperTranscriber(whisperConfig)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("configure local transcription: %w", err))
		} else {
			config.transcriber = transcriber
		}
	}

	if len(validationErrors) > 0 {
		return startupConfig{}, formatConfigurationErrors(validationErrors)
	}
	return config, nil
}

func formatConfigurationErrors(validationErrors []error) error {
	lines := make([]string, 0, len(validationErrors))
	for _, err := range validationErrors {
		for line := range strings.SplitSeq(err.Error(), "\n") {
			lines = append(lines, "- "+line)
		}
	}
	return errors.New("invalid startup configuration:\n" + strings.Join(lines, "\n"))
}
