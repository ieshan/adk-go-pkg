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
	"iter"
	"sync"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/artifact"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// Compile-time interface checks.
var _ agent.InvocationContext = (*FakeInvocationContext)(nil)
var _ agent.CallbackContext = (*FakeCallbackContext)(nil)
var _ agent.ReadonlyContext = (*FakeReadonlyContext)(nil)
var _ agent.Artifacts = (*FakeArtifacts)(nil)
var _ agent.Memory = (*FakeMemory)(nil)

// ---------------------------------------------------------------------------
// FakeAgent
// ---------------------------------------------------------------------------

// FakeAgent wraps an agent.Agent created via agent.New for testing.
// Since agent.Agent requires an unexported internal() method, we cannot
// implement it directly from outside the agent package. Instead, we use
// agent.New with a configurable Run function and embed the resulting
// agent.Agent for direct use.
//
// FakeAgent tracks calls to Run for test assertions.
type FakeAgent struct {
	agent.Agent // the underlying real agent created via agent.New
	mu          sync.RWMutex
	callCount   int
	lastCtx     agent.InvocationContext
	runFn       func(agent.InvocationContext) iter.Seq2[*session.Event, error]
}

// NewFakeAgent creates a FakeAgent with a name and a default no-op Run that
// yields no events.
func NewFakeAgent(name string) *FakeAgent {
	f := &FakeAgent{}
	f.runFn = func(ic agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {}
	}

	ag, err := agent.New(agent.Config{
		Name: name,
		Run:  f.trackedRun,
	})
	if err != nil {
		panic("testutil: NewFakeAgent: " + err.Error())
	}
	f.Agent = ag
	return f
}

// WithDescription sets the description (builder pattern).
// This must be called before the agent is used; it creates a new underlying
// agent with the description set.
func (f *FakeAgent) WithDescription(desc string) *FakeAgent {
	ag, err := agent.New(agent.Config{
		Name:        f.Agent.Name(),
		Description: desc,
		SubAgents:   f.Agent.SubAgents(),
		Run:         f.trackedRun,
	})
	if err != nil {
		panic("testutil: WithDescription: " + err.Error())
	}
	f.Agent = ag
	return f
}

// WithSubAgents adds child agents (builder pattern).
// This creates a new underlying agent with the sub-agents set.
func (f *FakeAgent) WithSubAgents(agents ...agent.Agent) *FakeAgent {
	ag, err := agent.New(agent.Config{
		Name:        f.Agent.Name(),
		Description: f.Agent.Description(),
		SubAgents:   agents,
		Run:         f.trackedRun,
	})
	if err != nil {
		panic("testutil: WithSubAgents: " + err.Error())
	}
	f.Agent = ag
	return f
}

// WithRunFunc configures the Run behavior (builder pattern).
func (f *FakeAgent) WithRunFunc(fn func(agent.InvocationContext) iter.Seq2[*session.Event, error]) *FakeAgent {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runFn = fn
	return f
}

// trackedRun is the Run function passed to agent.New. It records the call
// and delegates to the configured runFn.
func (f *FakeAgent) trackedRun(ic agent.InvocationContext) iter.Seq2[*session.Event, error] {
	f.mu.Lock()
	f.callCount++
	f.lastCtx = ic
	runFn := f.runFn
	f.mu.Unlock()
	return runFn(ic)
}

// CallCount returns how many times Run was invoked.
func (f *FakeAgent) CallCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.callCount
}

// LastContext returns the last InvocationContext passed to Run.
func (f *FakeAgent) LastContext() agent.InvocationContext {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.lastCtx
}

// Reset clears call tracking.
func (f *FakeAgent) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount = 0
	f.lastCtx = nil
}

// ---------------------------------------------------------------------------
// FakeInvocationContext
// ---------------------------------------------------------------------------

// FakeInvocationContext implements agent.InvocationContext for testing.
type FakeInvocationContext struct {
	context.Context
	agentVal       agent.Agent
	artifactsVal   agent.Artifacts
	memoryVal      agent.Memory
	sessionVal     session.Session
	invIDVal       string
	branchVal      string
	userContentVal *genai.Content
	runConfigVal   *agent.RunConfig
	ended          bool
}

// NewFakeInvocationContext creates a FakeInvocationContext with
// context.Background() and sensible defaults.
func NewFakeInvocationContext() *FakeInvocationContext {
	return &FakeInvocationContext{
		Context:      context.Background(),
		sessionVal:   NewFakeSession(),
		invIDVal:     "test-invocation",
		runConfigVal: &agent.RunConfig{},
	}
}

