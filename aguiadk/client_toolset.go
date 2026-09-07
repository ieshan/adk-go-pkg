package aguiadk

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/google/jsonschema-go/jsonschema"

	"github.com/ieshan/adk-go-pkg/agui"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
)

// ClientToolMode controls how client tool results are received.
type ClientToolMode int

const (
	// ClientToolModeNextRun ends the run after emitting tool call events.
	// The client fulfills the tool calls and starts a new run with the results.
	ClientToolModeNextRun ClientToolMode = iota
	// ClientToolModeInline keeps the SSE connection open while waiting for
	// results submitted via the /tool-result endpoint.
	ClientToolModeInline
	// ClientToolModeHandBack ends the run with a plain RUN_FINISHED (no
	// interrupt outcome) and optional MESSAGES_SNAPSHOT. The client receives
	// the tool call and starts a new run with the result. Unlike NextRun,
	// no RunStore entry is saved and no interrupt schema is emitted — the
	// hand-back is a clean finish that the client interprets as “your turn.”
	ClientToolModeHandBack
)

// ClientToolConfig configures client tool handling in the bridge.
type ClientToolConfig struct {
	// Mode controls how client tool results are received.
	Mode ClientToolMode

	// ResultHandler receives inline tool results. Required for ClientToolModeInline.
	// If nil and Mode is Inline, one is created automatically by the handler.
	ResultHandler *agui.ToolResultHandler

	// Timeout is the max wait for inline tool results. Default: 5 minutes.
	Timeout time.Duration
}

// clientToolsCtxKey is the context key for per-request client tool state.
type clientToolsCtxKey struct{}

// clientToolsCtx carries per-request client tool state through the ADK
// runner to the ClientToolset.
type clientToolsCtx struct {
	tools         []types.Tool
	emitter       *agui.EventEmitter
	resultHandler *agui.ToolResultHandler
	mode          ClientToolMode
	timeout       time.Duration
}

// WithClientTools injects per-request client tool state into the context.
// The bridge calls this before runner.Run so that ClientToolset.Tools can
// read the state and build per-request proxy tools.
func WithClientTools(ctx context.Context, ctc clientToolsCtx) context.Context {
	return context.WithValue(ctx, clientToolsCtxKey{}, ctc)
}

// WithClientToolsContext builds a context carrying per-request client tool
// state for ClientToolset.Tools to read. It is intended for tests and
// integrations that invoke ClientToolset.Tools directly (without going
// through the bridge's runInternal, which sets the context itself via the
// unexported WithClientTools).
//
// Production code should not need this — the bridge handles context
// injection automatically when Config.ClientTools is set.
func WithClientToolsContext(
	ctx context.Context,
	tools []types.Tool,
	emitter *agui.EventEmitter,
	cfg *ClientToolConfig,
) context.Context {
	mode := ClientToolModeNextRun
	timeout := time.Duration(0)
	var resultHandler *agui.ToolResultHandler
	if cfg != nil {
		mode = cfg.Mode
		timeout = cfg.Timeout
		resultHandler = cfg.ResultHandler
	}
	return WithClientTools(ctx, clientToolsCtx{
		tools:         tools,
		emitter:       emitter,
		resultHandler: resultHandler,
		mode:          mode,
		timeout:       timeout,
	})
}

// ClientToolset implements tool.Toolset, wrapping AG-UI client tools as
// ADK FunctionTools. It must be added to llmagent.Config.Toolsets at agent
// construction time. Per-request tool definitions are injected via context
// by the bridge before runner.Run.
type ClientToolset struct{}

// NewClientToolset creates a ClientToolset. The returned instance must be
// added to llmagent.Config.Toolsets so the ADK runner resolves it on each run.
// Per-request tool definitions are provided dynamically via context.
func NewClientToolset() *ClientToolset {
	return &ClientToolset{}
}

// Name implements tool.Toolset.
func (c *ClientToolset) Name() string { return "agui_client_tools" }

