package discord

import (
	"log/slog"

	"github.com/disgoorg/snowflake/v2"
	"github.com/vextryl/vexbot/internal/session"
)

type transcriptUploader interface {
	Upload(snowflake.ID, string, int) error
}

// deliverTranscript uses a normal status message for transcription progress
// and final attachment delivery. If that message cannot be created, it falls
// back to the standalone transcript upload used before progress reporting.
func deliverTranscript(
	job session.TranscriptionJob,
	guildID, channelID snowflake.ID,
	status transcriptDeliveryStatus,
	uploader transcriptUploader,
	logger *slog.Logger,
) {
	activeStatus, created := createTranscriptProgressStatus(guildID, channelID, status, logger)
	if !created {
		uploadCompletedTranscript(job.Completion, guildID, channelID, uploader, logger)
		return
	}

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
				activeStatus.update(TranscriptStatusUpdate{Phase: string(session.TranscriptionPhaseFailed)})
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
		if err := status.Complete(channelID, activeStatus.messageID, result.TranscriptPath, result.LineCount); err != nil {
			logError(logger, "uploading transcript to Discord",
				"err", err,
				slog.String("guild_id", guildID.String()),
				slog.String("channel_id", channelID.String()),
				slog.String("transcript_path", result.TranscriptPath),
			)
			activeStatus.update(TranscriptStatusUpdate{Phase: "delivery_failed", Percent: 100})
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
	guildID, channelID snowflake.ID,
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

		if err := uploader.Upload(channelID, result.TranscriptPath, result.LineCount); err != nil {
			logError(logger, "uploading transcript to Discord",
				"err", err,
				slog.String("guild_id", guildID.String()),
				slog.String("channel_id", channelID.String()),
				slog.String("transcript_path", result.TranscriptPath),
			)
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