// WithAgent sets the agent.
func (f *FakeInvocationContext) WithAgent(a agent.Agent) *FakeInvocationContext {
	f.agentVal = a
	return f
}

// WithArtifacts sets the artifacts.
func (f *FakeInvocationContext) WithArtifacts(art agent.Artifacts) *FakeInvocationContext {
	f.artifactsVal = art
	return f
}

// WithMemory sets the memory.
func (f *FakeInvocationContext) WithMemory(m agent.Memory) *FakeInvocationContext {
	f.memoryVal = m
	return f
}

// WithSession sets the session.
func (f *FakeInvocationContext) WithSession(s session.Session) *FakeInvocationContext {
	f.sessionVal = s
	return f
}

// WithInvocationID sets the invocation ID.
func (f *FakeInvocationContext) WithInvocationID(id string) *FakeInvocationContext {
	f.invIDVal = id
	return f
}

// WithBranch sets the branch.
func (f *FakeInvocationContext) WithBranch(b string) *FakeInvocationContext {
	f.branchVal = b
	return f
}

// WithUserContent sets the user content.
func (f *FakeInvocationContext) WithUserContent(c *genai.Content) *FakeInvocationContext {
	f.userContentVal = c
	return f
}

// WithRunConfig sets the run config.
func (f *FakeInvocationContext) WithRunConfig(rc *agent.RunConfig) *FakeInvocationContext {
	f.runConfigVal = rc
	return f
}

// Agent implements agent.InvocationContext.
func (f *FakeInvocationContext) Agent() agent.Agent { return f.agentVal }

// Artifacts implements agent.InvocationContext.
func (f *FakeInvocationContext) Artifacts() agent.Artifacts { return f.artifactsVal }

// Memory implements agent.InvocationContext.
func (f *FakeInvocationContext) Memory() agent.Memory { return f.memoryVal }

// Session implements agent.InvocationContext.
func (f *FakeInvocationContext) Session() session.Session { return f.sessionVal }

// InvocationID implements agent.InvocationContext.
func (f *FakeInvocationContext) InvocationID() string { return f.invIDVal }

// Branch implements agent.InvocationContext.
func (f *FakeInvocationContext) Branch() string { return f.branchVal }

// UserContent implements agent.InvocationContext.
func (f *FakeInvocationContext) UserContent() *genai.Content { return f.userContentVal }

// RunConfig implements agent.InvocationContext.
func (f *FakeInvocationContext) RunConfig() *agent.RunConfig { return f.runConfigVal }

// EndInvocation implements agent.InvocationContext.
func (f *FakeInvocationContext) EndInvocation() { f.ended = true }

// Ended implements agent.InvocationContext.
func (f *FakeInvocationContext) Ended() bool { return f.ended }

// WithContext implements agent.InvocationContext.
func (f *FakeInvocationContext) WithContext(ctx context.Context) agent.InvocationContext {
	cp := *f
	cp.Context = ctx
	return &cp
}

// ---------------------------------------------------------------------------
// FakeCallbackContext
// ---------------------------------------------------------------------------

// FakeCallbackContext implements agent.CallbackContext for testing.
type FakeCallbackContext struct {
	context.Context
	agentNameVal     string
	userContentVal   *genai.Content
	invIDVal         string
	readonlyStateVal session.ReadonlyState
	stateVal         session.State
	artifactsVal     agent.Artifacts
	userIDVal        string
	appNameVal       string
	sessionIDVal     string
	branchVal        string
}

// NewFakeCallbackContext creates a FakeCallbackContext with sensible defaults.
func NewFakeCallbackContext() *FakeCallbackContext {
	return &FakeCallbackContext{
		Context:          context.Background(),
		agentNameVal:     "test-agent",
		invIDVal:         "test-invocation",
		readonlyStateVal: NewFakeState(),
		stateVal:         NewFakeState(),
		userIDVal:        "test-user",
		appNameVal:       "test-app",
		sessionIDVal:     "test-session",
	}
}

// WithAgentName sets the agent name.
func (f *FakeCallbackContext) WithAgentName(name string) *FakeCallbackContext {
	f.agentNameVal = name
	return f
}

// WithUserContent sets the user content.
func (f *FakeCallbackContext) WithUserContent(c *genai.Content) *FakeCallbackContext {
	f.userContentVal = c
	return f
}

// WithInvocationID sets the invocation ID.
func (f *FakeCallbackContext) WithInvocationID(id string) *FakeCallbackContext {
	f.invIDVal = id
	return f
}

