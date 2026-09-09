// Package member adapts Discord member and channel data for VexBot metadata.
package member

import (
	"context"
	"fmt"

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

// MemberCache is the portion of the Discord member cache used while resolving
// recording participants.
type MemberCache interface {
	MemberLookup
	AddMember(discord.Member)
}

// MemberFetcher retrieves a guild member when it is absent from the gateway
// cache.
type MemberFetcher func(context.Context, snowflake.ID, snowflake.ID) (*discord.Member, error)

// LookupFailure describes a member that could not be resolved from Discord
// after it was absent from the local cache.
type LookupFailure struct {
	UserID snowflake.ID
	Err    error
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

// ResolveDisplayNames records each participant's effective Discord name. It
// uses the local cache first, then retrieves only missing members from Discord.
// Failed lookups retain the user-ID fallback and are returned for logging.
func ResolveDisplayNames(
	ctx context.Context,
	cache MemberCache,
	fetch MemberFetcher,
	guildID snowflake.ID,
	recordings []wav.File,
) (map[string]string, []LookupFailure) {
	displayNames := make(map[string]string, len(recordings))
	failures := make([]LookupFailure, 0)
	for _, recording := range recordings {
		userID := recording.UserID
		userIDString := userID.String()
		if member, ok := cache.Member(guildID, userID); ok {
			if displayName := speaker.Normalize(member.EffectiveName()); displayName != "" {
				displayNames[userIDString] = displayName
				continue
			}
		}

		member, err := fetch(ctx, guildID, userID)
		if err != nil {
			failures = append(failures, LookupFailure{UserID: userID, Err: err})
			displayNames[userIDString] = userIDString
			continue
		}
		if member == nil {
			failures = append(failures, LookupFailure{UserID: userID, Err: fmt.Errorf("discord returned no member")})
			displayNames[userIDString] = userIDString
			continue
		}

		cache.AddMember(*member)
		if displayName := speaker.Normalize(member.EffectiveName()); displayName != "" {
			displayNames[userIDString] = displayName
			continue
		}
		displayNames[userIDString] = userIDString
	}
	return displayNames, failures
}
