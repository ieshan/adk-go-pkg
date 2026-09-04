package agui

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log/slog"
	"sync"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sync/errgroup"
)

// NewMCPAppsMiddleware creates a Middleware that injects UI-enabled MCP tools
// into the agent's tool list and handles proxied MCP requests from the frontend.
// When servers is empty, the middleware is a pass-through.
func NewMCPAppsMiddleware(servers []MCPClientConfig) Middleware {
	if len(servers) == 0 {
		return func(next Agent) Agent { return next }
	}

	serverByHash := make(map[string]MCPClientConfig, len(servers))
	serverByID := make(map[string]MCPClientConfig, len(servers))
	for _, s := range servers {
		serverByHash[GetServerHash(s)] = s
		if s.ServerID != "" {
			serverByID[s.ServerID] = s
		}
	}

	return func(next Agent) Agent {
		mw := &mcpAppsMiddleware{
			servers:      servers,
			serverByHash: serverByHash,
			serverByID:   serverByID,
			next:         next,
		}
		return AgentFunc(mw.run)
	}
}

type mcpAppsMiddleware struct {
	servers      []MCPClientConfig
	serverByHash map[string]MCPClientConfig
	serverByID   map[string]MCPClientConfig
	next         Agent

	fetchOnce sync.Once
	uiTools   []uiToolInfo
}

type uiToolInfo struct {
	Tool         types.Tool
	ServerConfig MCPClientConfig
	ResourceURI  string
	ServerHash   string
	OriginalName string
}

func (m *mcpAppsMiddleware) run(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
	ch := make(chan events.Event, 128)
	emitter := NewEventEmitter(ch)

	go func() {
		defer close(ch)
		m.runLoop(ctx, input, emitter)
	}()

	return ChanToIter(ctx, ch)
}

func (m *mcpAppsMiddleware) runLoop(ctx context.Context, input types.RunAgentInput, emitter *EventEmitter) {
	// Check for proxied MCP request
	if req, ok := extractProxiedRequest(input); ok {
		m.handleProxiedRequest(ctx, input, req, emitter)
		return
	}

	// Fetch UI tools (cached)
	uiTools := m.fetchUITools(ctx)

	// Build tool name → uiToolInfo map
	uiToolMap := make(map[string]uiToolInfo, len(uiTools))
	augmentedTools := make([]types.Tool, len(input.Tools))
	copy(augmentedTools, input.Tools)
	for _, ui := range uiTools {
		augmentedTools = append(augmentedTools, ui.Tool)
		uiToolMap[ui.Tool.Name] = ui
	}

	// Augment input
	augmentedInput := input
	augmentedInput.Tools = augmentedTools

	// Run agent and intercept events
	var bufferedRunFinished *events.RunFinishedEvent
	runStarted := false

	for ev, err := range m.next.Run(ctx, augmentedInput) {
		if err != nil {
			_ = emitter.RunErrorWithOptions(fmt.Sprintf("agui: agent error: %v", err))
			return
		}
		switch e := ev.(type) {
		case *events.RunStartedEvent:
			if !runStarted {
				runStarted = true
				if emitErr := emitter.emit(ev); emitErr != nil {
					return
				}
			}
		case *events.RunFinishedEvent:
			bufferedRunFinished = e
		case *events.RunErrorEvent:
			if emitErr := emitter.emit(ev); emitErr != nil {
				return
			}
			return
		default:
			if emitErr := emitter.emit(ev); emitErr != nil {
				return
			}
		}
	}

	// Find pending UI tool calls from input messages
	pendingCalls := getPendingUIToolCalls(input.Messages, uiToolMap)

	// Execute pending UI tool calls
	for _, tc := range pendingCalls {
		ui := uiToolMap[tc.Function.Name]

		var args map[string]any
		if tc.Function.Arguments != "" {
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		}
		if args == nil {
			args = make(map[string]any)
		}

		result, callErr := CallMCPTool(ctx, ui.ServerConfig, ui.OriginalName, args)
		content := ExtractTextContent(result)
		if callErr != nil {
			content = fmt.Sprintf("Error: %v", callErr)
		}

		resultMsgID := events.GenerateMessageID()
		_ = emitter.ToolCallResult(resultMsgID, tc.ID, content)

		// Emit ACTIVITY_SNAPSHOT
		activityContent := map[string]any{
			"result":      content,
			"resourceUri": ui.ResourceURI,
			"serverHash":  ui.ServerHash,
			"serverId":    ui.ServerConfig.ServerID,
			"toolInput":   args,
		}
		replace := true
		_ = emitter.ActivitySnapshot(tc.ID, MCPAppsActivityType, activityContent, &replace)
	}

	// Flush buffered RUN_FINISHED
	if bufferedRunFinished != nil {
		if emitErr := emitter.emit(bufferedRunFinished); emitErr != nil {
			return
		}
	}
}

