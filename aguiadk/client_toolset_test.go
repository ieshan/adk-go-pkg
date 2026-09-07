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
		t.Error("got empty name, want non-empty")
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
		t.Errorf("got %d tools, want 0", len(tools))
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
		t.Fatalf("got %d tools, want 2", len(tools))
	}
	if tools[0].Name() != "search" {
		t.Errorf("tool 0: got %q, want %q", tools[0].Name(), "search")
	}
	if tools[1].Name() != "navigate" {
		t.Errorf("tool 1: got %q, want %q", tools[1].Name(), "navigate")
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
	if len(tools) == 0 {
		t.Fatal("no tools returned")
	}
	if !tools[0].IsLongRunning() {
		t.Error("got NextRun tool not long-running, want long-running")
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
	if len(tools) == 0 {
		t.Fatal("no tools returned")
	}
	if tools[0].IsLongRunning() {
		t.Error("got Inline tool long-running, want NOT long-running")
	}
}

// Compile-time check that ClientToolset implements tool.Toolset.
var _ tool.Toolset = (*aguiadk.ClientToolset)(nil)
