package agui_test

import (
	"errors"
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ieshan/adk-go-pkg/agui"
)

func TestStepTracker_Lifecycle(t *testing.T) {
	ch := make(chan events.Event, 16)
	em := agui.NewEventEmitter(ch)

	called := false
	err := agui.StepTracker(em, "planning", func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("StepTracker returned error: %v", err)
	}
	if !called {
		t.Fatal("fn was not called")
	}

	got := drain(ch)
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}
	if got[0].Type() != events.EventTypeStepStarted {
		t.Errorf("event[0] type = %s, want STEP_STARTED", got[0].Type())
	}
	if got[1].Type() != events.EventTypeStepFinished {
		t.Errorf("event[1] type = %s, want STEP_FINISHED", got[1].Type())
	}
}

func TestStepTracker_ErrorPropagation(t *testing.T) {
	ch := make(chan events.Event, 16)
	em := agui.NewEventEmitter(ch)

	want := errors.New("step failed")
	err := agui.StepTracker(em, "failing-step", func() error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected error %v, got %v", want, err)
	}

	// Both STEP_STARTED and STEP_FINISHED should still be emitted.
	got := drain(ch)
	if len(got) != 2 {
		t.Fatalf("expected 2 events even on error, got %d", len(got))
	}
	if got[0].Type() != events.EventTypeStepStarted {
		t.Errorf("event[0] type = %s, want STEP_STARTED", got[0].Type())
	}
	if got[1].Type() != events.EventTypeStepFinished {
		t.Errorf("event[1] type = %s, want STEP_FINISHED", got[1].Type())
	}
}

func TestActivityTracker_SnapshotAndDelta(t *testing.T) {
	ch := make(chan events.Event, 16)
	em := agui.NewEventEmitter(ch)

	tracker := agui.NewActivityTracker(em, "msg-1", "progress")

	if err := tracker.Snapshot(map[string]any{"pct": 50}, new(false)); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	patch := []events.JSONPatchOperation{
		{Op: "replace", Path: "/pct", Value: 100},
	}
	if err := tracker.Delta(patch); err != nil {
		t.Fatalf("Delta: %v", err)
	}

	got := drain(ch)
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}
	if got[0].Type() != events.EventTypeActivitySnapshot {
		t.Errorf("event[0] type = %s, want ACTIVITY_SNAPSHOT", got[0].Type())
	}
	if got[1].Type() != events.EventTypeActivityDelta {
		t.Errorf("event[1] type = %s, want ACTIVITY_DELTA", got[1].Type())
	}
}
