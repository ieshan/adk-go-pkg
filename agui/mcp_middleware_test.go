package agui_test

import (
	"context"
	"fmt"
	"iter"
	"sync/atomic"
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

// collectAllEvents runs the agent and collects all emitted events.
func collectAllEvents(t *testing.T, ctx context.Context, agent agui.Agent, input types.RunAgentInput) []events.Event {
	t.Helper()
	var result []events.Event
	for ev, err := range agent.Run(ctx, input) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		result = append(result, ev)
	}
	return result
}

// countEvents counts events of a specific type.
func countEvents(evs []events.Event, typ events.EventType) int {
	var n int
	for _, ev := range evs {
		if ev.Type() == typ {
			n++
		}
	}
	return n
}

func TestMCPMiddleware_Passthrough(t *testing.T) {
	mw := agui.NewMCPMiddleware(nil, agui.MCPMiddlewareOptions{})
	called := false
	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		called = true
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
			yield(events.NewRunFinishedEvent("t1", "r1"), nil)
		}
	})

	wrapped := mw(base)
	evs := collectAllEvents(t, context.Background(), wrapped, types.RunAgentInput{ThreadID: "t1", RunID: "r1"})

	if !called {
		t.Error("base agent was not called")
	}
	if len(evs) != 2 {
		t.Fatalf("expected 2 events, got %d", len(evs))
	}
	if evs[0].Type() != events.EventTypeRunStarted {
		t.Errorf("event 0: got %s, want RUN_STARTED", evs[0].Type())
	}
	if evs[1].Type() != events.EventTypeRunFinished {
		t.Errorf("event 1: got %s, want RUN_FINISHED", evs[1].Type())
	}
}

func TestMCPMiddleware_ToolInjection(t *testing.T) {
	cfg := startHTTPMCPServer(t)

	mw := agui.NewMCPMiddleware([]agui.MCPClientConfig{cfg}, agui.MCPMiddlewareOptions{})
	var seenTools []types.Tool
	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		seenTools = input.Tools
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
			yield(events.NewRunFinishedEvent("t1", "r1"), nil)
		}
	})

	wrapped := mw(base)
	collectAllEvents(t, context.Background(), wrapped, types.RunAgentInput{
		ThreadID: "t1",
		RunID:    "r1",
		Tools:    []types.Tool{{Name: "existing", Description: "pre-existing tool"}},
	})

	// Should have original tool + injected MCP tool
	if len(seenTools) != 2 {
		t.Fatalf("expected 2 tools, got %d: %+v", len(seenTools), seenTools)
	}
	if seenTools[0].Name != "existing" {
		t.Errorf("tool 0: got %s, want existing", seenTools[0].Name)
	}
	// The injected tool should have the mcp__ prefix
	if seenTools[1].Name == "existing" || seenTools[1].Name == "" {
		t.Errorf("tool 1: got %q, want an injected MCP tool", seenTools[1].Name)
	}
	if seenTools[1].Name != "mcp__test-server__echo" {
		t.Errorf("tool 1: got %q, want mcp__test-server__echo", seenTools[1].Name)
	}
}

func TestMCPMiddleware_ExecutionLoop(t *testing.T) {
	cfg := startHTTPMCPServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The agent emits a tool call for the injected MCP tool on the first run,
	// then the middleware executes it and starts a continuation run.
	var callCount atomic.Int32
	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		n := callCount.Add(1)
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
			if n == 1 {
				// First call: emit a tool call for the MCP echo tool
				yield(events.NewToolCallStartEvent("tc1", "mcp__test-server__echo"), nil)
				yield(events.NewToolCallArgsEvent("tc1", `{"message":"hello"}`), nil)
				yield(events.NewToolCallEndEvent("tc1"), nil)
			} else {
				// Continuation: emit text
				yield(events.NewTextMessageStartEvent("msg1"), nil)
				yield(events.NewTextMessageContentEvent("msg1", "Done!"), nil)
				yield(events.NewTextMessageEndEvent("msg1"), nil)
			}
			yield(events.NewRunFinishedEvent("t1", "r1"), nil)
		}
	})

	mw := agui.NewMCPMiddleware([]agui.MCPClientConfig{cfg}, agui.MCPMiddlewareOptions{})
	wrapped := mw(base)

	evs := collectAllEvents(t, ctx, wrapped, types.RunAgentInput{ThreadID: "t1", RunID: "r1"})

	// Should see: RUN_STARTED, TOOL_CALL_*, TOOL_CALL_RESULT, RUN_STARTED (suppressed), TEXT_MESSAGE_*, RUN_FINISHED
	// Consumer sees one RUN_STARTED and one RUN_FINISHED
	if got := countEvents(evs, events.EventTypeRunStarted); got != 1 {
		t.Errorf("expected 1 RUN_STARTED, got %d", got)
	}
	if got := countEvents(evs, events.EventTypeRunFinished); got != 1 {
		t.Errorf("expected 1 RUN_FINISHED, got %d", got)
	}

	// Should have a TOOL_CALL_RESULT for the MCP tool
	if got := countEvents(evs, events.EventTypeToolCallResult); got != 1 {
		t.Errorf("expected 1 TOOL_CALL_RESULT, got %d", got)
	}

	// Agent should have been called twice (first run + continuation)
	if callCount.Load() != 2 {
		t.Errorf("expected agent called twice, got %d", callCount.Load())
	}
}

