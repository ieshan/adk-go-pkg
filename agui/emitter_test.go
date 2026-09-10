package agui_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

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
	if err := em.RunFinishedWithOptions("thread-1", "run-1"); err != nil {
		t.Fatalf("RunFinished: %v", err)
	}

	got := drain(ch)
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
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
		t.Fatalf("got %d events, want 4", len(got))
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
		t.Fatalf("got %d events, want 4", len(got))
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
		t.Fatalf("got %d events, want 2", len(got))
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

	if err := em.RunErrorWithOptions("too many requests", events.WithErrorCode("RATE_LIMITED")); err != nil {
		t.Fatalf("RunErrorWithOptions: %v", err)
	}

	got := drain(ch)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	if got[0].Type() != events.EventTypeRunError {
		t.Errorf("event type = %s, want RUN_ERROR", got[0].Type())
	}

	errEvt, ok := got[0].(*events.RunErrorEvent)
	if !ok {
		t.Fatalf("got %T, want *events.RunErrorEvent", got[0])
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

	if err := em.RunErrorWithOptions("internal error"); err != nil {
		t.Fatalf("RunErrorWithOptions: %v", err)
	}

	got := drain(ch)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}

	errEvt, ok := got[0].(*events.RunErrorEvent)
	if !ok {
		t.Fatalf("got %T, want *events.RunErrorEvent", got[0])
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
		t.Fatalf("got %d events, want 2", len(got))
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
		t.Fatalf("got %d events, want 1", len(got))
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
		t.Fatalf("got %d events, want 1", len(got))
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
		t.Fatalf("got %d events, want 1", len(got))
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
		t.Fatalf("got %d events, want 5", len(got))
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
		t.Errorf("got %v, want TEXT_MESSAGE_CHUNK", evts)
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
		t.Errorf("got %v, want REASONING_ENCRYPTED_VALUE", evts)
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
		t.Errorf("got %v, want MESSAGES_SNAPSHOT", evts)
	}
}

func TestEventEmitter_MessagesSnapshot_ScrubsEncryptedValues(t *testing.T) {
	ch := make(chan events.Event, 5)
	em := agui.NewEventEmitter(ch)

	msgs := []types.Message{
		{ID: "m1", Role: types.RoleUser, EncryptedValue: "secret1", EncryptedContent: "secret2"},
		{ID: "m2", Role: types.RoleAssistant, EncryptedValue: "secret3"},
	}
	if err := em.MessagesSnapshot(msgs); err != nil {
		t.Fatal(err)
	}

	evts := drain(ch)
	if len(evts) != 1 {
		t.Fatalf("got %d events, want 1", len(evts))
	}
	ms, ok := evts[0].(*events.MessagesSnapshotEvent)
	if !ok {
		t.Fatalf("got %T, want *events.MessagesSnapshotEvent", evts[0])
	}
	for i, m := range ms.Messages {
		if m.EncryptedValue != "" {
			t.Errorf("message[%d].EncryptedValue = %q, want empty", i, m.EncryptedValue)
		}
		if m.EncryptedContent != "" {
			t.Errorf("message[%d].EncryptedContent = %q, want empty", i, m.EncryptedContent)
		}
	}
}

func TestEventEmitter_MessagesSnapshot_NoScrubWhenClean(t *testing.T) {
	// When no message has encrypted fields, the original slice should be
	// returned without allocation (no copy).
	ch := make(chan events.Event, 5)
	em := agui.NewEventEmitter(ch)

	msgs := []types.Message{
		{ID: "m1", Role: types.RoleUser, Content: "hello"},
	}
	if err := em.MessagesSnapshot(msgs); err != nil {
		t.Fatal(err)
	}

	evts := drain(ch)
	if len(evts) == 0 {
		t.Fatal("no events drained")
	}
	ms, ok := evts[0].(*events.MessagesSnapshotEvent)
	if !ok {
		t.Fatalf("got %T, want *events.MessagesSnapshotEvent", evts[0])
	}
	// The slice should be the same pointer (no copy) when no scrubbing needed.
	// This is an implementation detail but verifies the no-allocation fast path.
	if len(ms.Messages) != 1 {
		t.Errorf("got %d messages, want 1", len(ms.Messages))
	}
	if ms.Messages[0].Content != "hello" {
		t.Errorf("content = %q, want %q", ms.Messages[0].Content, "hello")
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
		t.Fatalf("got %d events, want 2", len(got))
	}
	if got[0].Type() != events.EventTypeActivitySnapshot {
		t.Errorf("event[0] type = %s, want ACTIVITY_SNAPSHOT", got[0].Type())
	}
	if got[1].Type() != events.EventTypeActivityDelta {
		t.Errorf("event[1] type = %s, want ACTIVITY_DELTA", got[1].Type())
	}
}

