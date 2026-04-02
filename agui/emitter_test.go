package agui_test

import (
	"strings"
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func TestRunLifecycle(t *testing.T) {
	ch := make(chan events.Event, 16)
	em := agui.NewEventEmitter(ch)

	if err := em.RunStarted("thread-1", "run-1"); err != nil {
		t.Fatalf("RunStarted: %v", err)
	}
	if err := em.RunFinished("thread-1", "run-1"); err != nil {
		t.Fatalf("RunFinished: %v", err)
	}

	got := drain(ch)
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}

	if got[0].Type() != events.EventTypeRunStarted {
		t.Errorf("event[0] type = %s, want RUN_STARTED", got[0].Type())
	}
	if got[1].Type() != events.EventTypeRunFinished {
		t.Errorf("event[1] type = %s, want RUN_FINISHED", got[1].Type())
	}
}

func TestTextMessageSequence(t *testing.T) {
	ch := make(chan events.Event, 16)
	em := agui.NewEventEmitter(ch)

	msgID := em.GenerateMessageID()
	if err := em.TextMessageStart(msgID, nil); err != nil {
		t.Fatalf("TextMessageStart: %v", err)
	}
	if err := em.TextMessageContent(msgID, "Hello, "); err != nil {
		t.Fatalf("TextMessageContent: %v", err)
	}
	if err := em.TextMessageContent(msgID, "world!"); err != nil {
		t.Fatalf("TextMessageContent: %v", err)
	}
	if err := em.TextMessageEnd(msgID); err != nil {
		t.Fatalf("TextMessageEnd: %v", err)
	}

	got := drain(ch)
	if len(got) != 4 {
		t.Fatalf("expected 4 events, got %d", len(got))
	}

	expected := []events.EventType{
		events.EventTypeTextMessageStart,
		events.EventTypeTextMessageContent,
		events.EventTypeTextMessageContent,
		events.EventTypeTextMessageEnd,
	}
	for i, want := range expected {
		if got[i].Type() != want {
			t.Errorf("event[%d] type = %s, want %s", i, got[i].Type(), want)
		}
	}
}

func TestToolCallSequence(t *testing.T) {
	ch := make(chan events.Event, 16)
	em := agui.NewEventEmitter(ch)

	tcID := em.GenerateToolCallID()
	if err := em.ToolCallStart(tcID, "search", new("msg-parent")); err != nil {
		t.Fatalf("ToolCallStart: %v", err)
	}
	if err := em.ToolCallArgs(tcID, `{"query":`); err != nil {
		t.Fatalf("ToolCallArgs: %v", err)
	}
	if err := em.ToolCallArgs(tcID, `"hello"}`); err != nil {
		t.Fatalf("ToolCallArgs: %v", err)
	}
	if err := em.ToolCallEnd(tcID); err != nil {
		t.Fatalf("ToolCallEnd: %v", err)
	}

	got := drain(ch)
	if len(got) != 4 {
		t.Fatalf("expected 4 events, got %d", len(got))
	}

	expected := []events.EventType{
		events.EventTypeToolCallStart,
		events.EventTypeToolCallArgs,
		events.EventTypeToolCallArgs,
		events.EventTypeToolCallEnd,
	}
	for i, want := range expected {
		if got[i].Type() != want {
			t.Errorf("event[%d] type = %s, want %s", i, got[i].Type(), want)
		}
	}
}

func TestStateSnapshotAndDelta(t *testing.T) {
	ch := make(chan events.Event, 16)
	em := agui.NewEventEmitter(ch)

	state := map[string]any{"counter": 0}
	if err := em.StateSnapshot(state); err != nil {
		t.Fatalf("StateSnapshot: %v", err)
	}

	delta := []events.JSONPatchOperation{
		{Op: "replace", Path: "/counter", Value: 1},
	}
	if err := em.StateDelta(delta); err != nil {
		t.Fatalf("StateDelta: %v", err)
	}

	got := drain(ch)
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}

	if got[0].Type() != events.EventTypeStateSnapshot {
		t.Errorf("event[0] type = %s, want STATE_SNAPSHOT", got[0].Type())
	}
	if got[1].Type() != events.EventTypeStateDelta {
		t.Errorf("event[1] type = %s, want STATE_DELTA", got[1].Type())
	}
}

