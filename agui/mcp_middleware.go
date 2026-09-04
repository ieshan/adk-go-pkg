package agui

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log/slog"
	"strings"
	"sync"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sync/errgroup"
)

// NewMCPMiddleware creates a Middleware that injects MCP server tools into
// the agent's tool list and executes MCP tool calls server-side in a loop.
// When servers is empty, the middleware is a pass-through.
func NewMCPMiddleware(servers []MCPClientConfig, opts MCPMiddlewareOptions) Middleware {
	if len(servers) == 0 {
		return func(next Agent) Agent { return next }
	}

	maxIter := opts.MaxIterations
	if maxIter <= 0 {
		maxIter = defaultMaxIterations
	}

	return func(next Agent) Agent {
		mw := &mcpMiddleware{
			servers:       servers,
			next:          next,
			maxIterations: maxIter,
		}
		return AgentFunc(mw.run)
	}
}

type mcpMiddleware struct {
	servers       []MCPClientConfig
	next          Agent
	maxIterations int

	listOnce    sync.Once
	listedTools []listedTool
}

// listedTool pairs a raw MCP tool with its origin server config.
// Cached once per middleware instance; name resolution happens per run.
type listedTool struct {
	mcpTool      *mcp.Tool
	serverConfig MCPClientConfig
}

// listAllTools lists tools from all MCP servers in parallel, caching the
// result via sync.Once. Failed servers are skipped with a warning.
func (m *mcpMiddleware) listAllTools(ctx context.Context) []listedTool {
	m.listOnce.Do(func() {
		var all []listedTool

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
					all = append(all, listedTool{
						mcpTool:      tool,
						serverConfig: srv,
					})
				}
				mu.Unlock()
				return nil
			})
		}
		_ = g.Wait()
		m.listedTools = all
	})

	return m.listedTools
}

// resolveTools builds namespaced tool names from cached listings, seeding
// the used-name set with existing tool names to avoid collisions.
func (m *mcpMiddleware) resolveTools(ctx context.Context, existingNames map[string]struct{}) []ResolvedMCPTool {
	listed := m.listAllTools(ctx)
	used := make(map[string]struct{}, len(existingNames)+len(listed))
	for name := range existingNames {
		used[name] = struct{}{}
	}

	resolved := make([]ResolvedMCPTool, 0, len(listed))
	for _, lt := range listed {
		name := MakeUniqueToolName(lt.serverConfig.ServerID, lt.mcpTool.Name, used)
		resolved = append(resolved, ResolvedMCPTool{
			MCPTool:        lt.mcpTool,
			NamespacedName: name,
			ServerConfig:   lt.serverConfig,
		})
	}
	return resolved
}

func (m *mcpMiddleware) run(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
	ch := make(chan events.Event, 128)
	emitter := NewEventEmitter(ch)

	go func() {
		defer close(ch)
		m.runLoop(ctx, input, emitter)
	}()

	return ChanToIter(ctx, ch)
}

