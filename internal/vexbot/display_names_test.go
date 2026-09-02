package vexbot

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/vextryl/vexbot/internal/speaker"
	"github.com/vextryl/vexbot/internal/wav"
)

type testMemberLookup map[snowflake.ID]discord.Member

func (lookup testMemberLookup) Member(_ snowflake.ID, userID snowflake.ID) (discord.Member, bool) {
	member, ok := lookup[userID]
	return member, ok
}

func TestSnapshotDisplayNamesUsesEffectiveNameAndIDFallback(t *testing.T) {
	lookup := testMemberLookup{
		42: {
			Nick: stringPointer(" Mörk\t🐉\n"),
			User: discord.User{ID: 42, Username: "username"},
		},
	}
	names := snapshotDisplayNames(lookup, 7, []wav.File{
		{UserID: 42},
		{UserID: 99},
	})
	if got, want := names["42"], "Mörk 🐉"; got != want {
		t.Fatalf("display name = %q, want %q", got, want)
	}
	if got, want := names["99"], "99"; got != want {
		t.Fatalf("fallback name = %q, want %q", got, want)
	}
}

func TestDisplayNameForUserNormalizesInjectedWhitespace(t *testing.T) {
	if got, want := speaker.Resolve("42", map[string]string{"42": "Alex\n⚔️"}), "Alex ⚔️"; got != want {
		t.Fatalf("display name = %q, want %q", got, want)
	}
}

func stringPointer(value string) *string {
	return &value
}