func TestGenerateIDsUniqueness(t *testing.T) {
	ch := make(chan events.Event, 1)
	em := agui.NewEventEmitter(ch)

	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := em.GenerateMessageID()
		if seen[id] {
			t.Fatalf("duplicate message ID: %s", id)
		}
		seen[id] = true
		if !strings.HasPrefix(id, "msg-") {
			t.Errorf("message ID %q lacks msg- prefix", id)
		}
	}

	seenTC := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := em.GenerateToolCallID()
		if seenTC[id] {
			t.Fatalf("duplicate tool call ID: %s", id)
		}
		seenTC[id] = true
		if !strings.HasPrefix(id, "tool-") {
			t.Errorf("tool call ID %q lacks tool- prefix", id)
		}
	}
}

func TestRunErrorWithCode(t *testing.T) {
	ch := make(chan events.Event, 16)
	em := agui.NewEventEmitter(ch)

	if err := em.RunError("too many requests", new("RATE_LIMITED")); err != nil {
		t.Fatalf("RunError: %v", err)
	}

	got := drain(ch)
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Type() != events.EventTypeRunError {
		t.Errorf("event type = %s, want RUN_ERROR", got[0].Type())
	}

	errEvt, ok := got[0].(*events.RunErrorEvent)
	if !ok {
		t.Fatalf("expected *events.RunErrorEvent, got %T", got[0])
	}
	if errEvt.Message != "too many requests" {
		t.Errorf("message = %q, want %q", errEvt.Message, "too many requests")
	}
	if errEvt.Code == nil || *errEvt.Code != "RATE_LIMITED" {
		t.Errorf("code = %v, want RATE_LIMITED", errEvt.Code)
	}
}

func TestRunErrorWithoutCode(t *testing.T) {
	ch := make(chan events.Event, 16)
	em := agui.NewEventEmitter(ch)

	if err := em.RunError("internal error", nil); err != nil {
		t.Fatalf("RunError: %v", err)
	}

	got := drain(ch)
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}

	errEvt, ok := got[0].(*events.RunErrorEvent)
	if !ok {
		t.Fatalf("expected *events.RunErrorEvent, got %T", got[0])
	}
	if errEvt.Code != nil {
		t.Errorf("code = %v, want nil", errEvt.Code)
	}
}

