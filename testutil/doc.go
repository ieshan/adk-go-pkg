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
//   - FakeEmbedding: generates deterministic embedding vectors for testing semantic search
//   - RunnerBuilder: constructs runner.Runner with all fakes pre-wired
//
// Most fakes support the builder pattern for configuration and record calls
// for assertions; all are thread-safe. Simple delegating wrappers
// (FakeArtifacts, FakeMemory) are immutable after construction and do not
// record calls.
//
// # Quick Start
//
// Create a fake LLM and use it to test a planner:
//
//	llm := testutil.NewFakeLLM(testutil.NewTextResponse(`{"steps":[...]}`))
//	planner := myplanner.New(myplanner.Config{Model: llm})
//	plan, err := planner.GeneratePlan(ctx, req)
//	require.Equal(t, 1, llm.CallCount())
//
// Run an agent end-to-end:
//
//	events, err := testutil.RunAgent(ctx, ag, llm, "Hello")
package testutil
