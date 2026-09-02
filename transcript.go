package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	combinedTranscriptFileName  = "transcript.txt"
	transcriptContinuationLimit = 5 * time.Second
)

type transcriptLine struct {
	UserID       string
	SessionStart time.Duration
	SessionEnd   time.Duration
	Text         string
}

func writeCombinedTranscript(directory string, transcriptions map[string]Transcription) (string, int, error) {
	contents, err := os.ReadFile(filepath.Join(directory, timelineFileName))
	if err != nil {
		return "", 0, fmt.Errorf("read session timeline: %w", err)
	}

	var timeline sessionTimeline
	if err := json.Unmarshal(contents, &timeline); err != nil {
		return "", 0, fmt.Errorf("parse session timeline: %w", err)
	}
	if timeline.Version != 1 {
		return "", 0, fmt.Errorf("unsupported session timeline version %d", timeline.Version)
	}

	lines, err := buildTranscriptLines(timeline, transcriptions)
	if err != nil {
		return "", 0, err
	}

	var output strings.Builder
	for _, line := range lines {
		fmt.Fprintf(
			&output,
			"[%s] %s: %s\n",
			formatTranscriptTimestamp(line.SessionStart),
			line.UserID,
			line.Text,
		)
	}

	path := filepath.Join(directory, combinedTranscriptFileName)
	if err := os.WriteFile(path, []byte(output.String()), 0o644); err != nil {
		return "", 0, fmt.Errorf("write combined transcript: %w", err)
	}

	return path, len(lines), nil
}

func buildTranscriptLines(timeline sessionTimeline, transcriptions map[string]Transcription) ([]transcriptLine, error) {
	spansByUser := make(map[string][]timelineSpan)
	for _, span := range timeline.Spans {
		spansByUser[span.UserID] = append(spansByUser[span.UserID], span)
	}
	for _, spans := range spansByUser {
		sort.Slice(spans, func(i, j int) bool {
			return spans[i].WAVStartMS < spans[j].WAVStartMS
		})
	}

	type lineKey struct {
		userID    string
		spanIndex int
	}
	lineText := make(map[lineKey]*strings.Builder)
	lineStarts := make(map[lineKey]time.Duration)
	lineEnds := make(map[lineKey]time.Duration)

	for userID, transcription := range transcriptions {
		spans := spansByUser[userID]
		for tokenIndex, token := range transcription.Tokens {
			spanIndex, ok := findTimelineSpan(spans, token.WAVStart)
			if !ok {
				return nil, fmt.Errorf("map token %d for user %s to session timeline", tokenIndex, userID)
			}

			span := spans[spanIndex]
			key := lineKey{userID: userID, spanIndex: spanIndex}
			if lineText[key] == nil {
				lineText[key] = &strings.Builder{}
				lineStarts[key] = time.Duration(span.SessionStartMS)*time.Millisecond +
					(token.WAVStart - time.Duration(span.WAVStartMS)*time.Millisecond)
				lineEnds[key] = time.Duration(span.SessionEndMS) * time.Millisecond
			}
			lineText[key].WriteString(token.Text)
		}
	}

	lines := make([]transcriptLine, 0, len(lineText))
	for key, text := range lineText {
		if value := strings.TrimSpace(text.String()); value != "" {
			lines = append(lines, transcriptLine{
				UserID:       key.userID,
				SessionStart: lineStarts[key],
				SessionEnd:   lineEnds[key],
				Text:         value,
			})
		}
	}
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].SessionStart == lines[j].SessionStart {
			return lines[i].UserID < lines[j].UserID
		}
		return lines[i].SessionStart < lines[j].SessionStart
	})

	return mergeTranscriptContinuations(lines), nil
}

func mergeTranscriptContinuations(lines []transcriptLine) []transcriptLine {
	linesByUser := make(map[string][]transcriptLine)
	for _, line := range lines {
		linesByUser[line.UserID] = append(linesByUser[line.UserID], line)
	}

	merged := make([]transcriptLine, 0, len(lines))
	for _, speakerLines := range linesByUser {
		sort.Slice(speakerLines, func(i, j int) bool {
			return speakerLines[i].SessionStart < speakerLines[j].SessionStart
		})

		for _, line := range speakerLines {
			last := len(merged) - 1
			if last >= 0 && canContinueTranscriptLine(merged[last], line) {
				merged[last].Text = joinTranscriptText(merged[last].Text, line.Text)
				merged[last].SessionEnd = line.SessionEnd
				continue
			}
			merged = append(merged, line)
		}
	}

	sort.Slice(merged, func(i, j int) bool {
		if merged[i].SessionStart == merged[j].SessionStart {
			return merged[i].UserID < merged[j].UserID
		}
		return merged[i].SessionStart < merged[j].SessionStart
	})

	return merged
}

func canContinueTranscriptLine(first, second transcriptLine) bool {
	return first.UserID == second.UserID &&
		!endsTranscriptSentence(first.Text) &&
		second.SessionStart-first.SessionEnd <= transcriptContinuationLimit
}

func endsTranscriptSentence(text string) bool {
	return strings.HasSuffix(strings.TrimSpace(text), ".") ||
		strings.HasSuffix(strings.TrimSpace(text), "!") ||
		strings.HasSuffix(strings.TrimSpace(text), "?")
}

func joinTranscriptText(first, second string) string {
	second = strings.TrimSpace(second)
	if second == "" {
		return first
	}
	if strings.HasPrefix(second, ".") || strings.HasPrefix(second, ",") ||
		strings.HasPrefix(second, "!") || strings.HasPrefix(second, "?") ||
		strings.HasPrefix(second, ";") || strings.HasPrefix(second, ":") ||
		strings.HasPrefix(second, "'") {
		return first + second
	}
	return first + " " + second
}

func findTimelineSpan(spans []timelineSpan, wavTime time.Duration) (int, bool) {
	for index, span := range spans {
		start := time.Duration(span.WAVStartMS) * time.Millisecond
		end := time.Duration(span.WAVEndMS) * time.Millisecond
		if wavTime >= start && wavTime < end {
			return index, true
		}
	}

	return 0, false
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