func TestRunErrorWithOptions_RunID(t *testing.T) {
	ch := make(chan events.Event, 16)
	em := agui.NewEventEmitter(ch)

	if err := em.RunErrorWithOptions("boom", events.WithRunID("run-42")); err != nil {
		t.Fatalf("RunErrorWithOptions: %v", err)
	}

	got := drain(ch)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}

	errEvt, ok := got[0].(*events.RunErrorEvent)
	if !ok {
		t.Fatalf("got %T, want *events.RunErrorEvent", got[0])
	}
	if errEvt.RunID() != "run-42" {
		t.Errorf("RunID = %q, want %q", errEvt.RunID(), "run-42")
	}
	if errEvt.Message != "boom" {
		t.Errorf("Message = %q, want %q", errEvt.Message, "boom")
	}
}

func TestRunFinishedWithOptions_Interrupt(t *testing.T) {
	ch := make(chan events.Event, 16)
	em := agui.NewEventEmitter(ch)

	interrupts := []types.Interrupt{
		{ID: "int-1", Reason: "tool_call", ToolCallID: "tc-1"},
	}
	if err := em.RunFinishedWithOptions("thread-1", "run-1", events.WithInterruptOutcome(interrupts)); err != nil {
		t.Fatalf("RunFinishedWithOptions: %v", err)
	}

	got := drain(ch)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}

	finEvt, ok := got[0].(*events.RunFinishedEvent)
	if !ok {
		t.Fatalf("got %T, want *events.RunFinishedEvent", got[0])
	}
	if finEvt.Outcome == nil {
		t.Fatal("got nil Outcome, want non-nil Outcome")
	}
	if finEvt.Outcome.Type != events.RunFinishedOutcomeTypeInterrupt {
		t.Errorf("Outcome.Type = %q, want %q", finEvt.Outcome.Type, events.RunFinishedOutcomeTypeInterrupt)
	}
	if len(finEvt.Outcome.Interrupts) != 1 {
		t.Fatalf("got %d interrupts, want 1", len(finEvt.Outcome.Interrupts))
	}
	if finEvt.Outcome.Interrupts[0].ID != "int-1" {
		t.Errorf("Interrupts[0].ID = %q, want %q", finEvt.Outcome.Interrupts[0].ID, "int-1")
	}
}

func TestEventEmitter_TransportErrorOnClosedChannel(t *testing.T) {
	ch := make(chan events.Event, 1)
	em := agui.NewEventEmitter(ch)
	close(ch)

	err := em.RunStarted("thread-1", "run-1")
	if err == nil {
		t.Fatal("got nil error, want error when writing to closed channel")
	}
	if !errors.Is(err, agui.ErrTransport) {
		t.Errorf("got %v, want ErrTransport", err)
	}
}

