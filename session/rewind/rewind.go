// Package rewind provides utilities for truncating (rewinding) ADK sessions
// to a prior event, discarding all events that came after the target.
//
// Rewinding is useful in agentic workflows when you need to roll back a session
// to an earlier state — for example, after a tool call produced an undesirable
// result, or to replay a branch of conversation from a specific checkpoint.
//
// # How it works
//
// Rewind fetches the session, locates the target event (by ID or 0-based index),
// collects all events up to and including that event, replays their StateDelta
// entries to rebuild the session state, then persists the truncated session
// via a create-before-delete swap to minimise the window where data is missing.
//
// # Example
//
//	ctx := context.Background()
//	svc := session.InMemoryService()
//
//	// ... populate session with events ...
//
//	rewound, err := rewind.Rewind(ctx, svc, "my-app", "user-1", "session-abc", targetEventID)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println("events remaining:", rewound.Events().Len())
package rewind

import (
	"context"
	"fmt"
	"strings"

	"github.com/ieshan/adk-go-pkg/internal/jsonutil"
	"google.golang.org/adk/session"
)

// Rewind truncates a session to the event with the given ID, discarding all
// subsequent events and recalculating the session state by replaying only the
// retained events' StateDelta values.
//
// The targetEventID must match the ID of an existing event in the session;
// otherwise an error is returned. If targetEventID identifies the last event,
// no events are dropped (effectively a no-op).
//
// Example:
//
//	rewound, err := rewind.Rewind(ctx, svc, "my-app", "user-1", "session-xyz", eventID)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println("events now:", rewound.Events().Len())
func Rewind(ctx context.Context, svc session.Service, appName, userID, sessionID, targetEventID string) (session.Session, error) {
	sess, err := fetchSession(ctx, svc, appName, userID, sessionID)
	if err != nil {
		return nil, err
	}

	evts := sess.Events()
	n := evts.Len()
	if n == 0 {
		return nil, fmt.Errorf("rewind: session %q has no events", sessionID)
	}

	targetIndex := -1
	for i := range n {
		if evts.At(i).ID == targetEventID {
			targetIndex = i
			break
		}
	}
	if targetIndex < 0 {
		return nil, fmt.Errorf("rewind: event %q not found in session %q", targetEventID, sessionID)
	}

	return applyRewind(ctx, svc, appName, userID, sessionID, sess, targetIndex)
}

// RewindToIndex truncates a session to the event at the given 0-based index,
// discarding all subsequent events and recalculating the session state by
// replaying only the retained events' StateDelta values.
//
// targetIndex must be in [0, eventCount-1]; otherwise an error is returned.
//
// Example:
//
//	// Keep only the first event.
//	rewound, err := rewind.RewindToIndex(ctx, svc, "my-app", "user-1", "session-xyz", 0)
//	if err != nil {
//	    log.Fatal(err)
//	}
func RewindToIndex(ctx context.Context, svc session.Service, appName, userID, sessionID string, targetIndex int) (session.Session, error) {
	sess, err := fetchSession(ctx, svc, appName, userID, sessionID)
	if err != nil {
		return nil, err
	}

	evts := sess.Events()
	n := evts.Len()
	if n == 0 {
		return nil, fmt.Errorf("rewind: session %q has no events", sessionID)
	}

	if targetIndex < 0 || targetIndex >= n {
		return nil, fmt.Errorf("rewind: index %d out of bounds for session %q with %d events", targetIndex, sessionID, n)
	}

	return applyRewind(ctx, svc, appName, userID, sessionID, sess, targetIndex)
}

// fetchSession retrieves the session from the service.
func fetchSession(ctx context.Context, svc session.Service, appName, userID, sessionID string) (session.Session, error) {
	resp, err := svc.Get(ctx, &session.GetRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return nil, fmt.Errorf("rewind: failed to get session %q: %w", sessionID, err)
	}
	return resp.Session, nil
}

