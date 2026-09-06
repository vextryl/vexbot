package discord

import (
	"fmt"
	"os"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

const transcriptProgressBarWidth = 10

// TranscriptStatusUpdate is the Discord-facing presentation state for a
// local transcription job.
type TranscriptStatusUpdate struct {
	Phase          string
	CompletedTurns int
	TotalTurns     int
	Percent        int
}

// TranscriptStatus creates and updates the normal Discord message used to
// report local transcription progress.
type TranscriptStatus struct {
	sender transcriptStatusSender
}

type transcriptStatusSender interface {
	CreateTranscriptStatusMessage(snowflake.ID, discord.MessageCreate) (snowflake.ID, error)
	UpdateTranscriptStatusMessage(snowflake.ID, snowflake.ID, discord.MessageUpdate) error
}

type channelStatusSender struct {
	channels rest.Channels
}

func (s channelStatusSender) CreateTranscriptStatusMessage(channelID snowflake.ID, message discord.MessageCreate) (snowflake.ID, error) {
	created, err := s.channels.CreateMessage(channelID, message)
	if err != nil {
		return 0, err
	}
	return created.ID, nil
}

func (s channelStatusSender) UpdateTranscriptStatusMessage(channelID, messageID snowflake.ID, update discord.MessageUpdate) error {
	_, err := s.channels.UpdateMessage(channelID, messageID, update)
	return err
}

// NewTranscriptStatus creates a status-message component backed by Disgo's
// channel REST API.
func NewTranscriptStatus(channels rest.Channels) *TranscriptStatus {
	return &TranscriptStatus{sender: channelStatusSender{channels: channels}}
}

// Create posts the initial normal Discord status message and returns its ID.
func (s *TranscriptStatus) Create(channelID snowflake.ID, update TranscriptStatusUpdate) (snowflake.ID, error) {
	messageID, err := s.sender.CreateTranscriptStatusMessage(channelID, discord.MessageCreate{
		Content: FormatTranscriptStatus(update),
	})
	if err != nil {
		return 0, fmt.Errorf("create transcription status message: %w", err)
	}
	return messageID, nil
}

// Update edits a previously created normal Discord status message.
func (s *TranscriptStatus) Update(channelID, messageID snowflake.ID, update TranscriptStatusUpdate) error {
	content := FormatTranscriptStatus(update)
	if err := s.sender.UpdateTranscriptStatusMessage(channelID, messageID, discord.MessageUpdate{Content: &content}); err != nil {
		return fmt.Errorf("update transcription status message: %w", err)
	}
	return nil
}

// Complete attaches the finished local transcript while marking the status
// message as complete.
func (s *TranscriptStatus) Complete(channelID, messageID snowflake.ID, transcriptPath string, lineCount int) error {
	transcript, err := os.Open(transcriptPath)
	if err != nil {
		return fmt.Errorf("open transcript file: %w", err)
	}
	defer transcript.Close()

	content := fmt.Sprintf(
		"Transcription complete — %d line(s).\n%s 100%% ✅\nHere is your transcript.",
		lineCount,
		formatProgressBar(100, false),
	)
	if err := s.sender.UpdateTranscriptStatusMessage(channelID, messageID, discord.MessageUpdate{
		Content: &content,
		Files: []*discord.File{
			discord.NewFile(transcriptAttachmentName, "VexBot transcript", transcript),
		},
	}); err != nil {
		return fmt.Errorf("attach transcript to status message: %w", err)
	}
	return nil
}

// FormatTranscriptStatus renders a readable, ten-cell progress bar and the
// current local-transcription phase.
func FormatTranscriptStatus(update TranscriptStatusUpdate) string {
	percent := normalizeProgressPercent(update.Percent)
	if update.Phase == "failed" {
		return fmt.Sprintf("Transcription failed.\n%s ❌\nLocal recordings were kept.", formatProgressBar(percent, true))
	}
	if update.Phase == "delivery_failed" {
		return fmt.Sprintf("Transcription complete.\n%s ❌\nDiscord delivery failed. The local transcript was kept.", formatProgressBar(percent, true))
	}
	indicator := "⏳"
	if update.Phase == "complete" {
		indicator = "⬆️"
	}
	return fmt.Sprintf(
		"Transcription in progress…\n%s %d%% %s\n%s",
		formatProgressBar(percent, false),
		percent,
		indicator,
		formatTranscriptStatusPhase(update),
	)
}

func formatTranscriptStatusPhase(update TranscriptStatusUpdate) string {
	switch update.Phase {
	case "preparing":
		return "Preparing recordings."
	case "transcribing":
		return fmt.Sprintf("Transcribed %d of %d conversation turns.", update.CompletedTurns, update.TotalTurns)
	case "combining":
		return "Combining the final transcript."
	case "complete":
		return "Transcript complete. Uploading…"
	default:
		return "Processing local transcription."
	}
}

func formatProgressBar(percent int, failed bool) string {
	if failed {
		return "[" + strings.Repeat("🟥", transcriptProgressBarWidth) + "]"
	}
	filled := normalizeProgressPercent(percent) / transcriptProgressBarWidth
	return "[" + strings.Repeat("🟩", filled) + strings.Repeat("⬜", transcriptProgressBarWidth-filled) + "]"
}

func normalizeProgressPercent(percent int) int {
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}
