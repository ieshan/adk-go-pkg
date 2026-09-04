package agui_test

import (
	"context"
	"iter"
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func TestMCPAppsMiddleware_Passthrough(t *testing.T) {
	mw := agui.NewMCPAppsMiddleware(nil)
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
}

func TestMCPAppsMiddleware_ProxiedRequest_UnknownServer(t *testing.T) {
	mw := agui.NewMCPAppsMiddleware([]agui.MCPClientConfig{
		{Type: "http", URL: "http://localhost:1", ServerID: "srv1"},
	})
	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		t.Error("base agent should not be called for proxied request")
		return nil
	})

	wrapped := mw(base)
	evs := collectAllEvents(t, context.Background(), wrapped, types.RunAgentInput{
		ThreadID: "t1",
		RunID:    "r1",
		ForwardedProps: map[string]any{
			"__proxiedMCPRequest": map[string]any{
				"serverId": "unknown",
				"method":   "ping",
			},
		},
	})

	// Should get RUN_STARTED + RUN_FINISHED (with error result, not RUN_ERROR)
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

func TestMCPAppsMiddleware_NoPendingUITools(t *testing.T) {
	mw := agui.NewMCPAppsMiddleware(nil)
	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
			yield(events.NewTextMessageStartEvent("msg1"), nil)
			yield(events.NewTextMessageContentEvent("msg1", "Hello!"), nil)
			yield(events.NewTextMessageEndEvent("msg1"), nil)
			yield(events.NewRunFinishedEvent("t1", "r1"), nil)
		}
	})

	wrapped := mw(base)
	evs := collectAllEvents(t, context.Background(), wrapped, types.RunAgentInput{ThreadID: "t1", RunID: "r1"})

	// Should get all 5 events forwarded
	if len(evs) != 5 {
		t.Fatalf("expected 5 events, got %d", len(evs))
	}
	if evs[4].Type() != events.EventTypeRunFinished {
		t.Errorf("last event: got %s, want RUN_FINISHED", evs[4].Type())
	}
}

func TestExtractProxiedRequest(t *testing.T) {
	t.Run("with proxied request", func(t *testing.T) {
		input := types.RunAgentInput{
			ForwardedProps: map[string]any{
				"__proxiedMCPRequest": map[string]any{
					"serverId": "srv1",
					"method":   "tools/call",
					"params":   map[string]any{"name": "echo"},
				},
			},
		}
		req, ok := agui.ExtractProxiedRequestForTest(input)
		if !ok {
			t.Fatal("expected proxied request to be found")
		}
		if req.ServerID != "srv1" {
			t.Errorf("ServerID: got %q, want srv1", req.ServerID)
		}
		if req.Method != "tools/call" {
			t.Errorf("Method: got %q, want tools/call", req.Method)
		}
	})

	t.Run("without proxied request", func(t *testing.T) {
		input := types.RunAgentInput{
			ForwardedProps: map[string]any{"other": "value"},
		}
		_, ok := agui.ExtractProxiedRequestForTest(input)
		if ok {
			t.Error("expected no proxied request")
		}
	})

	t.Run("nil forwarded props", func(t *testing.T) {
		input := types.RunAgentInput{}
		_, ok := agui.ExtractProxiedRequestForTest(input)
		if ok {
			t.Error("expected no proxied request with nil ForwardedProps")
		}
	})
}

func TestGetPendingUIToolCalls(t *testing.T) {
	uiToolMap := map[string]agui.UIToolInfoForTest{
		"mcp__srv__ui_tool": {Tool: types.Tool{Name: "mcp__srv__ui_tool"}},
	}

	t.Run("pending UI tool call", func(t *testing.T) {
		msgs := []types.Message{
			{ID: "m0", Role: types.RoleUser, Content: "do the thing"},
			{ID: "m1", Role: types.RoleAssistant, ToolCalls: []types.ToolCall{
				{ID: "tc1", Type: types.ToolCallTypeFunction, Function: types.FunctionCall{Name: "mcp__srv__ui_tool"}},
			}},
		}
		pending := agui.GetPendingUIToolCallsForTest(msgs, uiToolMap)
		if len(pending) != 1 {
			t.Fatalf("expected 1 pending call, got %d", len(pending))
		}
		if pending[0].ID != "tc1" {
			t.Errorf("pending call ID: got %s, want tc1", pending[0].ID)
		}
	})

	t.Run("resolved tool call", func(t *testing.T) {
		msgs := []types.Message{
			{ID: "m0", Role: types.RoleUser, Content: "do the thing"},
			{ID: "m1", Role: types.RoleAssistant, ToolCalls: []types.ToolCall{
				{ID: "tc1", Type: types.ToolCallTypeFunction, Function: types.FunctionCall{Name: "mcp__srv__ui_tool"}},
			}},
			{ID: "m2", Role: types.RoleTool, ToolCallID: "tc1", Content: "result"},
		}
		pending := agui.GetPendingUIToolCallsForTest(msgs, uiToolMap)
		if len(pending) != 0 {
			t.Fatalf("expected 0 pending calls, got %d", len(pending))
		}
	})

	t.Run("non-UI tool call", func(t *testing.T) {
		msgs := []types.Message{
			{ID: "m0", Role: types.RoleUser, Content: "do the thing"},
			{ID: "m1", Role: types.RoleAssistant, ToolCalls: []types.ToolCall{
				{ID: "tc1", Type: types.ToolCallTypeFunction, Function: types.FunctionCall{Name: "non_ui_tool"}},
			}},
		}
		pending := agui.GetPendingUIToolCallsForTest(msgs, uiToolMap)
		if len(pending) != 0 {
			t.Fatalf("expected 0 pending calls for non-UI tool, got %d", len(pending))
		}
	})
}

