// Package app assembles VexBot's runtime dependencies and runs the bot.
package app

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/snowflake/v2"
	"github.com/joho/godotenv"
	"github.com/vextryl/vexbot/internal/dave"
	discordbot "github.com/vextryl/vexbot/internal/discord"
	"github.com/vextryl/vexbot/internal/session"
)

func Run() {
	logger := newLogger(os.Stdout)
	runContext, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()

	// load discord token from .env file
	err := godotenv.Load()
	if err != nil {
		logger.Warn("loading .env file", "err", err)
	}

	config, err := loadStartupConfig(os.Getenv)
	if err != nil {
		logger.Error("startup configuration is invalid", "err", err)
		return
	}
	if config.transcriber == nil {
		logger.Info("local transcription is not configured")
	}

	// The voice manager needs to update the bot's voice state,
	// but the client needs the voice manager during construction.
	// A closure lets us resolve that dependency after the client exists.
	var client *bot.Client

	daveFailures := dave.NewDecryptFailureCounter()
	voiceManager := discordbot.NewVoiceManager(
		func(
			ctx context.Context,
			guildID snowflake.ID,
			channelID *snowflake.ID,
			selfMute bool,
			selfDeaf bool,
		) error {
			return client.UpdateVoiceState(
				ctx,
				guildID,
				channelID,
				selfMute,
				selfDeaf,
			)
		},
		config.botUserID,
		logger,
		daveFailures,
	)
	sessions := session.NewManager(config.transcriber, logger)

	client, err = disgo.New(
		config.token,
		bot.WithDefaultGateway(),
		bot.WithVoiceManager(voiceManager),
		bot.WithGatewayConfigOpts(
			gateway.WithIntents(
				gateway.IntentGuilds,
				gateway.IntentGuildVoiceStates,
			),
		),
		bot.WithCacheConfigOpts(
			cache.WithCaches(
				cache.FlagGuilds,
				cache.FlagMembers,
				cache.FlagChannels,
				cache.FlagVoiceStates,
			),
		),
	)
	if err != nil {
		logger.Error("creating Discord client", "err", err)
		return
	}

	// Close REST, voice, and gateway resources after the explicit shutdown
	// sequence below has finalized any active recording files.
	defer client.Close(context.Background())

	// add event listeners
	client.AddEventListeners(
		&events.ListenerAdapter{
			OnApplicationCommandInteraction: func(event *events.ApplicationCommandInteractionCreate) {
				discordbot.HandleApplicationCommandInteraction(runContext, event, client, voiceManager, sessions, config.retentionCount, daveFailures, logger)
			},
			OnGuildVoiceStateUpdate: func(event *events.GuildVoiceStateUpdate) {
				attributes := []any{
					slog.String("guild_id", event.VoiceState.GuildID.String()),
					slog.String("user_id", event.VoiceState.UserID.String()),
				}
				if event.VoiceState.ChannelID != nil {
					attributes = append(attributes, slog.String("channel_id", event.VoiceState.ChannelID.String()))
				}
				logger.Debug("voice state updated", attributes...)
			},
			OnVoiceServerUpdate: func(event *events.VoiceServerUpdate) {
				logger.Debug("voice server updated", slog.String("guild_id", event.GuildID.String()))
			},
		},
	)

	// open the Discord gateway to start receiving events
	err = client.OpenGateway(context.Background())
	if err != nil {
		logger.Error("opening Discord gateway", "err", err)
		return
	}

	// status
	logger.Info("connected to Discord")

	err = discordbot.RegisterCommands(client, config.guildID, logger)
	if err != nil {
		logger.Error("registering commands", "err", err)
		return
	}

	// Wait for a termination signal to gracefully shut down the bot
	// otherwise program will exit immediately
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)
	<-stop
	logger.Info("received shutdown signal")
	shutdown(context.Background(), client.Gateway, cancelRun, sessions, daveFailures, logger)
}

type commandIntake interface {
	Close(context.Context)
}

type recordingFinalizer interface {
	Finalize(context.Context) session.FinalizationResult
}

// shutdown first closes the Discord gateway so no new commands are accepted,
// then finalizes recordings without initiating local transcription.
func shutdown(ctx context.Context, intake commandIntake, cancelRun context.CancelFunc, recordings recordingFinalizer, daveFailures *dave.DecryptFailureCounter, logger *slog.Logger) session.FinalizationResult {
	if intake != nil {
		intake.Close(ctx)
	}
	if cancelRun != nil {
		cancelRun()
	}
	result := recordings.Finalize(ctx)
	for _, recording := range result.Recordings {
		dave.LogRecordingSummary(logger, daveFailures, recording.GuildID, recording.Directory)
	}
	if logger != nil {
		logger.Info("shutdown recording summary",
			slog.Int("finalized_sessions", result.FinalizedSessions),
			slog.Int("failed_sessions", result.FailedSessions),
		)
		if result.Err != nil {
			logger.Error("finalizing recordings during shutdown", "err", result.Err)
		}
	}
	return result
}

func newLogger(output io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}
