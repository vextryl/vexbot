package session

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/vextryl/vexbot/internal/transcript"
	"github.com/vextryl/vexbot/internal/turn"
	"github.com/vextryl/vexbot/internal/wav"
	"github.com/vextryl/vexbot/internal/whisper"
)

// TranscriptionPhase identifies the current stage of local transcription.
type TranscriptionPhase string

const (
	TranscriptionPhasePreparing    TranscriptionPhase = "preparing"
	TranscriptionPhaseTranscribing TranscriptionPhase = "transcribing"
	TranscriptionPhaseCombining    TranscriptionPhase = "combining"
	TranscriptionPhaseComplete     TranscriptionPhase = "complete"
	TranscriptionPhaseFailed       TranscriptionPhase = "failed"
)

// TranscriptionProgress describes the most recently available state of an
// asynchronous local transcription job.
type TranscriptionProgress struct {
	Phase          TranscriptionPhase
	CompletedTurns int
	TotalTurns     int
	Percent        int
}

// TranscriptionResult reports the outcome of asynchronous local
// transcription. TranscriptPath and LineCount are set only on success.
type TranscriptionResult struct {
	TranscriptPath string
	LineCount      int
	Err            error
}

// TranscriptionJob exposes the progress and eventual result of one local
// transcription job. Progress coalesces stale updates when a consumer falls
// behind, so an unread progress stream never delays transcription.
type TranscriptionJob struct {
	Progress   <-chan TranscriptionProgress
	Completion <-chan TranscriptionResult
}

type transcriptionRunner func(
	context.Context,
	string,
	[]wav.File,
	whisper.Transcriber,
	func(int),
	func(int, int, turn.Result),
) ([]turn.Result, error)

func newTranscriptionRunner() transcriptionRunner { return turn.Transcribe }

// StartTranscription begins asynchronous local transcription. It returns a
// job that reports progress and exactly one completion result, plus false
// when local transcription is unavailable for the recording.
func (m *Manager) StartTranscription(recording StoppedRecording) (TranscriptionJob, bool) {
	if m.transcriber == nil || len(recording.Files) == 0 {
		return TranscriptionJob{}, false
	}

	progress := make(chan TranscriptionProgress, 32)
	completion := make(chan TranscriptionResult, 1)
	publishProgress := func(update TranscriptionProgress) {
		select {
		case progress <- update:
			return
		default:
		}
		<-progress
		progress <- update
	}

	go func() {
		defer close(progress)
		defer close(completion)

		publishProgress(TranscriptionProgress{Phase: TranscriptionPhasePreparing})
		results, err := m.transcribeRecording(recording, publishProgress)
		if err != nil {
			publishProgress(TranscriptionProgress{Phase: TranscriptionPhaseFailed})
			completion <- TranscriptionResult{Err: err}
			return
		}

		publishProgress(TranscriptionProgress{
			Phase:          TranscriptionPhaseCombining,
			CompletedTurns: len(results),
			TotalTurns:     len(results),
			Percent:        95,
		})
		transcriptPath, lineCount, err := transcript.Write(recording.Directory, recording.DisplayNames, results)
		if err != nil {
			err = fmt.Errorf("write combined transcript: %w", err)
			m.logError("writing combined transcript",
				"err", err,
				slog.String("guild_id", recording.GuildID.String()),
				slog.String("directory", recording.Directory),
			)
			publishProgress(TranscriptionProgress{Phase: TranscriptionPhaseFailed})
			completion <- TranscriptionResult{Err: err}
			return
		}

		m.logInfo("combined transcript saved",
			slog.String("guild_id", recording.GuildID.String()),
			slog.String("path", transcriptPath),
			slog.Int("line_count", lineCount),
		)
		publishProgress(TranscriptionProgress{
			Phase:          TranscriptionPhaseComplete,
			CompletedTurns: len(results),
			TotalTurns:     len(results),
			Percent:        100,
		})
		completion <- TranscriptionResult{TranscriptPath: transcriptPath, LineCount: lineCount}
	}()

	return TranscriptionJob{Progress: progress, Completion: completion}, true
}

func (m *Manager) transcribeRecording(recording StoppedRecording, publishProgress func(TranscriptionProgress)) ([]turn.Result, error) {
	results, err := m.transcribe(
		context.Background(),
		recording.Directory,
		recording.Files,
		m.transcriber,
		func(total int) {
			m.logInfo("starting local transcription",
				slog.String("guild_id", recording.GuildID.String()),
				slog.String("directory", recording.Directory),
				slog.Int("total_turns", total),
			)
			publishProgress(TranscriptionProgress{
				Phase:      TranscriptionPhaseTranscribing,
				TotalTurns: total,
			})
		},
		func(completed, total int, result turn.Result) {
			if result.Err != nil {
				m.logError("transcribing turn",
					"err", result.Err,
					slog.String("guild_id", recording.GuildID.String()),
					slog.String("user_id", result.Turn.UserID),
					slog.Int("completed_turns", completed),
					slog.Int("total_turns", total),
				)
			}

			percent := 90
			if total > 0 {
				percent = completed * 90 / total
			}
			publishProgress(TranscriptionProgress{
				Phase:          TranscriptionPhaseTranscribing,
				CompletedTurns: completed,
				TotalTurns:     total,
				Percent:        percent,
			})
		},
	)
	if err != nil {
		err = fmt.Errorf("prepare local transcription: %w", err)
		m.logError("preparing local transcription",
			"err", err,
			slog.String("guild_id", recording.GuildID.String()),
			slog.String("directory", recording.Directory),
		)
		return nil, err
	}

	failed := 0
	for _, result := range results {
		if result.Err != nil {
			failed++
		}
	}
	m.logInfo("local transcription finished",
		slog.String("guild_id", recording.GuildID.String()),
		slog.Int("succeeded_turns", len(results)-failed),
		slog.Int("failed_turns", failed),
	)
	return results, nil
}
