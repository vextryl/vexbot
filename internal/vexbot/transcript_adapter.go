package vexbot

import "github.com/vextryl/vexbot/internal/transcript"

func writeCombinedTranscript(directory string, displayNames map[string]string, results []TurnTranscription) (string, int, error) {
	entries := make([]transcript.Entry, 0, len(results))
	for _, result := range results {
		text := ""
		for _, token := range result.Transcription.Tokens {
			text += token.Text
		}
		entries = append(entries, transcript.Entry{
			UserID:       result.Turn.UserID,
			SessionStart: result.Turn.SessionStart,
			SessionEnd:   result.Turn.SessionEnd,
			Text:         text,
			Err:          result.Err,
		})
	}
	return transcript.Write(directory, displayNames, entries)
}
