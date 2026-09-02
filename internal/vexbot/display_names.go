package vexbot

import (
	"github.com/vextryl/vexbot/internal/speaker"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

type memberLookup interface {
	Member(snowflake.ID, snowflake.ID) (discord.Member, bool)
}

func snapshotDisplayNames(lookup memberLookup, guildID snowflake.ID, recordings []RecordingFile) map[string]string {
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
