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

type transcriptDeliveryStatus interface {
	transcriptStatusMessage
	Complete(snowflake.ID, snowflake.ID, snowflake.ID, string, int) error
}

type activeTranscriptStatus struct {
	guildID       snowflake.ID
	channelID     snowflake.ID
	messageID     snowflake.ID
	status        transcriptStatusMessage
	logger        *slog.Logger
	lastThreshold int
	terminal      bool
	recipientID   snowflake.ID
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
	active, ok := createTranscriptProgressStatus(guildID, channelID, status, logger)
	if !ok {
		return false
	}

	go active.updateProgress(progress)
	return true
}

func createTranscriptProgressStatus(
	guildID, channelID snowflake.ID,
	status transcriptStatusMessage,
	logger *slog.Logger,
) (*activeTranscriptStatus, bool) {
	messageID, err := status.Create(channelID, TranscriptStatusUpdate{Phase: string(session.TranscriptionPhasePreparing)})
	if err != nil {
		logError(logger, "creating transcription status message",
			"err", err,
			slog.String("guild_id", guildID.String()),
			slog.String("channel_id", channelID.String()),
		)
		return nil, false
	}
	return &activeTranscriptStatus{
		guildID:   guildID,
		channelID: channelID,
		messageID: messageID,
		status:    status,
		logger:    logger,
	}, true
}

func (s *activeTranscriptStatus) updateProgress(progress <-chan session.TranscriptionProgress) {
	for update := range progress {
		if s.applyProgress(update) {
			return
		}
	}
}

// applyProgress returns true when the update is terminal and no further
// progress should be rendered for this status message.
func (s *activeTranscriptStatus) applyProgress(update session.TranscriptionProgress) bool {
	if s.terminal {
		return true
	}
	statusUpdate := transcriptStatusUpdateFromProgress(update)
	if update.Phase == session.TranscriptionPhaseFailed {
		statusUpdate.RecipientID = s.recipientID
		s.update(statusUpdate)
		s.terminal = true
		return true
	}

	threshold := transcriptionProgressThreshold(update.Percent)
	if threshold <= s.lastThreshold {
		return false
	}
	statusUpdate.Percent = threshold
	if !s.update(statusUpdate) {
		s.terminal = true
		return true
	}
	s.lastThreshold = threshold
	s.terminal = threshold == 100
	return s.terminal
}

func (s *activeTranscriptStatus) update(update TranscriptStatusUpdate) bool {
	return updateTranscriptStatusMessage(s.status, s.channelID, s.messageID, update, s.guildID, s.logger)
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