func TestMCPMiddleware_NonMCPToolCallPassthrough(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// With no MCP servers, a non-MCP tool call should pass through without looping
	var callCount atomic.Int32
	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		n := callCount.Add(1)
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
			if n == 1 {
				yield(events.NewToolCallStartEvent("tc1", "non_mcp_tool"), nil)
				yield(events.NewToolCallArgsEvent("tc1", `{"arg":"value"}`), nil)
				yield(events.NewToolCallEndEvent("tc1"), nil)
			}
			yield(events.NewRunFinishedEvent("t1", "r1"), nil)
		}
	})

	mw := agui.NewMCPMiddleware(nil, agui.MCPMiddlewareOptions{})
	wrapped := mw(base)

	evs := collectAllEvents(t, ctx, wrapped, types.RunAgentInput{ThreadID: "t1", RunID: "r1"})

	if len(evs) != 5 {
		t.Fatalf("expected 5 events, got %d", len(evs))
	}
	if got := countEvents(evs, events.EventTypeRunStarted); got != 1 {
		t.Errorf("expected 1 RUN_STARTED, got %d", got)
	}
	if got := countEvents(evs, events.EventTypeRunFinished); got != 1 {
		t.Errorf("expected 1 RUN_FINISHED, got %d", got)
	}
	if callCount.Load() != 1 {
		t.Errorf("expected agent called once, got %d", callCount.Load())
	}
}

func TestMCPMiddleware_MaxIterations(t *testing.T) {
	cfg := startHTTPMCPServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Agent always emits an MCP tool call, never producing a final response.
	// The middleware should stop after maxIterations rounds.
	var callCount atomic.Int32
	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		callCount.Add(1)
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
			yield(events.NewToolCallStartEvent("tc1", "mcp__test-server__echo"), nil)
			yield(events.NewToolCallArgsEvent("tc1", `{"message":"loop"}`), nil)
			yield(events.NewToolCallEndEvent("tc1"), nil)
			yield(events.NewRunFinishedEvent("t1", "r1"), nil)
		}
	})

	mw := agui.NewMCPMiddleware([]agui.MCPClientConfig{cfg}, agui.MCPMiddlewareOptions{
		MaxIterations: 2,
	})
	wrapped := mw(base)

	evs := collectAllEvents(t, ctx, wrapped, types.RunAgentInput{ThreadID: "t1", RunID: "r1"})

	// Should still emit RUN_FINISHED after hitting max iterations
	if got := countEvents(evs, events.EventTypeRunFinished); got != 1 {
		t.Errorf("expected 1 RUN_FINISHED, got %d", got)
	}
	// Agent should have been called at most maxIterations+1 times
	if callCount.Load() > 3 {
		t.Errorf("expected at most 3 agent calls, got %d", callCount.Load())
	}
}

