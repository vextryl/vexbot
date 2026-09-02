package discord

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/vextryl/vexbot/internal/session"
)

func TestUploadCompletedTranscriptUploadsSuccessfulResult(t *testing.T) {
	completion := make(chan session.TranscriptionResult, 1)
	uploader := &testTranscriptUploader{called: make(chan struct{}, 1)}
	uploadCompletedTranscript(completion, 1, 2, uploader, nil)

	completion <- session.TranscriptionResult{
		TranscriptPath: "recordings/session/transcript.txt",
		LineCount:      12,
	}

	select {
	case <-uploader.called:
	case <-time.After(time.Second):
		t.Fatal("uploader was not called")
	}
	if uploader.channelID != 2 || uploader.path != "recordings/session/transcript.txt" || uploader.lineCount != 12 {
		t.Fatalf("upload = channel %v, path %q, lines %d", uploader.channelID, uploader.path, uploader.lineCount)
	}
}

func TestUploadCompletedTranscriptLogsTranscriptionFailure(t *testing.T) {
	completion := make(chan session.TranscriptionResult, 1)
	uploader := &testTranscriptUploader{called: make(chan struct{}, 1)}
	logs := newTestLogCapture()
	uploadCompletedTranscript(completion, 1, 2, uploader, slog.New(logs))

	completion <- session.TranscriptionResult{Err: errors.New("Whisper failed")}

	record := logs.next(t)
	if record.Message != "local transcription failed" || record.Level != slog.LevelError {
		t.Fatalf("log record = %s %q", record.Level, record.Message)
	}
	select {
	case <-uploader.called:
		t.Fatal("uploader was called after transcription failure")
	default:
	}
}

func TestUploadCompletedTranscriptLogsDeliveryFailure(t *testing.T) {
	want := errors.New("missing Attach Files permission")
	completion := make(chan session.TranscriptionResult, 1)
	uploader := &testTranscriptUploader{err: want, called: make(chan struct{}, 1)}
	logs := newTestLogCapture()
	uploadCompletedTranscript(completion, 1, 2, uploader, slog.New(logs))

	completion <- session.TranscriptionResult{TranscriptPath: "recordings/session/transcript.txt"}
	<-uploader.called
	record := logs.next(t)
	if record.Message != "uploading transcript to Discord" || record.Level != slog.LevelError {
		t.Fatalf("log record = %s %q", record.Level, record.Message)
	}
}

type testTranscriptUploader struct {
	channelID snowflake.ID
	path      string
	lineCount int
	err       error
	called    chan struct{}
}

func (u *testTranscriptUploader) Upload(channelID snowflake.ID, path string, lineCount int) error {
	u.channelID = channelID
	u.path = path
	u.lineCount = lineCount
	u.called <- struct{}{}
	return u.err
}

type testLogCapture struct {
	records chan slog.Record
}

func newTestLogCapture() *testLogCapture {
	return &testLogCapture{records: make(chan slog.Record, 1)}
}

func (h *testLogCapture) Enabled(context.Context, slog.Level) bool { return true }

func (h *testLogCapture) Handle(_ context.Context, record slog.Record) error {
	h.records <- record.Clone()
	return nil
}

func (h *testLogCapture) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *testLogCapture) WithGroup(string) slog.Handler { return h }

func (h *testLogCapture) next(t *testing.T) slog.Record {
	t.Helper()
	select {
	case record := <-h.records:
		return record
	case <-time.After(time.Second):
		t.Fatal("expected log record")
		return slog.Record{}
	}
}
