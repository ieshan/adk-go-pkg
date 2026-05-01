# Test Utilities (`testutil/`)

Package `testutil` provides fake implementations of all ADK-Go interfaces for deterministic testing without external LLM providers.

## Overview

The `testutil` package enables you to write fast, deterministic unit and integration tests for ADK-Go applications without calling real LLM APIs. It includes fakes for models, sessions, agents, tools, artifacts, memory, and the runner—along with helper functions for constructing common types.

**Key Features:**
- **Complete Interface Coverage**: Fakes for all major ADK-Go interfaces
- **Streaming Support**: FakeLLM handles both streaming and non-streaming modes
- **Call Recording**: All fakes record interactions for assertions
- **Thread-Safe**: All fakes use `sync.RWMutex` for safe concurrent access
- **Builder Pattern**: Fluent configuration API for all fakes
- **Error Injection**: Configure errors to test failure paths
- **No External Dependencies**: Only depends on ADK-Go and standard library

## Installation

The `testutil` package is included with the main module:

```bash
go get github.com/ieshan/adk-go-pkg
```

Import in test files:

```go
import "github.com/ieshan/adk-go-pkg/testutil"
```

## Quick Start

### Testing with FakeLLM

```go
func TestMyPlanner(t *testing.T) {
    // Create a fake LLM with preconfigured responses
    llm := testutil.NewFakeLLM(
        testutil.NewTextResponse(`{"steps":[{"description":"search web"}]}`),
    )

    // Use it like a real LLM
    planner := myplanner.New(myplanner.Config{Model: llm})
    plan, err := planner.GeneratePlan(ctx, &myplanner.PlanRequest{
        UserMessage: "Search for something",
    })

    // Assert on results and calls
    require.NoError(t, err)
    assert.Len(t, plan.Steps, 1)
    assert.Equal(t, 1, llm.CallCount())
}
```

### End-to-End Agent Testing

```go
func TestAgent_EndToEnd(t *testing.T) {
    // Setup fake LLM with conversation flow
    llm := testutil.NewFakeLLM(
        testutil.NewTextResponse("I'll help you search."),
        testutil.NewFunctionCallResponse(
            testutil.NewFunctionCall("search", map[string]any{"query": "test"}),
        ),
        testutil.NewTextResponse("Here are the results."),
    )

    // Build agent with fake LLM
    ag, err := llmagent.New(llmagent.Config{
        Name:        "search-agent",
        Model:       llm,
        Instruction: "You are a search assistant.",
    })
    require.NoError(t, err)

    // Run with convenience function
    events, err := testutil.RunAgent(ctx, ag, llm, "Search for test")
    require.NoError(t, err)
    assert.True(t, len(events) > 0)
}
```

## Fake Implementations

### FakeLLM

`FakeLLM` implements `model.LLM` for deterministic testing with configurable responses.

```go
// Basic usage with single response
llm := testutil.NewFakeLLM(testutil.NewTextResponse("Hello!"))

// Multiple responses for multi-turn conversations
llm := testutil.NewFakeLLM(
    testutil.NewTextResponse("First response"),
    testutil.NewTextResponse("Second response"),
)

// With streaming responses
llm := testutil.NewFakeLLM(
    testutil.NewPartialTextResponse("Partial "),
    testutil.NewPartialTextResponse("text "),
    testutil.NewTextResponse("complete"),
)

// Configure error
llm.SetError(errors.New("rate limit exceeded"))

// Builder pattern
llm := testutil.NewFakeLLM(response).WithName("custom-model")

// Assertions
assert.Equal(t, 2, llm.CallCount())
assert.NotNil(t, llm.LastCall())
```

**Response Queue Behavior:**
- Responses are returned in order from the queue
- If exhausted, the last response is repeated
- Supports both streaming (`Partial=true`) and final (`TurnComplete=true`) responses

### FakeSession, FakeState, FakeEvents

