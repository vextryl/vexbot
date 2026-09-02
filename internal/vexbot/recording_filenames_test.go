package vexbot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/disgoorg/snowflake/v2"
	"github.com/vextryl/vexbot/internal/wav"
)

func TestRenameRecordingFilesUsesSafeDisplayNames(t *testing.T) {
	dir := t.TempDir()
	files := []wav.File{
		{UserID: snowflake.ID(42), Path: filepath.Join(dir, "42.wav")},
		{UserID: snowflake.ID(99), Path: filepath.Join(dir, "99.wav")},
		{UserID: snowflake.ID(100), Path: filepath.Join(dir, "100.wav")},
	}
	for _, file := range files {
		if err := os.WriteFile(file.Path, []byte(file.UserID.String()), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}

	renamed, err := renameRecordingFiles(files, map[string]string{
		"42":  " Mörk 🐉 / GM:? ",
		"99":  "CON",
		"100": "Mörk 🐉 / GM:?",
	})
	if err != nil {
		t.Fatalf("renameRecordingFiles() error = %v", err)
	}
	if got, want := filepath.Base(renamed[0].Path), "Mörk 🐉 - GM--.wav"; got != want {
		t.Fatalf("first filename = %q, want %q", got, want)
	}
	if got, want := filepath.Base(renamed[1].Path), "CON-user.wav"; got != want {
		t.Fatalf("reserved filename = %q, want %q", got, want)
	}
	if got, want := filepath.Base(renamed[2].Path), "Mörk 🐉 - GM-- (100).wav"; got != want {
		t.Fatalf("collision filename = %q, want %q", got, want)
	}
	for _, file := range renamed {
		contents, err := os.ReadFile(file.Path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", file.Path, err)
		}
		if got, want := string(contents), file.UserID.String(); got != want {
			t.Fatalf("contents = %q, want %q", got, want)
		}
	}
}

func TestRecordingFileStemFallsBackForEmptyName(t *testing.T) {
	if got, want := recordingFileStem("\t\n", "42"), "user-42"; got != want {
		t.Fatalf("stem = %q, want %q", got, want)
	}
}

func TestRecordingFileStemDoesNotSplitUTF8(t *testing.T) {
	stem := recordingFileStem(strings.Repeat("🐉", 100), "42")
	if !utf8.ValidString(stem) || len(stem) > maxRecordingFileStemBytes {
		t.Fatalf("stem is not a valid truncated UTF-8 filename: %q", stem)
	}
}