// WithReadonlyState sets the readonly state.
func (f *FakeCallbackContext) WithReadonlyState(s session.ReadonlyState) *FakeCallbackContext {
	f.readonlyStateVal = s
	return f
}

// WithState sets the mutable state.
func (f *FakeCallbackContext) WithState(s session.State) *FakeCallbackContext {
	f.stateVal = s
	return f
}

// WithArtifacts sets the artifacts.
func (f *FakeCallbackContext) WithArtifacts(art agent.Artifacts) *FakeCallbackContext {
	f.artifactsVal = art
	return f
}

// WithUserID sets the user ID.
func (f *FakeCallbackContext) WithUserID(id string) *FakeCallbackContext {
	f.userIDVal = id
	return f
}

// WithAppName sets the app name.
func (f *FakeCallbackContext) WithAppName(name string) *FakeCallbackContext {
	f.appNameVal = name
	return f
}

// WithSessionID sets the session ID.
func (f *FakeCallbackContext) WithSessionID(id string) *FakeCallbackContext {
	f.sessionIDVal = id
	return f
}

// WithBranch sets the branch.
func (f *FakeCallbackContext) WithBranch(b string) *FakeCallbackContext {
	f.branchVal = b
	return f
}

// UserContent implements agent.ReadonlyContext.
func (f *FakeCallbackContext) UserContent() *genai.Content { return f.userContentVal }

// InvocationID implements agent.ReadonlyContext.
func (f *FakeCallbackContext) InvocationID() string { return f.invIDVal }

// AgentName implements agent.ReadonlyContext.
func (f *FakeCallbackContext) AgentName() string { return f.agentNameVal }

// ReadonlyState implements agent.ReadonlyContext.
func (f *FakeCallbackContext) ReadonlyState() session.ReadonlyState {
	return f.readonlyStateVal
}

// UserID implements agent.ReadonlyContext.
func (f *FakeCallbackContext) UserID() string { return f.userIDVal }

// AppName implements agent.ReadonlyContext.
func (f *FakeCallbackContext) AppName() string { return f.appNameVal }

// SessionID implements agent.ReadonlyContext.
func (f *FakeCallbackContext) SessionID() string { return f.sessionIDVal }

// Branch implements agent.ReadonlyContext.
func (f *FakeCallbackContext) Branch() string { return f.branchVal }

// Artifacts implements agent.CallbackContext.
func (f *FakeCallbackContext) Artifacts() agent.Artifacts { return f.artifactsVal }

// State implements agent.CallbackContext.
func (f *FakeCallbackContext) State() session.State { return f.stateVal }

// ---------------------------------------------------------------------------
// FakeReadonlyContext
// ---------------------------------------------------------------------------

// FakeReadonlyContext implements agent.ReadonlyContext for testing.
type FakeReadonlyContext struct {
	context.Context
	agentNameVal     string
	userContentVal   *genai.Content
	invIDVal         string
	readonlyStateVal session.ReadonlyState
	userIDVal        string
	appNameVal       string
	sessionIDVal     string
	branchVal        string
}

// NewFakeReadonlyContext creates a FakeReadonlyContext with sensible defaults.
func NewFakeReadonlyContext() *FakeReadonlyContext {
	return &FakeReadonlyContext{
		Context:          context.Background(),
		agentNameVal:     "test-agent",
		invIDVal:         "test-invocation",
		readonlyStateVal: NewFakeState(),
		userIDVal:        "test-user",
		appNameVal:       "test-app",
		sessionIDVal:     "test-session",
	}
}

// WithAgentName sets the agent name.
func (f *FakeReadonlyContext) WithAgentName(name string) *FakeReadonlyContext {
	f.agentNameVal = name
	return f
}

// WithUserContent sets the user content.
func (f *FakeReadonlyContext) WithUserContent(c *genai.Content) *FakeReadonlyContext {
	f.userContentVal = c
	return f
}

// WithInvocationID sets the invocation ID.
func (f *FakeReadonlyContext) WithInvocationID(id string) *FakeReadonlyContext {
	f.invIDVal = id
	return f
}

// WithReadonlyState sets the readonly state.
func (f *FakeReadonlyContext) WithReadonlyState(s session.ReadonlyState) *FakeReadonlyContext {
	f.readonlyStateVal = s
	return f
}

// WithUserID sets the user ID.
func (f *FakeReadonlyContext) WithUserID(id string) *FakeReadonlyContext {
	f.userIDVal = id
	return f
}

// WithAppName sets the app name.
func (f *FakeReadonlyContext) WithAppName(name string) *FakeReadonlyContext {
	f.appNameVal = name
	return f
}