Implement the `session.Session`, `session.State`, and `session.Events` interfaces.

```go
// Create session with builder
sess := testutil.NewFakeSession().
    WithID("session-123").
    WithAppName("my-app").
    WithUserID("user-1").
    WithState(map[string]any{
        "counter": 42,
        "theme":   "dark",
    }).
    WithEvents(
        testutil.NewTextEvent("user", "Hello"),
        testutil.NewTextEvent("model", "Hi there!"),
    )

// Access state
val, err := sess.State().Get("counter") // returns 42, nil

// State returns ErrStateKeyNotExist for missing keys
_, err := sess.State().Get("missing") // err == session.ErrStateKeyNotExist

// Iterate over all state entries
for key, val := range sess.State().All() {
    fmt.Printf("%s: %v\n", key, val)
}

// Access events
assert.Equal(t, 2, sess.Events().Len())
event := sess.Events().At(0)
```

### FakeAgent

`FakeAgent` wraps `agent.New()` to create testable agents with full hierarchy support.

```go
// Simple fake agent
agent := testutil.NewFakeAgent("my-agent")

// With description and sub-agents
subAgent := testutil.NewFakeAgent("sub-agent")
agent := testutil.NewFakeAgent("parent").
    WithDescription("A test agent").
    WithSubAgents(subAgent)

// With custom run function
agent := testutil.NewFakeAgent("custom").
    WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
        return func(yield func(*session.Event, error) bool) {
            e := testutil.NewTextEvent("model", "Custom response")
            yield(e, nil)
        }
    })

// Assertions
assert.Equal(t, 1, agent.CallCount())
assert.NotNil(t, agent.LastContext())

// Can be used anywhere agent.Agent is expected
runner, err := runner.New(runner.Config{
    Agent: agent,
    // ...
})
```

**Design Note:** Since `agent.Agent` has an unexported `internal()` method, `FakeAgent` wraps a real agent created via `agent.New()` and embeds it, allowing it to be used anywhere `agent.Agent` is expected.

### Context Fakes

#### FakeInvocationContext

Implements `agent.InvocationContext`:

```go
ctx := testutil.NewFakeInvocationContext().
    WithAgent(myAgent).
    WithArtifacts(artifacts).
    WithMemory(memory).
    WithSession(session).
    WithInvocationID("inv-123").
    WithBranch("main").
    WithUserContent(genai.NewContentFromText("Hello", "user")).
    WithRunConfig(&agent.RunConfig{})

// Use in agent.Run()
iter := agent.Run(ctx)
```

#### FakeCallbackContext

Implements `agent.CallbackContext`:

```go
ctx := testutil.NewFakeCallbackContext().
    WithUserID("user-1").
    WithAppName("my-app").
    WithSessionID("session-1").
    WithUserContent(genai.NewContentFromText("Hello", "user")).
    WithState(state).
    WithArtifacts(artifacts)
```

#### FakeReadonlyContext

Implements `agent.ReadonlyContext` (immutable context):

```go
ctx := testutil.NewFakeReadonlyContext().
    WithUserID("user-1").
    WithAppName("my-app").
    WithSessionID("session-1")
```

### FakeTool and FakeToolContext

```go
// Create a fake tool
tool := testutil.NewFakeTool("search").
    WithDescription("Search the web").
    WithIsLongRunning(false).
    WithDeclaration(&genai.FunctionDeclaration{
        Name:        "search",
        Description: "Search the web",
    }).
    WithRunFunc(func(ctx tool.Context, args map[string]any) (any, error) {
        return map[string]any{"results": []string{"result1", "result2"}}, nil
    })

// Create tool context for testing
callbackCtx := testutil.NewFakeCallbackContext()
toolCtx := testutil.NewFakeToolContext(callbackCtx).
    WithFunctionCallID("fc-123").
    WithActions(&session.EventActions{})

// Execute tool
result, err := tool.Run(toolCtx, map[string]any{"query": "test"})

// Assertions
assert.Equal(t, 1, tool.CallCount())
assert.Equal(t, "test", tool.LastArgs()["query"])
```