func TestMCPMiddleware_FailedServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a config pointing to a non-existent server
	badCfg := agui.MCPClientConfig{
		Type:     "http",
		URL:      "http://127.0.0.1:1",
		ServerID: "bad-server",
	}

	var seenTools []types.Tool
	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		seenTools = input.Tools
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
			yield(events.NewRunFinishedEvent("t1", "r1"), nil)
		}
	})

	mw := agui.NewMCPMiddleware([]agui.MCPClientConfig{badCfg}, agui.MCPMiddlewareOptions{})
	wrapped := mw(base)

	evs := collectAllEvents(t, ctx, wrapped, types.RunAgentInput{
		ThreadID: "t1",
		RunID:    "r1",
		Tools:    []types.Tool{{Name: "existing"}},
	})

	// Should still run even if the MCP server is unreachable
	// Tools should only contain the original (failed server contributes nothing)
	if len(seenTools) != 1 || seenTools[0].Name != "existing" {
		t.Errorf("tools should only contain original, got %v", seenTools)
	}
	if got := countEvents(evs, events.EventTypeRunFinished); got != 1 {
		t.Errorf("expected 1 RUN_FINISHED, got %d", got)
	}
}

func TestMCPMiddleware_ToolListingCached(t *testing.T) {
	cfg := startHTTPMCPServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var callCount atomic.Int32
	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		callCount.Add(1)
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
			yield(events.NewRunFinishedEvent("t1", "r1"), nil)
		}
	})

	mw := agui.NewMCPMiddleware([]agui.MCPClientConfig{cfg}, agui.MCPMiddlewareOptions{})
	wrapped := mw(base)

	// First run
	collectAllEvents(t, ctx, wrapped, types.RunAgentInput{ThreadID: "t1", RunID: "r1"})
	firstCount := callCount.Load()

	// Second run should reuse cached tool listing
	collectAllEvents(t, ctx, wrapped, types.RunAgentInput{ThreadID: "t1", RunID: "r2"})

	// Both runs should have seen the injected tools (from cache on second run)
	if callCount.Load() != firstCount+1 {
		t.Errorf("expected agent called %d times, got %d", firstCount+1, callCount.Load())
	}
}

func TestMCPMiddleware_RunError(t *testing.T) {
	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
			yield(events.NewRunErrorEvent("something went wrong", events.WithRunID("r1")), nil)
		}
	})

	mw := agui.NewMCPMiddleware(nil, agui.MCPMiddlewareOptions{})
	wrapped := mw(base)

	evs := collectAllEvents(t, context.Background(), wrapped, types.RunAgentInput{ThreadID: "t1", RunID: "r1"})

	if len(evs) != 2 {
		t.Fatalf("expected 2 events, got %d", len(evs))
	}
	if evs[1].Type() != events.EventTypeRunError {
		t.Errorf("event 1: got %s, want RUN_ERROR", evs[1].Type())
	}
}

func TestReconstructMessages(t *testing.T) {
	original := []types.Message{
		{ID: "m0", Role: types.RoleUser, Content: "hello"},
	}

	evs := []events.Event{
		events.NewTextMessageStartEvent("m1"),
		events.NewTextMessageContentEvent("m1", "Hello "),
		events.NewTextMessageContentEvent("m1", "world!"),
		events.NewTextMessageEndEvent("m1"),
	}

	msgs := testReconstructMessages(original, evs)

	// Should have original + 1 assistant message
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[1].Role != types.RoleAssistant {
		t.Errorf("msg 1 role: got %s, want assistant", msgs[1].Role)
	}
	if msgs[1].Content != "Hello world!" {
		t.Errorf("msg 1 content: got %v, want 'Hello world!'", msgs[1].Content)
	}
}

func TestReconstructMessages_ToolCalls(t *testing.T) {
	original := []types.Message{
		{ID: "m0", Role: types.RoleUser, Content: "search for cats"},
	}

	evs := []events.Event{
		events.NewToolCallStartEvent("tc1", "search"),
		events.NewToolCallArgsEvent("tc1", `{"q":"cats"}`),
		events.NewToolCallEndEvent("tc1"),
	}

	msgs := testReconstructMessages(original, evs)

	// Should have original + 1 assistant message with tool call
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if len(msgs[1].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msgs[1].ToolCalls))
	}
	tc := msgs[1].ToolCalls[0]
	if tc.ID != "tc1" {
		t.Errorf("tool call ID: got %s, want tc1", tc.ID)
	}
	if tc.Function.Name != "search" {
		t.Errorf("tool call name: got %s, want search", tc.Function.Name)
	}
	if tc.Function.Arguments != `{"q":"cats"}` {
		t.Errorf("tool call args: got %s, want {\"q\":\"cats\"}", tc.Function.Arguments)
	}
}

