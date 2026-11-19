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
		t.Fatalf("got %d events, want 6", len(got))
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

	// Spans and reasoning messages use distinct IDs: the span ID derives as
	// messageID+"-span"; message events carry messageID.
	spanID := msgID + "-span"
	start, ok := got[0].(*events.ReasoningStartEvent)
	if !ok {
		t.Fatalf("got %T, want *events.ReasoningStartEvent", got[0])
	}
	if start.MessageID != spanID {
		t.Errorf("REASONING_START messageId = %q, want %q", start.MessageID, spanID)
	}
	msgStart, ok := got[1].(*events.ReasoningMessageStartEvent)
	if !ok {
		t.Fatalf("got %T, want *events.ReasoningMessageStartEvent", got[1])
	}
	if msgStart.MessageID != msgID {
		t.Errorf("REASONING_MESSAGE_START messageId = %q, want %q", msgStart.MessageID, msgID)
	}
	msgEnd, ok := got[4].(*events.ReasoningMessageEndEvent)
	if !ok {
		t.Fatalf("got %T, want *events.ReasoningMessageEndEvent", got[4])
	}
	if msgEnd.MessageID != msgID {
		t.Errorf("REASONING_MESSAGE_END messageId = %q, want %q", msgEnd.MessageID, msgID)
	}
	end, ok := got[5].(*events.ReasoningEndEvent)
	if !ok {
		t.Fatalf("got %T, want *events.ReasoningEndEvent", got[5])
	}
	if end.MessageID != spanID {
		t.Errorf("REASONING_END messageId = %q, want %q", end.MessageID, spanID)
	}
}
