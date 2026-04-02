package aguiadk

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"

	"github.com/ieshan/adk-go-pkg/agui"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// ProxyToolset wraps AG-UI client tools as ADK FunctionTools.
// This enables inline tool mode where the ADK agent calls client-side tools
// and waits for results via the ToolResultHandler.
type ProxyToolset struct {
	tools         []tool.Tool
	emitter       *agui.EventEmitter
	resultHandler *agui.ToolResultHandler
	timeout       time.Duration
}

// NewProxyToolset creates a ProxyToolset from AG-UI tool definitions.
// Each AG-UI tool is wrapped as an ADK FunctionTool with IsLongRunning: true.
// When invoked, the handler emits TOOL_CALL_START/ARGS/END events and waits
// for the client to submit a result via the ToolResultHandler.
func NewProxyToolset(
	tools []types.Tool,
	emitter *agui.EventEmitter,
	resultHandler *agui.ToolResultHandler,
	timeout time.Duration,
) (*ProxyToolset, error) {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	wrapped := make([]tool.Tool, 0, len(tools))
	for _, t := range tools {
		ft, err := makeProxyTool(t, emitter, resultHandler, timeout)
		if err != nil {
			return nil, fmt.Errorf("aguiadk: failed to create proxy tool %q: %w", t.Name, err)
		}
		wrapped = append(wrapped, ft)
	}

	return &ProxyToolset{
		tools:         wrapped,
		emitter:       emitter,
		resultHandler: resultHandler,
		timeout:       timeout,
	}, nil
}

// Tools returns the wrapped ADK tools.
func (p *ProxyToolset) Tools() []tool.Tool {
	return p.tools
}

// makeProxyTool creates a single ADK FunctionTool from an AG-UI tool definition.
func makeProxyTool(
	t types.Tool,
	emitter *agui.EventEmitter,
	resultHandler *agui.ToolResultHandler,
	timeout time.Duration,
) (tool.Tool, error) {
	cfg := functiontool.Config{
		Name:          t.Name,
		Description:   t.Description,
		IsLongRunning: true,
	}

	handler := func(ctx tool.Context, args map[string]any) (map[string]any, error) {
		return proxyToolHandler(ctx, args, t.Name, emitter, resultHandler, timeout)
	}

	return functiontool.New[map[string]any, map[string]any](cfg, handler)
}

// proxyToolHandler emits tool call events and waits for the client result.
func proxyToolHandler(
	ctx tool.Context,
	args map[string]any,
	toolName string,
	emitter *agui.EventEmitter,
	resultHandler *agui.ToolResultHandler,
	timeout time.Duration,
) (map[string]any, error) {
	toolCallID := emitter.GenerateToolCallID()

	// Emit TOOL_CALL_START.
	if err := emitter.ToolCallStart(toolCallID, toolName, nil); err != nil {
		return nil, fmt.Errorf("aguiadk: failed to emit TOOL_CALL_START: %w", err)
	}

	// Emit TOOL_CALL_ARGS with serialized arguments.
	if args != nil {
		argsJSON, err := json.Marshal(args)
		if err != nil {
			return nil, fmt.Errorf("aguiadk: failed to marshal tool args: %w", err)
		}
		if err = emitter.ToolCallArgs(toolCallID, string(argsJSON)); err != nil {
			return nil, fmt.Errorf("aguiadk: failed to emit TOOL_CALL_ARGS: %w", err)
		}
	}

	// Emit TOOL_CALL_END.
	if err := emitter.ToolCallEnd(toolCallID); err != nil {
		return nil, fmt.Errorf("aguiadk: failed to emit TOOL_CALL_END: %w", err)
	}

	// Wait for the client to submit a result, using the tool context
	// (which embeds context.Context) so cancellation propagates properly.
	resultStr, err := resultHandler.Wait(ctx, toolCallID, timeout)
	if err != nil {
		return nil, fmt.Errorf("aguiadk: tool %q result wait failed: %w", toolName, err)
	}

	// Parse the result as JSON.
	var result map[string]any
	if err := json.Unmarshal([]byte(resultStr), &result); err != nil {
		// If it's not valid JSON, wrap it as a simple result.
		return map[string]any{"result": resultStr}, nil
	}
	return result, nil
}