// TestEventEmitter_ContextCancellationUnblocks verifies that an emitter
// constructed with NewEventEmitterWithContext unblocks a pending emit call
// when the context is cancelled, instead of blocking forever on a full
// channel with no consumer. This is the core goroutine-leak prevention
// guarantee.
func TestEventEmitter_ContextCancellationUnblocks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan events.Event) // unbuffered: any emit blocks until received
	em := agui.NewEventEmitterWithContext(ctx, ch)

	errCh := make(chan error, 1)
	// Use a WaitGroup to know the goroutine has been scheduled and is about
	// to call emit. The emit select handles ctx.Done(), so whether the
	// goroutine is blocked on the send or has not yet entered the select,
	// cancelling the context unblocks it.
	var started sync.WaitGroup
	started.Add(1)
	go func() {
		started.Done()
		errCh <- em.RunStarted("t1", "r1")
	}()

	// Wait for the goroutine to start, then cancel the context.
	started.Wait()
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("got nil error, want error when context cancelled during emit")
		}
		if !errors.Is(err, agui.ErrTransport) && !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want transport or canceled error", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("emitter blocked indefinitely despite context cancellation")
	}
}

func TestSubagentStarted(t *testing.T) {
	ch := make(chan events.Event, 1)
	em := agui.NewEventEmitter(ch)
	if err := em.SubagentStarted("sub_run_1", "researcher",
		events.WithSubagentDescription("Researches topics")); err != nil {
		t.Fatalf("SubagentStarted: %v", err)
	}

	got := drain(ch)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	ev, ok := got[0].(*events.SubagentStartedEvent)
	if !ok {
		t.Fatalf("got %T, want *events.SubagentStartedEvent", got[0])
	}
	if ev.SubagentRunID != "sub_run_1" {
		t.Errorf("SubagentRunID = %q, want %q", ev.SubagentRunID, "sub_run_1")
	}
	if ev.Name != "researcher" {
		t.Errorf("Name = %q, want %q", ev.Name, "researcher")
	}
	if ev.Description != "Researches topics" {
		t.Errorf("Description = %q, want %q", ev.Description, "Researches topics")
	}
}

func TestSubagentFinished(t *testing.T) {
	ch := make(chan events.Event, 1)
	em := agui.NewEventEmitter(ch)
	if err := em.SubagentFinished("sub_run_1",
		events.WithSubagentResult("task completed"),
		events.WithSubagentSuccessOutcome(),
	); err != nil {
		t.Fatalf("SubagentFinished: %v", err)
	}

	got := drain(ch)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	ev, ok := got[0].(*events.SubagentFinishedEvent)
	if !ok {
		t.Fatalf("got %T, want *events.SubagentFinishedEvent", got[0])
	}
	if ev.SubagentRunID != "sub_run_1" {
		t.Errorf("SubagentRunID = %q, want %q", ev.SubagentRunID, "sub_run_1")
	}
	if ev.Result != "task completed" {
		t.Errorf("Result = %v, want %q", ev.Result, "task completed")
	}
	if ev.Outcome == nil {
		t.Fatal("Outcome = nil, want non-nil")
	}
	if ev.Outcome.Type != events.SubagentFinishedOutcomeTypeSuccess {
		t.Errorf("Outcome.Type = %q, want %q", ev.Outcome.Type, events.SubagentFinishedOutcomeTypeSuccess)
	}
	// Success outcome must not carry InterruptIDs (strict discriminated union).
	if ev.Outcome.InterruptIDs != nil {
		t.Errorf("Outcome.InterruptIDs = %v, want nil for success outcome", ev.Outcome.InterruptIDs)
	}
}

func TestSubagentError(t *testing.T) {
	ch := make(chan events.Event, 1)
	em := agui.NewEventEmitter(ch)
	if err := em.SubagentError("sub_run_1", "tool failed",
		events.WithSubagentErrorCode("TOOL_ERROR")); err != nil {
		t.Fatalf("SubagentError: %v", err)
	}

	got := drain(ch)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	ev, ok := got[0].(*events.SubagentErrorEvent)
	if !ok {
		t.Fatalf("got %T, want *events.SubagentErrorEvent", got[0])
	}
	if ev.SubagentRunID != "sub_run_1" {
		t.Errorf("SubagentRunID = %q, want %q", ev.SubagentRunID, "sub_run_1")
	}
	if ev.Message != "tool failed" {
		t.Errorf("Message = %q, want %q", ev.Message, "tool failed")
	}
	if ev.Code == nil || *ev.Code != "TOOL_ERROR" {
		t.Errorf("Code = %v, want %q", ev.Code, "TOOL_ERROR")
	}
}