func TestMCPAppsMiddleware_ProxiedRequest_Ping(t *testing.T) {
	cfg := startHTTPMCPServer(t)
	mw := agui.NewMCPAppsMiddleware([]agui.MCPClientConfig{cfg})

	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		t.Error("base agent should not be called for proxied request")
		return nil
	})

	wrapped := mw(base)
	evs := collectAllEvents(t, context.Background(), wrapped, types.RunAgentInput{
		ThreadID: "t1",
		RunID:    "r1",
		ForwardedProps: map[string]any{
			"__proxiedMCPRequest": map[string]any{
				"serverId": cfg.ServerID,
				"method":   "ping",
			},
		},
	})

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

func TestMCPAppsMiddleware_ProxiedRequest_ToolsCall(t *testing.T) {
	cfg := startHTTPMCPServer(t)
	mw := agui.NewMCPAppsMiddleware([]agui.MCPClientConfig{cfg})

	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		t.Error("base agent should not be called for proxied request")
		return nil
	})

	wrapped := mw(base)
	evs := collectAllEvents(t, context.Background(), wrapped, types.RunAgentInput{
		ThreadID: "t1",
		RunID:    "r1",
		ForwardedProps: map[string]any{
			"__proxiedMCPRequest": map[string]any{
				"serverId": cfg.ServerID,
				"method":   "tools/call",
				"params": map[string]any{
					"name":      "echo",
					"arguments": map[string]any{"message": "proxied"},
				},
			},
		},
	})

	if len(evs) != 2 {
		t.Fatalf("expected 2 events, got %d", len(evs))
	}
	if evs[0].Type() != events.EventTypeRunStarted {
		t.Errorf("event 0: got %s, want RUN_STARTED", evs[0].Type())
	}
	finished, ok := evs[1].(*events.RunFinishedEvent)
	if !ok {
		t.Fatalf("event 1: got %T, want *RunFinishedEvent", evs[1])
	}
	if finished.Result == nil {
		t.Fatal("RUN_FINISHED result is nil")
	}
}

func TestMCPAppsMiddleware_UIToolInjection(t *testing.T) {
	cfg := startHTTPMCPServerWithUI(t)

	var seenTools []types.Tool
	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		seenTools = input.Tools
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
			yield(events.NewRunFinishedEvent("t1", "r1"), nil)
		}
	})

	mw := agui.NewMCPAppsMiddleware([]agui.MCPClientConfig{cfg})
	wrapped := mw(base)

	collectAllEvents(t, context.Background(), wrapped, types.RunAgentInput{
		ThreadID: "t1",
		RunID:    "r1",
		Tools:    []types.Tool{{Name: "existing"}},
	})

	// Should have original + 1 UI tool (plain_tool is not UI-enabled)
	if len(seenTools) != 2 {
		t.Fatalf("expected 2 tools, got %d: %+v", len(seenTools), seenTools)
	}
	if seenTools[0].Name != "existing" {
		t.Errorf("tool 0: got %s, want existing", seenTools[0].Name)
	}
	if seenTools[1].Name != "mcp__ui-server__ui_tool" {
		t.Errorf("tool 1: got %s, want mcp__ui-server__ui_tool", seenTools[1].Name)
	}
}

