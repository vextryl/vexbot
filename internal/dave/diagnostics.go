package dave

import (
	"log/slog"
	"sync"

	"github.com/disgoorg/snowflake/v2"
)

// DecryptFailureCounter tracks DAVE decrypt-frame failures independently for
// each active guild recording.
type DecryptFailureCounter struct {
	mu     sync.Mutex
	counts map[snowflake.ID]int
}

// NewDecryptFailureCounter creates an empty per-recording failure counter.
func NewDecryptFailureCounter() *DecryptFailureCounter {
	return &DecryptFailureCounter{counts: make(map[snowflake.ID]int)}
}

// Reset begins a fresh count for a recording in guildID.
func (c *DecryptFailureCounter) Reset(guildID snowflake.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[guildID] = 0
}

// Record adds one dropped DAVE decrypt-frame failure for guildID.
func (c *DecryptFailureCounter) Record(guildID snowflake.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[guildID]++
}

// Take returns a recording's count and removes it so the next recording in
// the same guild starts with a clean diagnostic scope.
func (c *DecryptFailureCounter) Take(guildID snowflake.ID) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	count := c.counts[guildID]
	delete(c.counts, guildID)
	return count
}

// LogRecordingSummary consumes and logs one recording's DAVE decrypt failure
// diagnostics. It is safe to call when no failures occurred.
func LogRecordingSummary(logger *slog.Logger, failures *DecryptFailureCounter, guildID snowflake.ID, directory string) {
	if failures == nil {
		return
	}
	count := failures.Take(guildID)
	if logger != nil {
		logger.Info("DAVE decrypt failure summary",
			slog.String("guild_id", guildID.String()),
			slog.String("directory", directory),
			slog.Int("dropped_failure_count", count),
			slog.Bool("failures_occurred", count > 0),
		)
	}
}