func TestStepStartedFinished(t *testing.T) {
	ch := make(chan events.Event, 16)
	em := agui.NewEventEmitter(ch)

	if err := em.StepStarted("planning"); err != nil {
		t.Fatalf("StepStarted: %v", err)
	}
	if err := em.StepFinished("planning"); err != nil {
		t.Fatalf("StepFinished: %v", err)
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

func TestToolCallResult(t *testing.T) {
	ch := make(chan events.Event, 16)
	em := agui.NewEventEmitter(ch)

	if err := em.ToolCallResult("msg-1", "tool-1", `{"result": "ok"}`); err != nil {
		t.Fatalf("ToolCallResult: %v", err)
	}

	got := drain(ch)
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Type() != events.EventTypeToolCallResult {
		t.Errorf("event type = %s, want TOOL_CALL_RESULT", got[0].Type())
	}
}

func TestCustomEvent(t *testing.T) {
	ch := make(chan events.Event, 16)
	em := agui.NewEventEmitter(ch)

	if err := em.Custom("my.event", map[string]any{"key": "val"}); err != nil {
		t.Fatalf("Custom: %v", err)
	}

	got := drain(ch)
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Type() != events.EventTypeCustom {
		t.Errorf("event type = %s, want CUSTOM", got[0].Type())
	}
}

func TestRawEvent(t *testing.T) {
	ch := make(chan events.Event, 16)
	em := agui.NewEventEmitter(ch)

	if err := em.Raw(map[string]any{"raw": true}, new("openai")); err != nil {
		t.Fatalf("Raw: %v", err)
	}

	got := drain(ch)
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Type() != events.EventTypeRaw {
		t.Errorf("event type = %s, want RAW", got[0].Type())
	}
}

func TestReasoningLifecycle(t *testing.T) {
	ch := make(chan events.Event, 16)
	em := agui.NewEventEmitter(ch)

	msgID := em.GenerateMessageID()
	if err := em.ReasoningStart(msgID); err != nil {
		t.Fatalf("ReasoningStart: %v", err)
	}
	if err := em.ReasoningMessageStart(msgID, "reasoning"); err != nil {
		t.Fatalf("ReasoningMessageStart: %v", err)
	}
	if err := em.ReasoningMessageContent(msgID, "thinking..."); err != nil {
		t.Fatalf("ReasoningMessageContent: %v", err)
	}
	if err := em.ReasoningMessageEnd(msgID); err != nil {
		t.Fatalf("ReasoningMessageEnd: %v", err)
	}
	if err := em.ReasoningEnd(msgID); err != nil {
		t.Fatalf("ReasoningEnd: %v", err)
	}

	got := drain(ch)
	if len(got) != 5 {
		t.Fatalf("expected 5 events, got %d", len(got))
	}

	expected := []events.EventType{
		events.EventTypeReasoningStart,
		events.EventTypeReasoningMessageStart,
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

func TestEventEmitter_TextMessageChunk(t *testing.T) {
	ch := make(chan events.Event, 5)
	em := agui.NewEventEmitter(ch)
	if err := em.TextMessageChunk(new("msg1"), new("assistant"), new("hello")); err != nil {
		t.Fatal(err)
	}
	evts := drain(ch)
	if len(evts) != 1 || evts[0].Type() != events.EventTypeTextMessageChunk {
		t.Errorf("expected TEXT_MESSAGE_CHUNK, got %v", evts)
	}
}

func TestEventEmitter_ReasoningEncryptedValue(t *testing.T) {
	ch := make(chan events.Event, 5)
	em := agui.NewEventEmitter(ch)
	if err := em.ReasoningEncryptedValue(events.ReasoningEncryptedValueSubtypeMessage, "entity1", "encrypted-data"); err != nil {
		t.Fatal(err)
	}
	evts := drain(ch)
	if len(evts) != 1 || evts[0].Type() != events.EventTypeReasoningEncryptedValue {
		t.Errorf("expected REASONING_ENCRYPTED_VALUE, got %v", evts)
	}
}

func TestEventEmitter_MessagesSnapshot(t *testing.T) {
	ch := make(chan events.Event, 5)
	em := agui.NewEventEmitter(ch)
	if err := em.MessagesSnapshot([]types.Message{{ID: "m1", Role: types.RoleUser}}); err != nil {
		t.Fatal(err)
	}
	evts := drain(ch)
	if len(evts) != 1 || evts[0].Type() != events.EventTypeMessagesSnapshot {
		t.Errorf("expected MESSAGES_SNAPSHOT, got %v", evts)
	}
}

func TestActivitySnapshotAndDelta(t *testing.T) {
	ch := make(chan events.Event, 16)
	em := agui.NewEventEmitter(ch)

	if err := em.ActivitySnapshot("msg-1", "progress", map[string]any{"pct": 50}, new(false)); err != nil {
		t.Fatalf("ActivitySnapshot: %v", err)
	}

	patch := []events.JSONPatchOperation{
		{Op: "replace", Path: "/pct", Value: 100},
	}
	if err := em.ActivityDelta("msg-1", "progress", patch); err != nil {
		t.Fatalf("ActivityDelta: %v", err)
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
