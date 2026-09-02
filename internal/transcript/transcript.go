package transcript

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vextryl/vexbot/internal/speaker"
	"github.com/vextryl/vexbot/internal/turn"
)

const combinedTranscriptFileName = "transcript.txt"

type line struct {
	UserID       string
	SessionStart time.Duration
	SessionEnd   time.Duration
	Text         string
}

func Write(directory string, displayNames map[string]string, results []turn.Result) (string, int, error) {
	lines := buildLines(results)

	var output strings.Builder
	for _, line := range lines {
		fmt.Fprintf(
			&output,
			"[%s] %s: %s\n",
			formatTranscriptTimestamp(line.SessionStart),
			speaker.Resolve(line.UserID, displayNames),
			line.Text,
		)
	}

	path := filepath.Join(directory, combinedTranscriptFileName)
	if err := os.WriteFile(path, []byte(output.String()), 0o644); err != nil {
		return "", 0, fmt.Errorf("write combined transcript: %w", err)
	}

	return path, len(lines), nil
}

func buildLines(results []turn.Result) []line {
	type candidateLine struct {
		line
		blankAudio bool
	}

	candidates := make([]candidateLine, 0, len(results))
	for _, result := range results {
		if result.Err != nil {
			candidates = append(candidates, candidateLine{line: line{
				UserID:       result.Turn.UserID,
				SessionStart: result.Turn.SessionStart,
				SessionEnd:   result.Turn.SessionEnd,
			}})
			continue
		}

		var text strings.Builder
		for _, token := range result.Transcription.Tokens {
			text.WriteString(token.Text)
		}
		value := strings.TrimSpace(text.String())
		if value == "" {
			candidates = append(candidates, candidateLine{line: line{
				UserID:       result.Turn.UserID,
				SessionStart: result.Turn.SessionStart,
				SessionEnd:   result.Turn.SessionEnd,
			}})
			continue
		}
		candidates = append(candidates, candidateLine{
			line: line{
				UserID:       result.Turn.UserID,
				SessionStart: result.Turn.SessionStart,
				SessionEnd:   result.Turn.SessionEnd,
				Text:         value,
			},
			blankAudio: strings.EqualFold(value, "[BLANK_AUDIO]"),
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].SessionStart == candidates[j].SessionStart {
			return candidates[i].UserID < candidates[j].UserID
		}
		return candidates[i].SessionStart < candidates[j].SessionStart
	})

	lines := make([]line, 0, len(candidates))
	for index := 0; index < len(candidates); {
		if !candidates[index].blankAudio {
			if candidates[index].Text != "" {
				lines = append(lines, candidates[index].line)
			}
			index++
			continue
		}

		runEnd := index + 1
		for runEnd < len(candidates) && candidates[runEnd].blankAudio {
			runEnd++
		}
		if runEnd-index > 1 {
			for _, candidate := range candidates[index:runEnd] {
				candidate.Text = "[unintelligible audio]"
				lines = append(lines, candidate.line)
			}
		}
		index = runEnd
	}

	return lines
}

func formatTranscriptTimestamp(timestamp time.Duration) string {
	totalSeconds := int(timestamp / time.Second)
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}
