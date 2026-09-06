package recording

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRetentionCount(t *testing.T) {
	for _, test := range []struct {
		value string
		want  int
		valid bool
	}{{"", DefaultRetentionCount, true}, {"5", 5, true}, {" 3 ", 3, true}, {"0", 0, false}, {"many", 0, false}} {
		got, err := RetentionCount(test.value)
		if (err == nil) != test.valid {
			t.Fatalf("RetentionCount(%q) error = %v, valid = %v", test.value, err, test.valid)
		}
		if test.valid && got != test.want {
			t.Fatalf("RetentionCount(%q) = %d, want %d", test.value, got, test.want)
		}
	}
}

func TestPruneForNewSessionKeepsMostRecentCompletedSessions(t *testing.T) {
	directory := t.TempDir()
	oldest := createCompletedSession(t, directory, "20260901T120000Z-one")
	middle := createCompletedSession(t, directory, "20260901T130000Z-two")
	newest := createCompletedSession(t, directory, "20260901T140000Z-three")
	createIncompleteSession(t, directory, "20260901T150000Z-in-progress")
	result, err := PruneForNewSession(directory, 3, nil)
	if err != nil {
		t.Fatalf("PruneForNewSession() error = %v", err)
	}
	if result.CompletedSessions != 3 || len(result.RemovedDirectories) != 1 || result.RemovedDirectories[0] != oldest {
		t.Fatalf("PruneForNewSession() = %#v, want oldest session removed", result)
	}
	assertPathMissing(t, oldest)
	assertPathExists(t, middle)
	assertPathExists(t, newest)
	assertPathExists(t, filepath.Join(directory, "20260901T150000Z-in-progress"))
}

func TestPruneForNewSessionProtectsTranscribingSessions(t *testing.T) {
	directory := t.TempDir()
	oldest := createCompletedSession(t, directory, "20260901T120000Z-one")
	protected := createCompletedSession(t, directory, "20260901T130000Z-two")
	newest := createCompletedSession(t, directory, "20260901T140000Z-three")
	result, err := PruneForNewSession(directory, 3, map[string]struct{}{protected: {}})
	if err != nil {
		t.Fatalf("PruneForNewSession() error = %v", err)
	}
	if result.ProtectedSessions != 1 || len(result.RemovedDirectories) != 1 || result.RemovedDirectories[0] != oldest {
		t.Fatalf("PruneForNewSession() = %#v, want protected session retained", result)
	}
	assertPathMissing(t, oldest)
	assertPathExists(t, protected)
	assertPathExists(t, newest)
}

func createCompletedSession(t *testing.T, parent, name string) string {
	t.Helper()
	path := filepath.Join(parent, name)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("Mkdir(%q) error = %v", path, err)
	}
	if err := os.WriteFile(filepath.Join(path, timelineFileName), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	return path
}

func createIncompleteSession(t *testing.T, parent, name string) {
	t.Helper()
	if err := os.Mkdir(filepath.Join(parent, name), 0o755); err != nil {
		t.Fatalf("Mkdir(%q) error = %v", name, err)
	}
}

func assertPathExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat(%q) error = %v, want existing path", path, err)
	}
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Stat(%q) error = %v, want missing path", path, err)
	}
}