func (m *mcpAppsMiddleware) handleProxiedRequest(ctx context.Context, input types.RunAgentInput, req *ProxiedMCPRequest, emitter *EventEmitter) {
	// Look up server by ID first, then by hash
	serverConfig, ok := m.serverByID[req.ServerID]
	if !ok {
		serverConfig, ok = m.serverByHash[req.ServerHash]
	}

	_ = emitter.RunStarted(input.ThreadID, input.RunID)

	if !ok {
		_ = emitter.RunFinishedWithOptions(input.ThreadID, input.RunID,
			events.WithResult(map[string]any{
				"error": fmt.Sprintf("agui: unknown MCP server (id=%q, hash=%q)", req.ServerID, req.ServerHash),
			}))
		return
	}

	result, err := ExecuteMCPRequest(ctx, serverConfig, req.Method, req.Params)
	if err != nil {
		_ = emitter.RunFinishedWithOptions(input.ThreadID, input.RunID,
			events.WithResult(map[string]any{
				"error": fmt.Sprintf("agui: proxied MCP request failed: %v", err),
			}))
		return
	}

	_ = emitter.RunFinishedWithOptions(input.ThreadID, input.RunID,
		events.WithSuccessOutcome(),
		events.WithResult(result))
}

// fetchUITools lists tools from all servers and filters for UI-enabled tools
// (those with _meta["ui/resourceUri"]). Cached via sync.Once.
func (m *mcpAppsMiddleware) fetchUITools(ctx context.Context) []uiToolInfo {
	m.fetchOnce.Do(func() {
		used := make(map[string]struct{})
		var all []uiToolInfo

		g, gctx := errgroup.WithContext(ctx)
		var mu sync.Mutex

		for _, srv := range m.servers {
			srv := srv
			g.Go(func() error {
				tools, err := ListMCPTools(gctx, srv)
				if err != nil {
					slog.Warn("agui: failed to list MCP tools, skipping server",
						"server", srv.ServerID, "url", srv.URL, "error", err)
					return nil
				}
				mu.Lock()
				for _, tool := range tools {
					// Check for UI resource URI in _meta
					resourceURI := extractResourceURI(tool)
					if resourceURI == "" {
						continue // skip non-UI tools
					}
					name := MakeUniqueToolName(srv.ServerID, tool.Name, used)
					all = append(all, uiToolInfo{
						Tool: types.Tool{
							Name:        name,
							Description: tool.Description,
							Parameters:  tool.InputSchema,
						},
						ServerConfig: srv,
						ResourceURI:  resourceURI,
						ServerHash:   GetServerHash(srv),
						OriginalName: tool.Name,
					})
				}
				mu.Unlock()
				return nil
			})
		}
		_ = g.Wait()
		m.uiTools = all
	})

	return m.uiTools
}

// extractResourceURI extracts the ui/resourceUri from a tool's _meta field.
func extractResourceURI(tool *mcp.Tool) string {
	if tool == nil || tool.Meta == nil {
		return ""
	}
	uri, _ := tool.Meta["ui/resourceUri"].(string)
	return uri
}

// extractProxiedRequest checks if the input contains a proxied MCP request
// in ForwardedProps.
func extractProxiedRequest(input types.RunAgentInput) (*ProxiedMCPRequest, bool) {
	if input.ForwardedProps == nil {
		return nil, false
	}

	// ForwardedProps is any — try to extract __proxiedMCPRequest
	var props map[string]any
	switch v := input.ForwardedProps.(type) {
	case map[string]any:
		props = v
	case string:
		if err := json.Unmarshal([]byte(v), &props); err != nil {
			return nil, false
		}
	default:
		// Try JSON round-trip
		data, err := json.Marshal(input.ForwardedProps)
		if err != nil {
			return nil, false
		}
		if err := json.Unmarshal(data, &props); err != nil {
			return nil, false
		}
	}

	raw, ok := props["__proxiedMCPRequest"]
	if !ok {
		return nil, false
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var req ProxiedMCPRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, false
	}
	return &req, true
}

// getPendingUIToolCalls scans input messages for assistant tool calls that
// have no matching tool result, filtered to only UI tools.
func getPendingUIToolCalls(messages []types.Message, uiToolMap map[string]uiToolInfo) []types.ToolCall {
	resolved := make(map[string]bool)
	for _, msg := range messages {
		if msg.ToolCallID != "" {
			resolved[msg.ToolCallID] = true
		}
	}

	var pending []types.ToolCall
	for _, msg := range messages {
		if msg.Role != types.RoleAssistant {
			continue
		}
		for _, tc := range msg.ToolCalls {
			if resolved[tc.ID] {
				continue
			}
			if _, isUI := uiToolMap[tc.Function.Name]; isUI {
				pending = append(pending, tc)
			}
		}
	}
	return pending
}