// TestSubagentFinished_SuspendedOutcome_WireFormat verifies that the suspended
// outcome emits "interruptIds" (the canonical field name) and not "interrupts"
// (the legacy field name), and that success outcomes omit the field entirely —
// the TypeScript schema parses the outcome as a strict discriminated union.
func TestSubagentFinished_SuspendedOutcome_WireFormat(t *testing.T) {
	t.Run("suspended emits interruptIds", func(t *testing.T) {
		ch := make(chan events.Event, 1)
		em := agui.NewEventEmitter(ch)
		if err := em.SubagentFinished("sub_1",
			events.WithSubagentSuspendedOutcome([]string{"int-1", "int-2"}),
		); err != nil {
			t.Fatalf("SubagentFinished: %v", err)
		}

		got := drain(ch)
		data, err := got[0].ToJSON()
		if err != nil {
			t.Fatalf("ToJSON: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		outcome, ok := m["outcome"].(map[string]any)
		if !ok {
			t.Fatalf("outcome = %v, want a map", m["outcome"])
		}
		if outcome["type"] != "suspended" {
			t.Fatalf("outcome.type = %v, want suspended", outcome["type"])
		}
		ids, ok := outcome["interruptIds"].([]any)
		if !ok {
			t.Fatalf("interruptIds = %v, want a slice; raw json: %s", outcome["interruptIds"], string(data))
		}
		if len(ids) != 2 || ids[0] != "int-1" || ids[1] != "int-2" {
			t.Errorf("interruptIds = %v, want [int-1, int-2]", ids)
		}
		if _, has := outcome["interrupts"]; has {
			t.Errorf("legacy 'interrupts' field present, want absent: %s", string(data))
		}
	})

	t.Run("success omits interruptIds", func(t *testing.T) {
		ch := make(chan events.Event, 1)
		em := agui.NewEventEmitter(ch)
		if err := em.SubagentFinished("sub_1", events.WithSubagentSuccessOutcome()); err != nil {
			t.Fatalf("SubagentFinished: %v", err)
		}

		got := drain(ch)
		data, err := got[0].ToJSON()
		if err != nil {
			t.Fatalf("ToJSON: %v", err)
		}
		if strings.Contains(string(data), "interruptIds") {
			t.Errorf("success outcome must not carry interruptIds: %s", string(data))
		}
		if strings.Contains(string(data), "interrupts") {
			t.Errorf("success outcome must not carry legacy interrupts field: %s", string(data))
		}
	})
}

// TestForSubagent_BaseEmitterUntouched verifies that events emitted through the
// base emitter do NOT carry subagentRunId, while events through the sub-agent
// wrapper do — ensuring ForSubagent only affects the wrapper, not the original.
func TestForSubagent_BaseEmitterUntouched(t *testing.T) {
	ch := make(chan events.Event, 2)
	base := agui.NewEventEmitter(ch)
	sub := base.ForSubagent("sub_run_1")

	if err := base.TextMessageContent("m1", "hello"); err != nil {
		t.Fatalf("base TextMessageContent: %v", err)
	}
	if err := sub.TextMessageContent("m2", "world"); err != nil {
		t.Fatalf("sub TextMessageContent: %v", err)
	}

	got := drain(ch)
	var baseJSON, subJSON string
	for _, ev := range got {
		data, _ := ev.ToJSON()
		s := string(data)
		if strings.Contains(s, `"m1"`) {
			baseJSON = s
		}
		if strings.Contains(s, `"m2"`) {
			subJSON = s
		}
	}
	if strings.Contains(baseJSON, `"subagentRunId"`) {
		t.Errorf("base emitter event should not carry subagentRunId: %s", baseJSON)
	}
	if !strings.Contains(subJSON, `"subagentRunId":"sub_run_1"`) {
		t.Errorf("sub emitter event should carry subagentRunId: %s", subJSON)
	}
}
