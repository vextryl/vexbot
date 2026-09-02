package discord

import (
	"log/slog"

	"github.com/disgoorg/snowflake/v2"
	"github.com/vextryl/vexbot/internal/session"
)

type transcriptUploader interface {
	Upload(snowflake.ID, string, int) error
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
