package discord

import (
	"fmt"
	"os"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

const transcriptAttachmentName = "transcript.txt"

// TranscriptUploader sends completed local transcripts to Discord channels.
type TranscriptUploader struct {
	sender transcriptMessageSender
}

type transcriptMessageSender interface {
	CreateTranscriptMessage(snowflake.ID, discord.MessageCreate) error
}

type channelMessageSender struct {
	channels rest.Channels
}

func (s channelMessageSender) CreateTranscriptMessage(channelID snowflake.ID, message discord.MessageCreate) error {
	_, err := s.channels.CreateMessage(channelID, message)
	return err
}

// NewTranscriptUploader creates an uploader backed by Disgo's channel REST API.
func NewTranscriptUploader(channels rest.Channels) *TranscriptUploader {
	return &TranscriptUploader{
		sender: channelMessageSender{channels: channels},
	}
}

// Upload attaches the local transcript file to a normal Discord channel message.
func (u *TranscriptUploader) Upload(channelID, recipientID snowflake.ID, transcriptPath string, lineCount int) error {
	transcript, err := os.Open(transcriptPath)
	if err != nil {
		return fmt.Errorf("open transcript file: %w", err)
	}
	defer transcript.Close()

	if err := u.sender.CreateTranscriptMessage(channelID, discord.MessageCreate{
		Content: fmt.Sprintf("Transcript ready. %d line(s).\n%s", lineCount, terminalTranscriptMessage(recipientID, "here is your final transcript.")),
		Files: []*discord.File{
			discord.NewFile(transcriptAttachmentName, "VexBot transcript", transcript),
		},
		AllowedMentions: allowedTranscriptMention(recipientID),
	}); err != nil {
		return fmt.Errorf("send transcript to Discord: %w", err)
	}

	return nil
}
