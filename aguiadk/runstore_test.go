package aguiadk_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ieshan/adk-go-pkg/aguiadk"
)

// fakeClock provides a controllable clock for RunStore tests, allowing
// deterministic time advancement instead of time.Sleep.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(1000, 0)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func TestRunStore_SaveLoadDelete(t *testing.T) {
	s := aguiadk.NewRunStore()
	t.Cleanup(s.Stop)

	key := aguiadk.RunKey("thread-1", "run-1")
	run := &aguiadk.PausedRun{
		ThreadID:  "thread-1",
		RunID:     "run-1",
		SessionID: "sess-1",
		Pending: []aguiadk.PendingToolCall{
			{ID: "fc-1", Name: "approve", Args: map[string]any{"x": 1}},
		},
		State: map[string]any{"status": "awaiting_approval"},
	}

	s.Save(key, run)

	loaded, ok := s.Load(key)
	if !ok {
		t.Fatal("got miss on Load, want hit for saved run")
	}
	if loaded.SessionID != "sess-1" {
		t.Errorf("SessionID = %q, want %q", loaded.SessionID, "sess-1")
	}
	if len(loaded.Pending) != 1 || loaded.Pending[0].ID != "fc-1" {
		t.Errorf("Pending = %+v, want one entry with ID fc-1", loaded.Pending)
	}
	if loaded.State["status"] != "awaiting_approval" {
		t.Errorf("State[status] = %v, want %q", loaded.State["status"], "awaiting_approval")
	}

	s.Delete(key)
	if _, ok := s.Load(key); ok {
		t.Fatal("got hit on Load after Delete, want miss")
	}
}

func TestRunStore_LoadAndDelete(t *testing.T) {
	s := aguiadk.NewRunStore()
	t.Cleanup(s.Stop)

	key := aguiadk.RunKey("thread-1", "run-1")
	s.Save(key, &aguiadk.PausedRun{ThreadID: "thread-1", RunID: "run-1"})

	if _, ok := s.LoadAndDelete(key); !ok {
		t.Fatal("got miss on LoadAndDelete, want hit")
	}
	if _, ok := s.LoadAndDelete(key); ok {
		t.Fatal("got hit on second LoadAndDelete, want miss")
	}
}

func TestRunStore_ConcurrentLoadAndDelete(t *testing.T) {
	s := aguiadk.NewRunStore()
	t.Cleanup(s.Stop)

	key := aguiadk.RunKey("thread-1", "run-1")
	s.Save(key, &aguiadk.PausedRun{ThreadID: "thread-1", RunID: "run-1"})

	var winners int32
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := s.LoadAndDelete(key); ok {
				atomic.AddInt32(&winners, 1)
			}
		}()
	}
	wg.Wait()

	if winners != 1 {
		t.Errorf("got %d concurrent winners, want 1", winners)
	}
}

func TestRunStore_TTLExpiry(t *testing.T) {
	s := aguiadk.NewRunStoreWithTTL(50 * time.Millisecond)
	t.Cleanup(s.Stop)

	// Inject a controllable clock so we can advance time deterministically
	// instead of sleeping.
	clock := newFakeClock()
	aguiadk.SetRunStoreClockForTest(s, clock.Now)

	key := aguiadk.RunKey("thread-1", "run-1")
	s.Save(key, &aguiadk.PausedRun{ThreadID: "thread-1", RunID: "run-1"})

	if _, ok := s.Load(key); !ok {
		t.Fatal("got miss on Load before TTL expiry, want hit")
	}

	// Advance the clock past the TTL.
	clock.Advance(60 * time.Millisecond)

	if _, ok := s.Load(key); ok {
		t.Fatal("got hit on Load after TTL expiry, want miss")
	}
}

