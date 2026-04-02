package agui_test

import (
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ieshan/adk-go-pkg/agui"
)

func TestReasoningTracker_FullLifecycle(t *testing.T) {
	ch := make(chan events.Event, 16)
	em := agui.NewEventEmitter(ch)

	msgID := em.GenerateMessageID()
	tracker := agui.NewReasoningTracker(em, msgID)

	// Start -> Content -> Content -> End
	if err := tracker.Start("reasoning"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := tracker.Content("Let me think..."); err != nil {
		t.Fatalf("Content 1: %v", err)
	}
	if err := tracker.Content("The answer is 42."); err != nil {
		t.Fatalf("Content 2: %v", err)
	}
	if err := tracker.End(); err != nil {
		t.Fatalf("End: %v", err)
	}

	got := drain(ch)
	if len(got) != 6 {
		t.Fatalf("expected 6 events, got %d", len(got))
	}

	expected := []events.EventType{
		events.EventTypeReasoningStart,
		events.EventTypeReasoningMessageStart,
		events.EventTypeReasoningMessageContent,
		events.EventTypeReasoningMessageContent,
		events.EventTypeReasoningMessageEnd,
		events.EventTypeReasoningEnd,
	}
	for i, want := range expected {
		if got[i].Type() != want {
			t.Errorf("event[%d] type = %s, want %s", i, got[i].Type(), want)
		}
	}
}
