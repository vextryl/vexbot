package discord

import (
	"reflect"
	"testing"

	"github.com/disgoorg/snowflake/v2"
	"github.com/vextryl/vexbot/internal/wav"
)

func TestTranscriptParticipantNamesExcludesFallbackUserIDs(t *testing.T) {
	files := []wav.File{{UserID: snowflake.ID(42)}, {UserID: snowflake.ID(99)}, {UserID: snowflake.ID(100)}}
	got := transcriptParticipantNames(files, map[string]string{
		"42":  "Alex",
		"99":  "99",
		"100": "  Mörk 🐉  ",
	})
	want := []string{"Alex", "Mörk 🐉"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("transcriptParticipantNames() = %#v, want %#v", got, want)
	}
}
