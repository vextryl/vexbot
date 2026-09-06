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
	discordbot "github.com/vextryl/vexbot/internal/discord"
	"github.com/vextryl/vexbot/internal/recording"
	"github.com/vextryl/vexbot/internal/session"
	"github.com/vextryl/vexbot/internal/whisper"
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

	// parse token
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		logger.Error("Discord token is not configured")
		return
	}

	// parse guild ID
	guildID := os.Getenv("DISCORD_GUILD_ID")
	if guildID == "" {
		logger.Error("Discord guild ID is not configured")
		return
	}

	retentionCount, err := recording.RetentionCount(os.Getenv(recording.RetentionCountEnv))
	if err != nil {
		logger.Error("configuring recording retention", "err", err)
		return
	}

	transcriber, err := whisper.NewWhisperTranscriberFromEnv()
	if err != nil {
		logger.Error("configuring local transcription", "err", err)
		return
	}
	if transcriber == nil {
		logger.Info("local transcription is not configured")
	}

	// disgo requires botUserID for the voice manager
	botUserID, err := discordbot.BotUserIDFromToken(token)
	if err != nil {
		logger.Error("getting bot user ID from token", "err", err)
		return
	}

	// The voice manager needs to update the bot's voice state,
	// but the client needs the voice manager during construction.
	// A closure lets us resolve that dependency after the client exists.
	var client *bot.Client

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
		botUserID,
		logger,
	)
	sessions := session.NewManager(transcriber, logger)

	client, err = disgo.New(
		token,
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
				discordbot.HandleApplicationCommandInteraction(runContext, event, client, voiceManager, sessions, retentionCount, logger)
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

	err = discordbot.RegisterCommands(client, guildID, logger)
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
	shutdown(context.Background(), client.Gateway, cancelRun, sessions, logger)
}

type commandIntake interface {
	Close(context.Context)
}

type recordingFinalizer interface {
	Finalize(context.Context) session.FinalizationResult
}

// shutdown first closes the Discord gateway so no new commands are accepted,
// then finalizes recordings without initiating local transcription.
func shutdown(ctx context.Context, intake commandIntake, cancelRun context.CancelFunc, recordings recordingFinalizer, logger *slog.Logger) session.FinalizationResult {
	if intake != nil {
		intake.Close(ctx)
	}
	if cancelRun != nil {
		cancelRun()
	}
	result := recordings.Finalize(ctx)
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
