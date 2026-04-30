// Copyright 2025 ieshan
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package testutil

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

func TestFakeTool_Basic(t *testing.T) {
	ft := NewFakeTool("my_tool").
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
	ft := NewFakeTool("adder").
		WithRunFunc(func(ctx tool.Context, args map[string]any) (any, error) {
			a, _ := args["a"].(float64)
			b, _ := args["b"].(float64)
			return map[string]any{"sum": a + b}, nil
		})

	cbCtx := NewFakeCallbackContext()
	tc := NewFakeToolContext(cbCtx)

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
	ft := NewFakeTool("failer").
		WithRunFunc(func(ctx tool.Context, args map[string]any) (any, error) {
			return nil, errors.New("tool failed")
		})

	cbCtx := NewFakeCallbackContext()
	tc := NewFakeToolContext(cbCtx)

	_, err := ft.Run(tc, map[string]any{})
	if err == nil || err.Error() != "tool failed" {
		t.Errorf("Run() error = %v, want 'tool failed'", err)
	}
}

func TestFakeTool_RunDefault(t *testing.T) {
	ft := NewFakeTool("noop")
	cbCtx := NewFakeCallbackContext()
	tc := NewFakeToolContext(cbCtx)

	result, err := ft.Run(tc, map[string]any{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(result) != 0 {
		t.Errorf("default Run() should return empty map, got %v", result)
	}
}

func TestFakeTool_ProcessRequest(t *testing.T) {
	ft := NewFakeTool("search").
		WithDeclaration(&genai.FunctionDeclaration{
			Name:        "search",
			Description: "Search for things",
		})

	cbCtx := NewFakeCallbackContext()
	tc := NewFakeToolContext(cbCtx)
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
	ft := NewFakeTool("reset")
	cbCtx := NewFakeCallbackContext()
	tc := NewFakeToolContext(cbCtx)
	_, _ = ft.Run(tc, map[string]any{})

	ft.Reset()
	if ft.CallCount() != 0 {
		t.Errorf("after Reset, CallCount() = %d, want 0", ft.CallCount())
	}
}

func TestFakeToolContext(t *testing.T) {
	cbCtx := NewFakeCallbackContext().
		WithUserID("u-1").
		WithAppName("app-1")
	tc := NewFakeToolContext(cbCtx).
		WithFunctionCallID("fc-123")

	if tc.FunctionCallID() != "fc-123" {
		t.Errorf("FunctionCallID() = %q, want %q", tc.FunctionCallID(), "fc-123")
	}
	if tc.Actions() == nil {
		t.Error("Actions() should not be nil")
	}
}

func TestFakeToolContext_RequestConfirmation(t *testing.T) {
	cbCtx := NewFakeCallbackContext()
	tc := NewFakeToolContext(cbCtx)

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
	memSvc := NewFakeMemoryService()
	memSvc.PreloadMemory("u-1", "app-1", NewMemoryEntry("m1", "hello world", "model"))

	cbCtx := NewFakeCallbackContext()
	tc := NewFakeToolContext(cbCtx).
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
	cbCtx := NewFakeCallbackContext()
	tc := NewFakeToolContext(cbCtx)

	_, err := tc.SearchMemory(context.Background(), "hello")
	if err == nil {
		t.Error("SearchMemory() without service should return error")
	}
}

func TestFakeToolset(t *testing.T) {
	t1 := NewFakeTool("tool1")
	t2 := NewFakeTool("tool2")
	ts := NewFakeToolset("my-set", t1, t2)

	if ts.Name() != "my-set" {
		t.Errorf("Name() = %q, want %q", ts.Name(), "my-set")
	}

	rc := NewFakeReadonlyContext()
	tools, err := ts.Tools(rc)
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	if len(tools) != 2 {
		t.Errorf("Tools() returned %d tools, want 2", len(tools))
	}
}

func TestFakeToolset_Error(t *testing.T) {
	ts := NewFakeToolset("err-set").WithError(errors.New("broken"))
	rc := NewFakeReadonlyContext()
	_, err := ts.Tools(rc)
	if err == nil || err.Error() != "broken" {
		t.Errorf("Tools() error = %v, want 'broken'", err)
	}
}
