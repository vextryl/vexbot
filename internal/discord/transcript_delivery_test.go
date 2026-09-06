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
	uploadCompletedTranscript(completion, 1, 2, 3, uploader, nil)

	completion <- session.TranscriptionResult{
		TranscriptPath: "recordings/session/transcript.txt",
		LineCount:      12,
	}

	select {
	case <-uploader.called:
	case <-time.After(time.Second):
		t.Fatal("uploader was not called")
	}
	if uploader.channelID != 2 || uploader.recipientID != 3 || uploader.path != "recordings/session/transcript.txt" || uploader.lineCount != 12 {
		t.Fatalf("upload = channel %v, recipient %v, path %q, lines %d", uploader.channelID, uploader.recipientID, uploader.path, uploader.lineCount)
	}
}

func TestUploadCompletedTranscriptLogsTranscriptionFailure(t *testing.T) {
	completion := make(chan session.TranscriptionResult, 1)
	uploader := &testTranscriptUploader{called: make(chan struct{}, 1)}
	logs := newTestLogCapture()
	uploadCompletedTranscript(completion, 1, 2, 3, uploader, slog.New(logs))

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
	uploadCompletedTranscript(completion, 1, 2, 3, uploader, slog.New(logs))

	completion <- session.TranscriptionResult{TranscriptPath: "recordings/session/transcript.txt"}
	<-uploader.called
	record := logs.next(t)
	if record.Message != "uploading transcript to Discord" || record.Level != slog.LevelError {
		t.Fatalf("log record = %s %q", record.Level, record.Message)
	}
}

func TestDeliverTranscriptUpdatesStatusThenAttachesTranscript(t *testing.T) {
	progress := make(chan session.TranscriptionProgress, 2)
	completion := make(chan session.TranscriptionResult, 1)
	status := newTestTranscriptDeliveryStatus()
	uploader := &testTranscriptUploader{called: make(chan struct{}, 1)}

	deliverTranscript(session.TranscriptionJob{Progress: progress, Completion: completion}, 1, 2, 3, status, uploader, nil)
	progress <- session.TranscriptionProgress{Phase: session.TranscriptionPhaseTranscribing, CompletedTurns: 1, TotalTurns: 10, Percent: 10}
	progress <- session.TranscriptionProgress{Phase: session.TranscriptionPhaseComplete, CompletedTurns: 10, TotalTurns: 10, Percent: 100}
	close(progress)
	completion <- session.TranscriptionResult{TranscriptPath: "recordings/session/transcript.txt", LineCount: 12}
	close(completion)

	events := status.waitForEvents(t, 3)
	if got, want := events[0], (testTranscriptDeliveryEvent{kind: "update", update: TranscriptStatusUpdate{Phase: "transcribing", CompletedTurns: 1, TotalTurns: 10, Percent: 10}}); got != want {
		t.Fatalf("first event = %#v, want %#v", got, want)
	}
	if got, want := events[1], (testTranscriptDeliveryEvent{kind: "update", update: TranscriptStatusUpdate{Phase: "complete", CompletedTurns: 10, TotalTurns: 10, Percent: 100}}); got != want {
		t.Fatalf("second event = %#v, want %#v", got, want)
	}
	if got, want := events[2], (testTranscriptDeliveryEvent{kind: "complete", recipientID: 3, path: "recordings/session/transcript.txt", lineCount: 12}); got != want {
		t.Fatalf("final event = %#v, want %#v", got, want)
	}
	select {
	case <-uploader.called:
		t.Fatal("fallback uploader was called despite a status message")
	default:
	}
}

func TestDeliverTranscriptReportsTranscriptionFailureInStatus(t *testing.T) {
	progress := make(chan session.TranscriptionProgress, 1)
	completion := make(chan session.TranscriptionResult, 1)
	status := newTestTranscriptDeliveryStatus()
	logs := newTestLogCapture()

	deliverTranscript(session.TranscriptionJob{Progress: progress, Completion: completion}, 1, 2, 3, status, &testTranscriptUploader{called: make(chan struct{}, 1)}, slog.New(logs))
	progress <- session.TranscriptionProgress{Phase: session.TranscriptionPhaseFailed}
	close(progress)
	completion <- session.TranscriptionResult{Err: errors.New("Whisper failed")}
	close(completion)

	events := status.waitForEvents(t, 1)
	if got, want := events[0], (testTranscriptDeliveryEvent{kind: "update", update: TranscriptStatusUpdate{Phase: "failed", RecipientID: 3}}); got != want {
		t.Fatalf("failure event = %#v, want %#v", got, want)
	}
	record := logs.next(t)
	if record.Message != "local transcription failed" || record.Level != slog.LevelError {
		t.Fatalf("log record = %s %q", record.Level, record.Message)
	}
}

func TestDeliverTranscriptReportsAttachmentFailureInStatus(t *testing.T) {
	progress := make(chan session.TranscriptionProgress, 1)
	completion := make(chan session.TranscriptionResult, 1)
	status := newTestTranscriptDeliveryStatus()
	status.completeErr = errors.New("missing Attach Files permission")
	logs := newTestLogCapture()

	deliverTranscript(session.TranscriptionJob{Progress: progress, Completion: completion}, 1, 2, 3, status, &testTranscriptUploader{called: make(chan struct{}, 1)}, slog.New(logs))
	progress <- session.TranscriptionProgress{Phase: session.TranscriptionPhaseComplete, Percent: 100}
	close(progress)
	completion <- session.TranscriptionResult{TranscriptPath: "recordings/session/transcript.txt"}
	close(completion)

	events := status.waitForEvents(t, 3)
	if got, want := events[2], (testTranscriptDeliveryEvent{kind: "update", update: TranscriptStatusUpdate{Phase: "delivery_failed", Percent: 100, RecipientID: 3}}); got != want {
		t.Fatalf("delivery failure event = %#v, want %#v", got, want)
	}
	record := logs.next(t)
	if record.Message != "uploading transcript to Discord" || record.Level != slog.LevelError {
		t.Fatalf("log record = %s %q", record.Level, record.Message)
	}
}