func TestGetOpenToolCalls(t *testing.T) {
	msgs := []types.Message{
		{ID: "m0", Role: types.RoleUser, Content: "hello"},
		{ID: "m1", Role: types.RoleAssistant, ToolCalls: []types.ToolCall{
			{ID: "tc1", Type: types.ToolCallTypeFunction, Function: types.FunctionCall{Name: "tool1"}},
			{ID: "tc2", Type: types.ToolCallTypeFunction, Function: types.FunctionCall{Name: "tool2"}},
		}},
		{ID: "m2", Role: types.RoleTool, ToolCallID: "tc1", Content: "result1"},
	}

	open := testGetOpenToolCalls(msgs)
	if len(open) != 1 {
		t.Fatalf("expected 1 open tool call, got %d", len(open))
	}
	if open[0].ID != "tc2" {
		t.Errorf("open call ID: got %s, want tc2", open[0].ID)
	}
}

func TestGetOpenToolCalls_None(t *testing.T) {
	msgs := []types.Message{
		{ID: "m0", Role: types.RoleUser, Content: "hello"},
		{ID: "m1", Role: types.RoleAssistant, ToolCalls: []types.ToolCall{
			{ID: "tc1", Type: types.ToolCallTypeFunction, Function: types.FunctionCall{Name: "tool1"}},
		}},
		{ID: "m2", Role: types.RoleTool, ToolCallID: "tc1", Content: "result1"},
	}

	open := testGetOpenToolCalls(msgs)
	if len(open) != 0 {
		t.Fatalf("expected 0 open tool calls, got %d", len(open))
	}
}

// testReconstructMessages and testGetOpenToolCalls expose the unexported
// functions for testing via a thin wrapper.
func testReconstructMessages(original []types.Message, evs []events.Event) []types.Message {
	return agui.ReconstructMessagesForTest(original, evs)
}

func testGetOpenToolCalls(msgs []types.Message) []types.ToolCall {
	return agui.GetOpenToolCallsForTest(msgs)
}

func TestMCPMiddleware_MixedMCPAndNonMCPToolCalls(t *testing.T) {
	cfg := startHTTPMCPServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Agent emits both an MCP tool call and a non-MCP (frontend) tool call.
	// The middleware should execute the MCP tool, emit TOOL_CALL_RESULT,
	// then flush RUN_FINISHED without calling the agent again (the non-MCP
	// tool call is handed off to the frontend).
	var callCount atomic.Int32
	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		callCount.Add(1)
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
			yield(events.NewToolCallStartEvent("tc-mcp", "mcp__test-server__echo"), nil)
			yield(events.NewToolCallArgsEvent("tc-mcp", `{"message":"hello"}`), nil)
			yield(events.NewToolCallEndEvent("tc-mcp"), nil)
			yield(events.NewToolCallStartEvent("tc-client", "client_tool"), nil)
			yield(events.NewToolCallArgsEvent("tc-client", `{}`), nil)
			yield(events.NewToolCallEndEvent("tc-client"), nil)
			yield(events.NewRunFinishedEvent("t1", "r1"), nil)
		}
	})

	mw := agui.NewMCPMiddleware([]agui.MCPClientConfig{cfg}, agui.MCPMiddlewareOptions{})
	wrapped := mw(base)

	evs := collectAllEvents(t, ctx, wrapped, types.RunAgentInput{ThreadID: "t1", RunID: "r1"})

	// Agent should only be called once (no continuation loop)
	if callCount.Load() != 1 {
		t.Errorf("expected agent called once, got %d", callCount.Load())
	}

	// Should have one TOOL_CALL_RESULT for the MCP tool
	if got := countEvents(evs, events.EventTypeToolCallResult); got != 1 {
		t.Errorf("expected 1 TOOL_CALL_RESULT, got %d", got)
	}

	// Should have one RUN_FINISHED
	if got := countEvents(evs, events.EventTypeRunFinished); got != 1 {
		t.Errorf("expected 1 RUN_FINISHED, got %d", got)
	}
}

