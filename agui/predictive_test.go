package agui_test

import (
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"

	"github.com/ieshan/adk-go-pkg/agui"
)

func newPredictiveTracker(t *testing.T) (*agui.PredictiveStateTracker, chan events.Event) {
	t.Helper()
	ch := make(chan events.Event, 64)
	emitter := agui.NewEventEmitter(ch)
	state, err := agui.NewStateManager(map[string]any{})
	if err != nil {
		t.Fatalf("NewStateManager: %v", err)
	}
	return agui.NewPredictiveStateTracker(state, emitter), ch
}

func drainEvents(ch <-chan events.Event) []events.Event {
	var out []events.Event
	for {
		select {
		case ev := <-ch:
			out = append(out, ev)
		default:
			return out
		}
	}
}

func TestPredictiveState_PredictiveDelta(t *testing.T) {
	tracker, ch := newPredictiveTracker(t)

	err := tracker.PredictiveDelta([]events.JSONPatchOperation{
		{Op: "add", Path: "/draft", Value: "hello"},
	})
	if err != nil {
		t.Fatalf("PredictiveDelta: %v", err)
	}

	evts := drainEvents(ch)
	if len(evts) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evts))
	}
	sd, ok := evts[0].(*events.StateDeltaEvent)
	if !ok {
		t.Fatalf("expected *StateDeltaEvent, got %T", evts[0])
	}
	if len(sd.Delta) != 1 {
		t.Fatalf("expected 1 op, got %d", len(sd.Delta))
	}
	if sd.Delta[0].Path != "/_predictive/draft" {
		t.Errorf("path = %q, want %q", sd.Delta[0].Path, "/_predictive/draft")
	}
}

func TestPredictiveState_Commit(t *testing.T) {
	tracker, ch := newPredictiveTracker(t)

	// Predict then commit.
	if err := tracker.PredictiveDelta([]events.JSONPatchOperation{
		{Op: "add", Path: "/draft", Value: "preview"},
	}); err != nil {
		t.Fatalf("PredictiveDelta: %v", err)
	}
	drainEvents(ch)

	if err := tracker.Commit("/recipe/steps", []any{"step1", "step2"}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	evts := drainEvents(ch)
	if len(evts) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evts))
	}
	sd, ok := evts[0].(*events.StateDeltaEvent)
	if !ok {
		t.Fatalf("expected *StateDeltaEvent, got %T", evts[0])
	}
	if sd.Delta[0].Path != "/recipe/steps" {
		t.Errorf("commit path = %q, want %q", sd.Delta[0].Path, "/recipe/steps")
	}
}

func TestPredictiveState_Clear(t *testing.T) {
	tracker, ch := newPredictiveTracker(t)

	// Seed the predictive namespace.
	if err := tracker.PredictiveDelta([]events.JSONPatchOperation{
		{Op: "add", Path: "/draft", Value: "preview"},
	}); err != nil {
		t.Fatalf("PredictiveDelta: %v", err)
	}
	drainEvents(ch)

	if err := tracker.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	evts := drainEvents(ch)
	if len(evts) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evts))
	}
	sd, ok := evts[0].(*events.StateDeltaEvent)
	if !ok {
		t.Fatalf("expected *StateDeltaEvent, got %T", evts[0])
	}
	if sd.Delta[0].Op != "remove" {
		t.Errorf("clear op = %q, want %q", sd.Delta[0].Op, "remove")
	}
	if sd.Delta[0].Path != "/_predictive" {
		t.Errorf("clear path = %q, want %q", sd.Delta[0].Path, "/_predictive")
	}
}

func TestPredictiveState_EmptyPatchNoop(t *testing.T) {
	tracker, ch := newPredictiveTracker(t)

	if err := tracker.PredictiveDelta(nil); err != nil {
		t.Fatalf("PredictiveDelta(nil): %v", err)
	}

	evts := drainEvents(ch)
	if len(evts) != 0 {
		t.Errorf("expected 0 events for nil patch, got %d", len(evts))
	}
}

func TestPredictiveState_StateIsolation(t *testing.T) {
	tracker, ch := newPredictiveTracker(t)

	// A prediction should not affect the real state path.
	if err := tracker.PredictiveDelta([]events.JSONPatchOperation{
		{Op: "add", Path: "/steps", Value: []any{"predicted"}},
	}); err != nil {
		t.Fatalf("PredictiveDelta: %v", err)
	}
	drainEvents(ch)

	// Commit to the real path — it should not carry the predicted value.
	if err := tracker.Commit("/steps", []any{"committed"}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	drainEvents(ch)

	// Verify the real state has the committed value, not the predicted one.
	snap := tracker.State().Snapshot()
	m, ok := snap.(map[string]any)
	if !ok {
		t.Fatalf("Snapshot type = %T, want map[string]any", snap)
	}
	steps, ok := m["steps"].([]any)
	if !ok {
		t.Fatalf("state[steps] type = %T, want []any", m["steps"])
	}
	if len(steps) != 1 || steps[0] != "committed" {
		t.Errorf("state[steps] = %v, want [committed]", steps)
	}

	// The predictive namespace should still hold the predicted value.
	pred, ok := m["_predictive"].(map[string]any)
	if !ok {
		t.Fatalf("state[_predictive] type = %T, want map[string]any", m["_predictive"])
	}
	predSteps, ok := pred["steps"].([]any)
	if !ok {
		t.Fatalf("predictive[steps] type = %T, want []any", pred["steps"])
	}
	if len(predSteps) != 1 || predSteps[0] != "predicted" {
		t.Errorf("predictive[steps] = %v, want [predicted]", predSteps)
	}
}

func TestPredictiveState_FullCycle(t *testing.T) {
	tracker, ch := newPredictiveTracker(t)

	// Stream growing predictions — each should preserve the prior one's
	// siblings, only updating /_predictive/draft.
	for _, s := range []string{"step", "step1", "step1 step2"} {
		if err := tracker.PredictiveDelta([]events.JSONPatchOperation{
			{Op: "add", Path: "/draft", Value: s},
		}); err != nil {
			t.Fatalf("PredictiveDelta: %v", err)
		}
	}
	drainEvents(ch)

	// Verify the last prediction survived (not wiped by subsequent calls).
	snap := tracker.State().Snapshot()
	m, _ := snap.(map[string]any)
	pred, _ := m["_predictive"].(map[string]any)
	if pred["draft"] != "step1 step2" {
		t.Errorf("predictive[draft] = %v, want %q (prior predictions were wiped)", pred["draft"], "step1 step2")
	}

	// Commit the final value.
	if err := tracker.Commit("/steps", []any{"step1", "step2"}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	drainEvents(ch)

	// Clear the prediction.
	if err := tracker.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	evts := drainEvents(ch)
	if len(evts) != 1 {
		t.Fatalf("expected 1 clear event, got %d", len(evts))
	}
}
