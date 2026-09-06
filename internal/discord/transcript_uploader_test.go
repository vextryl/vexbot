package discord

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

func TestTranscriptUploaderUploadsTranscriptAttachment(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "completed-transcript.txt")
	if err := os.WriteFile(path, []byte("[00:00] Alex: Hello.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	sender := &testTranscriptMessageSender{}
	uploader := &TranscriptUploader{sender: sender, uploadLimit: 1024}
	if err := uploader.Upload(42, 99, path, 3); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	if sender.channelID != 42 {
		t.Fatalf("channel ID = %v, want 42", sender.channelID)
	}
	if sender.message.Content != "Transcript ready. 3 line(s).\n<@99> — here is your final transcript." {
		t.Fatalf("message content = %q", sender.message.Content)
	}
	if sender.message.AllowedMentions == nil || len(sender.message.AllowedMentions.Users) != 1 || sender.message.AllowedMentions.Users[0] != 99 {
		t.Fatalf("allowed mentions = %#v", sender.message.AllowedMentions)
	}
	if len(sender.message.Files) != 1 {
		t.Fatalf("attachment count = %d, want 1", len(sender.message.Files))
	}
	attachment := sender.message.Files[0]
	if attachment.Name != transcriptAttachmentName || attachment.Description != "VexBot transcript" {
		t.Fatalf("attachment = %#v", attachment)
	}
	if string(sender.contents) != "[00:00] Alex: Hello.\n" {
		t.Fatalf("attachment contents = %q", sender.contents)
	}
}

func TestTranscriptUploaderAllowsTranscriptAtConfiguredLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), transcriptAttachmentName)
	contents := []byte("1234")
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	sender := &testTranscriptMessageSender{}
	err := (&TranscriptUploader{sender: sender, uploadLimit: int64(len(contents))}).Upload(42, 99, path, 1)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if !sender.called {
		t.Fatal("sender was not called for an exactly-at-limit transcript")
	}
}

func TestTranscriptUploaderRejectsOversizedTranscriptWithoutSending(t *testing.T) {
	path := filepath.Join(t.TempDir(), transcriptAttachmentName)
	if err := os.WriteFile(path, []byte("12345"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	sender := &testTranscriptMessageSender{}
	err := (&TranscriptUploader{sender: sender, uploadLimit: 4}).Upload(42, 99, path, 1)
	tooLarge, ok := isTranscriptTooLarge(err)
	if !ok || tooLarge.SizeBytes != 5 || tooLarge.LimitBytes != 4 {
		t.Fatalf("Upload() error = %v, want a 5-byte/4-byte limit error", err)
	}
	if sender.called {
		t.Fatal("sender was called for an oversized transcript")
	}
}

func TestTranscriptUploaderReportsFileStatFailure(t *testing.T) {
	want := errors.New("disk unavailable")
	sender := &testTranscriptMessageSender{}
	err := (&TranscriptUploader{
		sender: sender,
		stat: func(string) (os.FileInfo, error) {
			return nil, want
		},
	}).Upload(42, 99, "recordings/session/transcript.txt", 1)
	if !errors.Is(err, want) {
		t.Fatalf("Upload() error = %v, want wrapped %v", err, want)
	}
	if sender.called {
		t.Fatal("sender was called after a file stat failure")
	}
}

func TestTranscriptUploaderRejectsMissingFile(t *testing.T) {
	sender := &testTranscriptMessageSender{}
	err := (&TranscriptUploader{sender: sender}).Upload(42, 99, filepath.Join(t.TempDir(), "missing.txt"), 0)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Upload() error = %v, want missing-file error", err)
	}
	if sender.called {
		t.Fatal("sender was called for missing transcript")
	}
}

func TestTranscriptUploaderReportsDiscordDeliveryFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), transcriptAttachmentName)
	if err := os.WriteFile(path, []byte("transcript"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	want := errors.New("missing Attach Files permission")
	sender := &testTranscriptMessageSender{err: want}
	err := (&TranscriptUploader{sender: sender}).Upload(42, 99, path, 1)
	if !errors.Is(err, want) {
		t.Fatalf("Upload() error = %v, want wrapped %v", err, want)
	}
}

type testTranscriptMessageSender struct {
	channelID snowflake.ID
	message   discord.MessageCreate
	contents  []byte
	err       error
	called    bool
}

func (s *testTranscriptMessageSender) CreateTranscriptMessage(channelID snowflake.ID, message discord.MessageCreate) error {
	s.called = true
	s.channelID = channelID
	s.message = message
	if len(message.Files) == 1 {
		contents, err := io.ReadAll(message.Files[0].Reader)
		if err != nil {
			return err
		}
		s.contents = contents
	}
	return s.err
}
