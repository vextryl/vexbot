// Package delivery presents local VexBot artifacts in Discord messages.
package delivery

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

const (
	// TranscriptUploadLimitEnv configures VexBot's explicit attachment ceiling
	// for completed transcript delivery.
	TranscriptUploadLimitEnv = "DISCORD_TRANSCRIPT_UPLOAD_LIMIT_BYTES"

	// DefaultTranscriptUploadLimitBytes is VexBot's conservative 16 MiB
	// attachment ceiling. Configure the environment variable to match the
	// actual limit available to the bot's Discord server and account.
	DefaultTranscriptUploadLimitBytes int64 = 16 * 1024 * 1024
)

// TranscriptUploadLimit parses VexBot's explicit transcript attachment limit.
func TranscriptUploadLimit(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultTranscriptUploadLimitBytes, nil
	}
	limit, err := strconv.ParseInt(value, 10, 64)
	if err != nil || limit <= 0 {
		return 0, fmt.Errorf("%s must be a positive number of bytes", TranscriptUploadLimitEnv)
	}
	return limit, nil
}

// Attachment identifies a local file to be delivered through Discord.
type Attachment struct {
	Path        string
	Name        string
	Description string
}

// AttachmentTooLargeError reports that a local attachment exceeds VexBot's
// configured Discord attachment ceiling.
type AttachmentTooLargeError struct {
	SizeBytes  int64
	LimitBytes int64
}

func (e *AttachmentTooLargeError) Error() string {
	return fmt.Sprintf("attachment is %d bytes, exceeding the configured upload limit of %d bytes", e.SizeBytes, e.LimitBytes)
}

type attachmentFileStat func(string) (fs.FileInfo, error)

func inspectAttachment(path string, limit int64, stat attachmentFileStat) error {
	if stat == nil {
		stat = os.Stat
	}
	info, err := stat(path)
	if err != nil {
		return fmt.Errorf("stat attachment file: %w", err)
	}
	if info.Size() > normalizeTranscriptUploadLimit(limit) {
		return &AttachmentTooLargeError{
			SizeBytes:  info.Size(),
			LimitBytes: normalizeTranscriptUploadLimit(limit),
		}
	}
	return nil
}

func normalizeTranscriptUploadLimit(limit int64) int64 {
	if limit <= 0 {
		return DefaultTranscriptUploadLimitBytes
	}
	return limit
}

func isAttachmentTooLarge(err error) (*AttachmentTooLargeError, bool) {
	var tooLarge *AttachmentTooLargeError
	matched := errors.As(err, &tooLarge)
	return tooLarge, matched
}