func TestMCPAppsMiddleware_PendingUIToolExecution(t *testing.T) {
	cfg := startHTTPMCPServerWithUI(t)

	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
			yield(events.NewRunFinishedEvent("t1", "r1"), nil)
		}
	})

	mw := agui.NewMCPAppsMiddleware([]agui.MCPClientConfig{cfg})
	wrapped := mw(base)

	// Input messages contain a pending UI tool call (no tool result message)
	evs := collectAllEvents(t, context.Background(), wrapped, types.RunAgentInput{
		ThreadID: "t1",
		RunID:    "r1",
		Messages: []types.Message{
			{ID: "m0", Role: types.RoleUser, Content: "use the UI tool"},
			{ID: "m1", Role: types.RoleAssistant, ToolCalls: []types.ToolCall{
				{ID: "tc1", Type: types.ToolCallTypeFunction, Function: types.FunctionCall{
					Name:      "mcp__ui-server__ui_tool",
					Arguments: `{}`,
				}},
			}},
		},
	})

	// Should see TOOL_CALL_RESULT and ACTIVITY_SNAPSHOT before RUN_FINISHED
	var hasToolCallResult, hasActivitySnapshot bool
	for _, ev := range evs {
		if ev.Type() == events.EventTypeToolCallResult {
			hasToolCallResult = true
		}
		if ev.Type() == events.EventTypeActivitySnapshot {
			hasActivitySnapshot = true
		}
	}
	if !hasToolCallResult {
		t.Error("expected TOOL_CALL_RESULT event")
	}
	if !hasActivitySnapshot {
		t.Error("expected ACTIVITY_SNAPSHOT event")
	}
	if len(evs) == 0 {
		t.Fatal("no events collected")
	}
	if evs[len(evs)-1].Type() != events.EventTypeRunFinished {
		t.Errorf("last event: got %s, want RUN_FINISHED", evs[len(evs)-1].Type())
	}
}

func TestMCPAppsMiddleware_ActivitySnapshotContent(t *testing.T) {
	cfg := startHTTPMCPServerWithUI(t)

	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
			yield(events.NewRunFinishedEvent("t1", "r1"), nil)
		}
	})

	mw := agui.NewMCPAppsMiddleware([]agui.MCPClientConfig{cfg})
	wrapped := mw(base)

	evs := collectAllEvents(t, context.Background(), wrapped, types.RunAgentInput{
		ThreadID: "t1",
		RunID:    "r1",
		Messages: []types.Message{
			{ID: "m0", Role: types.RoleUser, Content: "use the UI tool"},
			{ID: "m1", Role: types.RoleAssistant, ToolCalls: []types.ToolCall{
				{ID: "tc1", Type: types.ToolCallTypeFunction, Function: types.FunctionCall{
					Name:      "mcp__ui-server__ui_tool",
					Arguments: `{}`,
				}},
			}},
		},
	})

	var activity *events.ActivitySnapshotEvent
	for _, ev := range evs {
		if a, ok := ev.(*events.ActivitySnapshotEvent); ok {
			activity = a
			break
		}
	}
	if activity == nil {
		t.Fatal("expected ACTIVITY_SNAPSHOT event")
	}
	if activity.ActivityType != "mcp-apps" {
		t.Errorf("activityType: got %q, want mcp-apps", activity.ActivityType)
	}
	content, ok := activity.Content.(map[string]any)
	if !ok {
		t.Fatalf("content type = %T, want map[string]any", activity.Content)
	}
	if content["resourceUri"] != "ui://server/ui_tool" {
		t.Errorf("resourceUri: got %v, want ui://server/ui_tool", content["resourceUri"])
	}
	if content["serverId"] != "ui-server" {
		t.Errorf("serverId: got %v, want ui-server", content["serverId"])
	}
	if content["serverHash"] == "" {
		t.Error("serverHash should not be empty")
	}
	if activity.Replace == nil || !*activity.Replace {
		t.Error("expected replace=true on ACTIVITY_SNAPSHOT")
	}
}

func TestMCPAppsMiddleware_FailedServer(t *testing.T) {
	goodCfg := startHTTPMCPServerWithUI(t)
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

	mw := agui.NewMCPAppsMiddleware([]agui.MCPClientConfig{badCfg, goodCfg})
	wrapped := mw(base)

	collectAllEvents(t, context.Background(), wrapped, types.RunAgentInput{
		ThreadID: "t1",
		RunID:    "r1",
		Tools:    []types.Tool{{Name: "existing"}},
	})

	// Should have original + 1 UI tool from the good server (bad server skipped)
	if len(seenTools) != 2 {
		t.Fatalf("expected 2 tools (original + UI tool from good server), got %d: %+v", len(seenTools), seenTools)
	}
	if seenTools[0].Name != "existing" {
		t.Errorf("tool 0: got %s, want existing", seenTools[0].Name)
	}
	if seenTools[1].Name != "mcp__ui-server__ui_tool" {
		t.Errorf("tool 1: got %s, want mcp__ui-server__ui_tool", seenTools[1].Name)
	}
}
