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
	"fmt"
	"sync"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"
)

// Compile-time interface checks.
var _ tool.Tool = (*FakeTool)(nil)
var _ tool.Context = (*FakeToolContext)(nil)
var _ tool.Toolset = (*FakeToolset)(nil)

// ---------------------------------------------------------------------------
// FakeTool
// ---------------------------------------------------------------------------

// FakeTool implements tool.Tool for testing. It also optionally implements
// ProcessRequest and Run if configured.
//
// Thread-safe.
type FakeTool struct {
	mu               sync.RWMutex
	nameVal          string
	descriptionVal   string
	isLongRunning    bool
	declarationVal   *genai.FunctionDeclaration
	runFn            func(tool.Context, map[string]any) (any, error)
	processRequestFn func(tool.Context, *model.LLMRequest) error
	callCount        int
	lastArgs         map[string]any
	lastCtx          tool.Context
}

// NewFakeTool creates a FakeTool with the given name.
func NewFakeTool(name string) *FakeTool {
	return &FakeTool{
		nameVal: name,
	}
}

// WithDescription sets the description (builder pattern).
func (f *FakeTool) WithDescription(desc string) *FakeTool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.descriptionVal = desc
	return f
}

// WithIsLongRunning sets the long-running flag (builder pattern).
func (f *FakeTool) WithIsLongRunning(v bool) *FakeTool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.isLongRunning = v
	return f
}

// WithDeclaration sets the function declaration (builder pattern).
func (f *FakeTool) WithDeclaration(decl *genai.FunctionDeclaration) *FakeTool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.declarationVal = decl
	return f
}

// WithRunFunc configures the Run behavior (builder pattern).
// The function receives the tool context and the deserialized args map,
// and returns the result and an error.
func (f *FakeTool) WithRunFunc(fn func(tool.Context, map[string]any) (any, error)) *FakeTool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runFn = fn
	return f
}

// WithProcessRequestFunc configures the ProcessRequest behavior (builder
// pattern).
func (f *FakeTool) WithProcessRequestFunc(fn func(tool.Context, *model.LLMRequest) error) *FakeTool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.processRequestFn = fn
	return f
}

// Name implements tool.Tool.
func (f *FakeTool) Name() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.nameVal
}

// Description implements tool.Tool.
func (f *FakeTool) Description() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.descriptionVal
}

// IsLongRunning implements tool.Tool.
func (f *FakeTool) IsLongRunning() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.isLongRunning
}

// ProcessRequest implements the request processor interface.
// If no ProcessRequestFunc is set, it packs the tool declaration into the
// request if a declaration is configured. Otherwise it is a no-op.
func (f *FakeTool) ProcessRequest(ctx tool.Context, req *model.LLMRequest) error {
	f.mu.RLock()
	fn := f.processRequestFn
	decl := f.declarationVal
	f.mu.RUnlock()

	if fn != nil {
		return fn(ctx, req)
	}

	if decl != nil {
		if req.Tools == nil {
			req.Tools = make(map[string]any)
		}
		req.Tools[f.Name()] = f
		if req.Config == nil {
			req.Config = &genai.GenerateContentConfig{}
		}
		var funcTool *genai.Tool
		for _, t := range req.Config.Tools {
			if t != nil && t.FunctionDeclarations != nil {
				funcTool = t
				break
			}
		}
		if funcTool == nil {
			req.Config.Tools = append(req.Config.Tools, &genai.Tool{
				FunctionDeclarations: []*genai.FunctionDeclaration{decl},
			})
		} else {
			funcTool.FunctionDeclarations = append(funcTool.FunctionDeclarations, decl)
		}
	}
	return nil
}

// Declaration returns the function declaration.
func (f *FakeTool) Declaration() *genai.FunctionDeclaration {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.declarationVal
}

// Run executes the tool. If no RunFunc is set, it returns an empty result.
func (f *FakeTool) Run(ctx tool.Context, args any) (map[string]any, error) {
	f.mu.Lock()
	f.callCount++
	margs, _ := args.(map[string]any)
	f.lastArgs = margs
	f.lastCtx = ctx
	fn := f.runFn
	f.mu.Unlock()

	if fn == nil {
		return map[string]any{}, nil
	}

	result, err := fn(ctx, margs)
	if err != nil {
		return nil, err
	}
	if m, ok := result.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{"result": result}, nil
}

// CallCount returns how many times Run was invoked.
func (f *FakeTool) CallCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.callCount
}

// LastArgs returns the args from the most recent Run call.
func (f *FakeTool) LastArgs() map[string]any {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.lastArgs
}