func (m *mcpMiddleware) runLoop(ctx context.Context, input types.RunAgentInput, emitter *EventEmitter) {
	// Resolve MCP tools (listing cached, name resolution per run)
	existingNames := make(map[string]struct{}, len(input.Tools))
	for _, t := range input.Tools {
		existingNames[t.Name] = struct{}{}
	}
	resolved := m.resolveTools(ctx, existingNames)

	// Build tool map for looking up MCP tools by namespaced name
	toolMap := make(map[string]ResolvedMCPTool, len(resolved))
	for _, r := range resolved {
		toolMap[r.NamespacedName] = r
	}

	// Augment input tools with MCP tools
	augmentedTools := make([]types.Tool, len(input.Tools))
	copy(augmentedTools, input.Tools)
	for _, r := range resolved {
		augmentedTools = append(augmentedTools, types.Tool{
			Name:        r.NamespacedName,
			Description: r.MCPTool.Description,
			Parameters:  r.MCPTool.InputSchema,
		})
	}

	currentInput := input
	currentInput.Tools = augmentedTools
	iteration := 0
	runStarted := false

	for {
		// Collect events from next.Run
		var collectedEvents []events.Event
		var runErr error
		var bufferedRunFinished *events.RunFinishedEvent

		for ev, err := range m.next.Run(ctx, currentInput) {
			if err != nil {
				runErr = err
				break
			}
			switch e := ev.(type) {
			case *events.RunStartedEvent:
				if runStarted {
					// Suppress continuation RUN_STARTED
					continue
				}
				runStarted = true
				collectedEvents = append(collectedEvents, ev)
				if emitErr := emitter.emit(ev); emitErr != nil {
					return
				}
			case *events.RunFinishedEvent:
				bufferedRunFinished = e
			case *events.RunErrorEvent:
				// Forward immediately, stop
				if emitErr := emitter.emit(ev); emitErr != nil {
					return
				}
				return
			default:
				collectedEvents = append(collectedEvents, ev)
				if emitErr := emitter.emit(ev); emitErr != nil {
					return
				}
			}
		}

		if runErr != nil {
			_ = emitter.RunErrorWithOptions(fmt.Sprintf("agui: agent error: %v", runErr))
			return
		}

		// Reconstruct messages from original + events
		messages := reconstructMessages(currentInput.Messages, collectedEvents)

		// Find open tool calls (assistant tool calls without matching results)
		openCalls := getOpenToolCalls(messages)

		// Filter to only MCP tool calls
		var mcpCalls []types.ToolCall
		for _, tc := range openCalls {
			if _, isMCP := toolMap[tc.Function.Name]; isMCP {
				mcpCalls = append(mcpCalls, tc)
			}
		}

		// No MCP tool calls → flush RUN_FINISHED and complete
		if len(mcpCalls) == 0 {
			if bufferedRunFinished != nil {
				if emitErr := emitter.emit(bufferedRunFinished); emitErr != nil {
					return
				}
			}
			return
		}

		// Check max iterations
		iteration++
		if iteration > m.maxIterations {
			slog.Warn("agui: MCP middleware reached max iterations",
				"maxIterations", m.maxIterations)
			if bufferedRunFinished != nil {
				if emitErr := emitter.emit(bufferedRunFinished); emitErr != nil {
					return
				}
			}
			return
		}

		// Execute MCP tool calls in parallel
		results := make([]toolCallResult, len(mcpCalls))
		g, gctx := errgroup.WithContext(ctx)
		for i, tc := range mcpCalls {
			i, tc := i, tc
			g.Go(func() error {
				resolved := toolMap[tc.Function.Name]
				var args map[string]any
				if tc.Function.Arguments != "" {
					// Best-effort parse; if it fails, pass empty args
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
				}
				if args == nil {
					args = make(map[string]any)
				}
				result, err := CallMCPTool(gctx, resolved.ServerConfig, resolved.MCPTool.Name, args)
				results[i] = toolCallResult{
					toolCallID: tc.ID,
					content:    ExtractTextContent(result),
					err:        err,
				}
				return nil
			})
		}
		_ = g.Wait()

		// Emit TOOL_CALL_RESULT events and build tool result messages
		for _, res := range results {
			content := res.content
			if res.err != nil {
				content = fmt.Sprintf("Error: %v", res.err)
			}
			resultMsgID := events.GenerateMessageID()
			_ = emitter.ToolCallResult(resultMsgID, res.toolCallID, content)

			// Add tool result message to the conversation
			messages = append(messages, types.Message{
				ID:         resultMsgID,
				Role:       types.RoleTool,
				Content:    content,
				ToolCallID: res.toolCallID,
			})
		}

		// Check if there are still-open non-MCP tool calls (e.g. frontend
		// tools). If so, flush RUN_FINISHED and hand off to the frontend
		// instead of looping back to the agent.
		stillOpen := getOpenToolCalls(messages)
		hasNonMCPOpen := false
		for _, tc := range stillOpen {
			if _, isMCP := toolMap[tc.Function.Name]; !isMCP {
				hasNonMCPOpen = true
				break
			}
		}
		if hasNonMCPOpen {
			if bufferedRunFinished != nil {
				if emitErr := emitter.emit(bufferedRunFinished); emitErr != nil {
					return
				}
			}
			return
		}

		// Build continuation input
		currentInput = types.RunAgentInput{
			ThreadID:       input.ThreadID,
			RunID:          input.RunID,
			ParentRunID:    input.ParentRunID,
			State:          input.State,
			Messages:       messages,
			Tools:          augmentedTools,
			Context:        input.Context,
			ForwardedProps: input.ForwardedProps,
		}
	}
}

