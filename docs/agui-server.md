# Generic AG-UI Server Library (`agui/`)

Package `agui` provides a generic AG-UI protocol server for Go applications.

## Overview

The `agui` package implements the [AG-UI](https://github.com/ag-ui-protocol/ag-ui)
(Agent-User Interaction) protocol, enabling any Go application to serve
AG-UI-compatible frontends like CopilotKit or AG-UI Vue. It handles HTTP/SSE
streaming, event emission, state management, client tool orchestration, and
middleware composition.

**Key property:** this package has **zero dependency on ADK-Go**. You can use it
with any agent framework, a hand-rolled LLM client, or no LLM at all. For
ADK-Go integration, see the companion [`aguiadk`](aguiadk-bridge.md) package.

## Table of Contents

- [Overview](#overview)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Agent Interface](#agent-interface)
- [ChanToIter](#chantoiter)
- [EventEmitter](#eventemitter)
- [State Management](#state-management)
- [Predictive State](#predictive-state)
- [Client Tool Orchestration](#client-tool-orchestration)
- [Middleware](#middleware)
- [Activity and Steps](#activity-and-steps)
- [Reasoning](#reasoning)
- [HTTP Handler](#http-handler)
- [Capabilities Discovery](#capabilities-discovery)
- [Client Agent](#client-agent)
- [Sub-agent Events](#sub-agent-events)
- [Token Usage Telemetry](#token-usage-telemetry)
- [CORS](#cors)
- [Error Handling](#error-handling)
- [Production Notes](#production-notes)
- [Testing](#testing)
- [Full Example](#full-example)

## Installation

```bash
go get github.com/ieshan/adk-go-pkg/agui
```

## Quick Start

A minimal agent that streams "Hello from AG-UI!" to any AG-UI frontend:

```go
package main

import (
	"context"
	"iter"
	"log"
	"net/http"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func main() {
	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		ch := make(chan events.Event, 64)
		emitter := agui.NewEventEmitter(ch)
		go func() {
			defer close(ch)
			emitter.RunStarted(input.ThreadID, input.RunID)
			msgID := emitter.GenerateMessageID()
			role := "assistant"
			emitter.TextMessageStart(msgID, &role)
			emitter.TextMessageContent(msgID, "Hello from AG-UI!")
			emitter.TextMessageEnd(msgID)
			emitter.RunFinishedWithOptions(input.ThreadID, input.RunID)
		}()
		return agui.ChanToIter(ctx, ch)
	})

	handler, err := agui.Handler(agui.Config{Agent: agent})
	if err != nil {
		log.Fatal(err)
	}
	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}
```

## Agent Interface

### Agent

```go
type Agent interface {
    Run(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error]
}
```

`Agent` is the core interface. Anything that accepts a `RunAgentInput` and
returns an iterator of AG-UI events can serve as an agent. The iterator may
yield `(event, nil)` pairs for normal events or `(nil, err)` to signal an
error (which the handler translates into a `RUN_ERROR` SSE event).

### AgentFunc

```go
type AgentFunc func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error]
```

`AgentFunc` is an adapter that lets you use an ordinary function as an `Agent`,
similar to `http.HandlerFunc`:

```go
package main

import (
	"context"
	"iter"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func myAgent(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
	ch := make(chan events.Event, 64)
	emitter := agui.NewEventEmitter(ch)
	go func() {
		defer close(ch)
		emitter.RunStarted(input.ThreadID, input.RunID)
		// ... emit events ...
		emitter.RunFinishedWithOptions(input.ThreadID, input.RunID)
	}()
	return agui.ChanToIter(ctx, ch)
}

func main() {
	var agent agui.Agent = agui.AgentFunc(myAgent)
	_ = agent
}
```

## ChanToIter

```go
func ChanToIter(ctx context.Context, ch <-chan events.Event) iter.Seq2[events.Event, error]
```

`ChanToIter` bridges Go channels to the `iter.Seq2` iterator expected by the
`Agent` interface. It reads events from `ch` until the channel is closed. If
`ctx` is cancelled, it yields a context cancellation error and stops.

The standard pattern is:

1. Create a buffered channel: `ch := make(chan events.Event, 64)`
2. Create an emitter: `emitter := agui.NewEventEmitter(ch)`
3. Launch a goroutine that emits events and closes the channel when done
4. Return `agui.ChanToIter(ctx, ch)`

```go
package main

import (
	"context"
	"iter"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func streamingAgent(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
	ch := make(chan events.Event, 64)
	emitter := agui.NewEventEmitter(ch)

	go func() {
		defer close(ch)
		emitter.RunStarted(input.ThreadID, input.RunID)

		msgID := emitter.GenerateMessageID()
		role := "assistant"
		emitter.TextMessageStart(msgID, &role)

		for _, word := range []string{"Hello ", "world", "!"} {
			emitter.TextMessageContent(msgID, word)
		}

		emitter.TextMessageEnd(msgID)
		emitter.RunFinishedWithOptions(input.ThreadID, input.RunID)
	}()

	return agui.ChanToIter(ctx, ch)
}

func main() {
	_ = agui.AgentFunc(streamingAgent)
}
```

## EventEmitter

```go
type EventEmitter struct { /* unexported */ }

func NewEventEmitter(out chan<- events.Event) *EventEmitter
func NewEventEmitterWithContext(ctx context.Context, out chan<- events.Event) *EventEmitter
```

`EventEmitter` provides typed, ergonomic methods for every AG-UI event type.
Create one by passing a writable channel. Most emit methods return `error` —
sends to a closed channel or a cancelled context (when using
`NewEventEmitterWithContext`) return an error wrapping `ErrTransport`. The
exceptions are `GenerateMessageID()` and `GenerateToolCallID()` (which return
`string`) and `ForSubagent()` (which returns `*EventEmitter`).

`NewEventEmitter` is not bound to any context: emit calls block until the
channel accepts the event. If the channel is closed, emit recovers the panic
and returns an error wrapping `ErrTransport`. **Prefer
`NewEventEmitterWithContext`** for streaming runs where the consumer may stop
iterating before the producer finishes — when `ctx` is cancelled, pending and
future emit calls unblock promptly with an error wrapping `ErrTransport` and
the context's error (`ctx.Err()`), preventing goroutine leaks on early stream
termination or client disconnect.

### Errors

```go
var ErrTransport = errors.New("agui: transport error")
```

`ErrTransport` indicates a transport-level error (e.g. the client disconnected
and the event channel is closed). Use `errors.Is` to distinguish transport
errors from encoding errors:

- A **transport error** means the client is gone and the run should be
  cancelled — stop emitting.
- An **encoding error** means the event content is malformed. Any
  `WriteEvent` error (including encoding failures) stops the stream — see
  [Encoding Errors](#encoding-errors) below.

```go
if err := emitter.TextMessageContent(msgID, delta); err != nil {
    if errors.Is(err, agui.ErrTransport) {
        // Client is gone — stop the producer goroutine.
        return
    }
    // Any other error from the emitter is also fatal — stop emitting.
    log.Printf("emit error: %v", err)
    return
}
```

### ID Generation

| Method | Description |
|--------|-------------|
| `GenerateMessageID() string` | Returns a unique message ID |
| `GenerateToolCallID() string` | Returns a unique tool call ID |

### Run Lifecycle

| Method | Description |
|--------|-------------|
| `RunStarted(threadID, runID string) error` | Emits `RUN_STARTED` |
| `RunFinishedWithOptions(threadID, runID string, opts ...events.RunFinishedOption) error` | Emits `RUN_FINISHED` with optional configuration (e.g., `events.WithSuccessOutcome`, `events.WithInterruptOutcome`) |
| `RunErrorWithOptions(message string, opts ...events.RunErrorOption) error` | Emits `RUN_ERROR` with optional configuration (e.g., `events.WithRunID`, `events.WithErrorCode`) |
| `RunFinishedWithUsage(threadID, runID string, usage []events.TokenUsage, opts ...events.RunFinishedOption) error` | Emits `RUN_FINISHED` with aggregated token usage telemetry; falls back to `RunFinishedWithOptions` when `usage` is empty/nil. See [Token Usage Telemetry](#token-usage-telemetry). |
| `RunErrorWithUsage(message string, usage []events.TokenUsage, opts ...events.RunErrorOption) error` | Emits `RUN_ERROR` with aggregated token usage telemetry; falls back to `RunErrorWithOptions` when `usage` is empty/nil. See [Token Usage Telemetry](#token-usage-telemetry). |

### Text Messages

| Method | Description |
|--------|-------------|
| `TextMessageStart(messageID string, role *string) error` | Emits `TEXT_MESSAGE_START` |
| `TextMessageStartWithID(messageID string, role *string, name string) error` | Emits `TEXT_MESSAGE_START` with an optional role and sub-agent `name` (via `events.WithName`) for attribution |
| `TextMessageContent(messageID, delta string) error` | Emits `TEXT_MESSAGE_CONTENT` with a text delta |
| `TextMessageEnd(messageID string) error` | Emits `TEXT_MESSAGE_END` |
| `TextMessageChunk(messageID, role, delta *string) error` | Convenience chunk event |

### Tool Calls

| Method | Description |
|--------|-------------|
| `ToolCallStart(toolCallID, toolCallName string, parentMessageID *string) error` | Emits `TOOL_CALL_START` |
| `ToolCallArgs(toolCallID, delta string) error` | Emits `TOOL_CALL_ARGS` |
| `ToolCallEnd(toolCallID string) error` | Emits `TOOL_CALL_END` |
| `ToolCallResult(messageID, toolCallID, content string) error` | Emits `TOOL_CALL_RESULT` |
| `ToolCallChunk(toolCallID, toolCallName, parentMessageID, delta *string) error` | Convenience chunk event |

### State

| Method | Description |
|--------|-------------|
| `StateSnapshot(snapshot any) error` | Emits `STATE_SNAPSHOT` |
| `StateDelta(delta []events.JSONPatchOperation) error` | Emits `STATE_DELTA` with JSON Patch ops |
| `MessagesSnapshot(messages []types.Message) error` | Emits `MESSAGES_SNAPSHOT`; `EncryptedValue` and `EncryptedContent` fields are scrubbed (zeroed) before emission to prevent leaking ciphertext to the client. The scrub is allocation-free when no message carries encrypted fields. |

### Steps and Activity

| Method | Description |
|--------|-------------|
| `StepStarted(stepName string) error` | Emits `STEP_STARTED` |
| `StepFinished(stepName string) error` | Emits `STEP_FINISHED` |
| `ActivitySnapshot(messageID, activityType string, content any, replace *bool) error` | Emits `ACTIVITY_SNAPSHOT` |
| `ActivityDelta(messageID, activityType string, patch []events.JSONPatchOperation) error` | Emits `ACTIVITY_DELTA` |

### Reasoning

| Method | Description |
|--------|-------------|
| `ReasoningStart(messageID string) error` | Emits `REASONING_START` |
| `ReasoningMessageStart(messageID, role string) error` | Emits `REASONING_MESSAGE_START` |
| `ReasoningMessageContent(messageID, delta string) error` | Emits `REASONING_MESSAGE_CONTENT` |
| `ReasoningMessageEnd(messageID string) error` | Emits `REASONING_MESSAGE_END` |
| `ReasoningEnd(messageID string) error` | Emits `REASONING_END` |
| `ReasoningMessageChunk(messageID, delta *string) error` | Convenience chunk event |
| `ReasoningEncryptedValue(subtype events.ReasoningEncryptedValueSubtype, entityID, encryptedValue string) error` | Emits `REASONING_ENCRYPTED_VALUE` |

### Custom and Raw

| Method | Description |
|--------|-------------|
| `Custom(name string, value any) error` | Emits `CUSTOM` event |
| `Raw(event any, source *string) error` | Emits `RAW` event |

## State Management

```go
type StateManager struct { /* unexported */ }

func NewStateManager(initial any) (*StateManager, error)
```

`StateManager` tracks shared application state using RFC 6902 JSON Patch
operations (backed by [`github.com/evanphx/json-patch/v5`](https://github.com/evanphx/json-patch)).
It is safe for concurrent use.

`StateManager` also serves as the project's "DocState" — it provides the same
`Apply`/`Snapshot` semantics as the AG-UI example server's `docstate.go`, plus
`Diff` and `Set`. There is no separate `DocState` type; creating one would be a
redundant subset of `StateManager`.

> **Note:** `NewStateManager` only accepts JSON-object initial state — arrays
> and scalars fail because the state is always stored as `map[string]any`.
> `Diff` likewise requires a JSON-object `newState`.

### Methods

| Method | Description |
|--------|-------------|
| `Snapshot() any` | Returns a deep copy of the current state |
| `Set(state any) error` | Replaces the entire state |
| `Apply(patch []events.JSONPatchOperation) error` | Applies JSON Patch operations |
| `Diff(newState any) ([]events.JSONPatchOperation, error)` | Computes a patch from current to new state |

### Supported Patch Operations

`Apply` supports all six RFC 6902 operations: `add`, `remove`, `replace`,
`move`, `copy`, and `test`.

### Example: State with Deltas

```go
package main

import (
	"context"
	"fmt"
	"iter"
	"log"
	"net/http"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func main() {
	sm, err := agui.NewStateManager(map[string]any{"count": 0})
	if err != nil {
		log.Fatal(err)
	}

	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		ch := make(chan events.Event, 64)
		emitter := agui.NewEventEmitter(ch)
		go func() {
			defer close(ch)
			emitter.RunStarted(input.ThreadID, input.RunID)

			// Send initial state snapshot.
			emitter.StateSnapshot(sm.Snapshot())

			// Compute and apply a delta.
			newState := map[string]any{"count": 1, "status": "running"}
			patch, _ := sm.Diff(newState)
			sm.Apply(patch)
			emitter.StateDelta(patch)

			// Direct patch application.
			directPatch := []events.JSONPatchOperation{
				{Op: "replace", Path: "/count", Value: 2},
				{Op: "add", Path: "/message", Value: "done"},
			}
			sm.Apply(directPatch)
			emitter.StateDelta(directPatch)

			emitter.RunFinishedWithOptions(input.ThreadID, input.RunID)
		}()
		return agui.ChanToIter(ctx, ch)
	})

	handler, err := agui.Handler(agui.Config{Agent: agent})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("state:", sm.Snapshot())
	log.Fatal(http.ListenAndServe(":8080", handler))
}
```

## Predictive State

`PredictiveStateTracker` streams ghosted state deltas under the `/_predictive`
namespace, then commits the final value to the real path and clears the
prediction. This lets a UI render an optimistic preview while the agent is
still generating, then settle to the committed value on completion. Clients
that ignore every delta under `/_predictive` still end in the correct
committed state.

```go
type PredictiveStateTracker struct { /* unexported */ }

func NewPredictiveStateTracker(state *StateManager, emitter *EventEmitter) *PredictiveStateTracker
```

### Methods

| Method | Description |
|--------|-------------|
| `PredictiveDelta(patch []events.JSONPatchOperation) error` | Applies `patch` under `/_predictive` (paths prefixed automatically) and emits a `STATE_DELTA` with the prefixed paths. The namespace is created lazily on first use; subsequent calls preserve existing predictive state. |
| `Commit(path string, value any) error` | Applies `value` to the real state `path` (creating intermediate objects if needed) and emits a `STATE_DELTA` with the real, un-prefixed path. Independent of any prediction — a dropped or garbled prediction cannot corrupt it. |
| `Clear() error` | Removes the `/_predictive` namespace and emits a `STATE_DELTA` for the removal. Best-effort: if the namespace does not exist, the apply fails silently but the delta is still emitted so clients that tracked it can clean up. |
| `State() *StateManager` | Returns the underlying `StateManager` for inspection/testing. |

### Example: Predictive → Commit → Clear

```go
sm, _ := agui.NewStateManager(nil)
emitter := agui.NewEventEmitter(ch)
predictive := agui.NewPredictiveStateTracker(sm, emitter)

// Stream a ghosted draft while the agent is still working.
predictive.PredictiveDelta([]events.JSONPatchOperation{
    {Op: "add", Path: "/draft", Value: "partial answer..."},
})

// Commit the final value to the real path.
predictive.Commit("/answer", "final answer")

// Clear the predictive namespace.
predictive.Clear()
```

## Client Tool Orchestration

AG-UI supports two modes for client-side tool execution:

### ToolModeNextRun (default)

The agent emits tool call events (`TOOL_CALL_START` / `TOOL_CALL_ARGS` /
`TOOL_CALL_END`) and then finishes the run. The frontend executes the tool and
sends results in the next `RunAgentInput`. No extra server infrastructure is
needed.

```go
package main

import (
	"context"
	"iter"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func toolAgent(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
	ch := make(chan events.Event, 64)
	emitter := agui.NewEventEmitter(ch)
	go func() {
		defer close(ch)
		emitter.RunStarted(input.ThreadID, input.RunID)

		// Request a client-side tool call.
		toolCallID := emitter.GenerateToolCallID()
		emitter.ToolCallStart(toolCallID, "get_weather", nil)
		emitter.ToolCallArgs(toolCallID, `{"city":"London"}`)
		emitter.ToolCallEnd(toolCallID)

		// End the run; the frontend will call back with the result.
		emitter.RunFinishedWithOptions(input.ThreadID, input.RunID)
	}()
	return agui.ChanToIter(ctx, ch)
}

func main() {
	_ = agui.AgentFunc(toolAgent)
}
```

### ToolModeInline

The SSE connection stays open while the frontend executes the tool. The
frontend POSTs the result to a separate `/tool-result` endpoint, and the agent
goroutine receives it via `ToolResultHandler.Wait`.

```go
package main

import (
	"context"
	"iter"
	"log"
	"net/http"
	"time"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func main() {
	toolHandler := agui.NewToolResultHandler()

	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		ch := make(chan events.Event, 64)
		emitter := agui.NewEventEmitter(ch)
		go func() {
			defer close(ch)
			emitter.RunStarted(input.ThreadID, input.RunID)

			// Emit tool call.
			toolCallID := emitter.GenerateToolCallID()
			emitter.ToolCallStart(toolCallID, "get_weather", nil)
			emitter.ToolCallArgs(toolCallID, `{"city":"London"}`)
			emitter.ToolCallEnd(toolCallID)

			// Wait for the frontend to submit the result.
			result, err := toolHandler.Wait(ctx, toolCallID, 5*time.Minute)
			if err != nil {
				emitter.RunErrorWithOptions("tool timed out: " + err.Error())
				return
			}

			// Use the result.
			msgID := emitter.GenerateMessageID()
			role := "assistant"
			emitter.TextMessageStart(msgID, &role)
			emitter.TextMessageContent(msgID, "The weather is: "+result)
			emitter.TextMessageEnd(msgID)

			emitter.RunFinishedWithOptions(input.ThreadID, input.RunID)
		}()
		return agui.ChanToIter(ctx, ch)
	})

	cfg := agui.Config{
		Agent:             agent,
		ToolMode:          agui.ToolModeInline,
		ToolResultHandler: toolHandler,
	}

	handler, err := agui.Handler(cfg)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/agent", handler)
	mux.Handle("/api/tool-result", agui.ToolResultEndpoint(toolHandler))
	log.Fatal(http.ListenAndServe(":8080", mux))
}
```

### ToolResultHandler API

```go
var ErrToolCallTimeout = errors.New("agui: tool call timed out")
var ErrNoPendingToolCall = errors.New("agui: no pending tool call")
```

| Method | Description |
|--------|-------------|
| `NewToolResultHandler() *ToolResultHandler` | Creates a new handler |
| `Wait(ctx context.Context, toolCallID string, timeout time.Duration) (string, error)` | Blocks until a result arrives or timeout. Returns an error wrapping `ErrToolCallTimeout` on timeout. |
| `SubmitResult(toolCallID, content string) error` | Delivers a result for a pending call. Returns an error wrapping `ErrNoPendingToolCall` if no waiter is registered. |
| `HasPendingToolCall(toolCallID string) bool` | Reports whether a waiter is currently registered for the given tool call ID. Tests can poll this to deterministically wait for `Wait` to register before calling `SubmitResult`. |

### ToolResultEndpoint

```go
func ToolResultEndpoint(handler *ToolResultHandler) http.Handler
```

Returns an `http.Handler` for `POST /tool-result`. Expects a JSON body:

```json
{"toolCallId": "tc_abc123", "content": "{\"temp\": 22}"}
```

When `SubmitResult` finds no pending call for the given `toolCallId`, the
endpoint responds with `404 Not Found`.

## Middleware

```go
type Middleware func(next Agent) Agent

func Chain(middlewares ...Middleware) Middleware
```

Middlewares wrap an `Agent`, returning a new `Agent`. `Chain` composes them
left-to-right: `Chain(a, b, c)(agent)` produces `a(b(c(agent)))`.

### Built-in Middlewares

The `agui` package ships two MCP-related middlewares:

- **`NewMCPMiddleware`** — Injects MCP server tools into the agent's tool list and executes them server-side in an agentic loop (up to `MaxIterations` rounds).
- **`NewMCPAppsMiddleware`** — Injects UI-enabled MCP tools (tools with `_meta["ui/resourceUri"]`) and handles proxied MCP requests from frontends via `ForwardedProps`.

See [AG-UI MCP Support](agui-mcp.md) for full MCP middleware documentation.

### Example: Logging and Auth

```go
package main

import (
	"context"
	"fmt"
	"iter"
	"log"
	"net/http"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

// loggingMiddleware logs the start and end of each run.
func loggingMiddleware(next agui.Agent) agui.Agent {
	return agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		fmt.Printf("run started: thread=%s run=%s\n", input.ThreadID, input.RunID)
		result := next.Run(ctx, input)
		fmt.Printf("run iterator created: thread=%s run=%s\n", input.ThreadID, input.RunID)
		return result
	})
}

// authMiddleware rejects requests without a valid thread prefix.
func authMiddleware(next agui.Agent) agui.Agent {
	return agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		if len(input.ThreadID) < 3 {
			ch := make(chan events.Event, 1)
			emitter := agui.NewEventEmitter(ch)
			go func() {
				defer close(ch)
				code := "AUTH_FAILED"
				emitter.RunErrorWithOptions("invalid thread ID", events.WithErrorCode(code))
			}()
			return agui.ChanToIter(ctx, ch)
		}
		return next.Run(ctx, input)
	})
}

func myAgent(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
	ch := make(chan events.Event, 64)
	emitter := agui.NewEventEmitter(ch)
	go func() {
		defer close(ch)
		emitter.RunStarted(input.ThreadID, input.RunID)
		msgID := emitter.GenerateMessageID()
		role := "assistant"
		emitter.TextMessageStart(msgID, &role)
		emitter.TextMessageContent(msgID, "Authenticated!")
		emitter.TextMessageEnd(msgID)
		emitter.RunFinishedWithOptions(input.ThreadID, input.RunID)
	}()
	return agui.ChanToIter(ctx, ch)
}

func main() {
	handler, err := agui.Handler(agui.Config{
		Agent:       agui.AgentFunc(myAgent),
		Middlewares: []agui.Middleware{loggingMiddleware, authMiddleware},
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(http.ListenAndServe(":8080", handler))
}
```

## Activity and Steps

### StepTracker

```go
func StepTracker(emitter *EventEmitter, stepName string, fn func() error) error
```

`StepTracker` wraps a function call with `STEP_STARTED` / `STEP_FINISHED`
events. If the function returns an error, the step is still marked as finished.

```go
package main

import (
	"context"
	"fmt"
	"iter"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func steppedAgent(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
	ch := make(chan events.Event, 64)
	emitter := agui.NewEventEmitter(ch)
	go func() {
		defer close(ch)
		emitter.RunStarted(input.ThreadID, input.RunID)

		agui.StepTracker(emitter, "fetch-data", func() error {
			fmt.Println("fetching data...")
			return nil
		})

		agui.StepTracker(emitter, "process", func() error {
			fmt.Println("processing...")
			return nil
		})

		emitter.RunFinishedWithOptions(input.ThreadID, input.RunID)
	}()
	return agui.ChanToIter(ctx, ch)
}

func main() {
	_ = agui.AgentFunc(steppedAgent)
}
```

### ActivityTracker

```go
type ActivityTracker struct { /* unexported */ }

func NewActivityTracker(emitter *EventEmitter, messageID, activityType string) *ActivityTracker
```

`ActivityTracker` provides scoped helpers for emitting `ACTIVITY_SNAPSHOT` and
`ACTIVITY_DELTA` events tied to a specific message and activity type.

| Method | Description |
|--------|-------------|
| `Snapshot(content any, replace *bool) error` | Emits `ACTIVITY_SNAPSHOT` |
| `Delta(patch []events.JSONPatchOperation) error` | Emits `ACTIVITY_DELTA` |

```go
package main

import (
	"context"
	"iter"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func activityAgent(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
	ch := make(chan events.Event, 64)
	emitter := agui.NewEventEmitter(ch)
	go func() {
		defer close(ch)
		emitter.RunStarted(input.ThreadID, input.RunID)

		msgID := emitter.GenerateMessageID()
		tracker := agui.NewActivityTracker(emitter, msgID, "progress")

		// Send initial snapshot.
		tracker.Snapshot(map[string]any{"percent": 0, "status": "starting"}, nil)

		// Send deltas as progress updates.
		tracker.Delta([]events.JSONPatchOperation{
			{Op: "replace", Path: "/percent", Value: 50},
			{Op: "replace", Path: "/status", Value: "halfway"},
		})

		replace := true
		tracker.Snapshot(map[string]any{"percent": 100, "status": "done"}, &replace)

		emitter.RunFinishedWithOptions(input.ThreadID, input.RunID)
	}()
	return agui.ChanToIter(ctx, ch)
}

func main() {
	_ = agui.AgentFunc(activityAgent)
}
```

## Reasoning

```go
type ReasoningTracker struct { /* unexported */ }

func NewReasoningTracker(emitter *EventEmitter, messageID string) *ReasoningTracker
```

`ReasoningTracker` emits the full reasoning event sequence for model thinking
visibility. It manages the `REASONING_START` / `REASONING_MESSAGE_START` /
`REASONING_MESSAGE_CONTENT` / `REASONING_MESSAGE_END` / `REASONING_END`
lifecycle.

| Method | Description |
|--------|-------------|
| `Start(role string) error` | Emits `REASONING_START` + `REASONING_MESSAGE_START` |
| `Content(delta string) error` | Emits `REASONING_MESSAGE_CONTENT` |
| `End() error` | Emits `REASONING_MESSAGE_END` + `REASONING_END` |

```go
package main

import (
	"context"
	"iter"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func reasoningAgent(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
	ch := make(chan events.Event, 64)
	emitter := agui.NewEventEmitter(ch)
	go func() {
		defer close(ch)
		emitter.RunStarted(input.ThreadID, input.RunID)

		// Show thinking process.
		thinkID := emitter.GenerateMessageID()
		reasoning := agui.NewReasoningTracker(emitter, thinkID)
		reasoning.Start("assistant")
		reasoning.Content("The user asked about weather. ")
		reasoning.Content("I should use the weather API...")
		reasoning.End()

		// Then send the actual response.
		msgID := emitter.GenerateMessageID()
		role := "assistant"
		emitter.TextMessageStart(msgID, &role)
		emitter.TextMessageContent(msgID, "Let me check the weather for you.")
		emitter.TextMessageEnd(msgID)

		emitter.RunFinishedWithOptions(input.ThreadID, input.RunID)
	}()
	return agui.ChanToIter(ctx, ch)
}

func main() {
	_ = agui.AgentFunc(reasoningAgent)
}
```

## HTTP Handler

### Config

```go
type Config struct {
    // Agent processes runs. Required.
    Agent Agent

    // Middlewares applied in order before the agent. Optional.
    Middlewares []Middleware

    // ToolMode controls client tool result flow. Default: ToolModeNextRun.
    // Stored on Config for use by the agent/bridge code (e.g. aguiadk.Handler
    // wires it); not consumed by agui.Handler directly.
    ToolMode ToolMode

    // ToolTimeout is the max wait for inline tool results. Default: 5 minutes.
    // Stored on Config for use by the agent/bridge code; the inline
    // tool-result waiting is done by the agent/bridge, not by agui.Handler.
    ToolTimeout time.Duration

    // ToolResultHandler for inline mode. Created automatically if nil
    // and ToolMode is ToolModeInline. Stored on Config for use by the
    // agent/bridge code (e.g. aguiadk.Handler wires it); not consumed by
    // agui.Handler directly.
    ToolResultHandler *ToolResultHandler

    // OnError is an optional callback for handler errors.
    OnError func(err error)

    // CORS configures cross-origin resource sharing. If nil, no CORS headers are set.
    CORS *CORSConfig

    // KeepaliveInterval controls SSE ping frequency. Default: 0 (disabled).
    // Recommended: 15-30 seconds behind proxies.
    KeepaliveInterval time.Duration

    // MaxBodySize limits request body size in bytes. Default: 10 MB (10 << 20).
    MaxBodySize int64

    // Capabilities describes the agent's capabilities for GET discovery
    // (GET / or GET /capabilities). Optional; when nil, discovery requests
    // return 404.
    Capabilities *AgentCapabilities
}
```

### Handler

```go
func Handler(cfg Config) (http.Handler, error)
```

`Handler` returns `nil` and `ErrAgentRequired` if `Config.Agent` is not set.

`Handler` returns an `http.Handler` that:

1. On `GET /` (exact `r.URL.Path == "/"` match) or `GET /capabilities` (path suffix match — ending in `/capabilities` — so the handler works correctly when mounted at a sub-path via `http.ServeMux`), returns the configured `AgentCapabilities` as JSON (404 if `Config.Capabilities` is nil)
2. Returns `405 Method Not Allowed` for any other HTTP method that is not `GET` or `POST`
3. Accepts `POST` requests with a JSON `RunAgentInput` body, limited to `MaxBodySize` bytes (default 10 MB) via `http.MaxBytesReader`
4. Performs `Accept` header negotiation: rejects protobuf transport requests (`application/vnd.ag-ui.event+proto`, `application/x-protobuf`, `application/protobuf`, `application/vnd.google.protobuf`) with `406 Not Acceptable`; accepts all other `Accept` values (including `text/event-stream` and JSON) and serves SSE
5. Sets SSE headers (`Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`)
6. Applies the middleware chain to the agent
7. Starts a keepalive ping goroutine when `KeepaliveInterval > 0` (writes SSE pings on the configured interval until the request context is cancelled)
8. Iterates the agent's event stream, writing each event as an SSE message
9. On iterator error, emits a `RUN_ERROR` event (carrying `input.RunID`) and calls `OnError` if configured
10. On SSE write error, calls `OnError` if configured and stops the stream

> **Mounting / sub-paths:** `agui.Handler` matches capabilities discovery with
> an exact `r.URL.Path == "/"` for the root endpoint and a path-suffix match
> (ending in `/capabilities`) for the capabilities endpoint, so it works
> correctly when mounted at sub-paths (e.g. `/api/agent/`). For
> `aguiadk.Handler`, the `/tool-result` route also matches by path suffix
> (ending in `/tool-result`), so the combined handler can be mounted at any
> sub-path without breaking either endpoint.

## Capabilities Discovery

The handler supports optional capabilities discovery via `GET /` or
`GET /capabilities`. When `Config.Capabilities` is set, these endpoints return
the configured `AgentCapabilities` as JSON. When nil, they return 404.

```go
type AgentCapabilities struct {
    Identity       *IdentityCapabilities       `json:"identity,omitempty"`
    Transport      *TransportCapabilities      `json:"transport,omitempty"`
    State          *StateCapabilities          `json:"state,omitempty"`
    Messages       *MessageCapabilities        `json:"messages,omitempty"`
    Tools          *ToolCapabilities           `json:"tools,omitempty"`
    HumanInTheLoop *HumanInTheLoopCapabilities `json:"humanInTheLoop,omitempty"`
    Activities     *ActivityCapabilities       `json:"activities,omitempty"`
    Reasoning      *ReasoningCapabilities      `json:"reasoning,omitempty"`
}
```

All sub-capabilities are optional pointers so an agent can advertise only the
capabilities it supports.

### Sub-capability Fields

| Type | Fields |
|------|--------|
| `IdentityCapabilities` | `Name`, `Type`, `Description` |
| `TransportCapabilities` | `Streaming`, `Binary`, `Protobuf` |
| `StateCapabilities` | `Snapshots`, `Deltas` |
| `MessageCapabilities` | `Snapshots`, `StreamingText` |
| `ToolCapabilities` | `Supported`, `ClientTools`, `ServerTools`, `Streaming` |
| `HumanInTheLoopCapabilities` | `Interrupts` |
| `ActivityCapabilities` | `Snapshots`, `Deltas` |
| `ReasoningCapabilities` | `Supported`, `Streaming`, `Encrypted` |

### Example

```go
handler, err := agui.Handler(agui.Config{
    Agent: agent,
    Capabilities: &agui.AgentCapabilities{
        Identity: &agui.IdentityCapabilities{
            Name:        "my-agent",
            Type:        "custom",
            Description: "A helpful assistant",
        },
        Transport: &agui.TransportCapabilities{Streaming: true},
        State:     &agui.StateCapabilities{Snapshots: true, Deltas: true},
        Messages:  &agui.MessageCapabilities{Snapshots: false, StreamingText: true},
        Tools:     &agui.ToolCapabilities{Supported: true, ClientTools: true},
    },
})
```

## Client Agent

`ClientAgent` is an `agui.Agent` implementation that streams events from a
remote AG-UI endpoint over HTTP/SSE. It implements the client side of the
AG-UI protocol, decoding SSE frames into typed events and propagating context
cancellation to release HTTP reader goroutines.

```go
type ClientConfig struct {
    Endpoint   string            // Remote AG-UI SSE URL (required)
    APIKey     string            // Optional bearer token
    AuthHeader string            // Overrides "Authorization" header name
    AuthScheme string            // Overrides "Bearer" scheme
    Headers    map[string]string // Additional headers per request
}

func NewClientAgent(cfg ClientConfig) *ClientAgent
```

### Example

```go
client := agui.NewClientAgent(agui.ClientConfig{
    Endpoint: "https://example.com/api/agent",
    APIKey:   os.Getenv("AGUI_API_KEY"),
})

for ev, err := range client.Run(ctx, input) {
    if err != nil { /* handle */ }
    // process ev
}
```

## Sub-agent Events

The `agui` package extends the AG-UI protocol with explicit sub-agent lifecycle
events so frontends can attribute streamed events to the emitting sub-agent and
render sub-agent activity.

| Event Type | Description |
|------------|-------------|
| `SUBAGENT_STARTED` | Marks the beginning of a sub-agent run |
| `SUBAGENT_FINISHED` | Marks the completion (or suspension) of a sub-agent run |
| `SUBAGENT_ERROR` | Marks the failure of a sub-agent run |

### Lifecycle Methods

```go
func (e *EventEmitter) SubagentStarted(subagentRunID, name string, opts ...events.SubagentStartedOption) error
func (e *EventEmitter) SubagentFinished(subagentRunID string, opts ...events.SubagentFinishedOption) error
func (e *EventEmitter) SubagentError(subagentRunID, message string, opts ...events.SubagentErrorOption) error
```

### Exported Event Types and Constructors

The underlying exported types are the canonical SDK types in the `events`
package:

| Type / Constructor | Description |
|--------------------|-------------|
| `events.EventTypeSubagentStarted` / `events.EventTypeSubagentFinished` / `events.EventTypeSubagentError` | Event-type constants (`events.EventType`) |
| `events.SubagentStartedEvent` / `events.NewSubagentStartedEvent(subagentRunID, name string, opts ...events.SubagentStartedOption) *events.SubagentStartedEvent` | Event + constructor for `SUBAGENT_STARTED` |
| `events.SubagentFinishedEvent` / `events.NewSubagentFinishedEvent(subagentRunID string, opts ...events.SubagentFinishedOption) *events.SubagentFinishedEvent` | Event + constructor for `SUBAGENT_FINISHED` |
| `events.SubagentErrorEvent` / `events.NewSubagentErrorEvent(subagentRunID, message string, opts ...events.SubagentErrorOption) *events.SubagentErrorEvent` | Event + constructor for `SUBAGENT_ERROR` |
| `events.SubagentStartedOption` / `events.SubagentFinishedOption` / `events.SubagentErrorOption` | Option function types for each event |
| `events.SubagentFinishedOutcome` | Discriminates success vs. suspended outcomes (carries `Type` and `InterruptIDs`) |
| `events.SubagentFinishedOutcomeType` | String enum type for the outcome variant |
| `events.SubagentFinishedOutcomeTypeSuccess` / `events.SubagentFinishedOutcomeTypeSuspended` | Outcome-type constants (`"success"` / `"suspended"`) |

All three event types implement the `events.Event` interface and provide
`Validate()` and `ToJSON()` methods.

### SubagentStarted Options

| Option | Description |
|--------|-------------|
| `events.WithSubagentDescription(desc string)` | Sets the description on a `SUBAGENT_STARTED` event |
| `events.WithParentSubagentRunID(id string)` | Sets the parent sub-agent run ID |
| `events.WithParentToolCall(parentToolCallID, parentMessageID string)` | Sets the parent tool call ID and message ID |

### SubagentFinished Options

| Option | Description |
|--------|-------------|
| `events.WithSubagentResult(result any)` | Sets the result payload |
| `events.WithSubagentSuccessOutcome()` | Marks the outcome as `success` |
| `events.WithSubagentSuspendedOutcome(interruptIDs []string)` | Marks the outcome as `suspended` with the given interrupt IDs |

`events.SubagentFinishedOutcome` discriminates between success and suspended
outcomes. Use `events.WithSubagentSuccessOutcome()` for normal completion and
`events.WithSubagentSuspendedOutcome(interruptIDs)` when the sub-agent paused
on an interrupt. The suspended outcome carries `interruptIds` (not
`interrupts`) in the wire JSON; the success outcome omits the field entirely
(the TypeScript schema parses the outcome as a strict discriminated union).

### SubagentError Options

| Option | Description |
|--------|-------------|
| `events.WithSubagentErrorCode(code string)` | Sets the error code |

### Stream Attribution with ForSubagent

```go
func (e *EventEmitter) ForSubagent(subagentRunID string) *EventEmitter
```

`ForSubagent` returns a new `EventEmitter` sharing the same output channel and
context as `e`, but wrapping every emitted event with a `subagentRunId` field
for stream attribution. Events emitted through the returned emitter carry
`subagentRunId` in their JSON payload so frontends can attribute streamed text,
tool calls, state deltas, etc. to the emitting sub-agent.

The returned emitter is lightweight — it does not allocate a new channel or
goroutine. **Lifecycle events (`SubagentStarted`/`Finished`/`Error`) should be
emitted through the original emitter, not the sub-agent wrapper**, since they
carry `subagentRunId` as their own dedicated field.

### Example: Sub-agent Lifecycle

```go
package main

import (
	"context"
	"iter"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func subagentAgent(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
	ch := make(chan events.Event, 64)
	emitter := agui.NewEventEmitterWithContext(ctx, ch)
	go func() {
		defer close(ch)
		emitter.RunStarted(input.ThreadID, input.RunID)

		// Delegate to a sub-agent.
		subRunID := emitter.GenerateMessageID()
		_ = emitter.SubagentStarted(subRunID, "researcher",
			events.WithSubagentDescription("Looks up facts"))

		// Stream the sub-agent's output through an attributed emitter.
		subEmitter := emitter.ForSubagent(subRunID)
		msgID := emitter.GenerateMessageID()
		role := "assistant"
		_ = subEmitter.TextMessageStart(msgID, &role)
		_ = subEmitter.TextMessageContent(msgID, "Found 3 relevant sources...")
		_ = subEmitter.TextMessageEnd(msgID)

		// Mark the sub-agent as complete (use the original emitter).
		_ = emitter.SubagentFinished(subRunID,
			events.WithSubagentSuccessOutcome())

		// Continue with the root agent's response.
		rootMsgID := emitter.GenerateMessageID()
		_ = emitter.TextMessageStart(rootMsgID, &role)
		_ = emitter.TextMessageContent(rootMsgID, "Based on the research...")
		_ = emitter.TextMessageEnd(rootMsgID)

		_ = emitter.RunFinishedWithOptions(input.ThreadID, input.RunID)
	}()
	return agui.ChanToIter(ctx, ch)
}

func main() {
	_ = agui.AgentFunc(subagentAgent)
}
```

## CUSTOM Events for ADK Part Types

The `aguiadk` bridge translates ADK `genai.Part` variants that have no
built-in AG-UI event into `CUSTOM` events. Clients unaware of these custom
events silently ignore them, which is intended AG-UI behavior.

| ADK Part / Event Field | Custom Event Name | Payload |
|------------------------|-------------------|---------|
| `genai.ExecutableCode` | `executable_code` | `{code, language}` |
| `genai.CodeExecutionResult` | `code_execution_result` | `{outcome, output}` |
| `session.Event.CitationMetadata` | `citation_metadata` | The full `genai.CitationMetadata` struct |
| `session.Event.GroundingMetadata` | `grounding_metadata` | The full `genai.GroundingMetadata` struct |
| `session.Event` model error (root agent) | `model_error` | `{code, message}` |

Server-side built-in tool calls (`genai.ToolCall` / `genai.ToolResponse`,
e.g. Google Search, Code Execution) are translated to standard
`TOOL_CALL_START` / `TOOL_CALL_ARGS` / `TOOL_CALL_END` / `TOOL_CALL_RESULT`
events — not `CUSTOM` events. When `ClientTools` is configured and the
built-in tool name is not in the client set, an `ACTIVITY_SNAPSHOT` is
emitted instead (matching the behavior for server-side `FunctionCall`
parts).

Sub-agent model errors are emitted as `SUBAGENT_ERROR` (not `CUSTOM`) so the
sub-agent lifecycle is balanced. Root-agent model errors are emitted as
`CUSTOM` events rather than `RUN_ERROR` because the ADK runner may retry
transient failures; if the runner eventually returns a Go error, the bridge's
error path emits `RUN_ERROR` as before.

## Token Usage Telemetry

`events.TokenUsage` mirrors the AG-UI `TokenUsageSchema`. `RunFinishedWithUsage` and
`RunErrorWithUsage` emit `RUN_FINISHED` / `RUN_ERROR` events carrying aggregated
token usage telemetry. When usage is empty or nil, they fall back to the plain
event so the wire format stays canonical for runs without telemetry.

> **When to use which:** `RunFinishedWithUsage` is a superset of
> `RunFinishedWithOptions` — it accepts the same options and adds a `usage`
> field. Use it when you have token telemetry; use `RunFinishedWithOptions`
> when you know you don't. The same applies to `RunErrorWithUsage` vs.
> `RunErrorWithOptions`.

```go
// events.TokenUsage is the canonical SDK type (see events package for the
// full struct definition). RunFinishedWithUsage and RunErrorWithUsage use
// events.WithUsage / events.WithErrorUsage options on the canonical
// RunFinishedEvent / RunErrorEvent types.

func AggregateTokenUsage(entries []events.TokenUsage) []events.TokenUsage
func (e *EventEmitter) RunFinishedWithUsage(threadID, runID string, usage []events.TokenUsage, opts ...events.RunFinishedOption) error
func (e *EventEmitter) RunErrorWithUsage(message string, usage []events.TokenUsage, opts ...events.RunErrorOption) error
```

`AggregateTokenUsage` merges entries by `(Provider, Model)`, summing each
token-count field. Entries with nil counts contribute zero to the aggregate.

> **Note:** `RunFinishedWithUsage` and `RunErrorWithUsage` use the canonical
> `events.WithUsage` / `events.WithErrorUsage` options on the canonical
> `events.RunFinishedEvent` / `events.RunErrorEvent` types. They do **not**
> aggregate the usage slice — if you want aggregated totals, call
> `AggregateTokenUsage` yourself before passing the slice to these methods.

## CORS

The handler supports optional cross-origin resource sharing via the `CORS` field
on `Config`. When set, CORS headers are applied and OPTIONS preflight requests
are handled automatically.

```go
type CORSConfig struct {
    // AllowOrigins lists permitted origin URIs. Default: ["*"].
    AllowOrigins []string

    // AllowMethods lists permitted HTTP methods. Default: ["POST", "OPTIONS"].
    AllowMethods []string

    // AllowHeaders lists permitted request headers. Default: ["Content-Type", "Cache-Control"].
    AllowHeaders []string

    // ExposeHeaders lists response headers exposed to the client. Optional.
    ExposeHeaders []string

    // AllowCredentials permits cookies and credentials. Optional.
    AllowCredentials bool

    // MaxAge is the preflight cache duration. Default: 300s.
    MaxAge time.Duration
}
```

### CORSMiddleware

```go
func CORSMiddleware(cfg *CORSConfig) func(http.Handler) http.Handler
```

Returns an `http.Handler` middleware that sets CORS headers and handles OPTIONS
preflight requests. If `cfg` is nil, defaults are applied. This middleware is
applied automatically when `Config.CORS` is set, but can also be used
standalone.

### Example: CORS-Enabled Handler

```go
handler, err := agui.Handler(agui.Config{
    Agent: agent,
    CORS: &agui.CORSConfig{
        AllowOrigins:     []string{"https://myapp.example.com"},
        AllowCredentials: true,
    },
})
if err != nil {
    log.Fatal(err)
}
log.Fatal(http.ListenAndServe(":8080", handler))
```

## Error Handling

The handler and emitter distinguish two classes of errors:

### Transport Errors (`ErrTransport`)

A transport error means the client is gone (disconnected, closed channel). The
producer goroutine should stop emitting. See [EventEmitter > Errors](#errors)
for the `errors.Is` pattern.

### Encoding Errors

An encoding error means a single event is malformed (e.g., contains a value
that cannot be JSON-serialized). The emitter itself only returns transport
errors (channel closed, context cancelled) — it does not perform serialization.
Encoding errors occur downstream in `sseSession.WriteEvent` inside the HTTP
handler, when the SSE writer serializes the event to the wire format. **Any
`WriteEvent` error — including encoding failures — stops the stream:**
`WriteEvent` cancels the request context and returns the error, causing the
handler to call `OnError` (if set) and stop iterating.

### Handler Error Flow

The `agui.Handler` translates errors from the agent's event iterator into
`RUN_ERROR` SSE events:

1. When the agent's iterator yields `(nil, err)`, the handler emits a
   `RUN_ERROR` event carrying `err.Error()` and `input.RunID`, then calls
   `Config.OnError` (if set) and stops the stream.
2. When an SSE write fails (e.g., the client disconnected mid-stream), the
   handler calls `Config.OnError` (if set) and stops the stream. No
   `RUN_ERROR` is emitted because the client is already gone.

The bridge (`aguiadk`) follows the same pattern but additionally attaches
aggregated token usage to `RUN_ERROR` via `RunErrorWithUsage` when telemetry
was collected during the run.

### `OnError` Callback

`Config.OnError` is an optional callback invoked for both iterator errors and
SSE write errors. It is the single hook for logging, metrics, or alerting on
handler-level failures. It does not affect the SSE stream (which has already
stopped by the time it is called).

```go
handler, err := agui.Handler(agui.Config{
    Agent: agent,
    OnError: func(err error) {
        slog.Error("agui handler error", "error", err)
    },
})
```

## Production Notes

### Request Body Limits

`Config.MaxBodySize` defaults to 10 MB (`10 << 20`). Tune this for your
workload — large `RunAgentInput` payloads (e.g., with embedded context or
file attachments) may exceed the default. Set it explicitly:

```go
agui.Config{
    Agent:      agent,
    MaxBodySize: 50 << 20, // 50 MB
}
```

### SSE Keepalive Behind Proxies

Reverse proxies (nginx, Cloudflare, AWS ALB) often have idle timeouts that
close SSE connections if no data flows for a period. Set
`Config.KeepaliveInterval` to 15-30 seconds so the handler writes SSE ping
frames proactively:

```go
agui.Config{
    Agent:             agent,
    KeepaliveInterval: 15 * time.Second,
}
```

### CORS in Production

When serving browsers from a different origin, configure CORS explicitly.
See [CORS](#cors) for the full `CORSConfig` reference. Key production notes:

- **Credentials + wildcard:** When `AllowCredentials: true` and `AllowOrigins`
  is `["*"]` (the default), the middleware reflects the request's `Origin`
  header and adds `Vary: Origin` instead of sending `*` — browsers reject
  `Access-Control-Allow-Origin: *` with credentials. This is W3C-required
  behavior; the middleware handles it automatically.
- **Specific origins:** For production, list specific origins rather than
  relying on the wildcard:
  ```go
  CORS: &agui.CORSConfig{
      AllowOrigins:     []string{"https://app.example.com"},
      AllowCredentials: true,
  },
  ```

### Client Tool Timeouts

Inline tool mode (`ToolModeInline`) keeps the SSE connection open while the
frontend executes a tool. The agent/bridge code (e.g. `aguiadk.Handler`) waits
up to `Config.ToolTimeout` (default 5 minutes) for the client to POST a result
to `/tool-result`. If the client never responds, the agent goroutine unblocks
with a timeout error and emits `RUN_ERROR`. Note that `agui.Handler` itself
does not consume `ToolMode`/`ToolTimeout`/`ToolResultHandler` — these fields
are stored on `Config` and wired by the bridge. Tune `ToolTimeout` for your UX
— shorter timeouts fail fast but may interrupt legitimate user thinking time.

### Concurrency

`StateManager`, `EventEmitter` (when used with a channel), and
`ToolResultHandler` are safe for concurrent use. (`RunStore` is also
concurrency-safe, but it lives in the `aguiadk` package — see
[`aguiadk`](aguiadk-bridge.md) — not in `agui/`.) The `Agent` interface
contract does not require concurrency safety — each run gets its own goroutine
and channel. If your agent shares mutable state across runs, protect it
yourself.

## Testing

The `testutil` package provides fake implementations of all ADK-Go interfaces
(`FakeLLM`, `FakeAgent`, `FakeSession`, `FakeArtifactService`,
`FakeMemoryService`, `FakeSessionService`, `RunnerBuilder`) for deterministic
testing without external providers. See [testutil](testutil.md) for the full
reference.

### Testing a Generic `agui.Agent`

For agents implemented as `agui.AgentFunc`, test by driving the agent directly
and collecting events from the iterator:

```go
func TestMyAgent(t *testing.T) {
    agent := agui.AgentFunc(myAgent)
    input := types.RunAgentInput{
        ThreadID: "t1",
        RunID:    "r1",
        Messages: []types.Message{{Role: types.RoleUser, Content: "hi"}},
    }
    var got []events.Event
    for ev, err := range agent.Run(context.Background(), input) {
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
        got = append(got, ev)
    }
    // Assert on got: RUN_STARTED, TEXT_MESSAGE_*, RUN_FINISHED, etc.
}
```

### Testing a Bridge Agent

Use `testutil.FakeLLM` to build a deterministic ADK agent, wrap it with
`aguiadk.New`, and drive it the same way:

```go
func TestBridgeAgent(t *testing.T) {
    llm := testutil.NewFakeLLM(
        &model.LLMResponse{Content: genai.NewContentFromText("hi", genai.RoleModel), TurnComplete: true},
    )
    adkAgent, _ := llmagent.New(llmagent.Config{Name: "test", Model: llm})
    bridge, _ := aguiadk.New(aguiadk.Config{Agent: adkAgent, AppName: "test", UserID: "u"})
    defer aguiadk.Stop(bridge)

    input := types.RunAgentInput{ThreadID: "t1", RunID: "r1", Messages: []types.Message{{Role: types.RoleUser, Content: "hi"}}}
    for ev, err := range bridge.Run(context.Background(), input) {
        if err != nil { t.Fatalf("unexpected: %v", err) }
        _ = ev
    }
}
```

Run tests with the race detector: `go test -race ./...`.

## Full Example

A complete program with state management, steps, reasoning, and middleware:

```go
package main

import (
	"context"
	"fmt"
	"iter"
	"log"
	"net/http"
	"time"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func loggingMiddleware(next agui.Agent) agui.Agent {
	return agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		start := time.Now()
		fmt.Printf("[%s] run=%s thread=%s\n", start.Format(time.RFC3339), input.RunID, input.ThreadID)
		return next.Run(ctx, input)
	})
}

func main() {
	sm, err := agui.NewStateManager(map[string]any{
		"messages_processed": 0,
	})
	if err != nil {
		log.Fatal(err)
	}

	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		ch := make(chan events.Event, 64)
		emitter := agui.NewEventEmitter(ch)

		go func() {
			defer close(ch)
			emitter.RunStarted(input.ThreadID, input.RunID)

			// Send initial state.
			emitter.StateSnapshot(sm.Snapshot())

			// Step 1: Think about the response.
			agui.StepTracker(emitter, "reasoning", func() error {
				thinkID := emitter.GenerateMessageID()
				reasoning := agui.NewReasoningTracker(emitter, thinkID)
				reasoning.Start("assistant")
				reasoning.Content("Analyzing the user's request...")
				reasoning.End()
				return nil
			})

			// Step 2: Generate the response.
			agui.StepTracker(emitter, "responding", func() error {
				msgID := emitter.GenerateMessageID()
				role := "assistant"
				emitter.TextMessageStart(msgID, &role)
				emitter.TextMessageContent(msgID, "Here is my response based on careful reasoning.")
				emitter.TextMessageEnd(msgID)
				return nil
			})

			// Update state.
			patch := []events.JSONPatchOperation{
				{Op: "replace", Path: "/messages_processed", Value: 1},
			}
			sm.Apply(patch)
			emitter.StateDelta(patch)

			emitter.RunFinishedWithOptions(input.ThreadID, input.RunID)
		}()

		return agui.ChanToIter(ctx, ch)
	})

	handler, err := agui.Handler(agui.Config{
		Agent:       agent,
		Middlewares: []agui.Middleware{loggingMiddleware},
		OnError: func(err error) {
			log.Printf("handler error: %v", err)
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Println("AG-UI server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}
```