// LastCtx returns the tool.Context from the most recent Run call.
func (f *FakeTool) LastCtx() tool.Context {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.lastCtx
}

// Reset clears call tracking.
func (f *FakeTool) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount = 0
	f.lastArgs = nil
	f.lastCtx = nil
}

// ---------------------------------------------------------------------------
// FakeToolContext
// ---------------------------------------------------------------------------

// FakeToolContext implements tool.Context for testing.
type FakeToolContext struct {
	agent.CallbackContext // embedded for base context methods
	functionCallIDVal     string
	actionsVal            *session.EventActions
	memorySvc             memory.Service
	memoryUserID          string
	memoryAppName         string
	toolConf              *toolconfirmation.ToolConfirmation
}

// NewFakeToolContext creates a FakeToolContext wrapping the given
// CallbackContext.
func NewFakeToolContext(cbCtx agent.CallbackContext) *FakeToolContext {
	return &FakeToolContext{
		CallbackContext:   cbCtx,
		functionCallIDVal: "test-fc-id",
		actionsVal:        &session.EventActions{StateDelta: make(map[string]any), ArtifactDelta: make(map[string]int64)},
	}
}

// WithFunctionCallID sets the function call ID.
func (f *FakeToolContext) WithFunctionCallID(id string) *FakeToolContext {
	f.functionCallIDVal = id
	return f
}

// WithActions sets the event actions.
func (f *FakeToolContext) WithActions(actions *session.EventActions) *FakeToolContext {
	f.actionsVal = actions
	return f
}

// WithMemoryService sets the memory service for SearchMemory.
func (f *FakeToolContext) WithMemoryService(svc memory.Service, userID, appName string) *FakeToolContext {
	f.memorySvc = svc
	f.memoryUserID = userID
	f.memoryAppName = appName
	return f
}

// WithToolConfirmation sets the tool confirmation state.
func (f *FakeToolContext) WithToolConfirmation(tc *toolconfirmation.ToolConfirmation) *FakeToolContext {
	f.toolConf = tc
	return f
}

// FunctionCallID implements tool.Context.
func (f *FakeToolContext) FunctionCallID() string { return f.functionCallIDVal }

// Actions implements tool.Context.
func (f *FakeToolContext) Actions() *session.EventActions { return f.actionsVal }

// SearchMemory implements tool.Context.
func (f *FakeToolContext) SearchMemory(ctx context.Context, query string) (*memory.SearchResponse, error) {
	if f.memorySvc == nil {
		return nil, fmt.Errorf("memory service is not set")
	}
	return f.memorySvc.SearchMemory(ctx, &memory.SearchRequest{
		Query:   query,
		UserID:  f.memoryUserID,
		AppName: f.memoryAppName,
	})
}

// ToolConfirmation implements tool.Context.
func (f *FakeToolContext) ToolConfirmation() *toolconfirmation.ToolConfirmation {
	return f.toolConf
}

// RequestConfirmation implements tool.Context.
func (f *FakeToolContext) RequestConfirmation(hint string, payload any) error {
	if f.actionsVal == nil {
		f.actionsVal = &session.EventActions{}
	}
	if f.actionsVal.RequestedToolConfirmations == nil {
		f.actionsVal.RequestedToolConfirmations = make(map[string]toolconfirmation.ToolConfirmation)
	}
	f.actionsVal.RequestedToolConfirmations[f.functionCallIDVal] = toolconfirmation.ToolConfirmation{
		Hint:      hint,
		Confirmed: false,
		Payload:   payload,
	}
	f.actionsVal.SkipSummarization = true
	return nil
}

// ---------------------------------------------------------------------------
// FakeToolset
// ---------------------------------------------------------------------------

// FakeToolset implements tool.Toolset for testing.
type FakeToolset struct {
	nameVal  string
	toolsVal []tool.Tool
	errVal   error
}

// NewFakeToolset creates a FakeToolset with the given tools.
func NewFakeToolset(name string, tools ...tool.Tool) *FakeToolset {
	return &FakeToolset{
		nameVal:  name,
		toolsVal: tools,
	}
}

// WithError configures the toolset to return an error.
func (f *FakeToolset) WithError(err error) *FakeToolset {
	f.errVal = err
	return f
}

// Name implements tool.Toolset.
func (f *FakeToolset) Name() string { return f.nameVal }

// Tools implements tool.Toolset.
func (f *FakeToolset) Tools(_ agent.ReadonlyContext) ([]tool.Tool, error) {
	if f.errVal != nil {
		return nil, f.errVal
	}
	return f.toolsVal, nil
}
