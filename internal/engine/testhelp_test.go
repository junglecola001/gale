package engine

import (
	"testing"
	"time"
)

// shortenBackoff makes the probe's retry waits negligible for one test.
func shortenBackoff(t *testing.T) {
	t.Helper()
	old := probeBackoffBase
	probeBackoffBase = time.Millisecond
	t.Cleanup(func() { probeBackoffBase = old })
}
