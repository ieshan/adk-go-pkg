package aguiadk_test

import (
	"encoding/json"
	"runtime"
	"testing"
	"time"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"

	"github.com/ieshan/adk-go-pkg/agui"
	"github.com/ieshan/adk-go-pkg/aguiadk"

	"google.golang.org/genai"
)

// declarable is implemented by ADK function tools that expose their
// FunctionDeclaration. The tool.Tool interface only exposes Name,
// Description, and IsLongRunning, so we type-assert to this to inspect
// the input schema set from the AG-UI tool's Parameters.
type declarable interface {
	Declaration() *genai.FunctionDeclaration
}

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
		t.Fatalf("got %d tools, want 2", len(tools))
	}

	if tools[0].Name() != "search" {
		t.Errorf("got %q, want first tool name 'search'", tools[0].Name())
	}
	if tools[0].Description() == "" {
		t.Error("got empty description, want non-empty")
	}
	if tools[1].Name() != "calculator" {
		t.Errorf("got %q, want second tool name 'calculator'", tools[1].Name())
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
		t.Errorf("got %d tools, want 0", len(ts.Tools()))
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

	// Wait for the goroutine to start, then deterministically wait for
	// Wait to register its pending entry before submitting.
	<-ready
	for !handler.HasPendingToolCall(toolCallID) {
		runtime.Gosched()
	}

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
		t.Fatalf("got %q, want %q", res.value, `{"result":"hello back"}`)
	}

	// Drain and verify the emitted events.
	var evTypes []events.EventType
	for len(ch) > 0 {
		ev := <-ch
		evTypes = append(evTypes, ev.Type())
	}
	if len(evTypes) < 3 {
		t.Fatalf("got %d events, want at least 3 (start/args/end): %v", len(evTypes), evTypes)
	}
	if evTypes[0] != events.EventTypeToolCallStart {
		t.Errorf("got %s, want first event TOOL_CALL_START", evTypes[0])
	}
	if evTypes[1] != events.EventTypeToolCallArgs {
		t.Errorf("got %s, want second event TOOL_CALL_ARGS", evTypes[1])
	}
	if evTypes[2] != events.EventTypeToolCallEnd {
		t.Errorf("got %s, want third event TOOL_CALL_END", evTypes[2])
	}
}

func TestProxyToolset_InputSchema(t *testing.T) {
	t.Parallel()
	ch := make(chan events.Event, 64)
	emitter := agui.NewEventEmitter(ch)
	handler := agui.NewToolResultHandler()

	aguiTools := []types.Tool{
		{
			Name:        "search",
			Description: "Search",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
				"required": []any{"query"},
			},
		},
	}

	ts, err := aguiadk.NewProxyToolset(aguiTools, emitter, handler, 5*time.Second)
	if err != nil {
		t.Fatalf("NewProxyToolset failed: %v", err)
	}

	tools := ts.Tools()
	if len(tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(tools))
	}

	// Declaration is not part of the exported tool.Tool interface; type-assert
	// to the declarable interface that functiontool implements.
	dt, ok := tools[0].(declarable)
	if !ok {
		t.Fatalf("tool %q does not implement declarable (Declaration method)", tools[0].Name())
	}
	decl := dt.Declaration()
	if decl == nil {
		t.Fatal("got nil FunctionDeclaration, want non-nil")
	}
	if decl.Name != "search" {
		t.Errorf("decl.Name = %q, want %q", decl.Name, "search")
	}
	if decl.ParametersJsonSchema == nil {
		t.Fatal("got nil ParametersJsonSchema, want non-nil from tool Parameters")
	}
	// Marshal the schema back to JSON to inspect its shape generically,
	// since the concrete *jsonschema.Schema type lives in an indirect
	// dependency and we avoid importing it here.
	b, err := json.Marshal(decl.ParametersJsonSchema)
	if err != nil {
		t.Fatalf("marshal ParametersJsonSchema: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal ParametersJsonSchema: %v", err)
	}
	if got["type"] != "object" {
		t.Errorf("schema type = %v, want object", got["type"])
	}
	props, ok := got["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties = %T, want map[string]any", got["properties"])
	}
	if _, ok := props["query"]; !ok {
		t.Errorf("schema properties missing 'query': %v", props)
	}
}
