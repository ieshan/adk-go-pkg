package aguiadk

import "time"

// This file exports unexported functions and fields for use in external test
// files (package aguiadk_test). It is only compiled during testing and does
// not pollute the production binary.

// SetRunStoreClockForTest replaces the RunStore's internal clock with the
// provided function. This allows tests to control time advancement
// deterministically instead of relying on time.Sleep for TTL expiry and
// eviction ordering. Must be called before any Save/Load operations.
func SetRunStoreClockForTest(s *RunStore, now func() time.Time) {
	s.mu.Lock()
	s.now = now
	s.mu.Unlock()
}
