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
	"github.com/vextryl/vexbot/internal/speaker"
	"github.com/vextryl/vexbot/internal/wav"
)

// HandleApplicationCommandInteraction dispatches VexBot slash commands.
func HandleApplicationCommandInteraction(
	ctx context.Context,
	event *events.ApplicationCommandInteractionCreate,
	client *bot.Client,
	voiceManager voice.Manager,
	sessions *session.Manager,
	retentionCount int,
	logger *slog.Logger,
) {
	switch event.SlashCommandInteractionData().CommandName() {
	case "ping":
		handlePing(event, logger)

	case "join":
		handleJoin(ctx, event, client, voiceManager, sessions, retentionCount, logger)

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
	ctx context.Context,
	event *events.ApplicationCommandInteractionCreate,
	client *bot.Client,
	voiceManager voice.Manager,
	sessions *session.Manager,
	retentionCount int,
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
	voiceChannelID, err := UserVoiceChannelID(client.Caches, *guildID, user.ID)
	if err != nil {
		_ = event.CreateMessage(discord.MessageCreate{Content: "You must be in a voice channel to use this command."})
		return
	}
	if !sessions.Reserve(*guildID, voiceChannelID) {
		channelMention := "this server"
		if activeChannelID, ok := sessions.VoiceChannelID(*guildID); ok {
			channelMention = "<#" + activeChannelID.String() + ">"
		}
		_ = event.CreateMessage(discord.MessageCreate{
			Content: "I am already recording or connecting in " + channelMention + ".",
		})
		return
	}
	retention, err := recording.PruneForNewSession(wav.RecordingsDirectory, retentionCount, sessions.TranscribingDirectories())
	if err != nil {
		sessions.CancelReservation(*guildID)
		logError(logger, "maintaining recording storage",
			"err", err,
			slog.String("guild_id", guildID.String()),
			slog.Int("retention_count", retentionCount),
		)
		_ = event.CreateMessage(discord.MessageCreate{Content: "Unable to prepare recording storage. Please check the bot logs."})
		return
	}
	if logger != nil {
		logger.Info("recording storage checked",
			slog.String("guild_id", guildID.String()),
			slog.Int("retention_count", retentionCount),
			slog.Int("completed_sessions", retention.CompletedSessions),
			slog.Int("protected_sessions", retention.ProtectedSessions),
			slog.Int("removed_sessions", len(retention.RemovedDirectories)),
		)
		for _, directory := range retention.RemovedDirectories {
			logger.Info("removed old recording session", slog.String("directory", directory))
		}
	}

	err = event.CreateMessage(discord.MessageCreate{
		Content: "Attempting to join <#" + voiceChannelID.String() + ">...",
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
		session, voiceChannelID, err := JoinUserVoiceChannel(
			ctx,
			voiceManager,
			*guildID,
			user.ID,
			voiceChannelID,
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

		if !sessions.Start(session) {
			_ = session.Abort()
			voiceManager.RemoveConn(*guildID)
			if logger != nil {
				logger.Warn("aborted voice connection during shutdown",
					slog.String("guild_id", guildID.String()),
					slog.String("user_id", user.ID.String()),
				)
			}
			return
		}
		message := "Recording started in <#" + voiceChannelID.String() + ">."
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
	if channel, ok := client.Caches.Channel(stoppedRecording.VoiceChannelID); ok {
		stoppedRecording.TranscriptMetadata.VoiceChannel = channel.Name()
	}
	stoppedRecording.TranscriptMetadata.Participants = transcriptParticipantNames(stoppedRecording.Files, stoppedRecording.DisplayNames)
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
	message := "Recording stopped and I left <#" + stoppedRecording.VoiceChannelID.String() + ">."
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

func transcriptParticipantNames(files []wav.File, displayNames map[string]string) []string {
	participants := make([]string, 0, len(files))
	for _, file := range files {
		userID := file.UserID.String()
		if displayName := speaker.Normalize(displayNames[userID]); displayName != "" && displayName != userID {
			participants = append(participants, displayName)
		}
	}
	return participants
}