// WithSessionID sets the session ID.
func (f *FakeReadonlyContext) WithSessionID(id string) *FakeReadonlyContext {
	f.sessionIDVal = id
	return f
}

// WithBranch sets the branch.
func (f *FakeReadonlyContext) WithBranch(b string) *FakeReadonlyContext {
	f.branchVal = b
	return f
}

// UserContent implements agent.ReadonlyContext.
func (f *FakeReadonlyContext) UserContent() *genai.Content { return f.userContentVal }

// InvocationID implements agent.ReadonlyContext.
func (f *FakeReadonlyContext) InvocationID() string { return f.invIDVal }

// AgentName implements agent.ReadonlyContext.
func (f *FakeReadonlyContext) AgentName() string { return f.agentNameVal }

// ReadonlyState implements agent.ReadonlyContext.
func (f *FakeReadonlyContext) ReadonlyState() session.ReadonlyState {
	return f.readonlyStateVal
}

// UserID implements agent.ReadonlyContext.
func (f *FakeReadonlyContext) UserID() string { return f.userIDVal }

// AppName implements agent.ReadonlyContext.
func (f *FakeReadonlyContext) AppName() string { return f.appNameVal }

// SessionID implements agent.ReadonlyContext.
func (f *FakeReadonlyContext) SessionID() string { return f.sessionIDVal }

// Branch implements agent.ReadonlyContext.
func (f *FakeReadonlyContext) Branch() string { return f.branchVal }

// ---------------------------------------------------------------------------
// FakeArtifacts
// ---------------------------------------------------------------------------

// FakeArtifacts implements agent.Artifacts by delegating to an
// artifact.Service.
type FakeArtifacts struct {
	svc       artifact.Service
	appName   string
	userID    string
	sessionID string
}

// NewFakeArtifacts creates a FakeArtifacts that delegates to the given
// artifact.Service.
func NewFakeArtifacts(svc artifact.Service, appName, userID, sessionID string) *FakeArtifacts {
	return &FakeArtifacts{
		svc:       svc,
		appName:   appName,
		userID:    userID,
		sessionID: sessionID,
	}
}

// Save implements agent.Artifacts.
func (f *FakeArtifacts) Save(ctx context.Context, name string, data *genai.Part) (*artifact.SaveResponse, error) {
	return f.svc.Save(ctx, &artifact.SaveRequest{
		AppName:   f.appName,
		UserID:    f.userID,
		SessionID: f.sessionID,
		FileName:  name,
		Part:      data,
	})
}

// List implements agent.Artifacts.
func (f *FakeArtifacts) List(ctx context.Context) (*artifact.ListResponse, error) {
	return f.svc.List(ctx, &artifact.ListRequest{
		AppName:   f.appName,
		UserID:    f.userID,
		SessionID: f.sessionID,
	})
}

// Load implements agent.Artifacts.
func (f *FakeArtifacts) Load(ctx context.Context, name string) (*artifact.LoadResponse, error) {
	return f.svc.Load(ctx, &artifact.LoadRequest{
		AppName:   f.appName,
		UserID:    f.userID,
		SessionID: f.sessionID,
		FileName:  name,
	})
}

// LoadVersion implements agent.Artifacts.
func (f *FakeArtifacts) LoadVersion(ctx context.Context, name string, version int) (*artifact.LoadResponse, error) {
	return f.svc.Load(ctx, &artifact.LoadRequest{
		AppName:   f.appName,
		UserID:    f.userID,
		SessionID: f.sessionID,
		FileName:  name,
		Version:   int64(version),
	})
}

// ---------------------------------------------------------------------------
// FakeMemory
// ---------------------------------------------------------------------------

// FakeMemory implements agent.Memory by delegating to a memory.Service.
type FakeMemory struct {
	svc     memory.Service
	userID  string
	appName string
}

// NewFakeMemory creates a FakeMemory that delegates to the given
// memory.Service.
func NewFakeMemory(svc memory.Service, userID, appName string) *FakeMemory {
	return &FakeMemory{
		svc:     svc,
		userID:  userID,
		appName: appName,
	}
}

// AddSessionToMemory implements agent.Memory.
func (f *FakeMemory) AddSessionToMemory(ctx context.Context, s session.Session) error {
	return f.svc.AddSessionToMemory(ctx, s)
}

// SearchMemory implements agent.Memory.
func (f *FakeMemory) SearchMemory(ctx context.Context, query string) (*memory.SearchResponse, error) {
	return f.svc.SearchMemory(ctx, &memory.SearchRequest{
		Query:   query,
		UserID:  f.userID,
		AppName: f.appName,
	})
}
