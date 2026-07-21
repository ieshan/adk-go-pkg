package agui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// BuildMCPTransport creates an MCP transport from the given config.
// The transport type is determined by config.Type:
//   - "http" or "streamable": mcp.StreamableClientTransport
//   - "sse": mcp.SSEClientTransport
//
// Custom headers are injected via a header-round-tripping *http.Client.
func BuildMCPTransport(config MCPClientConfig) (mcp.Transport, error) {
	if config.URL == "" {
		return nil, fmt.Errorf("agui: MCP server URL is required")
	}

	httpClient := &http.Client{}
	if len(config.Headers) > 0 {
		httpClient.Transport = &headerRoundTripper{
			base:    http.DefaultTransport,
			headers: config.Headers,
		}
	}

	switch config.Type {
	case "http", "streamable", "":
		return &mcp.StreamableClientTransport{
			Endpoint:   config.URL,
			HTTPClient: httpClient,
		}, nil
	case "sse":
		return &mcp.SSEClientTransport{
			Endpoint:   config.URL,
			HTTPClient: httpClient,
		}, nil
	default:
		return nil, fmt.Errorf("agui: unsupported MCP transport type %q (use \"http\" or \"sse\")", config.Type)
	}
}

// headerRoundTripper injects custom headers into every request.
type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	for k, v := range h.headers {
		clone.Header.Set(k, v)
	}
	return h.base.RoundTrip(clone)
}

// connectMCP creates an MCP client session from the config.
// Returns the session and a close function that must be called when done.
func connectMCP(ctx context.Context, config MCPClientConfig) (*mcp.ClientSession, func() error, error) {
	transport, err := BuildMCPTransport(config)
	if err != nil {
		return nil, nil, fmt.Errorf("agui: building MCP transport: %w", err)
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "adk-go-pkg",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("agui: connecting to MCP server at %q: %w", config.URL, err)
	}

	return session, session.Close, nil
}

// ListMCPTools connects to an MCP server and lists all available tools,
// following pagination cursors until all pages are retrieved.
func ListMCPTools(ctx context.Context, config MCPClientConfig) ([]*mcp.Tool, error) {
	session, closeFn, err := connectMCP(ctx, config)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	var allTools []*mcp.Tool
	cursor := ""
	for {
		result, err := session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, fmt.Errorf("agui: listing tools from %q: %w", config.URL, err)
		}
		allTools = append(allTools, result.Tools...)
		if result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
	}
	return allTools, nil
}

// CallMCPTool connects to an MCP server and calls a tool by name with the
// given arguments.
func CallMCPTool(ctx context.Context, config MCPClientConfig, name string, args map[string]any) (*mcp.CallToolResult, error) {
	session, closeFn, err := connectMCP(ctx, config)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return nil, fmt.Errorf("agui: calling tool %q on %q: %w", name, config.URL, err)
	}
	return result, nil
}

// ReadMCPResource connects to an MCP server and reads a resource by URI.
func ReadMCPResource(ctx context.Context, config MCPClientConfig, uri string) (any, error) {
	session, closeFn, err := connectMCP(ctx, config)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	result, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		return nil, fmt.Errorf("agui: reading resource %q on %q: %w", uri, config.URL, err)
	}
	return result, nil
}

// ExecuteMCPRequest dispatches a proxied MCP request to the appropriate
// MCP operation based on the method name.
// Supported methods: "tools/call", "resources/read", "ping".
func ExecuteMCPRequest(ctx context.Context, config MCPClientConfig, method string, params map[string]any) (any, error) {
	switch method {
	case "tools/call":
		name, _ := params["name"].(string)
		args, _ := params["arguments"].(map[string]any)
		if args == nil {
			args = make(map[string]any)
		}
		return CallMCPTool(ctx, config, name, args)

	case "resources/read":
		uri, _ := params["uri"].(string)
		return ReadMCPResource(ctx, config, uri)

	case "ping":
		session, closeFn, err := connectMCP(ctx, config)
		if err != nil {
			return nil, err
		}
		defer closeFn()
		if err := session.Ping(ctx, nil); err != nil {
			return nil, fmt.Errorf("agui: ping to %q: %w", config.URL, err)
		}
		return map[string]any{"status": "ok"}, nil

	case "notifications/message":
		return nil, fmt.Errorf("agui: notifications/message is not supported by the Go MCP SDK")

	default:
		return nil, fmt.Errorf("agui: unsupported MCP method %q", method)
	}
}

// ExtractTextContent extracts all text content from an MCP CallToolResult
// and joins it with newlines. If no text content is present, it falls back
// to JSON-encoding the content array so non-text results are not silently
// dropped.
func ExtractTextContent(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	var parts []string
	for _, content := range result.Content {
		if tc, ok := content.(*mcp.TextContent); ok && tc != nil {
			parts = append(parts, tc.Text)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n")
	}
	// No text content — fall back to JSON so non-text results are visible.
	if len(result.Content) > 0 {
		raw, err := json.Marshal(result.Content)
		if err != nil {
			return ""
		}
		return string(raw)
	}
	return ""
}
