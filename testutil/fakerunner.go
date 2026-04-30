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

	"google.golang.org/adk/agent"
	"google.golang.org/adk/artifact"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/model"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// RunnerBuilder constructs a runner.Runner with fake services for testing.
type RunnerBuilder struct {
	appName     string
	ag          agent.Agent
	sessionSvc  session.Service
	artifactSvc artifact.Service
	memorySvc   memory.Service
	autoCreate  bool
}

// NewRunnerBuilder creates a builder with default fake services.
func NewRunnerBuilder() *RunnerBuilder {
	return &RunnerBuilder{
		appName:     "test-app",
		sessionSvc:  NewFakeSessionService(),
		artifactSvc: NewFakeArtifactService(),
		memorySvc:   NewFakeMemoryService(),
		autoCreate:  true,
	}
}

// WithAppName sets the app name. Defaults to "test-app".
func (b *RunnerBuilder) WithAppName(name string) *RunnerBuilder {
	b.appName = name
	return b
}

// WithAgent sets the root agent.
func (b *RunnerBuilder) WithAgent(a agent.Agent) *RunnerBuilder {
	b.ag = a
	return b
}

// WithSessionService sets a custom session service.
func (b *RunnerBuilder) WithSessionService(svc session.Service) *RunnerBuilder {
	b.sessionSvc = svc
	return b
}

// WithArtifactService sets a custom artifact service.
func (b *RunnerBuilder) WithArtifactService(svc artifact.Service) *RunnerBuilder {
	b.artifactSvc = svc
	return b
}

// WithMemoryService sets a custom memory service.
func (b *RunnerBuilder) WithMemoryService(svc memory.Service) *RunnerBuilder {
	b.memorySvc = svc
	return b
}

// WithAutoCreateSession enables or disables auto session creation.
func (b *RunnerBuilder) WithAutoCreateSession(v bool) *RunnerBuilder {
	b.autoCreate = v
	return b
}

// Build constructs the runner.Runner.
func (b *RunnerBuilder) Build() (*runner.Runner, error) {
	if b.ag == nil {
		return nil, fmt.Errorf("testutil: RunnerBuilder requires an agent")
	}
	return runner.New(runner.Config{
		AppName:         b.appName,
		Agent:           b.ag,
		SessionService:  b.sessionSvc,
		ArtifactService: b.artifactSvc,
		MemoryService:   b.memorySvc,
	})
}

// BuildWithFakes constructs the runner and returns it along with the fake
// services for assertion access. If custom (non-fake) services were provided,
// the corresponding fake field will be nil.
func (b *RunnerBuilder) BuildWithFakes() (*runner.Runner, *RunnerFakes, error) {
	r, err := b.Build()
	if err != nil {
		return nil, nil, err
	}

	fakes := &RunnerFakes{}
	if fs, ok := b.sessionSvc.(*FakeSessionService); ok {
		fakes.SessionService = fs
	}
	if fa, ok := b.artifactSvc.(*FakeArtifactService); ok {
		fakes.ArtifactService = fa
	}
	if fm, ok := b.memorySvc.(*FakeMemoryService); ok {
		fakes.MemoryService = fm
	}

	return r, fakes, nil
}

// RunnerFakes provides access to all fake services used by the runner.
type RunnerFakes struct {
	SessionService  *FakeSessionService
	ArtifactService *FakeArtifactService
	MemoryService   *FakeMemoryService
}

// RunAgent is a convenience function that creates a runner, session, and runs
// the agent with a single user message, collecting all events.
//
// This is the simplest way to test an agent end-to-end with a FakeLLM.
func RunAgent(ctx context.Context, ag agent.Agent, llm model.LLM, userMsg string, responses ...model.LLMResponse) ([]*session.Event, error) {
	fakeLLM := llm.(*FakeLLM)
	if fakeLLM == nil {
		return nil, fmt.Errorf("testutil: RunAgent requires a *FakeLLM")
	}

	sessionSvc := NewFakeSessionService()
	artifactSvc := NewFakeArtifactService()
	memorySvc := NewFakeMemoryService()

	r, err := runner.New(runner.Config{
		AppName:         "test-app",
		Agent:           ag,
		SessionService:  sessionSvc,
		ArtifactService: artifactSvc,
		MemoryService:   memorySvc,
	})
	if err != nil {
		return nil, fmt.Errorf("testutil: create runner: %w", err)
	}

	// Create a session.
	sessResp, err := sessionSvc.Create(ctx, &session.CreateRequest{
		AppName: "test-app",
		UserID:  "test-user",
	})
	if err != nil {
		return nil, fmt.Errorf("testutil: create session: %w", err)
	}

	userContent := genai.NewContentFromText(userMsg, genai.RoleUser)
	seq := r.Run(ctx, sessResp.Session.UserID(), sessResp.Session.ID(), userContent, agent.RunConfig{})
	return CollectEvents(seq)
}
