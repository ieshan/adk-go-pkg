package aguiadk

import (
	"fmt"

	"github.com/ieshan/adk-go-pkg/agui"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/mcptoolset"
)

// BuildMCPServerToolsets creates ADK mcptoolset.Toolset instances from
// MCP server configs. Add the returned toolsets to llmagent.Config.Toolsets
// at agent construction time so the ADK runner resolves MCP tools natively.
//
// Example:
//
//	toolsets, err := aguiadk.BuildMCPServerToolsets([]agui.MCPClientConfig{
//	    {Type: "http", URL: "https://example.com/mcp", ServerID: "srv1"},
//	})
//	if err != nil { ... }
//	agent := llmagent.New(llmagent.Config{
//	    Name:     "my-agent",
//	    Model:    model,
//	    Toolsets: toolsets,
//	})
func BuildMCPServerToolsets(servers []agui.MCPClientConfig) ([]tool.Toolset, error) {
	toolsets := make([]tool.Toolset, 0, len(servers))
	for _, s := range servers {
		transport, err := agui.BuildMCPTransport(s)
		if err != nil {
			return nil, fmt.Errorf("aguiadk: MCP transport for %q: %w", s.ServerID, err)
		}
		ts, err := mcptoolset.New(mcptoolset.Config{Transport: transport})
		if err != nil {
			return nil, fmt.Errorf("aguiadk: MCP toolset for %q: %w", s.ServerID, err)
		}
		toolsets = append(toolsets, ts)
	}
	return toolsets, nil
}