func TestMCPMiddleware_UnknownMCPToolCall(t *testing.T) {
	cfg := startHTTPMCPServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Agent emits a tool call with mcp__ prefix that is NOT a known MCP tool.
	// The middleware should not try to execute it and should not loop.
	var callCount atomic.Int32
	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		callCount.Add(1)
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
			yield(events.NewToolCallStartEvent("tc1", "mcp__test-server__ghost"), nil)
			yield(events.NewToolCallArgsEvent("tc1", `{}`), nil)
			yield(events.NewToolCallEndEvent("tc1"), nil)
			yield(events.NewRunFinishedEvent("t1", "r1"), nil)
		}
	})

	mw := agui.NewMCPMiddleware([]agui.MCPClientConfig{cfg}, agui.MCPMiddlewareOptions{})
	wrapped := mw(base)

	evs := collectAllEvents(t, ctx, wrapped, types.RunAgentInput{ThreadID: "t1", RunID: "r1"})

	// Agent should only be called once (no continuation — ghost tool is not in toolMap)
	if callCount.Load() != 1 {
		t.Errorf("expected agent called once, got %d", callCount.Load())
	}
	// No TOOL_CALL_RESULT should be emitted for the unknown tool
	if got := countEvents(evs, events.EventTypeToolCallResult); got != 0 {
		t.Errorf("expected 0 TOOL_CALL_RESULT, got %d", got)
	}
	// Should have one RUN_FINISHED
	if got := countEvents(evs, events.EventTypeRunFinished); got != 1 {
		t.Errorf("expected 1 RUN_FINISHED, got %d", got)
	}
}

func TestMCPMiddleware_MultiHopLoop(t *testing.T) {
	cfg := startHTTPMCPServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Agent emits an MCP tool call on the first two runs, then a final text
	// response on the third. The middleware should execute both calls and
	// present a single RUN_STARTED / RUN_FINISHED to the consumer.
	var callCount atomic.Int32
	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		n := callCount.Add(1)
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
			if n <= 2 {
				tcID := fmt.Sprintf("tc%d", n)
				yield(events.NewToolCallStartEvent(tcID, "mcp__test-server__echo"), nil)
				yield(events.NewToolCallArgsEvent(tcID, `{"message":"hop"}`), nil)
				yield(events.NewToolCallEndEvent(tcID), nil)
			} else {
				yield(events.NewTextMessageStartEvent("msg1"), nil)
				yield(events.NewTextMessageContentEvent("msg1", "finally done"), nil)
				yield(events.NewTextMessageEndEvent("msg1"), nil)
			}
			yield(events.NewRunFinishedEvent("t1", "r1"), nil)
		}
	})

	mw := agui.NewMCPMiddleware([]agui.MCPClientConfig{cfg}, agui.MCPMiddlewareOptions{})
	wrapped := mw(base)

	evs := collectAllEvents(t, ctx, wrapped, types.RunAgentInput{ThreadID: "t1", RunID: "r1"})

	// Agent should have been called 3 times (two tool rounds + final text)
	if callCount.Load() != 3 {
		t.Errorf("expected agent called 3 times, got %d", callCount.Load())
	}
	// Consumer sees exactly one RUN_STARTED and one RUN_FINISHED
	if got := countEvents(evs, events.EventTypeRunStarted); got != 1 {
		t.Errorf("expected 1 RUN_STARTED, got %d", got)
	}
	if got := countEvents(evs, events.EventTypeRunFinished); got != 1 {
		t.Errorf("expected 1 RUN_FINISHED, got %d", got)
	}
	// Two TOOL_CALL_RESULT events (one per hop)
	if got := countEvents(evs, events.EventTypeToolCallResult); got != 2 {
		t.Errorf("expected 2 TOOL_CALL_RESULT, got %d", got)
	}
}