// applyRewind persists the truncated session using a create-before-delete swap:
//
//  1. Collect events[0..targetIndex] and replay their StateDelta to build state.
//  2. Create a temporary session and append the kept events one by one.
//  3. Delete the original session.
//  4. Create a new session with the original ID and the recalculated state.
//  5. Append the kept events to the new session.
//  6. Delete the temporary session.
//  7. Return the final session (fetched fresh).
func applyRewind(ctx context.Context, svc session.Service, appName, userID, sessionID string, sess session.Session, targetIndex int) (session.Session, error) {
	evts := sess.Events()

	// Collect the events to retain (indices 0..targetIndex inclusive).
	kept := make([]*session.Event, targetIndex+1)
	for i := range targetIndex + 1 {
		kept[i] = evts.At(i)
	}

	// Recalculate state by replaying StateDelta of kept events (session-scoped keys only).
	replayedState := replayState(kept)

	// --- Step 2: create a temporary session and fill it ---
	tempID := "rewind-tmp-" + jsonutil.GenerateID(16)
	tmpResp, err := svc.Create(ctx, &session.CreateRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: tempID,
	})
	if err != nil {
		return nil, fmt.Errorf("rewind: failed to create temp session: %w", err)
	}
	tmpSess := tmpResp.Session

	for _, ev := range kept {
		if err := svc.AppendEvent(ctx, tmpSess, ev); err != nil {
			return nil, fmt.Errorf("rewind: failed to append event to temp session: %w", err)
		}
		// Refresh tmpSess so AppendEvent has the up-to-date handle.
		tmpSess, err = fetchSession(ctx, svc, appName, userID, tempID)
		if err != nil {
			return nil, fmt.Errorf("rewind: failed to refresh temp session: %w", err)
		}
	}

	// --- Step 3: delete original session ---
	if err := svc.Delete(ctx, &session.DeleteRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
	}); err != nil {
		return nil, fmt.Errorf("rewind: failed to delete original session: %w", err)
	}

	// --- Step 4: create new session with original ID and replayed state ---
	newResp, err := svc.Create(ctx, &session.CreateRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
		State:     replayedState,
	})
	if err != nil {
		return nil, fmt.Errorf("rewind: failed to recreate session with original ID: %w", err)
	}
	newSess := newResp.Session

	// --- Step 5: append kept events to new session ---
	for _, ev := range kept {
		if err := svc.AppendEvent(ctx, newSess, ev); err != nil {
			return nil, fmt.Errorf("rewind: failed to append event to new session: %w", err)
		}
		newSess, err = fetchSession(ctx, svc, appName, userID, sessionID)
		if err != nil {
			return nil, fmt.Errorf("rewind: failed to refresh new session: %w", err)
		}
	}

	// --- Step 6: delete temp session ---
	if err := svc.Delete(ctx, &session.DeleteRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: tempID,
	}); err != nil {
		// Non-fatal; log by returning a wrapped error but still return the new session.
		return newSess, fmt.Errorf("rewind: cleanup failed (temp session %q not deleted): %w", tempID, err)
	}

	// --- Step 7: return fresh copy of the final session ---
	return fetchSession(ctx, svc, appName, userID, sessionID)
}

// replayState builds a state map by replaying the StateDelta of each kept event
// in order, skipping app-scoped ("app:") and user-scoped ("user:") and
// temporary ("temp:") keys because those are managed separately by the service.
func replayState(kept []*session.Event) map[string]any {
	state := make(map[string]any)
	for _, ev := range kept {
		for k, v := range ev.Actions.StateDelta {
			if strings.HasPrefix(k, session.KeyPrefixApp) ||
				strings.HasPrefix(k, session.KeyPrefixUser) ||
				strings.HasPrefix(k, session.KeyPrefixTemp) {
				continue
			}
			state[k] = v
		}
	}
	return state
}
