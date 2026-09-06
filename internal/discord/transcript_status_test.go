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

func TestTranscriptStatusCreatesAndUpdatesProgressMessage(t *testing.T) {
	sender := &testTranscriptStatusSender{createdMessageID: 99}
	status := &TranscriptStatus{sender: sender}

	messageID, err := status.Create(42, TranscriptStatusUpdate{Phase: "preparing"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if messageID != 99 || sender.createdChannelID != 42 {
		t.Fatalf("Create() = message %v in channel %v", messageID, sender.createdChannelID)
	}
	if got, want := sender.created.Content, "Transcription in progress…\n[⬜ ⬜ ⬜ ⬜ ⬜ ⬜ ⬜ ⬜ ⬜ ⬜] 0% ⏳\nPreparing recordings."; got != want {
		t.Fatalf("created content = %q, want %q", got, want)
	}

	if err := status.Update(42, messageID, TranscriptStatusUpdate{
		Phase:          "transcribing",
		CompletedTurns: 12,
		TotalTurns:     30,
		Percent:        40,
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if sender.updatedChannelID != 42 || sender.updatedMessageID != 99 {
		t.Fatalf("Update() target = channel %v, message %v", sender.updatedChannelID, sender.updatedMessageID)
	}
	if got, want := *sender.updated.Content, "Transcription in progress…\n[🟩 🟩 🟩 🟩 ⬜ ⬜ ⬜ ⬜ ⬜ ⬜] 40% ⏳\nTranscribed 12 of 30 conversation turns."; got != want {
		t.Fatalf("updated content = %q, want %q", got, want)
	}
}

func TestTranscriptStatusAttachesCompletedTranscript(t *testing.T) {
	path := filepath.Join(t.TempDir(), transcriptAttachmentName)
	if err := os.WriteFile(path, []byte("[00:00] Alex: Hello.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	sender := &testTranscriptStatusSender{}
	if err := (&TranscriptStatus{sender: sender}).Complete(42, 99, 123, path, 3); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if got, want := *sender.updated.Content, "Transcription complete.\n[🟩 🟩 🟩 🟩 🟩 🟩 🟩 🟩 🟩 🟩] 100% ✅\n<@123> — here is your final transcript — 3 line(s)."; got != want {
		t.Fatalf("completion content = %q, want %q", got, want)
	}
	if sender.updated.AllowedMentions == nil || len(sender.updated.AllowedMentions.Users) != 1 || sender.updated.AllowedMentions.Users[0] != 123 {
		t.Fatalf("allowed mentions = %#v", sender.updated.AllowedMentions)
	}
	if len(sender.updated.Files) != 1 || sender.updated.Files[0].Name != transcriptAttachmentName {
		t.Fatalf("completion files = %#v", sender.updated.Files)
	}
	if got, want := string(sender.contents), "[00:00] Alex: Hello.\n"; got != want {
		t.Fatalf("attachment contents = %q, want %q", got, want)
	}
}

func TestTranscriptStatusReportsCreateUpdateAndFileFailures(t *testing.T) {
	createFailure := errors.New("missing Send Messages permission")
	status := &TranscriptStatus{sender: &testTranscriptStatusSender{createErr: createFailure}}
	if _, err := status.Create(42, TranscriptStatusUpdate{}); !errors.Is(err, createFailure) {
		t.Fatalf("Create() error = %v, want wrapped %v", err, createFailure)
	}

	updateFailure := errors.New("message no longer exists")
	status = &TranscriptStatus{sender: &testTranscriptStatusSender{updateErr: updateFailure}}
	if err := status.Update(42, 99, TranscriptStatusUpdate{}); !errors.Is(err, updateFailure) {
		t.Fatalf("Update() error = %v, want wrapped %v", err, updateFailure)
	}
	if err := status.Complete(42, 99, 123, filepath.Join(t.TempDir(), "missing.txt"), 0); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Complete() missing-file error = %v, want missing file", err)
	}
}

func TestFormatTranscriptStatusNormalizesProgressBar(t *testing.T) {
	if got, want := formatProgressBar(-1, false), "[⬜ ⬜ ⬜ ⬜ ⬜ ⬜ ⬜ ⬜ ⬜ ⬜]"; got != want {
		t.Fatalf("formatProgressBar(-1, false) = %q, want %q", got, want)
	}
	if got, want := formatProgressBar(1000, false), "[🟩 🟩 🟩 🟩 🟩 🟩 🟩 🟩 🟩 🟩]"; got != want {
		t.Fatalf("formatProgressBar(1000, false) = %q, want %q", got, want)
	}
}

func TestFormatTranscriptStatusRendersTerminalFailures(t *testing.T) {
	if got, want := FormatTranscriptStatus(TranscriptStatusUpdate{Phase: "failed"}), "Transcription failed.\n[🟥 🟥 🟥 🟥 🟥 🟥 🟥 🟥 🟥 🟥] ❌\nthe local recordings were kept."; got != want {
		t.Fatalf("failed status = %q, want %q", got, want)
	}
	if got, want := FormatTranscriptStatus(TranscriptStatusUpdate{Phase: "delivery_failed", Percent: 100}), "Discord delivery failed.\n[🟥 🟥 🟥 🟥 🟥 🟥 🟥 🟥 🟥 🟥] ❌\nthe local transcript was kept."; got != want {
		t.Fatalf("delivery failure status = %q, want %q", got, want)
	}
}

func TestFormatTranscriptStatusRendersTerminalRecipient(t *testing.T) {
	if got, want := FormatTranscriptStatus(TranscriptStatusUpdate{Phase: "failed", RecipientID: 123}), "Transcription failed.\n[🟥 🟥 🟥 🟥 🟥 🟥 🟥 🟥 🟥 🟥] ❌\n<@123> — the local recordings were kept."; got != want {
		t.Fatalf("recipient status = %q, want %q", got, want)
	}
}

func TestFormatTranscriptStatusRendersUploadIndicator(t *testing.T) {
	if got, want := FormatTranscriptStatus(TranscriptStatusUpdate{Phase: "complete", Percent: 100}), "Transcription in progress…\n[🟩 🟩 🟩 🟩 🟩 🟩 🟩 🟩 🟩 🟩] 100% ⬆️\nTranscript complete. Uploading…"; got != want {
		t.Fatalf("uploading status = %q, want %q", got, want)
	}
}

type testTranscriptStatusSender struct {
	createdChannelID snowflake.ID
	created          discord.MessageCreate
	createdMessageID snowflake.ID
	createErr        error

	updatedChannelID snowflake.ID
	updatedMessageID snowflake.ID
	updated          discord.MessageUpdate
	updateErr        error
	contents         []byte
}

func (s *testTranscriptStatusSender) CreateTranscriptStatusMessage(channelID snowflake.ID, message discord.MessageCreate) (snowflake.ID, error) {
	s.createdChannelID = channelID
	s.created = message
	return s.createdMessageID, s.createErr
}

func (s *testTranscriptStatusSender) UpdateTranscriptStatusMessage(channelID, messageID snowflake.ID, update discord.MessageUpdate) error {
	s.updatedChannelID = channelID
	s.updatedMessageID = messageID
	s.updated = update
	if len(update.Files) == 1 {
		contents, err := io.ReadAll(update.Files[0].Reader)
		if err != nil {
			return err
		}
		s.contents = contents
	}
	return s.updateErr
}