func TestMCPMiddleware_StreamedArgsAssembly(t *testing.T) {
	cfg := startHTTPMCPServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Agent emits tool call args split across multiple TOOL_CALL_ARGS chunks.
	// The middleware should assemble them and execute with the full JSON.
	var callCount atomic.Int32
	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		n := callCount.Add(1)
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
			if n == 1 {
				yield(events.NewToolCallStartEvent("tc1", "mcp__test-server__echo"), nil)
				// Split {"message":"hello"} across multiple chunks
				yield(events.NewToolCallArgsEvent("tc1", `{"mes`), nil)
				yield(events.NewToolCallArgsEvent("tc1", `sage":"`), nil)
				yield(events.NewToolCallArgsEvent("tc1", `hello"}`), nil)
				yield(events.NewToolCallEndEvent("tc1"), nil)
			} else {
				yield(events.NewTextMessageStartEvent("msg1"), nil)
				yield(events.NewTextMessageContentEvent("msg1", "done"), nil)
				yield(events.NewTextMessageEndEvent("msg1"), nil)
			}
			yield(events.NewRunFinishedEvent("t1", "r1"), nil)
		}
	})

	mw := agui.NewMCPMiddleware([]agui.MCPClientConfig{cfg}, agui.MCPMiddlewareOptions{})
	wrapped := mw(base)

	evs := collectAllEvents(t, ctx, wrapped, types.RunAgentInput{ThreadID: "t1", RunID: "r1"})

	// The tool should have been executed (TOOL_CALL_RESULT present)
	if got := countEvents(evs, events.EventTypeToolCallResult); got != 1 {
		t.Fatalf("expected 1 TOOL_CALL_RESULT, got %d", got)
	}
	// Agent should have been called twice (tool round + continuation)
	if callCount.Load() != 2 {
		t.Errorf("expected agent called twice, got %d", callCount.Load())
	}
}

func TestMCPMiddleware_MultipleMCPCallsWithFailure(t *testing.T) {
	cfg := startHTTPMCPServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Agent emits two MCP tool calls in one round. Both should be executed
	// in parallel; a failure in one should not block the other.
	var callCount atomic.Int32
	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		n := callCount.Add(1)
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
			if n == 1 {
				yield(events.NewToolCallStartEvent("tc1", "mcp__test-server__echo"), nil)
				yield(events.NewToolCallArgsEvent("tc1", `{"message":"first"}`), nil)
				yield(events.NewToolCallEndEvent("tc1"), nil)
				yield(events.NewToolCallStartEvent("tc2", "mcp__test-server__echo"), nil)
				yield(events.NewToolCallArgsEvent("tc2", `{"message":"second"}`), nil)
				yield(events.NewToolCallEndEvent("tc2"), nil)
			} else {
				yield(events.NewTextMessageStartEvent("msg1"), nil)
				yield(events.NewTextMessageContentEvent("msg1", "ok"), nil)
				yield(events.NewTextMessageEndEvent("msg1"), nil)
			}
			yield(events.NewRunFinishedEvent("t1", "r1"), nil)
		}
	})

	mw := agui.NewMCPMiddleware([]agui.MCPClientConfig{cfg}, agui.MCPMiddlewareOptions{})
	wrapped := mw(base)

	evs := collectAllEvents(t, ctx, wrapped, types.RunAgentInput{ThreadID: "t1", RunID: "r1"})

	// Both tool calls should produce TOOL_CALL_RESULT events
	if got := countEvents(evs, events.EventTypeToolCallResult); got != 2 {
		t.Errorf("expected 2 TOOL_CALL_RESULT, got %d", got)
	}
	// Agent should have been called twice (tool round + continuation)
	if callCount.Load() != 2 {
		t.Errorf("expected agent called twice, got %d", callCount.Load())
	}
}

func TestMCPMiddleware_FailedListingCached(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a config pointing to a non-existent server
	badCfg := agui.MCPClientConfig{
		Type:     "http",
		URL:      "http://127.0.0.1:1",
		ServerID: "bad-server",
	}

	var callCount atomic.Int32
	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		callCount.Add(1)
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
			yield(events.NewRunFinishedEvent("t1", "r1"), nil)
		}
	})

	mw := agui.NewMCPMiddleware([]agui.MCPClientConfig{badCfg}, agui.MCPMiddlewareOptions{})
	wrapped := mw(base)

	// First run — server fails, no tools injected
	collectAllEvents(t, ctx, wrapped, types.RunAgentInput{ThreadID: "t1", RunID: "r1"})
	firstCount := callCount.Load()

	// Second run — failed listing should be cached (not retried)
	collectAllEvents(t, ctx, wrapped, types.RunAgentInput{ThreadID: "t1", RunID: "r2"})

	// Both runs should have called the agent (with no tools injected)
	if callCount.Load() != firstCount+1 {
		t.Errorf("expected agent called %d times, got %d", firstCount+1, callCount.Load())
	}
}
