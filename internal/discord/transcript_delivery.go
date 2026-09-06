package discord

import (
	"log/slog"

	"github.com/disgoorg/snowflake/v2"
	"github.com/vextryl/vexbot/internal/session"
)

type transcriptUploader interface {
	Upload(snowflake.ID, snowflake.ID, string, int) error
}

// deliverTranscript uses a normal status message for transcription progress
// and final attachment delivery. If that message cannot be created, it falls
// back to the standalone transcript upload used before progress reporting.
func deliverTranscript(
	job session.TranscriptionJob,
	guildID, channelID, recipientID snowflake.ID,
	status transcriptDeliveryStatus,
	uploader transcriptUploader,
	logger *slog.Logger,
) {
	activeStatus, created := createTranscriptProgressStatus(guildID, channelID, status, logger)
	if !created {
		uploadCompletedTranscript(job.Completion, guildID, channelID, recipientID, uploader, logger)
		return
	}
	activeStatus.recipientID = recipientID

	go func() {
		result, receivedResult := waitForTranscriptionResult(job, activeStatus)
		if !receivedResult {
			logError(logger, "transcription completed without a result",
				slog.String("guild_id", guildID.String()),
				slog.String("channel_id", channelID.String()),
			)
			return
		}
		if result.Err != nil {
			if !activeStatus.terminal {
				activeStatus.update(TranscriptStatusUpdate{Phase: string(session.TranscriptionPhaseFailed), RecipientID: recipientID})
			}
			logError(logger, "local transcription failed",
				"err", result.Err,
				slog.String("guild_id", guildID.String()),
				slog.String("channel_id", channelID.String()),
			)
			return
		}

		if !activeStatus.terminal {
			activeStatus.applyProgress(session.TranscriptionProgress{
				Phase:   session.TranscriptionPhaseComplete,
				Percent: 100,
			})
		}
		if err := status.Complete(channelID, activeStatus.messageID, recipientID, result.TranscriptPath, result.LineCount); err != nil {
			phase := "delivery_failed"
			if _, tooLarge := isTranscriptTooLarge(err); tooLarge {
				phase = "attachment_too_large"
			}
			logTranscriptDeliveryFailure(logger, err, guildID, channelID, result.TranscriptPath)
			activeStatus.update(TranscriptStatusUpdate{Phase: phase, Percent: 100, RecipientID: recipientID})
			return
		}

		if logger != nil {
			logger.Info("uploaded transcript to Discord",
				slog.String("guild_id", guildID.String()),
				slog.String("channel_id", channelID.String()),
				slog.String("transcript_path", result.TranscriptPath),
				slog.Int("line_count", result.LineCount),
			)
		}
	}()
}

func waitForTranscriptionResult(job session.TranscriptionJob, status *activeTranscriptStatus) (session.TranscriptionResult, bool) {
	progress := job.Progress
	completion := job.Completion
	var result session.TranscriptionResult
	receivedResult := false

	for progress != nil || completion != nil {
		select {
		case update, ok := <-progress:
			if !ok {
				progress = nil
				continue
			}
			status.applyProgress(update)
		case value, ok := <-completion:
			if !ok {
				completion = nil
				continue
			}
			result = value
			receivedResult = true
			completion = nil
		}
	}
	return result, receivedResult
}

func uploadCompletedTranscript(
	completion <-chan session.TranscriptionResult,
	guildID, channelID, recipientID snowflake.ID,
	uploader transcriptUploader,
	logger *slog.Logger,
) {
	go func() {
		result, ok := <-completion
		if !ok {
			logError(logger, "transcription completed without a result",
				slog.String("guild_id", guildID.String()),
				slog.String("channel_id", channelID.String()),
			)
			return
		}
		if result.Err != nil {
			logError(logger, "local transcription failed",
				"err", result.Err,
				slog.String("guild_id", guildID.String()),
				slog.String("channel_id", channelID.String()),
			)
			return
		}

		if err := uploader.Upload(channelID, recipientID, result.TranscriptPath, result.LineCount); err != nil {
			logTranscriptDeliveryFailure(logger, err, guildID, channelID, result.TranscriptPath)
			return
		}

		if logger != nil {
			logger.Info("uploaded transcript to Discord",
				slog.String("guild_id", guildID.String()),
				slog.String("channel_id", channelID.String()),
				slog.String("transcript_path", result.TranscriptPath),
				slog.Int("line_count", result.LineCount),
			)
		}
	}()
}

func logTranscriptDeliveryFailure(logger *slog.Logger, err error, guildID, channelID snowflake.ID, transcriptPath string) {
	attributes := []any{
		"err", err,
		slog.String("guild_id", guildID.String()),
		slog.String("channel_id", channelID.String()),
		slog.String("transcript_path", transcriptPath),
	}
	if tooLarge, ok := isTranscriptTooLarge(err); ok {
		attributes = append(attributes,
			slog.Int64("transcript_size_bytes", tooLarge.SizeBytes),
			slog.Int64("upload_limit_bytes", tooLarge.LimitBytes),
		)
	}
	logError(logger, "uploading transcript to Discord", attributes...)
}
