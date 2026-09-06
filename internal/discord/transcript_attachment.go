package discord

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

// TranscriptTooLargeError reports that a local transcript exceeds VexBot's
// configured Discord attachment ceiling.
type TranscriptTooLargeError struct {
	SizeBytes  int64
	LimitBytes int64
}

func (e *TranscriptTooLargeError) Error() string {
	return fmt.Sprintf("transcript is %d bytes, exceeding the configured upload limit of %d bytes", e.SizeBytes, e.LimitBytes)
}

type transcriptFileStat func(string) (fs.FileInfo, error)

func inspectTranscriptAttachment(path string, limit int64, stat transcriptFileStat) error {
	if stat == nil {
		stat = os.Stat
	}
	info, err := stat(path)
	if err != nil {
		return fmt.Errorf("stat transcript file: %w", err)
	}
	if info.Size() > normalizeTranscriptUploadLimit(limit) {
		return &TranscriptTooLargeError{
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

func isTranscriptTooLarge(err error) (*TranscriptTooLargeError, bool) {
	var tooLarge *TranscriptTooLargeError
	matched := errors.As(err, &tooLarge)
	return tooLarge, matched
}
