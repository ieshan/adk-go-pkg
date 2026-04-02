package aguiadk_test

import (
	"testing"
	"time"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"

	"github.com/ieshan/adk-go-pkg/agui"
	"github.com/ieshan/adk-go-pkg/aguiadk"
)

func TestProxyToolset_ToolCreation(t *testing.T) {
	ch := make(chan events.Event, 64)
	emitter := agui.NewEventEmitter(ch)
	handler := agui.NewToolResultHandler()

	aguiTools := []types.Tool{
		{Name: "search", Description: "Search the web"},
		{Name: "calculator", Description: "Do math"},
	}

	ts, err := aguiadk.NewProxyToolset(aguiTools, emitter, handler, 5*time.Second)
	if err != nil {
		t.Fatalf("NewProxyToolset failed: %v", err)
	}

	tools := ts.Tools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	if tools[0].Name() != "search" {
		t.Errorf("expected first tool name 'search', got %q", tools[0].Name())
	}
	if tools[0].Description() == "" {
		t.Error("expected first tool to have a description")
	}
	if tools[1].Name() != "calculator" {
		t.Errorf("expected second tool name 'calculator', got %q", tools[1].Name())
	}
}

func TestProxyToolset_IsLongRunning(t *testing.T) {
	ch := make(chan events.Event, 64)
	emitter := agui.NewEventEmitter(ch)
	handler := agui.NewToolResultHandler()

	aguiTools := []types.Tool{
		{Name: "tool_a", Description: "Tool A"},
		{Name: "tool_b", Description: "Tool B"},
		{Name: "tool_c", Description: "Tool C"},
	}

	ts, err := aguiadk.NewProxyToolset(aguiTools, emitter, handler, 5*time.Second)
	if err != nil {
		t.Fatalf("NewProxyToolset failed: %v", err)
	}

	for _, tool := range ts.Tools() {
		if !tool.IsLongRunning() {
			t.Errorf("tool %q: expected IsLongRunning=true", tool.Name())
		}
	}
}

func TestProxyToolset_Empty(t *testing.T) {
	ch := make(chan events.Event, 64)
	emitter := agui.NewEventEmitter(ch)
	handler := agui.NewToolResultHandler()

	ts, err := aguiadk.NewProxyToolset(nil, emitter, handler, 5*time.Second)
	if err != nil {
		t.Fatalf("NewProxyToolset failed: %v", err)
	}

	if len(ts.Tools()) != 0 {
		t.Errorf("expected 0 tools, got %d", len(ts.Tools()))
	}
}

func TestProxyToolset_InlineRoundTrip(t *testing.T) {
	// This test exercises the ToolResultHandler round-trip mechanism that
	// the proxy tool handler uses: Wait blocks until SubmitResult delivers
	// a result, then verifies the emitted tool call events.

	ch := make(chan events.Event, 64)
	emitter := agui.NewEventEmitter(ch)
	handler := agui.NewToolResultHandler()

	aguiTools := []types.Tool{
		{Name: "echo", Description: "Echoes input back"},
	}

	if _, err := aguiadk.NewProxyToolset(aguiTools, emitter, handler, 5*time.Second); err != nil {
		t.Fatalf("NewProxyToolset failed: %v", err)
	}

	// Emit tool call events as the proxy handler would, and exercise
	// the ToolResultHandler Wait/SubmitResult round-trip.
	toolCallID := emitter.GenerateToolCallID()
	if err := emitter.ToolCallStart(toolCallID, "echo", nil); err != nil {
		t.Fatalf("ToolCallStart failed: %v", err)
	}
	if err := emitter.ToolCallArgs(toolCallID, `{"text":"hello"}`); err != nil {
		t.Fatalf("ToolCallArgs failed: %v", err)
	}
	if err := emitter.ToolCallEnd(toolCallID); err != nil {
		t.Fatalf("ToolCallEnd failed: %v", err)
	}

	// Start waiting for the result in a goroutine.
	type waitResult struct {
		value string
		err   error
	}
	resultCh := make(chan waitResult, 1)
	ready := make(chan struct{})
	go func() {
		ctx := t.Context()
		// Signal that we're about to call Wait (which registers the pending entry).
		close(ready)
		val, err := handler.Wait(ctx, toolCallID, 5*time.Second)
		resultCh <- waitResult{val, err}
	}()

	// Wait for the goroutine to start, then give Wait a moment to register.
	<-ready
	time.Sleep(10 * time.Millisecond)

	// Submit the result from the main goroutine.
	if err := handler.SubmitResult(toolCallID, `{"result":"hello back"}`); err != nil {
		t.Fatalf("SubmitResult failed: %v", err)
	}

	// Verify the round-trip result.
	res := <-resultCh
	if res.err != nil {
		t.Fatalf("Wait returned error: %v", res.err)
	}
	if res.value != `{"result":"hello back"}` {
		t.Fatalf("expected result %q, got %q", `{"result":"hello back"}`, res.value)
	}

	// Drain and verify the emitted events.
	var evTypes []events.EventType
	for len(ch) > 0 {
		ev := <-ch
		evTypes = append(evTypes, ev.Type())
	}
	if len(evTypes) < 3 {
		t.Fatalf("expected at least 3 events (start/args/end), got %d: %v", len(evTypes), evTypes)
	}
	if evTypes[0] != events.EventTypeToolCallStart {
		t.Errorf("expected first event TOOL_CALL_START, got %s", evTypes[0])
	}
	if evTypes[1] != events.EventTypeToolCallArgs {
		t.Errorf("expected second event TOOL_CALL_ARGS, got %s", evTypes[1])
	}
	if evTypes[2] != events.EventTypeToolCallEnd {
		t.Errorf("expected third event TOOL_CALL_END, got %s", evTypes[2])
	}
}
