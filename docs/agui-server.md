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
			emitter.RunFinished(input.ThreadID, input.RunID)
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
		emitter.RunFinished(input.ThreadID, input.RunID)
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
		emitter.RunFinished(input.ThreadID, input.RunID)
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
```

`EventEmitter` provides typed, ergonomic methods for every AG-UI event type.
Create one by passing a writable channel. All methods return `error` (currently
always `nil` for channel sends, but the signature is future-proof).

### ID Generation

| Method | Description |
|--------|-------------|
| `GenerateMessageID() string` | Returns a unique message ID |
| `GenerateToolCallID() string` | Returns a unique tool call ID |

### Run Lifecycle

| Method | Description |
|--------|-------------|
| `RunStarted(threadID, runID string) error` | Emits `RUN_STARTED` |
| `RunFinished(threadID, runID string) error` | Emits `RUN_FINISHED` |
| `RunError(message string, code *string) error` | Emits `RUN_ERROR` |

### Text Messages

| Method | Description |
|--------|-------------|
| `TextMessageStart(messageID string, role *string) error` | Emits `TEXT_MESSAGE_START` |
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
| `MessagesSnapshot(messages []types.Message) error` | Emits `MESSAGES_SNAPSHOT` |

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
| `ReasoningEncryptedValue(subtype, entityID, encryptedValue string) error` | Emits `REASONING_ENCRYPTED_VALUE` |

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
operations. It is safe for concurrent use.

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

			emitter.RunFinished(input.ThreadID, input.RunID)
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
		emitter.RunFinished(input.ThreadID, input.RunID)
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
				emitter.RunError("tool timed out: "+err.Error(), nil)
				return
			}

			// Use the result.
			msgID := emitter.GenerateMessageID()
			role := "assistant"
			emitter.TextMessageStart(msgID, &role)
			emitter.TextMessageContent(msgID, "The weather is: "+result)
			emitter.TextMessageEnd(msgID)

			emitter.RunFinished(input.ThreadID, input.RunID)
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

| Method | Description |
|--------|-------------|
| `NewToolResultHandler() *ToolResultHandler` | Creates a new handler |
| `Wait(ctx, toolCallID, timeout) (string, error)` | Blocks until a result arrives or timeout |
| `SubmitResult(toolCallID, content string) error` | Delivers a result for a pending call |

### ToolResultEndpoint

```go
func ToolResultEndpoint(handler *ToolResultHandler) http.Handler
```

Returns an `http.Handler` for `POST /tool-result`. Expects a JSON body:

```json
{"toolCallId": "tc_abc123", "content": "{\"temp\": 22}"}
```

## Middleware

```go
type Middleware func(next Agent) Agent

func Chain(middlewares ...Middleware) Middleware
```

Middlewares wrap an `Agent`, returning a new `Agent`. `Chain` composes them
left-to-right: `Chain(a, b, c)(agent)` produces `a(b(c(agent)))`.

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
				emitter.RunError("invalid thread ID", &code)
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
		emitter.RunFinished(input.ThreadID, input.RunID)
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

		emitter.RunFinished(input.ThreadID, input.RunID)
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

		emitter.RunFinished(input.ThreadID, input.RunID)
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

		emitter.RunFinished(input.ThreadID, input.RunID)
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
    ToolMode ToolMode

    // ToolTimeout is the max wait for inline tool results. Default: 5 minutes.
    ToolTimeout time.Duration

    // ToolResultHandler for inline mode. Created automatically if nil
    // and ToolMode is ToolModeInline.
    ToolResultHandler *ToolResultHandler

    // OnError is an optional callback for handler errors.
    OnError func(err error)
}
```

### Handler

```go
func Handler(cfg Config) (http.Handler, error)
```

`Handler` returns an `http.Handler` that:

1. Accepts `POST` requests with a JSON `RunAgentInput` body
2. Sets SSE headers (`Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`)
3. Applies the middleware chain to the agent
4. Iterates the agent's event stream, writing each event as an SSE message
5. On error, emits a `RUN_ERROR` event and calls `OnError` if configured

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

			emitter.RunFinished(input.ThreadID, input.RunID)
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