### FakeArtifactService

Implements `artifact.Service` with in-memory storage and versioning:

```go
svc := testutil.NewFakeArtifactService()

// Preload artifacts for test setup
svc.PreloadArtifact("my-app", "user-1", "session-1", "report.txt", 
    &genai.Part{Text: "quarterly report"})

// Or use through the interface
resp, err := svc.Save(ctx, &artifact.SaveRequest{
    AppName:   "my-app",
    UserID:    "user-1",
    SessionID: "session-1",
    FileName:  "data.json",
    Part:      &genai.Part{Text: "{}"},
})
// resp.Version == 1

// Load with version
loadResp, err := svc.Load(ctx, &artifact.LoadRequest{
    AppName:   "my-app",
    UserID:    "user-1",
    SessionID: "session-1",
    FileName:  "data.json",
    Version:   1,
})

// List artifacts
listResp, err := svc.List(ctx, &artifact.ListRequest{
    AppName:   "my-app",
    UserID:    "user-1",
    SessionID: "session-1",
})

// Assertions
assert.Equal(t, 2, svc.SaveCount())
assert.Equal(t, 1, svc.LoadCount())
```

**Features:**
- Automatic versioning (each save increments version)
- Supports user-scoped artifacts (filenames starting with "user:")
- Validates requests like the real service
- Records all operations for assertions

### FakeMemoryService

Implements `memory.Service` with configurable search:

```go
svc := testutil.NewFakeMemoryService()

// Preload memory entries
svc.PreloadMemory("user-1", "my-app",
    testutil.NewMemoryEntry("mem-1", "User prefers dark mode", "observation"),
    testutil.NewMemoryEntry("mem-2", "User likes cats", "observation"),
)

// Or configure custom search behavior
svc.WithSearchFunc(func(ctx context.Context, req *memory.SearchRequest) (*memory.SearchResponse, error) {
    return &memory.SearchResponse{
        Memories: []memory.Entry{
            {ID: "custom-1", Content: genai.NewContentFromText("Custom result", "model")},
        },
    }, nil
})

// Use the service
err := svc.AddSessionToMemory(ctx, session)
resp, err := svc.SearchMemory(ctx, &memory.SearchRequest{
    Query:   "preferences",
    UserID:  "user-1",
    AppName: "my-app",
})

// Assertions
assert.Equal(t, 1, svc.AddSessionCount())
assert.Equal(t, 1, svc.SearchCount())
```

### FakeSessionService

Implements `session.Service` with call tracking:

```go
svc := testutil.NewFakeSessionService()

// Preload sessions
sess := testutil.NewFakeSession().WithID("session-123")
svc.PreloadSession(sess)

// Create new session
createResp, err := svc.Create(ctx, &session.CreateRequest{
    AppName:   "my-app",
    UserID:    "user-1",
    SessionID: "new-session",
})

// Get session
getResp, err := svc.Get(ctx, &session.GetRequest{
    AppName:   "my-app",
    UserID:    "user-1",
    SessionID: "session-123",
})

// Append event (handles temp: key removal)
event := testutil.NewTextEvent("model", "Hello")
err := svc.AppendEvent(ctx, sess, event)

// Assertions
assert.Equal(t, 1, svc.CreateCount())
assert.Equal(t, 1, svc.AppendEventCount())
```

### RunnerBuilder

Constructs a `runner.Runner` with all fakes pre-wired:

```go
// Build runner with all fake services
r, fakes, err := testutil.NewRunnerBuilder().
    WithAppName("test-app").
    WithAgent(myAgent).
    WithAutoCreateSession(true).
    BuildWithFakes()

// Run the agent
events, err := testutil.CollectEvents(r.Run(ctx, "user-1", "session-1",
    genai.NewContentFromText("Hello", "user"), agent.RunConfig{}))

// Assert on fake interactions
assert.Equal(t, 1, fakes.SessionService.AppendEventCount())
assert.Equal(t, 1, fakes.ArtifactService.SaveCount())
```

