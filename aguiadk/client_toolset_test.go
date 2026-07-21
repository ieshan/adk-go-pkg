package aguiadk_test

import (
	"context"
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"

	"github.com/ieshan/adk-go-pkg/aguiadk"
	"github.com/ieshan/adk-go-pkg/testutil"

	"google.golang.org/adk/v2/tool"
)

func TestClientToolset_Name(t *testing.T) {
	ts := aguiadk.NewClientToolset()
	if ts.Name() == "" {
		t.Error("expected non-empty name")
	}
}

func TestClientToolset_ToolsEmptyContext(t *testing.T) {
	ts := aguiadk.NewClientToolset()
	readonlyCtx := testutil.NewFakeReadonlyContext()

	tools, err := ts.Tools(readonlyCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestClientToolset_ToolsWithClientTools(t *testing.T) {
	ts := aguiadk.NewClientToolset()

	clientTools := []types.Tool{
		{
			Name:        "search",
			Description: "Search the web",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Search query",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "navigate",
			Description: "Navigate to a URL",
		},
	}

	ctx := aguiadk.WithClientToolsContext(context.Background(), clientTools, nil,
		&aguiadk.ClientToolConfig{Mode: aguiadk.ClientToolModeNextRun},
	)
	readonlyCtx := testutil.NewFakeReadonlyContext().WithContext(ctx)

	tools, err := ts.Tools(readonlyCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].Name() != "search" {
		t.Errorf("tool 0: expected name %q, got %q", "search", tools[0].Name())
	}
	if tools[1].Name() != "navigate" {
		t.Errorf("tool 1: expected name %q, got %q", "navigate", tools[1].Name())
	}
}

func TestClientToolset_NextRunIsLongRunning(t *testing.T) {
	ts := aguiadk.NewClientToolset()

	clientTools := []types.Tool{
		{Name: "search", Description: "Search"},
	}

	ctx := aguiadk.WithClientToolsContext(context.Background(), clientTools, nil,
		&aguiadk.ClientToolConfig{Mode: aguiadk.ClientToolModeNextRun},
	)
	readonlyCtx := testutil.NewFakeReadonlyContext().WithContext(ctx)

	tools, err := ts.Tools(readonlyCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tools[0].IsLongRunning() {
		t.Error("expected NextRun tool to be long-running")
	}
}

func TestClientToolset_InlineNotLongRunning(t *testing.T) {
	ts := aguiadk.NewClientToolset()

	clientTools := []types.Tool{
		{Name: "search", Description: "Search"},
	}

	ctx := aguiadk.WithClientToolsContext(context.Background(), clientTools, nil,
		&aguiadk.ClientToolConfig{Mode: aguiadk.ClientToolModeInline},
	)
	readonlyCtx := testutil.NewFakeReadonlyContext().WithContext(ctx)

	tools, err := ts.Tools(readonlyCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tools[0].IsLongRunning() {
		t.Error("expected Inline tool to NOT be long-running")
	}
}

// Compile-time check that ClientToolset implements tool.Toolset.
var _ tool.Toolset = (*aguiadk.ClientToolset)(nil)
