# AG-UI MCP Support

The adk-go-pkg project provides two integration paths for MCP (Model Context
Protocol) servers. Each path handles MCP tool discovery and execution
independently — pick **one** based on your agent type.

## Which Path Should I Use?

| | Path A: `MCPMiddleware` | Path B: `BuildMCPServerToolsets` |
|---|---|---|
| **Agent type** | Raw `agui.AgentFunc` (no ADK) | ADK-Go `llmagent.Agent` |
| **MCP layer** | AG-UI event level (middleware) | ADK runner (native `mcptoolset`) |
| **Tool execution** | Middleware intercepts calls, executes server-side | ADK runner resolves and executes directly |
| **Config passed to** | `agui.NewMCPMiddleware` | `aguiadk.BuildMCPServerToolsets` |
| **Requires bridge?** | No — uses `agui.Handler` directly | Yes — uses `aguiadk.Handler` or `aguiadk.New` |

> **Do NOT use both paths for the same MCP server.** Registering the same
> `MCPClientConfig` in both `NewMCPMiddleware` and `BuildMCPServerToolsets`
> produces duplicate tools visible to the LLM and conflicting execution paths.
> Pass `MCPClientConfig` **once** to whichever path you choose.

Additionally, **MCPAppsMiddleware** provides UI-enabled tool injection and
proxied MCP request handling for frontend-driven MCP interactions. See
[UI-Enabled Tools with MCPAppsMiddleware](#ui-enabled-tools-with-mcpappsmiddleware).

## Table of Contents

- [Which Path Should I Use?](#which-path-should-i-use)
- [Path A: Generic AG-UI Agent with MCPMiddleware](#path-a-generic-ag-ui-agent-with-mcpmiddleware)
- [Path B: ADK-Go Agent with Bridge and mcptoolset](#path-b-adk-go-agent-with-bridge-and-mcptoolset)
- [The ADK-Go Bridge — What It Is and Why](#the-adk-go-bridge-what-it-is-and-why)
- [UI-Enabled Tools with MCPAppsMiddleware](#ui-enabled-tools-with-mcpappsmiddleware)
- [Configuration](#configuration)
- [MCP Client Helpers](#mcp-client-helpers)
- [Naming and Hash Helpers](#naming-and-hash-helpers)
- [Exported Types](#exported-types)
- [Architecture](#architecture)

## Path A: Generic AG-UI Agent with MCPMiddleware

Use this path when you implement `agui.AgentFunc` directly without ADK-Go.
The middleware wraps your agent, discovers MCP tools at runtime, injects them
into `input.Tools`, and executes MCP tool calls server-side in an agentic loop
(up to `MaxIterations` rounds).

```go
package main

import (
	"context"
	"iter"
	"log"
	"net/http"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func main() {
	// 1. Define your agent logic.
	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		ch := make(chan events.Event, 64)
		emitter := agui.NewEventEmitterWithContext(ctx, ch)
		go func() {
			defer close(ch)
			emitter.RunStarted(input.ThreadID, input.RunID)
			msgID := emitter.GenerateMessageID()
			role := "assistant"
			emitter.TextMessageStart(msgID, &role)
			emitter.TextMessageContent(msgID, "Hello with MCP tools!")
			emitter.TextMessageEnd(msgID)
			emitter.RunFinishedWithOptions(input.ThreadID, input.RunID)
		}()
		return agui.ChanToIter(ctx, ch)
	})

	// 2. Wrap with MCP middleware — discovers and executes MCP tools.
	mcpMW := agui.NewMCPMiddleware([]agui.MCPClientConfig{
		{Type: "http", URL: "https://learn.microsoft.com/api/mcp", ServerID: "mslearn"},
	}, agui.MCPMiddlewareOptions{MaxIterations: 32})

	// 3. Serve via AG-UI HTTP handler.
	handler, err := agui.Handler(agui.Config{
		Agent: mcpMW(agent),
	})
	if err != nil {
		log.Fatal(err)
	}

	http.Handle("/api/agent", handler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

**What the middleware does at runtime:**

1. Connects to each MCP server listed in `MCPClientConfig`
2. Lists available tools (cached after first call via `sync.Once`)
3. Injects namespaced MCP tools into `input.Tools` before calling your agent
4. After your agent emits tool call events, executes the MCP calls server-side
5. Feeds results back to the agent as new messages and repeats (up to `MaxIterations`)

> **Hand-off to frontend on open non-MCP tool calls:** When there are still-open
> non-MCP tool calls after the loop, `MCPMiddleware` flushes the buffered
> `RUN_FINISHED` instead of continuing, handing control back to the frontend
> to resolve those tool calls.

## Path B: ADK-Go Agent with Bridge and mcptoolset

Use this path when your agent is an ADK-Go `llmagent.Agent`. MCP tools become
native ADK toolsets via `mcptoolset` — the ADK runner handles connection, tool
listing, and execution internally. The bridge translates ADK session events
into AG-UI SSE events for the frontend.

```go
package main

import (
	"log"
	"net/http"

	"github.com/ieshan/adk-go-pkg/agui"
	"github.com/ieshan/adk-go-pkg/aguiadk"

	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
)

func main() {
	// 1. Create MCP toolsets from server configs.
	//    Each config becomes an ADK mcptoolset.Toolset that connects
	//    to the MCP server and lists tools lazily on first use.
	toolsets, err := aguiadk.BuildMCPServerToolsets([]agui.MCPClientConfig{
		{Type: "http", URL: "https://learn.microsoft.com/api/mcp", ServerID: "mslearn"},
	})
	if err != nil {
		log.Fatal(err)
	}

	// 2. Build your ADK LLM agent with MCP toolsets.
	//    Plug in your model here — see model/openai, model/anthropic,
	//    or use any model.LLM implementation.
	var llm model.LLM // = openai.New(openai.Config{...})

	adkAgent, err := llmagent.New(llmagent.Config{
		Name:     "my-agent",
		Model:    llm,
		Toolsets: toolsets,
	})
	if err != nil {
		log.Fatal(err)
	}

	// 3. Create the AG-UI HTTP handler via the bridge.
	//    aguiadk.Handler combines bridge creation + AG-UI SSE serving
	//    in a single call. This is the recommended entry point.
	handler, err := aguiadk.Handler(
		aguiadk.Config{
			Agent:   adkAgent,
			AppName: "my-chatbot",
			UserID:  "default-user",
		},
		agui.Config{},
	)
	if err != nil {
		log.Fatal(err)
	}

	http.Handle("/api/agent", handler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

**What happens at runtime:**

1. The ADK runner connects to MCP servers via `mcptoolset` and lists tools
2. The LLM sees MCP tools alongside any other ADK tools in `Toolsets`
3. When the LLM calls an MCP tool, the ADK runner executes it directly
4. The bridge translates ADK events (`session.Event`) into AG-UI SSE events
   (`TEXT_MESSAGE_START`, `TOOL_CALL_START`, `RUN_FINISHED`, etc.)

No `MCPMiddleware` is needed — the ADK runner handles everything natively.

## The ADK-Go Bridge — What It Is and Why

In Path B, the **bridge** is the adapter between ADK-Go and the AG-UI protocol.

### What It Is

`aguiadk.New(cfg)` returns an `agui.Agent` that wraps an ADK `agent.Agent`.
It implements the `agui.Agent` interface by:

- Creating ADK sessions from AG-UI thread IDs
- Running the ADK runner (`runner.Run`) with the agent
- Translating ADK `session.Event` values into AG-UI SSE events

### Why It's Needed

ADK agents speak ADK's internal protocol — sessions, events, LLM responses,
function calls, state deltas. AG-UI frontends expect SSE event streams
(`TEXT_MESSAGE_START`, `TOOL_CALL_START`, `RUN_FINISHED`, etc.). The bridge
translates between these two protocols so any ADK-Go agent can serve
AG-UI-compatible frontends like CopilotKit or AG-UI Vue.

### Two Entry Points

| Entry Point | Returns | When to Use |
|---|---|---|
| `aguiadk.New(cfg)` | `agui.Agent` | When you need custom middleware chains or want to control the `agui.Handler` setup yourself |
| `aguiadk.Handler(cfg, agCfg)` | `http.Handler` | **Recommended for most apps.** Combines `New()` + `agui.Handler()` + auto-wires `/tool-result` endpoint for inline tool mode |

#### Using `aguiadk.New` directly

```go
// New returns an agui.Agent — you must pass it to agui.Handler yourself.
bridgeAgent, err := aguiadk.New(aguiadk.Config{
	Agent:   adkAgent,
	AppName: "my-app",
	UserID:  "user-1",
})
if err != nil { /* handle */ }

// Now serve it — the bridge agent is an agui.Agent.
handler, err := agui.Handler(agui.Config{
	Agent:       bridgeAgent,
	Middlewares: []agui.Middleware{ /* optional custom middleware */ },
})
if err != nil { /* handle */ }
```

#### Using `aguiadk.Handler` (recommended)

```go
// Handler combines bridge + AG-UI SSE serving in one call.
// It also auto-wires the /tool-result endpoint for inline client tool mode.
handler, err := aguiadk.Handler(
	aguiadk.Config{
		Agent:   adkAgent,
		AppName: "my-app",
		UserID:  "user-1",
	},
	agui.Config{},
)
if err != nil { /* handle */ }
```

For the full Config reference (SessionService, ArtifactService, MemoryService,
ClientTools, RunStore, SuppressToolEvents, Presets, event translation details,
and more), see [ADK-Go AG-UI Bridge](aguiadk-bridge.md).

## UI-Enabled Tools with MCPAppsMiddleware

`MCPAppsMiddleware` is similar to `MCPMiddleware` but targets a different
use case: it injects **UI-enabled** MCP tools (tools with
`_meta["ui/resourceUri"]`) and handles **proxied MCP requests** sent from
frontends via `ForwardedProps`. This enables frontend-driven MCP interactions
where the UI directly calls MCP server methods (e.g., `tools/call`,
`resources/read`) without going through the agent's LLM loop.

```go
package main

import (
	"context"
	"iter"
	"log"
	"net/http"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func main() {
	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		ch := make(chan events.Event, 64)
		emitter := agui.NewEventEmitterWithContext(ctx, ch)
		go func() {
			defer close(ch)
			emitter.RunStarted(input.ThreadID, input.RunID)
			msgID := emitter.GenerateMessageID()
			role := "assistant"
			emitter.TextMessageStart(msgID, &role)
			emitter.TextMessageContent(msgID, "Hello with MCP apps!")
			emitter.TextMessageEnd(msgID)
			emitter.RunFinishedWithOptions(input.ThreadID, input.RunID)
		}()
		return agui.ChanToIter(ctx, ch)
	})

	mcpAppsMW := agui.NewMCPAppsMiddleware([]agui.MCPClientConfig{
		{Type: "http", URL: "https://my-mcp-server.com/mcp", ServerID: "apps"},
	})

	handler, err := agui.Handler(agui.Config{
		Agent: mcpAppsMW(agent),
	})
	if err != nil {
		log.Fatal(err)
	}

	http.Handle("/api/agent", handler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

**Differences from `MCPMiddleware`:**

- Injects only tools with `_meta["ui/resourceUri"]` (UI-enabled tools)
- Handles proxied MCP requests from frontends (see [Proxied MCP Requests](#proxied-mcp-requests))
- Does not run an agentic tool execution loop
- After the agent run, executes pending UI tool calls found in `input.Messages`, producing `TOOL_CALL_RESULT` + `ACTIVITY_SNAPSHOT` events
- After each pending UI tool execution, emits an `ACTIVITY_SNAPSHOT` event of type `mcp-apps` with content fields `result`, `resourceUri`, `serverHash`, `serverId`, `toolInput`, and `replace=true`

> **Pass-through on empty `servers`:** Both `MCPMiddleware` and
> `MCPAppsMiddleware` are no-ops (pass-through) when the `servers` slice is
> empty — the wrapped agent runs unchanged with no tool injection or
> proxied request handling.

## Configuration

### MCPClientConfig

| Field     | Type                | Description                                      |
|-----------|---------------------|--------------------------------------------------|
| `Type`    | `string`            | Transport type: `"http"`, `"streamable"`, or `""` (empty string) are all aliases for streamable HTTP; only `"sse"` selects SSE. An empty `Type` defaults to streamable HTTP. |
| `URL`     | `string`            | MCP server endpoint URL                          |
| `Headers` | `map[string]string` | Optional HTTP headers for authentication         |
| `ServerID`| `string`            | Optional. Server identifier for tool namespacing and proxied request routing (see [Proxied MCP Requests](#proxied-mcp-requests)). An empty `ServerID` produces a tool name like `mcp____tool` (with an empty sanitized segment, yielding four underscores between `mcp` and the tool name). |

### MCPMiddlewareOptions

`MCPMiddlewareOptions` configures `NewMCPMiddleware` behavior.

| Field           | Type   | Description                                                          |
|-----------------|--------|----------------------------------------------------------------------|
| `MaxIterations` | `int`  | Caps the number of server-side tool-execution rounds per run. Default: `32`. A value `<= 0` selects the default. When the limit is reached, the middleware logs a `slog.Warn` and flushes the buffered `RUN_FINISHED` — it does NOT emit a client-visible warning event. |

### Tool Naming

MCP tools are namespaced using the format: `mcp__{sanitizedServerID}__{sanitizedToolName}`

- Names are truncated to 64 characters
- Collisions are resolved with `_2`, `_3`, etc. suffixes
- Special characters are replaced with underscores

### Proxied MCP Requests

Frontends can send proxied MCP requests by including `__proxiedMCPRequest` in `ForwardedProps`:

```json
{
  "__proxiedMCPRequest": {
    "serverId": "apps",
    "method": "tools/call",
    "params": {"name": "echo", "arguments": {"message": "hello"}}
  }
}
```

The `serverId` field is used for lookup first; if not found, `serverHash`
(from `GetServerHash`) is used as a fallback. Both fields are optional. The
middleware resolves the server by `ServerID` first, then `ServerHash` as
fallback. If neither matches a configured server, the request fails with an
"unknown MCP server" error (emitted as a `RUN_FINISHED` event).

Supported methods: `tools/call`, `resources/read`, `ping`.

## MCP Client Helpers

The `agui/mcp_client.go` file exports these helpers for direct MCP server
interaction:

```go
// BuildMCPTransport constructs an mcp.Transport from the given config.
func BuildMCPTransport(config MCPClientConfig) (mcp.Transport, error)

// ListMCPTools connects to the server and returns its advertised tools.
func ListMCPTools(ctx context.Context, config MCPClientConfig) ([]*mcp.Tool, error)

// CallMCPTool invokes a single tool by name with the given arguments.
func CallMCPTool(ctx context.Context, config MCPClientConfig, name string, args map[string]any) (*mcp.CallToolResult, error)

// ReadMCPResource reads a resource by URI from the server.
func ReadMCPResource(ctx context.Context, config MCPClientConfig, uri string) (any, error)

// ExecuteMCPRequest sends an arbitrary MCP method/params pair to the server.
func ExecuteMCPRequest(ctx context.Context, config MCPClientConfig, method string, params map[string]any) (any, error)

// ExtractTextContent pulls the concatenated text content out of a CallToolResult.
// If no text content is present, it falls back to JSON-encoding the content
// array so non-text results (e.g., images, embedded resources) are not
// silently dropped.
func ExtractTextContent(result *mcp.CallToolResult) string
```

## Naming and Hash Helpers

The `agui/mcp_naming.go` file exports these helpers:

```go
// SanitizeSegment replaces special characters with underscores and trims
// leading/trailing underscores from a single name segment for use in tool namespacing.
func SanitizeSegment(s string) string

// MakeUniqueToolName builds a namespaced tool name (mcp__{server}__{tool}),
// appending _2, _3, etc. suffixes to resolve collisions against the used set.
func MakeUniqueToolName(serverID, toolName string, used map[string]struct{}) string

// GetServerHash returns a stable hash for the server config: SHA-256 of the
// JSON-encoded {Type, URL, Headers} tuple, first 16 hex chars. The JSON
// object uses lower-case keys: {"type":...,"url":...,"headers":...}. It does
// NOT include ServerID. This matters for frontend fallback routing — two
// configs differing only in ServerID share the same hash.
func GetServerHash(config MCPClientConfig) string
```

## Exported Types

The `agui/mcp_types.go` file exports these types:

### ResolvedMCPTool

`ResolvedMCPTool` represents an MCP tool that has been discovered from a
configured server and namespaced for injection into `input.Tools`. It pairs
the original MCP tool metadata with the server identity used for execution
routing.

### ProxiedMCPRequest

`ProxiedMCPRequest` is the structure decoded from the `__proxiedMCPRequest`
field in `ForwardedProps`. It carries a frontend-driven MCP method call to
be executed by the middleware against a configured server.

```go
type ProxiedMCPRequest struct {
	ServerHash string
	ServerID   string
	Method     string
	Params     map[string]any
}
```

## Architecture

### MCP-Specific Components

- **Shared MCP Client Layer** (`agui/mcp_client.go`): transport builder, connect/list/call functions
- **MCPMiddleware** (`agui/mcp_middleware.go`): tool injection + server-side execution loop (Path A)
- **MCPAppsMiddleware** (`agui/mcp_apps_middleware.go`): UI tools + proxied requests
- **Bridge MCP helper** (`aguiadk/mcp.go`): `BuildMCPServerToolsets` creates ADK `mcptoolset` instances from `MCPClientConfig` (Path B)

### Bridge Components (Path B)

- **Bridge** (`aguiadk/bridge.go`): translates ADK session events into AG-UI SSE events; implements `agui.Agent`
- **ClientToolset** (`aguiadk/client_toolset.go`): wraps AG-UI client tools (`RunAgentInput.Tools`) as ADK `FunctionTool`s for human-in-the-loop flows
- **Handler** (`aguiadk/handler.go`): convenience entry point — combines `aguiadk.New` + `agui.Handler` + auto-wires `/tool-result` endpoint

For full bridge architecture, Config reference, event translation, ClientToolset,
RunStore, Presets, and more, see [ADK-Go AG-UI Bridge](aguiadk-bridge.md).
