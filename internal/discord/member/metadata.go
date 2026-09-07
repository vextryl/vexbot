package member

import (
	"github.com/vextryl/vexbot/internal/speaker"
	"github.com/vextryl/vexbot/internal/wav"
)

// TranscriptParticipantNames returns known participant display names for a
// transcript header without exposing fallback Discord user IDs.
func TranscriptParticipantNames(files []wav.File, displayNames map[string]string) []string {
	participants := make([]string, 0, len(files))
	for _, file := range files {
		userID := file.UserID.String()
		if displayName := speaker.Normalize(displayNames[userID]); displayName != "" && displayName != userID {
			participants = append(participants, displayName)
		}
	}
	return participants
}
