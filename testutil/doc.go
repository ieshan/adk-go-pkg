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