**RunnerFakes provides access to:**
- `SessionService *FakeSessionService`
- `ArtifactService *FakeArtifactService`
- `MemoryService *FakeMemoryService`

### FakeEmbedding

Generates deterministic embedding vectors for testing semantic search and memory systems without calling external embedding APIs:

```go
fake := testutil.NewFakeEmbedding()

// Generate embedding
vec, err := fake.Embed(ctx, "hello world")
if err != nil {
    log.Fatal(err)
}
// vec is []float32 with 1536 dimensions (default)

// Use with memory kit for semantic search testing
kit, err := memory.New(memory.KitConfig{
    Storage:       storage,
    EmbeddingFunc: fake.AsFunc(),
})

// Configure custom dimension (e.g., for different embedding models)
fake384 := testutil.NewFakeEmbedding().WithDimension(384)
vec, _ := fake384.Embed(ctx, "test")
// vec has 384 dimensions

// Preconfigure specific embeddings for known texts
fake.WithPrecomputedEmbedding("user query", []float32{0.1, 0.2, 0.3})
vec, _ := fake.Embed(ctx, "user query") // Returns {0.1, 0.2, 0.3}
```

**Features:**
- Deterministic: same text always produces same vector (using SHA-256 hashing)
- Thread-safe: safe for concurrent use in parallel tests
- Configurable: custom dimensions, precomputed embeddings, error injection
- Call recording: tracks all Embed calls for assertions

**Deterministic Generation:**
Vectors are generated deterministically from the input text using SHA-256 hashing. This means:
- Tests are reproducible across runs
- Same text produces identical embeddings
- Different texts produce different embeddings
- No randomness, no external API calls

**Assertions:**
```go
assert.Equal(t, 2, fake.CallCount())
assert.Equal(t, "hello world", fake.LastCall())
calls := fake.Calls() // []string{"first", "second"}
```

## Helper Functions

### Content Builders

```go
// Create content
content := testutil.NewContent("Hello", genai.RoleUser)
userContent := testutil.NewUserContent("Hello")
modelContent := testutil.NewModelContent("Hi there!")

// With parts
content := testutil.NewContentWithParts("model",
    testutil.NewTextPart("Hello "),
    testutil.NewTextPart("world"),
)

// Inline data
imgPart := testutil.NewInlineDataPart("image/png", pngBytes)
```

### Event Builders

```go
// Basic events
event := testutil.NewEvent("model", content)
textEvent := testutil.NewTextEvent("model", "Hello!")

// Function call events
fcEvent := testutil.NewFunctionCallEvent("model",
    testutil.NewFunctionCall("search", map[string]any{"query": "test"}),
)

// Function response events
frEvent := testutil.NewFunctionResponseEvent("user",
    testutil.NewFunctionResponseForCall("search", map[string]any{"results": []string{}}),
)

// Transfer events
transferEvent := testutil.NewTransferEvent("model", "sub-agent")

// With invocation ID
invEvent := testutil.NewEventWithInvocationID("inv-123", "model", content)
```

### LLM Response Builders

```go
// Text responses
textResp := testutil.NewTextResponse("Hello!")              // TurnComplete=true
partialResp := testutil.NewPartialTextResponse("Partial...") // Partial=true

// Function calls
fcResp := testutil.NewFunctionCallResponse(
    testutil.NewFunctionCall("tool1", map[string]any{}),
    testutil.NewFunctionCall("tool2", map[string]any{}),
)

// Function responses
frResp := testutil.NewFunctionResponseLLMResponse(
    testutil.NewFunctionResponseForCall("tool1", map[string]any{"result": "ok"}),
)

// Error responses
errResp := testutil.NewErrorResponse("RATE_LIMIT", "Rate limit exceeded")
```

