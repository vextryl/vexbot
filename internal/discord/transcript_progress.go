package discord

import (
	"log/slog"

	"github.com/disgoorg/snowflake/v2"
	"github.com/vextryl/vexbot/internal/session"
)

type transcriptStatusMessage interface {
	Create(snowflake.ID, TranscriptStatusUpdate) (snowflake.ID, error)
	Update(snowflake.ID, snowflake.ID, TranscriptStatusUpdate) error
}

// startTranscriptProgressStatus creates a normal status message and updates it
// asynchronously from session transcription progress. It returns false only
// when the initial message could not be created.
func startTranscriptProgressStatus(
	progress <-chan session.TranscriptionProgress,
	guildID, channelID snowflake.ID,
	status transcriptStatusMessage,
	logger *slog.Logger,
) bool {
	messageID, err := status.Create(channelID, TranscriptStatusUpdate{Phase: string(session.TranscriptionPhasePreparing)})
	if err != nil {
		logError(logger, "creating transcription status message",
			"err", err,
			slog.String("guild_id", guildID.String()),
			slog.String("channel_id", channelID.String()),
		)
		return false
	}

	go updateTranscriptProgressStatus(progress, guildID, channelID, messageID, status, logger)
	return true
}

func updateTranscriptProgressStatus(
	progress <-chan session.TranscriptionProgress,
	guildID, channelID, messageID snowflake.ID,
	status transcriptStatusMessage,
	logger *slog.Logger,
) {
	lastThreshold := 0
	for update := range progress {
		statusUpdate := transcriptStatusUpdateFromProgress(update)
		if update.Phase == session.TranscriptionPhaseFailed {
			updateTranscriptStatusMessage(status, channelID, messageID, statusUpdate, guildID, logger)
			return
		}

		threshold := transcriptionProgressThreshold(update.Percent)
		if threshold <= lastThreshold {
			continue
		}
		statusUpdate.Percent = threshold
		if !updateTranscriptStatusMessage(status, channelID, messageID, statusUpdate, guildID, logger) {
			return
		}
		lastThreshold = threshold
		if threshold == 100 {
			return
		}
	}
}

func transcriptStatusUpdateFromProgress(progress session.TranscriptionProgress) TranscriptStatusUpdate {
	return TranscriptStatusUpdate{
		Phase:          string(progress.Phase),
		CompletedTurns: progress.CompletedTurns,
		TotalTurns:     progress.TotalTurns,
		Percent:        progress.Percent,
	}
}

func transcriptionProgressThreshold(percent int) int {
	percent = normalizeProgressPercent(percent)
	return percent / 10 * 10
}

func updateTranscriptStatusMessage(
	status transcriptStatusMessage,
	channelID, messageID snowflake.ID,
	update TranscriptStatusUpdate,
	guildID snowflake.ID,
	logger *slog.Logger,
) bool {
	if err := status.Update(channelID, messageID, update); err != nil {
		logError(logger, "updating transcription status message",
			"err", err,
			slog.String("guild_id", guildID.String()),
			slog.String("channel_id", channelID.String()),
			slog.String("message_id", messageID.String()),
		)
		return false
	}
	return true
}
