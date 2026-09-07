package testutil_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

var errToolFailed = errors.New("tool failed")
var errToolsetBroken = errors.New("broken")

func TestFakeTool_Basic(t *testing.T) {
	ft := testutil.NewFakeTool("my_tool").
		WithDescription("A test tool").
		WithIsLongRunning(true)

	if ft.Name() != "my_tool" {
		t.Errorf("Name() = %q, want %q", ft.Name(), "my_tool")
	}
	if ft.Description() != "A test tool" {
		t.Errorf("Description() = %q, want %q", ft.Description(), "A test tool")
	}
	if !ft.IsLongRunning() {
		t.Error("IsLongRunning() should be true")
	}
}

func TestFakeTool_Run(t *testing.T) {
	ft := testutil.NewFakeTool("adder").
		WithRunFunc(func(ctx agent.Context, args map[string]any) (any, error) {
			a, _ := args["a"].(float64)
			b, _ := args["b"].(float64)
			return map[string]any{"sum": a + b}, nil
		})

	cbCtx := testutil.NewFakeCallbackContext()
	tc := testutil.NewFakeToolContext(cbCtx)

	result, err := ft.Run(tc, map[string]any{"a": 3.0, "b": 4.0})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result["sum"] != 7.0 {
		t.Errorf("Run() result[sum] = %v, want 7.0", result["sum"])
	}

	if ft.CallCount() != 1 {
		t.Errorf("CallCount() = %d, want 1", ft.CallCount())
	}
	if ft.LastArgs()["a"] != 3.0 {
		t.Errorf("LastArgs()[a] = %v, want 3.0", ft.LastArgs()["a"])
	}
}

func TestFakeTool_RunError(t *testing.T) {
	ft := testutil.NewFakeTool("failer").
		WithRunFunc(func(ctx agent.Context, args map[string]any) (any, error) {
			return nil, errToolFailed
		})

	cbCtx := testutil.NewFakeCallbackContext()
	tc := testutil.NewFakeToolContext(cbCtx)

	_, err := ft.Run(tc, map[string]any{})
	if !errors.Is(err, errToolFailed) {
		t.Errorf("Run() error = %v, want errToolFailed", err)
	}
}

func TestFakeTool_RunDefault(t *testing.T) {
	ft := testutil.NewFakeTool("noop")
	cbCtx := testutil.NewFakeCallbackContext()
	tc := testutil.NewFakeToolContext(cbCtx)

	result, err := ft.Run(tc, map[string]any{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(result) != 0 {
		t.Errorf("got %v, want empty map", result)
	}
}

func TestFakeTool_ProcessRequest(t *testing.T) {
	ft := testutil.NewFakeTool("search").
		WithDeclaration(&genai.FunctionDeclaration{
			Name:        "search",
			Description: "Search for things",
		})

	cbCtx := testutil.NewFakeCallbackContext()
	tc := testutil.NewFakeToolContext(cbCtx)
	req := &model.LLMRequest{}

	err := ft.ProcessRequest(tc, req)
	if err != nil {
		t.Fatalf("ProcessRequest() error = %v", err)
	}
	if len(req.Config.Tools) == 0 {
		t.Error("ProcessRequest() should add tools to request")
	}
}

func TestFakeTool_Reset(t *testing.T) {
	ft := testutil.NewFakeTool("reset")
	cbCtx := testutil.NewFakeCallbackContext()
	tc := testutil.NewFakeToolContext(cbCtx)
	// error intentionally ignored: exercising Reset path, not validating Run output
	_, _ = ft.Run(tc, map[string]any{})

	ft.Reset()
	if ft.CallCount() != 0 {
		t.Errorf("after Reset, CallCount() = %d, want 0", ft.CallCount())
	}
}

func TestFakeToolContext(t *testing.T) {
	cbCtx := testutil.NewFakeCallbackContext().
		WithUserID("u-1").
		WithAppName("app-1")
	tc := testutil.NewFakeToolContext(cbCtx).
		WithFunctionCallID("fc-123")

	if tc.FunctionCallID() != "fc-123" {
		t.Errorf("FunctionCallID() = %q, want %q", tc.FunctionCallID(), "fc-123")
	}
	if tc.Actions() == nil {
		t.Error("Actions() should not be nil")
	}
}

func TestFakeToolContext_RequestConfirmation(t *testing.T) {
	cbCtx := testutil.NewFakeCallbackContext()
	tc := testutil.NewFakeToolContext(cbCtx)

	err := tc.RequestConfirmation("Please approve", nil)
	if err != nil {
		t.Fatalf("RequestConfirmation() error = %v", err)
	}

	actions := tc.Actions()
	if !actions.SkipSummarization {
		t.Error("SkipSummarization should be true after RequestConfirmation")
	}
	if len(actions.RequestedToolConfirmations) == 0 {
		t.Error("RequestedToolConfirmations should have entries")
	}
}

func TestFakeToolContext_SearchMemory(t *testing.T) {
	memSvc := testutil.NewFakeMemoryService()
	memSvc.PreloadMemory("u-1", "app-1", testutil.NewMemoryEntry("m1", "hello world", "model"))

	cbCtx := testutil.NewFakeCallbackContext()
	tc := testutil.NewFakeToolContext(cbCtx).
		WithMemoryService(memSvc, "u-1", "app-1")

	resp, err := tc.SearchMemory(context.Background(), "hello")
	if err != nil {
		t.Fatalf("SearchMemory() error = %v", err)
	}
	if len(resp.Memories) == 0 {
		t.Error("SearchMemory() should return preloaded entries")
	}
}

func TestFakeToolContext_SearchMemoryNotSet(t *testing.T) {
	cbCtx := testutil.NewFakeCallbackContext()
	tc := testutil.NewFakeToolContext(cbCtx)

	_, err := tc.SearchMemory(context.Background(), "hello")
	if err == nil {
		t.Error("SearchMemory() without service should return error")
	}
}

func TestFakeToolset(t *testing.T) {
	t1 := testutil.NewFakeTool("tool1")
	t2 := testutil.NewFakeTool("tool2")
	ts := testutil.NewFakeToolset("my-set", t1, t2)

	if ts.Name() != "my-set" {
		t.Errorf("Name() = %q, want %q", ts.Name(), "my-set")
	}

	rc := testutil.NewFakeReadonlyContext()
	tools, err := ts.Tools(rc)
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	if len(tools) != 2 {
		t.Errorf("Tools() returned %d tools, want 2", len(tools))
	}
}

func TestFakeToolset_Error(t *testing.T) {
	ts := testutil.NewFakeToolset("err-set").WithError(errToolsetBroken)
	rc := testutil.NewFakeReadonlyContext()
	_, err := ts.Tools(rc)
	if !errors.Is(err, errToolsetBroken) {
		t.Errorf("Tools() error = %v, want errToolsetBroken", err)
	}
}
