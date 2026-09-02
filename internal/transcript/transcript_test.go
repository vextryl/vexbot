package transcript

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestBuildLinesOrdersAndFilters(t *testing.T) {
	lines := buildLines([]Entry{
		{UserID: "alex", SessionStart: 5 * time.Second, Text: " Again."},
		{UserID: "sam", SessionStart: time.Second, Text: " Hello."},
		{UserID: "alex", SessionStart: 3 * time.Second, Err: errors.New("failed")},
		{UserID: "sam", SessionStart: 2 * time.Second, Text: "[BLANK_AUDIO]"},
	})
	if len(lines) != 2 || lines[0].UserID != "sam" || lines[1].Text != "Again." {
		t.Fatalf("lines = %#v", lines)
	}
}

func TestWriteUsesDisplayName(t *testing.T) {
	dir := t.TempDir()
	path, count, err := Write(dir, map[string]string{"42": "Mörk 🐉"}, []Entry{{UserID: "42", SessionStart: 62 * time.Second, Text: " Hello."}})
	if err != nil || count != 1 {
		t.Fatalf("Write() = %q, %d, %v", path, count, err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "[01:02] Mörk 🐉: Hello.\n" {
		t.Fatalf("contents = %q, err = %v", contents, err)
	}
}
