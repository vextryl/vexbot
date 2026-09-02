package main

import (
	"strings"
	"unicode"

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
			if displayName := normalizeDisplayName(member.EffectiveName()); displayName != "" {
				displayNames[userID] = displayName
				continue
			}
		}
		displayNames[userID] = userID
	}
	return displayNames
}

func displayNameForUser(userID string, displayNames map[string]string) string {
	if displayName := normalizeDisplayName(displayNames[userID]); displayName != "" {
		return displayName
	}
	return userID
}

func normalizeDisplayName(displayName string) string {
	return strings.Join(strings.FieldsFunc(displayName, func(character rune) bool {
		return unicode.IsSpace(character) || unicode.IsControl(character)
	}), " ")
}
