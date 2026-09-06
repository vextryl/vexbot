package recording

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// RetentionCountEnv configures how many completed recording sessions VexBot retains on disk.
	RetentionCountEnv = "RECORDING_RETENTION_COUNT"
	// DefaultRetentionCount retains the five most recent completed sessions.
	DefaultRetentionCount = 5
	timelineFileName      = "timeline.json"
)

// RetentionCount parses RECORDING_RETENTION_COUNT. An unset value uses the default; configured values must be positive.
func RetentionCount(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultRetentionCount, nil
	}
	count, err := strconv.Atoi(value)
	if err != nil || count < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", RetentionCountEnv)
	}
	return count, nil
}

// RetentionResult reports the completed-session cleanup performed before a new recording begins.
type RetentionResult struct {
	CompletedSessions  int
	ProtectedSessions  int
	RemovedDirectories []string
}

type completedSession struct {
	path      string
	startedAt time.Time
}

// PruneForNewSession removes the oldest completed session directories so creating one more recording leaves no more than retentionCount sessions. protectedDirectories are transcription jobs that must not be removed.
func PruneForNewSession(directory string, retentionCount int, protectedDirectories map[string]struct{}) (RetentionResult, error) {
	if retentionCount < 1 {
		return RetentionResult{}, fmt.Errorf("retention count must be positive")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		if os.IsNotExist(err) {
			return RetentionResult{}, nil
		}
		return RetentionResult{}, fmt.Errorf("read recordings directory: %w", err)
	}
	protected := make(map[string]struct{}, len(protectedDirectories))
	for path := range protectedDirectories {
		protected[filepath.Clean(path)] = struct{}{}
	}

	result := RetentionResult{}
	removable := make([]completedSession, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		startedAt, ok := recordingSessionStartedAt(entry.Name())
		if !ok {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		info, err := os.Stat(filepath.Join(path, timelineFileName))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return RetentionResult{}, fmt.Errorf("inspect recording session %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		result.CompletedSessions++
		if _, ok := protected[filepath.Clean(path)]; ok {
			result.ProtectedSessions++
			continue
		}
		removable = append(removable, completedSession{path: path, startedAt: startedAt})
	}

	sort.Slice(removable, func(first, second int) bool { return removable[first].startedAt.Before(removable[second].startedAt) })
	toRemove := result.CompletedSessions - (retentionCount - 1)
	if toRemove <= 0 {
		return result, nil
	}
	if toRemove > len(removable) {
		toRemove = len(removable)
	}
	for _, session := range removable[:toRemove] {
		if err := os.RemoveAll(session.path); err != nil {
			return result, fmt.Errorf("remove old recording session %q: %w", session.path, err)
		}
		result.RemovedDirectories = append(result.RemovedDirectories, session.path)
	}
	return result, nil
}

func recordingSessionStartedAt(name string) (time.Time, bool) {
	timestamp, suffix, ok := strings.Cut(name, "-")
	if !ok || suffix == "" {
		return time.Time{}, false
	}
	startedAt, err := time.Parse("20060102T150405Z", timestamp)
	if err != nil {
		return time.Time{}, false
	}
	return startedAt, true
}
