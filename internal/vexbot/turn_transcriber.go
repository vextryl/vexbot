package vexbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type TurnTranscription struct {
	Turn          transcriptionTurn
	Transcription Transcription
	Err           error
}

// transcribeRecordingTurns extracts and transcribes each speaker turn. Its
// returned transcriptions contain parsed Whisper data only: the temporary
// text and JSON sidecar paths are removed before this function returns.
func transcribeRecordingTurns(
	ctx context.Context,
	directory string,
	recordings []RecordingFile,
	transcriber Transcriber,
	onStart func(total int),
	onProgress func(completed, total int, result TurnTranscription),
) ([]TurnTranscription, error) {
	contents, err := os.ReadFile(filepath.Join(directory, timelineFileName))
	if err != nil {
		return nil, fmt.Errorf("read session timeline: %w", err)
	}

	var timeline sessionTimeline
	if err := json.Unmarshal(contents, &timeline); err != nil {
		return nil, fmt.Errorf("parse session timeline: %w", err)
	}
	if timeline.Version != 1 {
		return nil, fmt.Errorf("unsupported session timeline version %d", timeline.Version)
	}

	recordingsByUser := make(map[string]RecordingFile, len(recordings))
	for _, recording := range recordings {
		recordingsByUser[recording.UserID.String()] = recording
	}

	temporaryDir, err := os.MkdirTemp(directory, ".transcription-tmp-")
	if err != nil {
		return nil, fmt.Errorf("create temporary transcription directory: %w", err)
	}
	defer os.RemoveAll(temporaryDir)

	turns := buildTranscriptionTurns(timeline)
	if onStart != nil {
		onStart(len(turns))
	}
	results := make([]TurnTranscription, 0, len(turns))
	complete := func(result TurnTranscription) {
		results = append(results, result)
		if onProgress != nil {
			onProgress(len(results), len(turns), result)
		}
	}
	for _, turn := range turns {
		result := TurnTranscription{Turn: turn}
		recording, ok := recordingsByUser[turn.UserID]
		if !ok {
			result.Err = fmt.Errorf("find recording for user %s", turn.UserID)
			complete(result)
			continue
		}

		temporaryWAV, err := extractTurnWAV(temporaryDir, recording.Path, turn)
		if err != nil {
			result.Err = fmt.Errorf("extract audio: %w", err)
			complete(result)
			continue
		}

		transcription, err := transcriber.Transcribe(ctx, RecordingFile{
			UserID: recording.UserID,
			Path:   temporaryWAV,
		})
		cleanupErr := removeTemporaryTurnFiles(temporaryWAV)
		if err != nil {
			if cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("clean up temporary turn files: %w", cleanupErr))
			}
			result.Err = err
			complete(result)
			continue
		}
		if cleanupErr != nil {
			result.Err = fmt.Errorf("clean up temporary turn files: %w", cleanupErr)
			complete(result)
			continue
		}
		transcription.TextPath = ""
		transcription.JSONPath = ""
		result.Transcription = transcription
		complete(result)
	}

	return results, nil
}

func removeTemporaryTurnFiles(wavPath string) error {
	base := wavPath[:len(wavPath)-len(filepath.Ext(wavPath))]
	var cleanupErr error
	for _, path := range []string{wavPath, base + ".txt", base + ".json"} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove %s: %w", path, err))
		}
	}
	return cleanupErr
}
