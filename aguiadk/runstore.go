package aguiadk

import (
	"sync"
	"time"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/google/jsonschema-go/jsonschema"
)

const (
	// defaultRunTTL is how long a paused run survives without being resumed.
	defaultRunTTL = 30 * time.Minute
	// defaultMaxRuns caps the number of paused runs held at once.
	defaultMaxRuns = 1024
)

// PendingToolCall is a tool call awaiting human approval.
type PendingToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

// RequestedInputSnapshot captures the RequestedInput fields at pause time so
// the resume path can distinguish a free-form input request from a tool
// approval interrupt and use the user's payload directly.
type RequestedInputSnapshot struct {
	InterruptID    string
	Message        string
	ResponseSchema *jsonschema.Schema
	Payload        any
}

// PausedRun is the state captured when a run pauses on a tool-approval
// interrupt or a RequestedInput prompt.
type PausedRun struct {
	ThreadID       string
	RunID          string
	SessionID      string
	Pending        []PendingToolCall
	RequestedInput *RequestedInputSnapshot
	State          map[string]any
}

type runEntry struct {
	run *PausedRun
	at  time.Time
}

// RunStore is a concurrency-safe in-memory map of paused runs keyed by
// threadID+runID, with lazy TTL expiry and a bounded size (oldest evicted on
// overflow). It is the resume primitive for HITL: a paused run is saved on
// interrupt and claimed atomically on resume so two concurrent resumes cannot
// both execute the pending tool calls.
//
// This is deliberately process-local and non-durable. For multi-tenant
// production use, gate the endpoint behind auth and namespace keys by the
// authenticated principal.
type RunStore struct {
	mu         sync.Mutex
	runs       map[string]*runEntry
	ttl        time.Duration
	maxEntries int
	now        func() time.Time // injectable clock for tests
	done       chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup
}

// NewRunStore creates an empty RunStore with default TTL (30m) and size bound
// (1024) and starts a background cleanup goroutine. Call Stop to release
// resources.
func NewRunStore() *RunStore {
	return NewRunStoreWithTTL(defaultRunTTL)
}

// NewRunStoreWithTTL creates a RunStore with the given TTL and default size
// bound. A ttl <= 0 selects the default.
func NewRunStoreWithTTL(ttl time.Duration) *RunStore {
	if ttl <= 0 {
		ttl = defaultRunTTL
	}
	s := &RunStore{
		runs:       make(map[string]*runEntry),
		ttl:        ttl,
		maxEntries: defaultMaxRuns,
		now:        time.Now,
		done:       make(chan struct{}),
	}
	s.wg.Add(1)
	go s.cleanupLoop()
	return s
}

// NewRunStoreWithMaxEntries creates a RunStore with the given TTL and size
// bound. A ttl <= 0 selects the default. A maxEntries <= 0 selects the default.
func NewRunStoreWithMaxEntries(ttl time.Duration, maxEntries int) *RunStore {
	if ttl <= 0 {
		ttl = defaultRunTTL
	}
	if maxEntries <= 0 {
		maxEntries = defaultMaxRuns
	}
	s := &RunStore{
		runs:       make(map[string]*runEntry),
		ttl:        ttl,
		maxEntries: maxEntries,
		now:        time.Now,
		done:       make(chan struct{}),
	}
	s.wg.Add(1)
	go s.cleanupLoop()
	return s
}

// RunKey builds the lookup key for a paused run.
func RunKey(threadID, runID string) string {
	return threadID + "|" + runID
}

// Save records a paused run. It purges expired entries first, then evicts the
// oldest entry if the store is still at capacity. The caller's Pending slice
// and State map are shallow-copied: top-level containers are new, but nested
// mutable values (e.g. Args maps, State map values) are shared with the caller.
func (s *RunStore) Save(key string, run *PausedRun) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	s.purgeExpiredLocked(now)
	if _, exists := s.runs[key]; !exists && len(s.runs) >= s.maxEntries {
		s.evictOldestLocked()
	}

	state := make(map[string]any, len(run.State))
	for k, v := range run.State {
		state[k] = v
	}
	pending := append([]PendingToolCall(nil), run.Pending...)
	stored := &PausedRun{
		ThreadID:       run.ThreadID,
		RunID:          run.RunID,
		SessionID:      run.SessionID,
		Pending:        pending,
		RequestedInput: run.RequestedInput,
		State:          state,
	}
	s.runs[key] = &runEntry{run: stored, at: now}
}

// Load returns a paused run and whether it was present. An entry past its TTL
// is treated as a miss and deleted in-line. Non-destructive: use for
// validation before claiming with LoadAndDelete.
func (s *RunStore) Load(key string) (*PausedRun, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.runs[key]
	if !ok {
		return nil, false
	}
	if s.now().Sub(e.at) >= s.ttl {
		delete(s.runs, key)
		return nil, false
	}
	return e.run, true
}

// LoadAndDelete atomically returns a paused run and removes it from the store,
// so exactly one caller can claim a given paused run. An entry past its TTL is
// treated as a miss (and is still removed). This is the resume primitive: a
// plain Load-then-Delete is a TOCTOU race that lets two concurrent resumes of
// the same thread/run both observe the entry and both execute its pending
// tool calls.
func (s *RunStore) LoadAndDelete(key string) (*PausedRun, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.runs[key]
	if !ok {
		return nil, false
	}
	delete(s.runs, key)
	if s.now().Sub(e.at) >= s.ttl {
		return nil, false
	}
	return e.run, true
}

// Delete removes a paused run.
func (s *RunStore) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.runs, key)
}

// Stop signals the background cleanup goroutine to exit and waits for it to
// finish (with a 5-second timeout). Safe to call multiple times.
func (s *RunStore) Stop() {
	s.stopOnce.Do(func() { close(s.done) })
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

func (s *RunStore) cleanupLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.ttl / 4)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

func (s *RunStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredLocked(s.now())
}

func (s *RunStore) purgeExpiredLocked(now time.Time) {
	for k, e := range s.runs {
		if now.Sub(e.at) >= s.ttl {
			delete(s.runs, k)
		}
	}
}

func (s *RunStore) evictOldestLocked() {
	var oldestKey string
	var oldestAt time.Time
	first := true
	for k, e := range s.runs {
		if first || e.at.Before(oldestAt) {
			oldestKey, oldestAt, first = k, e.at, false
		}
	}
	if !first {
		delete(s.runs, oldestKey)
	}
}

// approvalsFromResume maps resume entries to per-tool-call approval decisions.
// An entry is approved when status is "resolved" and its payload does not carry
// approved:false. A non-boolean payload (e.g. a free-text result) is treated
// as approval so the resume can also carry a tool result directly.
func approvalsFromResume(entries []types.ResumeEntry) map[string]bool {
	approvals := make(map[string]bool, len(entries))
	for _, e := range entries {
		approved := e.Status == types.ResumeStatusResolved
		if approved {
			if m, ok := e.Payload.(map[string]any); ok {
				if v, ok := m["approved"].(bool); ok {
					approved = v
				}
			}
		}
		approvals[e.InterruptID] = approved
	}
	return approvals
}
