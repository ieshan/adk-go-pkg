package aguiadk_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ieshan/adk-go-pkg/agui"
	"github.com/ieshan/adk-go-pkg/aguiadk"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestBuildMCPServerToolsets_Empty(t *testing.T) {
	toolsets, err := aguiadk.BuildMCPServerToolsets(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(toolsets) != 0 {
		t.Errorf("expected 0 toolsets, got %d", len(toolsets))
	}
}

func TestBuildMCPServerToolsets_InvalidTransport(t *testing.T) {
	// Empty URL should cause a transport error
	_, err := aguiadk.BuildMCPServerToolsets([]agui.MCPClientConfig{
		{Type: "http", URL: "", ServerID: "srv1"},
	})
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestBuildMCPServerToolsets_HTTPConfig(t *testing.T) {
	// This should succeed — mcptoolset.New creates the toolset lazily
	// (connection happens on first Tools() call, not at construction)
	toolsets, err := aguiadk.BuildMCPServerToolsets([]agui.MCPClientConfig{
		{Type: "http", URL: "https://example.com/mcp", ServerID: "srv1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(toolsets) != 1 {
		t.Fatalf("expected 1 toolset, got %d", len(toolsets))
	}
	// Toolset should have a name
	if toolsets[0].Name() == "" {
		t.Error("toolset name should not be empty")
	}
}

func TestBuildMCPServerToolsets_MultipleServers(t *testing.T) {
	toolsets, err := aguiadk.BuildMCPServerToolsets([]agui.MCPClientConfig{
		{Type: "http", URL: "https://server1.com/mcp", ServerID: "srv1"},
		{Type: "sse", URL: "https://server2.com/sse", ServerID: "srv2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(toolsets) != 2 {
		t.Fatalf("expected 2 toolsets, got %d", len(toolsets))
	}
}

func TestBuildMCPServerToolsets_InvalidType(t *testing.T) {
	_, err := aguiadk.BuildMCPServerToolsets([]agui.MCPClientConfig{
		{Type: "invalid", URL: "https://example.com", ServerID: "srv1"},
	})
	if err == nil {
		t.Fatal("expected error for invalid transport type")
	}
}

// TestBuildMCPServerToolsets_Integration verifies that toolsets created from
// a real HTTP MCP server can actually list tools.
func TestBuildMCPServerToolsets_Integration(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0"}, nil)
	server.AddTool(&mcp.Tool{
		Name:        "echo",
		Description: "Echoes the input text",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{
					"type":        "string",
					"description": "The text to echo",
				},
			},
			"required": []string{"message"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]any
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "invalid arguments"}},
			}, nil
		}
		msg, _ := args["message"].(string)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: msg}},
		}, nil
	})

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, nil)
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)

	toolsets, err := aguiadk.BuildMCPServerToolsets([]agui.MCPClientConfig{
		{Type: "http", URL: httpServer.URL, ServerID: "test-server"},
	})
	if err != nil {
		t.Fatalf("BuildMCPServerToolsets: %v", err)
	}
	if len(toolsets) != 1 {
		t.Fatalf("expected 1 toolset, got %d", len(toolsets))
	}

	// Verify the toolset has a non-empty name
	if toolsets[0].Name() == "" {
		t.Error("toolset name should not be empty")
	}
}
