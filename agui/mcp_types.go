package agui

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxToolNameLength    = 64
	mcpToolNamePrefix    = "mcp"
	defaultMaxIterations = 32
	MCPAppsActivityType  = "mcp-apps"
)

// MCPClientConfig describes a single MCP server connection.
type MCPClientConfig struct {
	// Type is the transport type: "http" (streamable HTTP) or "sse".
	Type string
	// URL is the MCP server endpoint URL.
	URL string
	// Headers are optional HTTP headers sent with each request.
	Headers map[string]string
	// ServerID is an optional identifier for the server. Used for tool
	// namespacing and proxied request routing.
	ServerID string
}

// MCPMiddlewareOptions configures MCPMiddleware behavior.
type MCPMiddlewareOptions struct {
	// MaxIterations caps the number of tool-execution rounds per run.
	// Default: 32.
	MaxIterations int
}

// ResolvedMCPTool pairs a raw MCP tool with its namespaced AG-UI name and
// the server config it came from.
type ResolvedMCPTool struct {
	MCPTool        *mcp.Tool
	NamespacedName string
	ServerConfig   MCPClientConfig
}

// ProxiedMCPRequest is sent by the frontend to directly interact with an
// MCP server, bypassing the agent.
type ProxiedMCPRequest struct {
	ServerHash string
	ServerID   string
	Method     string
	Params     map[string]any
}
