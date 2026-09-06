package discord

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/vextryl/vexbot/internal/session"
)

func TestTranscriptProgressStatusUpdatesAtTenPercentThresholds(t *testing.T) {
	progress := make(chan session.TranscriptionProgress, 6)
	status := newTestProgressStatus()
	if !startTranscriptProgressStatus(progress, 1, 2, status, nil) {
		t.Fatal("startTranscriptProgressStatus() = false, want true")
	}

	progress <- session.TranscriptionProgress{Phase: session.TranscriptionPhaseTranscribing, CompletedTurns: 1, TotalTurns: 20, Percent: 5}
	progress <- session.TranscriptionProgress{Phase: session.TranscriptionPhaseTranscribing, CompletedTurns: 3, TotalTurns: 20, Percent: 15}
	progress <- session.TranscriptionProgress{Phase: session.TranscriptionPhaseTranscribing, CompletedTurns: 11, TotalTurns: 20, Percent: 50}
	progress <- session.TranscriptionProgress{Phase: session.TranscriptionPhaseCombining, CompletedTurns: 20, TotalTurns: 20, Percent: 95}
	progress <- session.TranscriptionProgress{Phase: session.TranscriptionPhaseComplete, CompletedTurns: 20, TotalTurns: 20, Percent: 100}
	close(progress)

	updates := status.waitForUpdates(t, 4)
	want := []TranscriptStatusUpdate{
		{Phase: "transcribing", CompletedTurns: 3, TotalTurns: 20, Percent: 10},
		{Phase: "transcribing", CompletedTurns: 11, TotalTurns: 20, Percent: 50},
		{Phase: "combining", CompletedTurns: 20, TotalTurns: 20, Percent: 90},
		{Phase: "complete", CompletedTurns: 20, TotalTurns: 20, Percent: 100},
	}
	assertTranscriptStatusUpdates(t, updates, want)
	if got, want := status.created, (TranscriptStatusUpdate{Phase: "preparing"}); got != want {
		t.Fatalf("initial status = %#v, want %#v", got, want)
	}
}

func TestTranscriptProgressStatusSerializesRapidUpdates(t *testing.T) {
	progress := make(chan session.TranscriptionProgress, 101)
	status := newTestProgressStatus()
	if !startTranscriptProgressStatus(progress, 1, 2, status, nil) {
		t.Fatal("startTranscriptProgressStatus() = false, want true")
	}

	for percent := 1; percent < 100; percent++ {
		progress <- session.TranscriptionProgress{
			Phase:          session.TranscriptionPhaseTranscribing,
			CompletedTurns: percent,
			TotalTurns:     100,
			Percent:        percent,
		}
	}
	progress <- session.TranscriptionProgress{Phase: session.TranscriptionPhaseComplete, CompletedTurns: 100, TotalTurns: 100, Percent: 100}
	close(progress)

	updates := status.waitForUpdates(t, 10)
	if status.maxConcurrentUpdates != 1 {
		t.Fatalf("maximum concurrent updates = %d, want 1", status.maxConcurrentUpdates)
	}
	for index, update := range updates {
		wantPercent := (index + 1) * 10
		if update.Percent != wantPercent {
			t.Fatalf("updates[%d].Percent = %d, want %d", index, update.Percent, wantPercent)
		}
	}
}

func TestTranscriptProgressStatusHandlesZeroTurnCompletion(t *testing.T) {
	progress := make(chan session.TranscriptionProgress, 3)
	status := newTestProgressStatus()
	if !startTranscriptProgressStatus(progress, 1, 2, status, nil) {
		t.Fatal("startTranscriptProgressStatus() = false, want true")
	}

	progress <- session.TranscriptionProgress{Phase: session.TranscriptionPhaseTranscribing, TotalTurns: 0}
	progress <- session.TranscriptionProgress{Phase: session.TranscriptionPhaseCombining, Percent: 95}
	progress <- session.TranscriptionProgress{Phase: session.TranscriptionPhaseComplete, Percent: 100}
	close(progress)

	assertTranscriptStatusUpdates(t, status.waitForUpdates(t, 2), []TranscriptStatusUpdate{
		{Phase: "combining", Percent: 90},
		{Phase: "complete", Percent: 100},
	})
}

func TestTranscriptProgressStatusReportsTranscriptionFailure(t *testing.T) {
	progress := make(chan session.TranscriptionProgress, 1)
	status := newTestProgressStatus()
	if !startTranscriptProgressStatus(progress, 1, 2, status, nil) {
		t.Fatal("startTranscriptProgressStatus() = false, want true")
	}

	progress <- session.TranscriptionProgress{Phase: session.TranscriptionPhaseFailed, Percent: 0}
	close(progress)

	assertTranscriptStatusUpdates(t, status.waitForUpdates(t, 1), []TranscriptStatusUpdate{{Phase: "failed"}})
}

func TestTranscriptProgressStatusReportsInitialCreationFailure(t *testing.T) {
	progress := make(chan session.TranscriptionProgress, 1)
	status := newTestProgressStatus()
	status.createErr = errors.New("missing Send Messages permission")
	if startTranscriptProgressStatus(progress, 1, 2, status, nil) {
		t.Fatal("startTranscriptProgressStatus() = true, want false")
	}

	progress <- session.TranscriptionProgress{Phase: session.TranscriptionPhaseComplete, Percent: 100}
	close(progress)
	select {
	case update := <-status.updateCalls:
		t.Fatalf("unexpected update after creation failure: %#v", update)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestTranscriptionProgressThreshold(t *testing.T) {
	for percent, want := range map[int]int{-1: 0, 9: 0, 10: 10, 95: 90, 100: 100, 101: 100} {
		if got := transcriptionProgressThreshold(percent); got != want {
			t.Fatalf("transcriptionProgressThreshold(%d) = %d, want %d", percent, got, want)
		}
	}
}

type testProgressStatus struct {
	mu                   sync.Mutex
	created              TranscriptStatusUpdate
	createErr            error
	updateCalls          chan TranscriptStatusUpdate
	inFlightUpdates      int
	maxConcurrentUpdates int
}

func newTestProgressStatus() *testProgressStatus {
	return &testProgressStatus{updateCalls: make(chan TranscriptStatusUpdate, 16)}
}

func (s *testProgressStatus) Create(_ snowflake.ID, update TranscriptStatusUpdate) (snowflake.ID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.created = update
	if s.createErr != nil {
		return 0, s.createErr
	}
	return 99, nil
}

func (s *testProgressStatus) Update(_ snowflake.ID, _ snowflake.ID, update TranscriptStatusUpdate) error {
	s.mu.Lock()
	s.inFlightUpdates++
	if s.inFlightUpdates > s.maxConcurrentUpdates {
		s.maxConcurrentUpdates = s.inFlightUpdates
	}
	s.inFlightUpdates--
	s.mu.Unlock()
	s.updateCalls <- update
	return nil
}

func (s *testProgressStatus) waitForUpdates(t *testing.T, count int) []TranscriptStatusUpdate {
	t.Helper()
	updates := make([]TranscriptStatusUpdate, 0, count)
	for range count {
		select {
		case update := <-s.updateCalls:
			updates = append(updates, update)
		case <-time.After(time.Second):
			t.Fatalf("received %d status updates, want %d", len(updates), count)
		}
	}
	return updates
}

func assertTranscriptStatusUpdates(t *testing.T, got, want []TranscriptStatusUpdate) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("status updates = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("status update %d = %#v, want %#v", index, got[index], want[index])
		}
	}
}