func TestRunStore_SaveCopiesCallerData(t *testing.T) {
	s := aguiadk.NewRunStore()
	t.Cleanup(s.Stop)

	key := aguiadk.RunKey("thread-1", "run-1")
	pending := []aguiadk.PendingToolCall{{ID: "fc-1", Name: "approve", Args: map[string]any{"x": 1}}}
	state := map[string]any{"status": "awaiting"}
	run := &aguiadk.PausedRun{
		ThreadID: "thread-1",
		RunID:    "run-1",
		Pending:  pending,
		State:    state,
	}

	s.Save(key, run)

	// Mutate the caller's data.
	pending[0].ID = "mutated"
	state["status"] = "mutated"

	loaded, _ := s.Load(key)
	if loaded == nil {
		t.Fatal("nil load result")
	}
	if loaded.Pending[0].ID != "fc-1" {
		t.Errorf("Pending[0].ID = %q, want %q (caller mutation leaked)", loaded.Pending[0].ID, "fc-1")
	}
	if loaded.State["status"] != "awaiting" {
		t.Errorf("State[status] = %v, want %q (caller mutation leaked)", loaded.State["status"], "awaiting")
	}
}

func TestRunStore_StopReleasesGoroutine(t *testing.T) {
	s := aguiadk.NewRunStoreWithTTL(10 * time.Millisecond)
	s.Stop()
	// Calling Stop twice should not panic.
	s.Stop()
}

func TestRunKey(t *testing.T) {
	got := aguiadk.RunKey("thread-1", "run-1")
	want := "thread-1|run-1"
	if got != want {
		t.Errorf("RunKey = %q, want %q", got, want)
	}
}

func TestRunStore_MaxEntriesEviction(t *testing.T) {
	// Verifies that saving beyond maxEntries evicts the oldest entry.
	s := aguiadk.NewRunStoreWithMaxEntries(time.Minute, 3)
	t.Cleanup(s.Stop)

	for i := 0; i < 4; i++ {
		key := aguiadk.RunKey("t", fmt.Sprintf("r%d", i))
		s.Save(key, &aguiadk.PausedRun{ThreadID: "t", RunID: fmt.Sprintf("r%d", i)})
	}

	// The oldest entry (r0) should have been evicted.
	if _, ok := s.Load(aguiadk.RunKey("t", "r0")); ok {
		t.Error("got r0 present, want evicted")
	}
	// r1, r2, r3 should still be present.
	for i := 1; i <= 3; i++ {
		key := aguiadk.RunKey("t", fmt.Sprintf("r%d", i))
		if _, ok := s.Load(key); !ok {
			t.Errorf("got r%d evicted, want present", i)
		}
	}
}

func TestRunStore_EvictionOrder(t *testing.T) {
	// Verifies that eviction removes the oldest entry by save time,
	// not by key order. Uses a controllable clock to ensure distinct
	// timestamps without sleeping.
	s := aguiadk.NewRunStoreWithMaxEntries(time.Minute, 2)
	t.Cleanup(s.Stop)

	clock := newFakeClock()
	aguiadk.SetRunStoreClockForTest(s, clock.Now)

	s.Save(aguiadk.RunKey("t", "r0"), &aguiadk.PausedRun{ThreadID: "t", RunID: "r0"})
	clock.Advance(1 * time.Millisecond)
	s.Save(aguiadk.RunKey("t", "r1"), &aguiadk.PausedRun{ThreadID: "t", RunID: "r1"})
	clock.Advance(1 * time.Millisecond)
	s.Save(aguiadk.RunKey("t", "r2"), &aguiadk.PausedRun{ThreadID: "t", RunID: "r2"})

	// r0 (oldest) should be evicted.
	if _, ok := s.Load(aguiadk.RunKey("t", "r0")); ok {
		t.Error("got r0 present, want evicted (oldest)")
	}
	// r1 and r2 should be present.
	if _, ok := s.Load(aguiadk.RunKey("t", "r1")); !ok {
		t.Error("got r1 evicted, want present")
	}
	if _, ok := s.Load(aguiadk.RunKey("t", "r2")); !ok {
		t.Error("got r2 evicted, want present")
	}
}
