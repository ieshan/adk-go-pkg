package agui_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ieshan/adk-go-pkg/agui"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// startHTTPMCPServer starts a real HTTP MCP server with an "echo" tool and
// returns the server URL and the MCPClientConfig for connecting to it.
// The server is automatically cleaned up when the test ends.
func startHTTPMCPServer(t *testing.T) agui.MCPClientConfig {
	t.Helper()
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

	return agui.MCPClientConfig{
		Type:     "http",
		URL:      httpServer.URL,
		ServerID: "test-server",
	}
}

// startHTTPMCPServerWithUI starts an HTTP MCP server with a UI-enabled tool
// (has _meta["ui/resourceUri"]) and a regular tool, returning the config.
func startHTTPMCPServerWithUI(t *testing.T) agui.MCPClientConfig {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "test-ui-server", Version: "1.0"}, nil)

	// UI-enabled tool
	server.AddTool(&mcp.Tool{
		Name:        "ui_tool",
		Description: "A UI-enabled tool",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		Meta:        map[string]any{"ui/resourceUri": "ui://server/ui_tool"},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "ui result"}},
		}, nil
	})

	// Regular tool (no UI resource)
	server.AddTool(&mcp.Tool{
		Name:        "plain_tool",
		Description: "A plain tool",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "plain result"}},
		}, nil
	})

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, nil)
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)

	return agui.MCPClientConfig{
		Type:     "http",
		URL:      httpServer.URL,
		ServerID: "ui-server",
	}
}

func TestExtractTextContent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		result *mcp.CallToolResult
		want   string
	}{
		{
			name:   "nil result",
			result: nil,
			want:   "",
		},
		{
			name:   "empty content",
			result: &mcp.CallToolResult{},
			want:   "",
		},
		{
			name: "single text",
			result: &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "hello"}},
			},
			want: "hello",
		},
		{
			name: "multiple text joined with newline",
			result: &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "hello "},
					&mcp.TextContent{Text: "world"},
				},
			},
			want: "hello \nworld",
		},
		{
			name: "non-text content falls back to JSON",
			result: &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.ImageContent{Data: []byte("base64data"), MIMEType: "image/png"},
				},
			},
			want: `[{"type":"image","mimeType":"image/png","data":"YmFzZTY0ZGF0YQ=="}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := agui.ExtractTextContent(tt.result)
			if got != tt.want {
				t.Errorf("ExtractTextContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildMCPTransport(t *testing.T) {
	t.Parallel()
	t.Run("http type", func(t *testing.T) {
		t.Parallel()
		tr, err := agui.BuildMCPTransport(agui.MCPClientConfig{
			Type: "http",
			URL:  "https://example.com/mcp",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tr == nil {
			t.Fatal("transport is nil")
		}
	})

	t.Run("sse type", func(t *testing.T) {
		t.Parallel()
		tr, err := agui.BuildMCPTransport(agui.MCPClientConfig{
			Type: "sse",
			URL:  "https://example.com/sse",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tr == nil {
			t.Fatal("transport is nil")
		}
	})

	t.Run("empty URL", func(t *testing.T) {
		t.Parallel()
		_, err := agui.BuildMCPTransport(agui.MCPClientConfig{
			Type: "http",
		})
		if err == nil {
			t.Fatal("got nil error, want error for empty URL")
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		t.Parallel()
		_, err := agui.BuildMCPTransport(agui.MCPClientConfig{
			Type: "invalid",
			URL:  "https://example.com",
		})
		if err == nil {
			t.Fatal("got nil error, want error for invalid type")
		}
	})

	t.Run("with headers", func(t *testing.T) {
		t.Parallel()
		tr, err := agui.BuildMCPTransport(agui.MCPClientConfig{
			Type:    "http",
			URL:     "https://example.com/mcp",
			Headers: map[string]string{"Authorization": "Bearer token"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tr == nil {
			t.Fatal("transport is nil")
		}
	})
}

func TestListMCPTools(t *testing.T) {
	t.Parallel()
	cfg := startHTTPMCPServer(t)
	tools, err := agui.ListMCPTools(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ListMCPTools: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(tools))
	}
	if tools[0].Name != "echo" {
		t.Errorf("tool name = %q, want %q", tools[0].Name, "echo")
	}
}

func TestCallMCPTool(t *testing.T) {
	t.Parallel()
	cfg := startHTTPMCPServer(t)
	result, err := agui.CallMCPTool(context.Background(), cfg, "echo", map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("CallMCPTool: %v", err)
	}
	text := agui.ExtractTextContent(result)
	if text != "hello" {
		t.Errorf("got %q, want %q", text, "hello")
	}
}

func TestExecuteMCPRequest_Ping(t *testing.T) {
	t.Parallel()
	cfg := startHTTPMCPServer(t)
	result, err := agui.ExecuteMCPRequest(context.Background(), cfg, "ping", nil)
	if err != nil {
		t.Fatalf("ExecuteMCPRequest ping: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok || m["status"] != "ok" {
		t.Errorf("ping result = %v, want {status: ok}", result)
	}
}

func TestExecuteMCPRequest_ToolsCall(t *testing.T) {
	t.Parallel()
	cfg := startHTTPMCPServer(t)
	result, err := agui.ExecuteMCPRequest(context.Background(), cfg, "tools/call", map[string]any{
		"name":      "echo",
		"arguments": map[string]any{"message": "test"},
	})
	if err != nil {
		t.Fatalf("ExecuteMCPRequest tools/call: %v", err)
	}
	callResult, ok := result.(*mcp.CallToolResult)
	if !ok {
		t.Fatalf("result type = %T, want *mcp.CallToolResult", result)
	}
	text := agui.ExtractTextContent(callResult)
	if text != "test" {
		t.Errorf("got %q, want %q", text, "test")
	}
}

func TestExecuteMCPRequest_ResourcesRead(t *testing.T) {
	t.Parallel()
	cfg := startHTTPMCPServer(t)
	_, err := agui.ExecuteMCPRequest(context.Background(), cfg, "resources/read", map[string]any{
		"uri": "test://resource",
	})
	if err == nil {
		t.Fatal("got nil error, want error for resources/read on server without resources")
	}
}

func TestExecuteMCPRequest_UnknownMethod(t *testing.T) {
	t.Parallel()
	cfg := startHTTPMCPServer(t)
	_, err := agui.ExecuteMCPRequest(context.Background(), cfg, "unknown/method", nil)
	if err == nil {
		t.Fatal("got nil error, want error for unknown method")
	}
}

func TestExecuteMCPRequest_NotificationsMessage(t *testing.T) {
	t.Parallel()
	cfg := startHTTPMCPServer(t)
	_, err := agui.ExecuteMCPRequest(context.Background(), cfg, "notifications/message", nil)
	if err == nil {
		t.Fatal("got nil error, want error for notifications/message")
	}
}

// FuzzExtractTextContent verifies that ExtractTextContent never panics and
// returns a string for any text content input.
func FuzzExtractTextContent(f *testing.F) {
	f.Add("hello world")
	f.Add("")
	f.Add("unicode-段")

	f.Fuzz(func(t *testing.T, text string) {
		result := &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}
		got := agui.ExtractTextContent(result)
		// When the content is a single text block, the output must equal the input text.
		if got != text {
			t.Errorf("ExtractTextContent with single text %q = %q, want %q", text, got, text)
		}
	})
}
