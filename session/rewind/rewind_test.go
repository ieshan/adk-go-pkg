package rewind_test

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/adk/session"

	"github.com/ieshan/adk-go-pkg/session/rewind"
)

const (
	testApp  = "test-app"
	testUser = "test-user"
)

// createSession creates a new session and returns it.
func createSession(t *testing.T, ctx context.Context, svc session.Service, sessionID string) session.Session {
	t.Helper()
	resp, err := svc.Create(ctx, &session.CreateRequest{
		AppName:   testApp,
		UserID:    testUser,
		SessionID: sessionID,
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	return resp.Session
}

// appendEventWithDelta appends an event with the given state delta to the session and returns the event ID.
func appendEventWithDelta(t *testing.T, ctx context.Context, svc session.Service, sess session.Session, delta map[string]any) string {
	t.Helper()
	ev := session.NewEvent("inv-" + sess.ID())
	ev.Actions.StateDelta = delta
	if err := svc.AppendEvent(ctx, sess, ev); err != nil {
		t.Fatalf("failed to append event: %v", err)
	}
	return ev.ID
}

// getSession fetches a fresh session copy from the service.
func getSession(t *testing.T, ctx context.Context, svc session.Service, sessionID string) session.Session {
	t.Helper()
	resp, err := svc.Get(ctx, &session.GetRequest{
		AppName:   testApp,
		UserID:    testUser,
		SessionID: sessionID,
	})
	if err != nil {
		t.Fatalf("failed to get session %s: %v", sessionID, err)
	}
	return resp.Session
}

// TestRewind_ByID verifies that rewinding by event ID truncates correctly.
// Creates 5 events, rewinds to the 3rd (index 2), expects 3 events to remain.
func TestRewind_ByID(t *testing.T) {
	ctx := context.Background()
	svc := session.InMemoryService()
	sessID := "rewind-by-id"

	sess := createSession(t, ctx, svc, sessID)

	var eventIDs []string
	for i := range 5 {
		// Need fresh session handle each time for AppendEvent to work
		sess = getSession(t, ctx, svc, sessID)
		id := appendEventWithDelta(t, ctx, svc, sess, map[string]any{
			fmt.Sprintf("key%d", i): i,
		})
		eventIDs = append(eventIDs, id)
	}

	// Rewind to event at index 2 (3rd event, 0-based)
	targetEventID := eventIDs[2]
	result, err := rewind.Rewind(ctx, svc, testApp, testUser, sessID, targetEventID)
	if err != nil {
		t.Fatalf("Rewind returned unexpected error: %v", err)
	}

	got := result.Events().Len()
	if got != 3 {
		t.Errorf("expected 3 events after rewind, got %d", got)
	}

	// Verify the last retained event ID matches the target
	lastEvent := result.Events().At(2)
	if lastEvent.ID != targetEventID {
		t.Errorf("last event ID = %q, want %q", lastEvent.ID, targetEventID)
	}
}

// TestRewind_ByIndex verifies that rewinding by index truncates correctly.
// Creates 5 events, rewinds to index 2, expects 3 events to remain.
func TestRewind_ByIndex(t *testing.T) {
	ctx := context.Background()
	svc := session.InMemoryService()
	sessID := "rewind-by-index"

	createSession(t, ctx, svc, sessID)

	var eventIDs []string
	for i := range 5 {
		sess := getSession(t, ctx, svc, sessID)
		id := appendEventWithDelta(t, ctx, svc, sess, map[string]any{
			fmt.Sprintf("key%d", i): i,
		})
		eventIDs = append(eventIDs, id)
	}

	result, err := rewind.RewindToIndex(ctx, svc, testApp, testUser, sessID, 2)
	if err != nil {
		t.Fatalf("RewindToIndex returned unexpected error: %v", err)
	}

	got := result.Events().Len()
	if got != 3 {
		t.Errorf("expected 3 events after rewind, got %d", got)
	}

	// Verify the last retained event matches index 2
	lastEvent := result.Events().At(2)
	if lastEvent.ID != eventIDs[2] {
		t.Errorf("last event ID = %q, want %q", lastEvent.ID, eventIDs[2])
	}
}

// TestRewind_EventNotFound verifies that rewinding to a non-existent event ID returns an error.
func TestRewind_EventNotFound(t *testing.T) {
	ctx := context.Background()
	svc := session.InMemoryService()
	sessID := "rewind-not-found"

	sess := createSession(t, ctx, svc, sessID)
	appendEventWithDelta(t, ctx, svc, sess, map[string]any{"k": "v"})

	_, err := rewind.Rewind(ctx, svc, testApp, testUser, sessID, "nonexistent-event-id")
	if err == nil {
		t.Fatal("expected error for non-existent event ID, got nil")
	}
}

// TestRewind_IndexOutOfBounds verifies that rewinding to an index beyond event count returns an error.
func TestRewind_IndexOutOfBounds(t *testing.T) {
	ctx := context.Background()
	svc := session.InMemoryService()
	sessID := "rewind-out-of-bounds"

	createSession(t, ctx, svc, sessID)
	for range 5 {
		sess := getSession(t, ctx, svc, sessID)
		appendEventWithDelta(t, ctx, svc, sess, map[string]any{"k": "v"})
	}

	_, err := rewind.RewindToIndex(ctx, svc, testApp, testUser, sessID, 10)
	if err == nil {
		t.Fatal("expected error for out-of-bounds index, got nil")
	}
}

// TestRewind_EmptySession verifies that rewinding a session with no events returns an error.
func TestRewind_EmptySession(t *testing.T) {
	ctx := context.Background()
	svc := session.InMemoryService()
	sessID := "rewind-empty"

	createSession(t, ctx, svc, sessID)

	_, err := rewind.Rewind(ctx, svc, testApp, testUser, sessID, "any-event-id")
	if err == nil {
		t.Fatal("expected error for empty session, got nil")
	}
}

// TestRewind_LastEvent verifies that rewinding to the last event is effectively a no-op.
// All events should remain.
func TestRewind_LastEvent(t *testing.T) {
	ctx := context.Background()
	svc := session.InMemoryService()
	sessID := "rewind-last-event"

	createSession(t, ctx, svc, sessID)

	var lastID string
	for i := range 5 {
		sess := getSession(t, ctx, svc, sessID)
		lastID = appendEventWithDelta(t, ctx, svc, sess, map[string]any{
			fmt.Sprintf("k%d", i): i,
		})
	}

	result, err := rewind.Rewind(ctx, svc, testApp, testUser, sessID, lastID)
	if err != nil {
		t.Fatalf("Rewind to last event returned error: %v", err)
	}

	got := result.Events().Len()
	if got != 5 {
		t.Errorf("expected 5 events (no-op rewind to last event), got %d", got)
	}
}

// TestRewind_StateRecalculation verifies that state is correctly replayed after rewind.
//
// Event 0: sets "a" = 1
// Event 1: sets "b" = 2
// Event 2: sets "a" = 3
//
// After rewinding to event 1 (index 1), state should be {"a": 1, "b": 2}.
func TestRewind_StateRecalculation(t *testing.T) {
	ctx := context.Background()
	svc := session.InMemoryService()
	sessID := "rewind-state-recalc"

	createSession(t, ctx, svc, sessID)

	deltas := []map[string]any{
		{"a": 1},
		{"b": 2},
		{"a": 3},
	}

	var eventIDs []string
	for _, delta := range deltas {
		sess := getSession(t, ctx, svc, sessID)
		id := appendEventWithDelta(t, ctx, svc, sess, delta)
		eventIDs = append(eventIDs, id)
	}

	// Rewind to event 1 (second event, sets "b"=2)
	result, err := rewind.Rewind(ctx, svc, testApp, testUser, sessID, eventIDs[1])
	if err != nil {
		t.Fatalf("Rewind returned error: %v", err)
	}

	state := result.State()

	valA, err := state.Get("a")
	if err != nil {
		t.Fatalf("state.Get(\"a\") returned error: %v", err)
	}
	if valA != 1 {
		t.Errorf("state[\"a\"] = %v, want 1", valA)
	}

	valB, err := state.Get("b")
	if err != nil {
		t.Fatalf("state.Get(\"b\") returned error: %v", err)
	}
	if valB != 2 {
		t.Errorf("state[\"b\"] = %v, want 2", valB)
	}

	// "a" should NOT be 3 after rewind (event 2 was discarded)
}

// TestRewindToIndex_Zero verifies that rewinding to index 0 leaves only the first event.
func TestRewindToIndex_Zero(t *testing.T) {
	ctx := context.Background()
	svc := session.InMemoryService()
	sessID := "rewind-index-zero"

	createSession(t, ctx, svc, sessID)

	var firstID string
	for i := range 5 {
		sess := getSession(t, ctx, svc, sessID)
		id := appendEventWithDelta(t, ctx, svc, sess, map[string]any{
			fmt.Sprintf("k%d", i): i,
		})
		if i == 0 {
			firstID = id
		}
	}

	result, err := rewind.RewindToIndex(ctx, svc, testApp, testUser, sessID, 0)
	if err != nil {
		t.Fatalf("RewindToIndex(0) returned error: %v", err)
	}

	got := result.Events().Len()
	if got != 1 {
		t.Errorf("expected 1 event after rewind to index 0, got %d", got)
	}

	ev := result.Events().At(0)
	if ev.ID != firstID {
		t.Errorf("retained event ID = %q, want %q", ev.ID, firstID)
	}
}
