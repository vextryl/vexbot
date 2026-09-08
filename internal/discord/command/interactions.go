package command

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/voice"
	"github.com/vextryl/vexbot/internal/dave"
	"github.com/vextryl/vexbot/internal/discord/delivery"
	"github.com/vextryl/vexbot/internal/discord/member"
	discordvoice "github.com/vextryl/vexbot/internal/discord/voice"
	"github.com/vextryl/vexbot/internal/recording"
	"github.com/vextryl/vexbot/internal/session"
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
	uploadLimit int64,
	failures *dave.DecryptFailureCounter,
	logger *slog.Logger,
) {
	switch event.SlashCommandInteractionData().CommandName() {
	case "ping":
		handlePing(event, logger)

	case "join":
		handleJoin(ctx, event, client, voiceManager, sessions, retentionCount, failures, logger)

	case "stop":
		handleStop(event, client, sessions, uploadLimit, failures, logger)

	case "status":
		handleStatus(event, sessions, logger)
	}
}

func handleStatus(event *events.ApplicationCommandInteractionCreate, sessions *session.Manager, logger *slog.Logger) {
	guildID := event.GuildID()
	if guildID == nil {
		_ = event.CreateMessage(discord.MessageCreate{Content: "This command can only be used in a server."})
		return
	}

	status := sessions.Status(*guildID)
	message := formatRecordingStatus(status)
	if err := event.CreateMessage(discord.MessageCreate{Content: message}); err != nil {
		logError(logger, "responding to status command", "err", err, slog.String("guild_id", guildID.String()))
	}
}

func formatRecordingStatus(status session.RecordingStatus) string {
	var message string
	switch status.State {
	case session.RecordingStateConnecting:
		message = "Connecting to <#" + status.VoiceChannelID.String() + ">..."
	case session.RecordingStateRecording:
		message = fmt.Sprintf(
			"Recording in <#%s> for %s. Captured audio from %d speaker(s).",
			status.VoiceChannelID,
			formatStatusElapsed(status.Elapsed),
			status.SpeakerCount,
		)
	default:
		message = "No active recording in this server."
	}
	return message
}

func formatStatusElapsed(elapsed time.Duration) string {
	if elapsed < time.Second {
		return "0s"
	}
	return elapsed.Round(time.Second).String()
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
	failures *dave.DecryptFailureCounter,
	logger *slog.Logger,
) {
	// Acknowledge before cache lookups, storage maintenance, or voice setup.
	ackStarted := time.Now()
	if err := event.DeferCreateMessage(false); err != nil {
		logError(logger, "acknowledging join command", "err", err,
			slog.Duration("ack_duration", time.Since(ackStarted)))
		return
	}
	if logger != nil {
		logger.Info("join command acknowledged", slog.Duration("ack_duration", time.Since(ackStarted)))
	}
	respond := func(message string) {
		if _, err := client.Rest.UpdateInteractionResponse(
			event.ApplicationID(), event.Token(), discord.MessageUpdate{Content: &message},
		); err != nil {
			logError(logger, "updating join command response", "err", err)
		}
	}
	guildID := event.GuildID()
	if guildID == nil {
		respond("This command can only be used in a server.")
		return
	}

	user := event.User()
	voiceChannelID, err := discordvoice.UserVoiceChannelID(client.Caches, *guildID, user.ID)
	if err != nil {
		respond("You must be in a voice channel to use this command.")
		return
	}
	if !sessions.Reserve(*guildID, voiceChannelID) {
		channelMention := "this server"
		if activeChannelID, ok := sessions.VoiceChannelID(*guildID); ok {
			channelMention = "<#" + activeChannelID.String() + ">"
		}
		respond("I am already recording or connecting in " + channelMention + ".")
		return
	}
	storageStarted := time.Now()
	retention, err := recording.PruneForNewSession(wav.RecordingsDirectory, retentionCount, sessions.TranscribingDirectories())
	if err != nil {
		sessions.CancelReservation(*guildID)
		logError(logger, "maintaining recording storage",
			"err", err,
			slog.String("guild_id", guildID.String()),
			slog.Int("retention_count", retentionCount),
		)
		respond("Unable to prepare recording storage. Please check the bot logs.")
		return
	}
	if logger != nil {
		logger.Info("recording storage checked",
			slog.Duration("storage_duration", time.Since(storageStarted)),
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

	respond("Attempting to join <#" + voiceChannelID.String() + ">...")

	go func() {
		joinStarted := time.Now()
		session, voiceChannelID, err := discordvoice.JoinUserVoiceChannel(
			ctx,
			voiceManager,
			*guildID,
			user.ID,
			voiceChannelID,
			event.Channel().ID(),
			failures,
			logger,
		)
		if err != nil {
			sessions.CancelReservation(*guildID)
			respond("Unable to join the voice channel. Please check the bot logs.")
			logError(logger, "joining voice channel",
				slog.Duration("voice_join_duration", time.Since(joinStarted)),
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
		respond("Recording started in <#" + voiceChannelID.String() + ">.")
		if logger != nil {
			logger.Info("joined voice channel and started recording",
				slog.Duration("voice_join_duration", time.Since(joinStarted)),
				slog.String("guild_id", guildID.String()),
				slog.String("user_id", user.ID.String()),
				slog.String("directory", session.RecorderDirectory()),
			)
		}
	}()
}

func handleStop(event *events.ApplicationCommandInteractionCreate, client *bot.Client, sessions *session.Manager, uploadLimit int64, failures *dave.DecryptFailureCounter, logger *slog.Logger) {
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
	stoppedRecording.DisplayNames = member.SnapshotDisplayNames(client.Caches, *guildID, stoppedRecording.Files)
	if channel, ok := client.Caches.Channel(stoppedRecording.VoiceChannelID); ok {
		stoppedRecording.TranscriptMetadata.VoiceChannel = channel.Name()
	}
	stoppedRecording.TranscriptMetadata.Participants = member.TranscriptParticipantNames(stoppedRecording.Files, stoppedRecording.DisplayNames)
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
	dave.LogRecordingSummary(logger, failures, *guildID, stoppedRecording.Directory)
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
		delivery.StartTranscriptDelivery(
			transcriptionJob,
			*guildID,
			stoppedRecording.TranscriptChannelID,
			event.User().ID,
			delivery.NewTranscriptStatus(client.Rest, uploadLimit),
			delivery.NewUploader(client.Rest, uploadLimit),
			logger,
		)
	}
}

func logError(logger *slog.Logger, message string, args ...any) {
	if logger != nil {
		logger.Error(message, args...)
	}
}