// Tools implements tool.Toolset. It reads per-request client tool definitions
// from the context and wraps each as an ADK FunctionTool.
func (c *ClientToolset) Tools(ctx agent.ReadonlyContext) ([]tool.Tool, error) {
	ctc, ok := ctx.Value(clientToolsCtxKey{}).(clientToolsCtx)
	if !ok || len(ctc.tools) == 0 {
		return nil, nil
	}

	tools := make([]tool.Tool, 0, len(ctc.tools))
	for _, t := range ctc.tools {
		ft, err := makeClientProxyTool(t, ctc)
		if err != nil {
			return nil, fmt.Errorf("aguiadk: failed to create client proxy tool %q: %w", t.Name, err)
		}
		tools = append(tools, ft)
	}
	return tools, nil
}

// makeClientProxyTool creates a single ADK FunctionTool from an AG-UI client
// tool definition. For NextRun and HandBack modes, IsLongRunning=true causes
// ADK to pause the run after the handler returns nil, emitting
// LongRunningToolIDs so the bridge can finish with an interrupt (NextRun) or
// a plain RUN_FINISHED (HandBack). For Inline mode, IsLongRunning=false and
// the handler blocks on the ToolResultHandler until the client submits a
// result via /tool-result.
func makeClientProxyTool(t types.Tool, ctc clientToolsCtx) (tool.Tool, error) {
	cfg := functiontool.Config{
		Name:          t.Name,
		Description:   t.Description,
		IsLongRunning: ctc.mode == ClientToolModeNextRun || ctc.mode == ClientToolModeHandBack,
	}

	if t.Parameters != nil {
		schema, err := toJSONSchema(t.Parameters)
		if err != nil {
			return nil, fmt.Errorf("aguiadk: tool %q parameters: %w", t.Name, err)
		}
		if schema != nil {
			cfg.InputSchema = schema
		}
	}

	handler := func(ctx agent.Context, args map[string]any) (map[string]any, error) {
		return clientProxyHandler(ctx, args, t.Name, ctc)
	}

	return functiontool.New[map[string]any, map[string]any](cfg, handler)
}

// clientProxyHandler is called by the ADK runner when a client tool is invoked.
// The bridge's eventTranslator already emits TOOL_CALL_START/ARGS/END events
// when it sees the FunctionCall in the ADK event stream, so this handler does
// not emit anything itself.
//
// ADK calls the tool handler *before* checking IsLongRunning: a nil result
// with IsLongRunning=true causes ADK to pause the run and emit
// LongRunningToolIDs, which the bridge's interrupt path turns into a
// RUN_FINISHED with Interrupts (NextRun) or a plain RUN_FINISHED (HandBack).
// In both cases the client fulfills the tool call and starts a new run with
// the result.
//
// For Inline mode, the handler waits for the client to submit a result via the
// /tool-result endpoint (ToolResultHandler.Wait).
func clientProxyHandler(
	ctx agent.Context,
	args map[string]any,
	toolName string,
	ctc clientToolsCtx,
) (map[string]any, error) {
	// NextRun: return nil so ADK's IsLongRunning check pauses the run. The
	// bridge's interrupt path handles the RUN_FINISHED with Interrupts. Waiting
	// here would block forever (no one submits a result on this connection) or
	// nil-deref if no ResultHandler was configured for NextRun mode.
	if ctc.mode == ClientToolModeNextRun || ctc.mode == ClientToolModeHandBack {
		return nil, nil
	}

	// Inline: wait for the client to submit a result. Use the ADK function call
	// ID as the key — the bridge's emitFunctionCall uses fc.ID as the AG-UI tool
	// call ID, so the client submits results with the same ID.
	timeout := ctc.timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	if ctc.resultHandler == nil {
		return nil, fmt.Errorf("aguiadk: inline tool %q has no result handler", toolName)
	}
	resultStr, err := ctc.resultHandler.Wait(ctx, ctx.FunctionCallID(), timeout)
	if err != nil {
		return nil, fmt.Errorf("aguiadk: tool %q result wait failed: %w", toolName, err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(resultStr), &result); err != nil {
		return map[string]any{"result": resultStr}, nil
	}
	return result, nil
}

// toJSONSchema converts an arbitrary client-supplied JSON Schema (the tool's
// parameters, decoded as any) to a jsonschema.Schema for the function tool config.
func toJSONSchema(params any) (*jsonschema.Schema, error) {
	if params == nil {
		return nil, nil
	}
	b, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if string(b) == "null" {
		return nil, nil
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("not a valid JSON Schema: %w", err)
	}
	return &s, nil
}
