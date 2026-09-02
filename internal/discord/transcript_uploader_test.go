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
	uploader := &TranscriptUploader{sender: sender}
	if err := uploader.Upload(42, path, 3); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	if sender.channelID != 42 {
		t.Fatalf("channel ID = %v, want 42", sender.channelID)
	}
	if sender.message.Content != "Transcript ready. 3 line(s)." {
		t.Fatalf("message content = %q", sender.message.Content)
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

func TestTranscriptUploaderRejectsMissingFile(t *testing.T) {
	sender := &testTranscriptMessageSender{}
	err := (&TranscriptUploader{sender: sender}).Upload(42, filepath.Join(t.TempDir(), "missing.txt"), 0)
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
	err := (&TranscriptUploader{sender: sender}).Upload(42, path, 1)
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
