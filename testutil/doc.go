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

// Package testutil provides fake implementations of ADK-Go interfaces for
// deterministic testing without external LLM providers.
//
// The package includes fakes for all core ADK-Go types:
//   - FakeLLM: implements model.LLM with configurable responses and streaming
//   - FakeSession, FakeState, FakeEvents: implement session interfaces
//   - FakeAgent: wraps agent.New for testing agent hierarchies
//   - FakeInvocationContext, FakeCallbackContext, FakeReadonlyContext: implement agent context interfaces
//   - FakeTool, FakeToolContext, FakeToolset: implement tool interfaces
//   - FakeArtifactService: implements artifact.Service with in-memory storage
//   - FakeMemoryService: implements memory.Service with configurable search
//   - FakeSessionService: implements session.Service with call tracking
//   - RunnerBuilder: constructs runner.Runner with all fakes pre-wired
//
// All fakes support the builder pattern for configuration, record calls for
// assertions, and are thread-safe.
//
// # Quick Start
//
// Create a fake LLM and use it to test a planner:
//
//	llm := testutil.NewFakeLLM(testutil.NewTextResponse(`{"steps":[...]}`))
//	planner := myplanner.New(myplanner.Config{Model: llm})
//	plan, err := planner.GeneratePlan(ctx, req)
//	require.Len(t, llm.Calls, 1)
//
// Run an agent end-to-end:
//
//	events, err := testutil.RunAgent(ctx, ag, llm, "Hello")
package testutil