### Function Call/Response Builders

```go
// Function calls
fc := testutil.NewFunctionCall("search", map[string]any{"query": "test"})
fcWithID := testutil.NewFunctionCallWithID("search", "fc-123", map[string]any{"query": "test"})

// Function responses
fr := testutil.NewFunctionResponseForCall("search", map[string]any{"results": []string{}})
frWithID := testutil.NewFunctionResponseWithID("search", "fc-123", map[string]any{"results": []string{}})
```

### LLM Request Builders

```go
// Simple request
req := testutil.NewLLMRequest(
    testutil.NewUserContent("Hello"),
)

// With config
req := testutil.NewLLMRequestWithConfig(
    &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](0.5)},
    testutil.NewUserContent("Hello"),
)
```

### Event Collection Helpers

```go
// Collect all events
events, err := testutil.CollectEvents(r.Run(ctx, userID, sessionID, content, config))

// Collect only final events
finalEvents, err := testutil.CollectFinalEvents(iter)

// Filter by author
modelEvents := testutil.FindEventsByAuthor(events, "model")

// Find function calls
fcEvents := testutil.FindFunctionCallEvents(events)

// Find function responses
frEvents := testutil.FindFunctionResponseEvents(events)

// Extract all text
text := testutil.ExtractTextFromEvents(events)
```

### Memory Entry Builder

```go
entry := testutil.NewMemoryEntry("mem-1", "User prefers dark mode", "observation")
```

## Usage Examples

### Testing a Planner with FakeLLM

```go
func TestPlanner_GeneratePlan(t *testing.T) {
    llm := testutil.NewFakeLLM(testutil.NewTextResponse(`{
        "steps": [
            {"description": "Search web", "toolName": "search", "args": {}, "dependsOn": []}
        ],
        "reasoning": "Need to find information"
    }`))

    planner := myplanner.New(myplanner.Config{Model: llm})
    plan, err := planner.GeneratePlan(ctx, &myplanner.PlanRequest{
        UserMessage: "Find information about Go",
    })

    require.NoError(t, err)
    assert.Len(t, plan.Steps, 1)
    assert.Equal(t, "Search web", plan.Steps[0].Description)

    // Verify LLM was called correctly
    require.Equal(t, 1, llm.CallCount())
    assert.Contains(t, llm.LastCall().Contents[0].Parts[0].Text, "Find information")
}
```

### Testing Tool Execution

```go
func TestSearchTool_Run(t *testing.T) {
    // Setup context
    cbCtx := testutil.NewFakeCallbackContext().
        WithUserID("user-1").
        WithAppName("my-app").
        WithSessionID("session-1")
    
    toolCtx := testutil.NewFakeToolContext(cbCtx).
        WithFunctionCallID("fc-123")

    // Create and run tool
    tool := searchtool.New()
    result, err := tool.Run(toolCtx, map[string]any{"query": "golang"})

    require.NoError(t, err)
    assert.NotNil(t, result)
}
```

### Testing Agent Hierarchy

```go
func TestAgent_Delegation(t *testing.T) {
    // Create sub-agent
    subAgent := testutil.NewFakeAgent("specialist").
        WithDescription("A specialist agent")

    // Create parent with sub-agent
    parent := testutil.NewFakeAgent("coordinator").
        WithDescription("Coordinates work").
        WithSubAgents(subAgent)

    // Test hierarchy
    found := parent.FindSubAgent("specialist")
    require.NotNil(t, found)
    assert.Equal(t, "specialist", found.Name())
    assert.Equal(t, "A specialist agent", found.Description())
}
```

### Full End-to-End Test with RunnerBuilder

