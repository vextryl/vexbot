package discord

import (
	"context"
	"log/slog"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/voice"
	"github.com/vextryl/vexbot/internal/recording"
	"github.com/vextryl/vexbot/internal/session"
)

// HandleApplicationCommandInteraction dispatches VexBot slash commands.
func HandleApplicationCommandInteraction(
	event *events.ApplicationCommandInteractionCreate,
	client *bot.Client,
	voiceManager voice.Manager,
	sessions *session.Manager,
	logger *slog.Logger,
) {
	switch event.SlashCommandInteractionData().CommandName() {
	case "ping":
		handlePing(event, logger)

	case "join":
		handleJoin(event, client, voiceManager, sessions, logger)

	case "stop":
		handleStop(event, client, sessions, logger)
	}
}

func handlePing(event *events.ApplicationCommandInteractionCreate, logger *slog.Logger) {
	err := event.CreateMessage(discord.MessageCreate{
		Content: "Pong!",
	})
	if err != nil {
		logError(logger, "responding to ping command", "err", err, slog.String("user_id", event.User().ID.String()))
	}
}

func handleJoin(
	event *events.ApplicationCommandInteractionCreate,
	client *bot.Client,
	voiceManager voice.Manager,
	sessions *session.Manager,
	logger *slog.Logger,
) {
	guildID := event.GuildID()
	if guildID == nil {
		_ = event.CreateMessage(discord.MessageCreate{
			Content: "This command can only be used in a server.",
		})
		return
	}

	user := event.User()
	if !sessions.Reserve(*guildID) {
		_ = event.CreateMessage(discord.MessageCreate{
			Content: "I am already recording or connecting in this server.",
		})
		return
	}

	err := event.CreateMessage(discord.MessageCreate{
		Content: "Attempting to join your voice channel...",
	})
	if err != nil {
		sessions.CancelReservation(*guildID)
		logError(logger, "responding to join command",
			"err", err,
			slog.String("guild_id", guildID.String()),
			slog.String("user_id", user.ID.String()),
		)
		return
	}

	go func() {
		session, voiceChannelName, err := JoinUserVoiceChannel(
			context.Background(),
			voiceManager,
			client.Caches,
			*guildID,
			user.ID,
			event.Channel().ID(),
			logger,
		)
		if err != nil {
			sessions.CancelReservation(*guildID)
			logError(logger, "joining voice channel",
				"err", err,
				slog.String("guild_id", guildID.String()),
				slog.String("user_id", user.ID.String()),
			)
			return
		}

		sessions.Start(session)
		message := "Recording started in #" + voiceChannelName + "."
		if _, err := client.Rest.UpdateInteractionResponse(
			event.ApplicationID(), event.Token(), discord.MessageUpdate{Content: &message},
		); err != nil {
			logError(logger, "updating join command response",
				"err", err,
				slog.String("guild_id", guildID.String()),
				slog.String("user_id", user.ID.String()),
			)
		}
		if logger != nil {
			logger.Info("joined voice channel and started recording",
				slog.String("guild_id", guildID.String()),
				slog.String("user_id", user.ID.String()),
				slog.String("directory", session.RecorderDirectory()),
			)
		}
	}()
}

func handleStop(event *events.ApplicationCommandInteractionCreate, client *bot.Client, sessions *session.Manager, logger *slog.Logger) {
	guildID := event.GuildID()
	if guildID == nil {
		_ = event.CreateMessage(discord.MessageCreate{
			Content: "This command can only be used in a server.",
		})
		return
	}

	stoppedRecording, err := sessions.Stop(context.Background(), *guildID, event.User().ID)
	if err != nil {
		logError(logger, "stopping recording",
			"err", err,
			slog.String("guild_id", guildID.String()),
			slog.String("user_id", event.User().ID.String()),
		)
		_ = event.CreateMessage(discord.MessageCreate{
			Content: "Unable to stop recording: " + err.Error(),
		})
		return
	}
	stoppedRecording.DisplayNames = SnapshotDisplayNames(client.Caches, *guildID, stoppedRecording.Files)
	if renamedFiles, err := recording.RenameFiles(stoppedRecording.Files, stoppedRecording.DisplayNames); err != nil {
		logError(logger, "renaming recording files",
			"err", err,
			slog.String("guild_id", guildID.String()),
			slog.String("directory", stoppedRecording.Directory),
		)
	} else {
		stoppedRecording.Files = renamedFiles
	}

	if logger != nil {
		logger.Info("recording stopped",
			slog.String("guild_id", guildID.String()),
			slog.String("user_id", event.User().ID.String()),
			slog.String("directory", stoppedRecording.Directory),
		)
	}
	message := "Recording stopped and I left the voice channel."
	var transcriptionJob session.TranscriptionJob
	transcriptionStarted := false
	if job, started := sessions.StartTranscription(stoppedRecording); started {
		transcriptionJob = job
		transcriptionStarted = true
		message += " Local transcription has started."
	} else {
		message += " Local transcription is not configured."
	}
	_ = event.CreateMessage(discord.MessageCreate{
		Content: message,
	})
	if transcriptionStarted {
		deliverTranscript(
			transcriptionJob,
			*guildID,
			stoppedRecording.TranscriptChannelID,
			event.User().ID,
			NewTranscriptStatus(client.Rest),
			NewTranscriptUploader(client.Rest),
			logger,
		)
	}
}

func logError(logger *slog.Logger, message string, args ...any) {
	if logger != nil {
		logger.Error(message, args...)
	}
}
