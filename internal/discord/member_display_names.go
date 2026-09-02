// Package discord contains helpers that adapt Discord-specific data for VexBot.
package discord

import (
	"github.com/vextryl/vexbot/internal/speaker"
	"github.com/vextryl/vexbot/internal/wav"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

// MemberLookup provides the Discord member cache needed to snapshot speaker
// names when recording stops.
type MemberLookup interface {
	Member(snowflake.ID, snowflake.ID) (discord.Member, bool)
}

// SnapshotDisplayNames records each participant's effective Discord name so
// later transcription does not depend on live Discord state.
func SnapshotDisplayNames(lookup MemberLookup, guildID snowflake.ID, recordings []wav.File) map[string]string {
	displayNames := make(map[string]string, len(recordings))
	for _, recording := range recordings {
		userID := recording.UserID.String()
		if member, ok := lookup.Member(guildID, recording.UserID); ok {
			if displayName := speaker.Normalize(member.EffectiveName()); displayName != "" {
				displayNames[userID] = displayName
				continue
			}
		}
		displayNames[userID] = userID
	}
	return displayNames
}
