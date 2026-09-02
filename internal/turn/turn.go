package turn

import (
	"sort"
	"time"

	"github.com/vextryl/vexbot/internal/wav"
)

const pauseThreshold = 2 * time.Second

// Turn identifies one continuous speaking turn in both the
// shared session timeline and the speaker's compact WAV recording.
type Turn struct {
	UserID       string
	SessionStart time.Duration
	SessionEnd   time.Duration
	WAVStart     time.Duration
	WAVEnd       time.Duration
}

// Build groups each speaker's nearby recording spans into
// turns. A real-session pause of two seconds or more starts a new turn.
func Build(timeline wav.Timeline) []Turn {
	spansByUser := make(map[string][]wav.Span)
	for _, span := range timeline.Spans {
		spansByUser[span.UserID] = append(spansByUser[span.UserID], span)
	}

	turns := make([]Turn, 0, len(timeline.Spans))
	for userID, spans := range spansByUser {
		sort.Slice(spans, func(i, j int) bool {
			return spans[i].SessionStartMS < spans[j].SessionStartMS
		})

		for _, span := range spans {
			turn := Turn{
				UserID:       userID,
				SessionStart: time.Duration(span.SessionStartMS) * time.Millisecond,
				SessionEnd:   time.Duration(span.SessionEndMS) * time.Millisecond,
				WAVStart:     time.Duration(span.WAVStartMS) * time.Millisecond,
				WAVEnd:       time.Duration(span.WAVEndMS) * time.Millisecond,
			}

			last := len(turns) - 1
			if last >= 0 && turns[last].UserID == turn.UserID &&
				turn.SessionStart-turns[last].SessionEnd < pauseThreshold {
				turns[last].SessionEnd = turn.SessionEnd
				turns[last].WAVEnd = turn.WAVEnd
				continue
			}
			turns = append(turns, turn)
		}
	}

	sort.Slice(turns, func(i, j int) bool {
		if turns[i].SessionStart == turns[j].SessionStart {
			return turns[i].UserID < turns[j].UserID
		}
		return turns[i].SessionStart < turns[j].SessionStart
	})

	return turns
}