```go
func TestEndToEnd_CompleteFlow(t *testing.T) {
    // Setup fake LLM with conversation flow
    llm := testutil.NewFakeLLM(
        testutil.NewTextResponse("I'll search for that."),
        testutil.NewFunctionCallResponse(
            testutil.NewFunctionCall("search", map[string]any{"query": "test"}),
        ),
        testutil.NewTextResponse("Here are the search results."),
    )

    // Create agent with fake LLM
    ag, err := llmagent.New(llmagent.Config{
        Name:        "search-assistant",
        Model:       llm,
        Instruction: "You help users search.",
    })
    require.NoError(t, err)

    // Build runner with all fakes
    r, fakes, err := testutil.NewRunnerBuilder().
        WithAppName("test-app").
        WithAgent(ag).
        BuildWithFakes()
    require.NoError(t, err)

    // Run the agent
    events, err := testutil.CollectEvents(r.Run(ctx, "user-1", "session-1",
        genai.NewContentFromText("Search for test", "user"), agent.RunConfig{}))
    require.NoError(t, err)

    // Assert on results
    assert.True(t, len(events) > 0)
    
    // Assert on service calls
    assert.Equal(t, 1, fakes.SessionService.CreateCount())
    assert.True(t, fakes.SessionService.AppendEventCount() > 0)
}
```

### Testing Error Handling

```go
func TestAgent_ErrorHandling(t *testing.T) {
    llm := testutil.NewFakeLLM()
    llm.SetError(errors.New("rate limit exceeded"))

    ag, _ := llmagent.New(llmagent.Config{Model: llm, Name: "test"})
    
    r, _ := runner.New(runner.Config{
        AppName:        "test",
        Agent:          ag,
        SessionService: testutil.NewFakeSessionService(),
    })

    events, err := testutil.CollectEvents(r.Run(ctx, "user", "session",
        genai.NewContentFromText("Hello", "user"), agent.RunConfig{}))

    // Verify error handling
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "rate limit")
}
```

## Design Decisions

### Thread Safety

All fakes use `sync.RWMutex` to protect mutable state:
- `FakeLLM`: protects `calls` slice and `callIndex`
- `FakeSession`/`FakeState`: protects `Data` map
- `FakeArtifactService`: protects `artifacts` map
- `FakeMemoryService`: protects `sessions` and `searches`
- `FakeSessionService`: protects `sessions` map

This allows safe concurrent access in parallel tests.

### Interface Compliance

Every fake includes a compile-time interface check:

```go
var _ model.LLM = (*FakeLLM)(nil)
var _ session.Session = (*FakeSession)(nil)
var _ agent.Agent = (*FakeAgent)(nil)
// ... etc
```

### FakeAgent Wrapping Strategy

The `agent.Agent` interface has an unexported `internal()` method, making direct implementation impossible from outside the `agent` package. `FakeAgent` solves this by:

1. Using `agent.New(agent.Config{...})` to create the underlying real agent
2. Embedding the `agent.Agent` returned by `agent.New`
3. Intercepting the `Run` function for call tracking
4. Delegating all other methods to the embedded agent

This allows `FakeAgent` to be passed anywhere `agent.Agent` is expected.

### Response Queue Behavior

`FakeLLM` returns responses in order from the queue. If exhausted, the last response is repeated. This enables:

- Multi-turn conversation testing with different responses per turn
- Simple single-response tests
- Infinite conversation simulation (repeating last response)

### Request Validation

`FakeArtifactService` validates requests using the same `Validate()` methods as the real service, ensuring tests catch validation errors early.

## Best Practices

1. **Use builder pattern** for complex fake configuration
2. **Record calls** and assert on them to verify interactions
3. **Test error paths** using `SetError()` methods
4. **Preload data** for complex initial state instead of building through API calls
5. **Use helper functions** for constructing common types (more readable, less boilerplate)
6. **Run with `-race`** flag to catch concurrency issues
7. **Reset fakes** between test cases when reusing

## Compatibility

- **Go 1.26+** — Uses `iter.Seq2` and range-over-func
- **ADK-Go v1.2.0+** (`google.golang.org/adk`)
- **GenAI v1.54.0+** (`google.golang.org/genai`)
