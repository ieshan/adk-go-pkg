# AG-UI MCP Support

The adk-go-pkg project provides two integration paths for MCP (Model Context Protocol) servers:

1. **MCPMiddleware** — generic AG-UI middleware that injects MCP tools and executes them server-side
2. **Bridge wiring** — uses ADK-Go's built-in `mcptoolset` for native ADK tool resolution

Additionally, **MCPAppsMiddleware** provides UI-enabled tool injection and proxied MCP request handling for frontend-driven MCP interactions.

## Quick Start

### Generic AG-UI Agent with MCPMiddleware

```go
import (
    "github.com/ieshan/adk-go-pkg/agui"
)

agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
    // your agent logic
})

mcpMW := agui.NewMCPMiddleware([]agui.MCPClientConfig{
    {Type: "http", URL: "https://learn.microsoft.com/api/mcp", ServerID: "mslearn"},
}, agui.MCPMiddlewareOptions{MaxIterations: 32})

handler, _ := agui.Handler(agui.Config{
    Agent: mcpMW(agent),
})
```

### ADK-Go Bridge with mcptoolset

```go
import (
    "github.com/ieshan/adk-go-pkg/agui"
    "github.com/ieshan/adk-go-pkg/aguiadk"
    "google.golang.org/adk/v2/agent/llmagent"
)

toolsets, err := aguiadk.BuildMCPServerToolsets([]agui.MCPClientConfig{
    {Type: "http", URL: "https://learn.microsoft.com/api/mcp", ServerID: "mslearn"},
})
if err != nil { /* handle */ }

adkAgent, err := llmagent.New(llmagent.Config{
    Name:     "my-agent",
    Model:    model,
    Toolsets: toolsets,
})
if err != nil { /* handle */ }

bridge, err := aguiadk.New(aguiadk.Config{Agent: adkAgent})
if err != nil { /* handle */ }
_ = bridge
```

### UI-Enabled Tools with MCPAppsMiddleware

```go
mcpAppsMW := agui.NewMCPAppsMiddleware([]agui.MCPClientConfig{
    {Type: "http", URL: "https://my-mcp-server.com/mcp", ServerID: "apps"},
})

handler, _ := agui.Handler(agui.Config{
    Agent: mcpAppsMW(agent),
})
```

## Configuration

### MCPClientConfig

| Field     | Type                | Description                                      |
|-----------|---------------------|--------------------------------------------------|
| `Type`    | `string`            | Transport type: `"http"` (streamable) or `"sse"` |
| `URL`     | `string`            | MCP server endpoint URL                          |
| `Headers` | `map[string]string` | Optional HTTP headers for authentication         |
| `ServerID`| `string`            | Server identifier for tool namespacing           |

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
(from `GetServerHash`) is used as a fallback. Both fields are optional but
at least one must be present.

Supported methods: `tools/call`, `resources/read`, `ping`.

## Architecture

- **Shared MCP Client Layer** (`agui/mcp_client.go`): transport builder, connect/list/call functions
- **MCPMiddleware** (`agui/mcp_middleware.go`): tool injection + server-side execution loop
- **MCPAppsMiddleware** (`agui/mcp_apps_middleware.go`): UI tools + proxied requests
- **Bridge wiring** (`aguiadk/mcp.go`): `BuildMCPServerToolsets` helper using ADK's `mcptoolset`
