package delivery

import (
	"fmt"
	"os"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

// Uploader sends local attachments to Discord channels.
type Uploader struct {
	sender      messageSender
	uploadLimit int64
	stat        attachmentFileStat
}

type messageSender interface {
	CreateMessage(snowflake.ID, discord.MessageCreate) error
}

type channelMessageSender struct {
	channels rest.Channels
}

func (s channelMessageSender) CreateMessage(channelID snowflake.ID, message discord.MessageCreate) error {
	_, err := s.channels.CreateMessage(channelID, message)
	return err
}

// NewUploader creates an attachment uploader backed by Disgo's channel REST API.
func NewUploader(channels rest.Channels, uploadLimit int64) *Uploader {
	return &Uploader{
		sender:      channelMessageSender{channels: channels},
		uploadLimit: uploadLimit,
		stat:        os.Stat,
	}
}

// Upload attaches a local file to a normal Discord channel message.
func (u *Uploader) Upload(channelID, recipientID snowflake.ID, attachment Attachment, content string) error {
	if err := inspectAttachment(attachment.Path, u.uploadLimit, u.stat); err != nil {
		return err
	}
	file, err := os.Open(attachment.Path)
	if err != nil {
		return fmt.Errorf("open attachment file: %w", err)
	}
	defer file.Close()

	if err := u.sender.CreateMessage(channelID, discord.MessageCreate{
		Content: content,
		Files: []*discord.File{
			discord.NewFile(attachment.Name, attachment.Description, file),
		},
		AllowedMentions: allowedRecipientMention(recipientID),
	}); err != nil {
		return fmt.Errorf("send attachment to Discord: %w", err)
	}

	return nil
}