func TestDeliverTranscriptReportsOversizedTranscriptInStatus(t *testing.T) {
	progress := make(chan session.TranscriptionProgress, 1)
	completion := make(chan session.TranscriptionResult, 1)
	status := newTestTranscriptDeliveryStatus()
	status.completeErr = &TranscriptTooLargeError{SizeBytes: 9, LimitBytes: 8}
	logs := newTestLogCapture()

	deliverTranscript(session.TranscriptionJob{Progress: progress, Completion: completion}, 1, 2, 3, status, &testTranscriptUploader{called: make(chan struct{}, 1)}, slog.New(logs))
	progress <- session.TranscriptionProgress{Phase: session.TranscriptionPhaseComplete, Percent: 100}
	close(progress)
	completion <- session.TranscriptionResult{TranscriptPath: "recordings/session/transcript.txt"}
	close(completion)

	events := status.waitForEvents(t, 3)
	if got, want := events[2], (testTranscriptDeliveryEvent{kind: "update", update: TranscriptStatusUpdate{Phase: "attachment_too_large", Percent: 100, RecipientID: 3}}); got != want {
		t.Fatalf("oversized-transcript event = %#v, want %#v", got, want)
	}
	record := logs.next(t)
	if record.Message != "uploading transcript to Discord" || record.Level != slog.LevelError {
		t.Fatalf("log record = %s %q", record.Level, record.Message)
	}
	attributes := make(map[string]any)
	record.Attrs(func(attr slog.Attr) bool {
		attributes[attr.Key] = attr.Value.Any()
		return true
	})
	if attributes["transcript_size_bytes"] != int64(9) || attributes["upload_limit_bytes"] != int64(8) {
		t.Fatalf("oversize log attributes = %#v", attributes)
	}
}

func TestDeliverTranscriptFallsBackWhenStatusCreationFails(t *testing.T) {
	progress := make(chan session.TranscriptionProgress)
	completion := make(chan session.TranscriptionResult, 1)
	status := newTestTranscriptDeliveryStatus()
	status.createErr = errors.New("missing Send Messages permission")
	uploader := &testTranscriptUploader{called: make(chan struct{}, 1)}

	deliverTranscript(session.TranscriptionJob{Progress: progress, Completion: completion}, 1, 2, 3, status, uploader, nil)
	completion <- session.TranscriptionResult{TranscriptPath: "recordings/session/transcript.txt", LineCount: 12}
	close(completion)

	select {
	case <-uploader.called:
	case <-time.After(time.Second):
		t.Fatal("fallback uploader was not called")
	}
	if uploader.channelID != 2 || uploader.recipientID != 3 || uploader.path != "recordings/session/transcript.txt" || uploader.lineCount != 12 {
		t.Fatalf("fallback upload = channel %v, recipient %v, path %q, lines %d", uploader.channelID, uploader.recipientID, uploader.path, uploader.lineCount)
	}
}

type testTranscriptUploader struct {
	channelID   snowflake.ID
	recipientID snowflake.ID
	path        string
	lineCount   int
	err         error
	called      chan struct{}
}

type testTranscriptDeliveryEvent struct {
	kind        string
	update      TranscriptStatusUpdate
	recipientID snowflake.ID
	path        string
	lineCount   int
}

type testTranscriptDeliveryStatus struct {
	createErr   error
	completeErr error
	events      chan testTranscriptDeliveryEvent
}

func newTestTranscriptDeliveryStatus() *testTranscriptDeliveryStatus {
	return &testTranscriptDeliveryStatus{events: make(chan testTranscriptDeliveryEvent, 8)}
}

func (s *testTranscriptDeliveryStatus) Create(_ snowflake.ID, _ TranscriptStatusUpdate) (snowflake.ID, error) {
	if s.createErr != nil {
		return 0, s.createErr
	}
	return 99, nil
}

func (s *testTranscriptDeliveryStatus) Update(_ snowflake.ID, _ snowflake.ID, update TranscriptStatusUpdate) error {
	s.events <- testTranscriptDeliveryEvent{kind: "update", update: update}
	return nil
}

func (s *testTranscriptDeliveryStatus) Complete(_ snowflake.ID, _ snowflake.ID, recipientID snowflake.ID, path string, lineCount int) error {
	s.events <- testTranscriptDeliveryEvent{kind: "complete", recipientID: recipientID, path: path, lineCount: lineCount}
	return s.completeErr
}

func (s *testTranscriptDeliveryStatus) waitForEvents(t *testing.T, count int) []testTranscriptDeliveryEvent {
	t.Helper()
	events := make([]testTranscriptDeliveryEvent, 0, count)
	for range count {
		select {
		case event := <-s.events:
			events = append(events, event)
		case <-time.After(time.Second):
			t.Fatalf("received %d delivery events, want %d", len(events), count)
		}
	}
	return events
}

func (u *testTranscriptUploader) Upload(channelID, recipientID snowflake.ID, path string, lineCount int) error {
	u.channelID = channelID
	u.recipientID = recipientID
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