type toolCallResult struct {
	toolCallID string
	content    string
	err        error
}

// reconstructMessages builds an updated message list from the original messages
// plus events emitted during the run.
func reconstructMessages(original []types.Message, evs []events.Event) []types.Message {
	msgs := make([]types.Message, len(original))
	copy(msgs, original)

	var currentAssistant *types.Message
	var currentTextBuilder strings.Builder

	for _, ev := range evs {
		switch e := ev.(type) {
		case *events.TextMessageStartEvent:
			currentAssistant = &types.Message{
				ID:   e.MessageID,
				Role: types.RoleAssistant,
			}
			if e.Role != nil {
				currentAssistant.Role = types.Role(*e.Role)
			}
			currentTextBuilder.Reset()

		case *events.TextMessageContentEvent:
			currentTextBuilder.WriteString(e.Delta)

		case *events.TextMessageEndEvent:
			if currentAssistant != nil {
				currentAssistant.Content = currentTextBuilder.String()
				msgs = append(msgs, *currentAssistant)
				currentAssistant = nil
			}

		case *events.ToolCallStartEvent:
			// Ensure there's an assistant message to attach tool calls to
			if currentAssistant == nil {
				msgID := events.GenerateMessageID()
				if e.ParentMessageID != nil && *e.ParentMessageID != "" {
					msgID = *e.ParentMessageID
				}
				currentAssistant = &types.Message{
					ID:   msgID,
					Role: types.RoleAssistant,
				}
			}
			currentAssistant.ToolCalls = append(currentAssistant.ToolCalls, types.ToolCall{
				ID:   e.ToolCallID,
				Type: types.ToolCallTypeFunction,
				Function: types.FunctionCall{
					Name: e.ToolCallName,
				},
			})

		case *events.ToolCallArgsEvent:
			// Append args to the last tool call on the current assistant message
			if currentAssistant != nil && len(currentAssistant.ToolCalls) > 0 {
				tc := &currentAssistant.ToolCalls[len(currentAssistant.ToolCalls)-1]
				tc.Function.Arguments += e.Delta
			}

		case *events.ToolCallEndEvent:
			// Finalize the assistant message with tool calls
			if currentAssistant != nil {
				msgs = append(msgs, *currentAssistant)
				currentAssistant = nil
			}

		case *events.ToolCallResultEvent:
			msgs = append(msgs, types.Message{
				ID:         e.MessageID,
				Role:       types.RoleTool,
				Content:    e.Content,
				ToolCallID: e.ToolCallID,
			})
		}
	}

	// Flush any pending assistant message
	if currentAssistant != nil {
		if currentTextBuilder.Len() > 0 && currentAssistant.Content == nil {
			currentAssistant.Content = currentTextBuilder.String()
		}
		msgs = append(msgs, *currentAssistant)
	}

	return msgs
}

// getOpenToolCalls returns assistant tool calls that have no matching tool
// result message.
func getOpenToolCalls(messages []types.Message) []types.ToolCall {
	// Track which tool call IDs have results
	resolved := make(map[string]bool)
	for _, msg := range messages {
		if msg.ToolCallID != "" {
			resolved[msg.ToolCallID] = true
		}
	}

	var open []types.ToolCall
	for _, msg := range messages {
		if msg.Role == types.RoleAssistant && len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				if !resolved[tc.ID] {
					open = append(open, tc)
				}
			}
		}
	}
	return open
}
