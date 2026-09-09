package member

import (
	"context"
	"errors"
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

func (lookup testMemberLookup) AddMember(member discord.Member) {
	lookup[member.User.ID] = member
}

func TestSnapshotDisplayNamesUsesEffectiveNameAndIDFallback(t *testing.T) {
	lookup := testMemberLookup{
		42: {
			Nick: stringPointer(" Mörk\t🐉\n"),
			User: discord.User{ID: 42, Username: "username"},
		},
	}
	names := SnapshotDisplayNames(lookup, 7, []wav.File{
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

func TestResolveDisplayNamesFetchesAndCachesMissingMember(t *testing.T) {
	cache := testMemberLookup{
		42: {User: discord.User{ID: 42, Username: "cached"}},
	}
	var calls []snowflake.ID
	names, failures := ResolveDisplayNames(context.Background(), cache, func(_ context.Context, guildID, userID snowflake.ID) (*discord.Member, error) {
		if guildID != 7 {
			t.Fatalf("guild ID = %v, want 7", guildID)
		}
		calls = append(calls, userID)
		return &discord.Member{Nick: stringPointer(" fetched name "), User: discord.User{ID: userID, Username: "username"}}, nil
	}, 7, []wav.File{{UserID: 42}, {UserID: 99}})

	if len(calls) != 1 || calls[0] != 99 {
		t.Fatalf("fetch calls = %v, want [99]", calls)
	}
	if len(failures) != 0 {
		t.Fatalf("failures = %v, want none", failures)
	}
	if got, want := names["42"], "cached"; got != want {
		t.Fatalf("cached display name = %q, want %q", got, want)
	}
	if got, want := names["99"], "fetched name"; got != want {
		t.Fatalf("fetched display name = %q, want %q", got, want)
	}
	if got, ok := cache.Member(7, 99); !ok || got.EffectiveName() != " fetched name " {
		t.Fatalf("fetched member was not cached: %#v, exists=%t", got, ok)
	}
}

func TestResolveDisplayNamesKeepsIDAfterLookupFailure(t *testing.T) {
	cache := testMemberLookup{}
	wantErr := errors.New("Discord unavailable")
	names, failures := ResolveDisplayNames(context.Background(), cache, func(_ context.Context, _, _ snowflake.ID) (*discord.Member, error) {
		return nil, wantErr
	}, 7, []wav.File{{UserID: 99}})

	if got, want := names["99"], "99"; got != want {
		t.Fatalf("fallback display name = %q, want %q", got, want)
	}
	if len(failures) != 1 || failures[0].UserID != 99 || !errors.Is(failures[0].Err, wantErr) {
		t.Fatalf("failures = %#v, want lookup failure for 99", failures)
	}
}

func stringPointer(value string) *string {
	return &value
}
