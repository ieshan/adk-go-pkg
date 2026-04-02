# ADK-Go AG-UI Bridge (`aguiadk/`)

Package `aguiadk` bridges ADK-Go agents to the AG-UI protocol.

## Overview

The `aguiadk` package translates ADK-Go session events into AG-UI events,
enabling any ADK-Go agent to serve AG-UI-compatible frontends like CopilotKit
or AG-UI Vue. It sits between the generic [`agui`](agui-server.md) server
library and the ADK-Go runner, handling:

- **Event translation** -- ADK text, function calls, thoughts, and state
  deltas are mapped to AG-UI events
- **Session management** -- AG-UI thread IDs are mapped to ADK sessions with
  automatic creation and expiry
- **Client tool proxy** -- AG-UI client tools can be exposed as ADK
  FunctionTools for inline execution

## Installation

```bash
go get github.com/ieshan/adk-go-pkg/aguiadk
```

## Quick Start

```go
package main

import (
	"log"
	"net/http"

	"github.com/ieshan/adk-go-pkg/agui"
	"github.com/ieshan/adk-go-pkg/aguiadk"
	"google.golang.org/adk/agent"
)

func main() {
	myAgent := agent.New("greeter", nil,
		agent.WithInstruction("You are a friendly greeter."),
	)

	handler, err := aguiadk.Handler(
		aguiadk.Config{
			Agent:   myAgent,
			AppName: "my-chatbot",
			UserID:  "default-user",
		},
		agui.Config{},
	)
	if err != nil {
		log.Fatal(err)
	}

	http.Handle("/api/agent", handler)
	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

## Config

```go
type Config struct {
    // Agent is the root ADK agent. Required.
    Agent agent.Agent

    // AppName is a static application name. Mutually exclusive with AppNameFunc.
    AppName string
    // AppNameFunc derives the application name from the HTTP request.
    // Mutually exclusive with AppName.
    AppNameFunc func(r *http.Request) string

    // UserID is a static user ID. Mutually exclusive with UserIDFunc.
    UserID string
    // UserIDFunc derives the user ID from the HTTP request.
    // Mutually exclusive with UserID.
    UserIDFunc func(r *http.Request) string

    // SessionService is the ADK session service. If nil, an in-memory service is used.
    SessionService session.Service
    // ArtifactService is an optional ADK artifact service.
    ArtifactService artifact.Service
    // MemoryService is an optional ADK memory service.
    MemoryService memory.Service

    // EmitMessagesSnapshot controls whether a MESSAGES_SNAPSHOT event is emitted
    // after the agent run completes. Default: false.
    EmitMessagesSnapshot bool
    // EmitStateSnapshot controls whether a STATE_SNAPSHOT event is emitted
    // at the start of the run. Default: true (nil pointer = true).
    EmitStateSnapshot *bool

    // SessionTimeout is the session manager timeout. Default: 20 minutes.
    SessionTimeout time.Duration
}
```

### Field Details

| Field | Default | Description |
|-------|---------|-------------|
| `Agent` | (required) | The root ADK-Go agent to run |
| `AppName` | `"default"` | Static app name for the ADK runner |
| `AppNameFunc` | nil | Dynamic app name from HTTP request (mutually exclusive with `AppName`) |
| `UserID` | `"anonymous"` | Static user ID for sessions |
| `UserIDFunc` | nil | Dynamic user ID from HTTP request (mutually exclusive with `UserID`) |
| `SessionService` | in-memory | ADK session storage backend |
| `ArtifactService` | nil | Optional artifact storage |
| `MemoryService` | nil | Optional memory service |
| `EmitMessagesSnapshot` | `false` | Emit `MESSAGES_SNAPSHOT` after run completes |
| `EmitStateSnapshot` | `true` | Emit `STATE_SNAPSHOT` at run start |
| `SessionTimeout` | 20 min | Idle timeout before sessions are cleaned up |

### Dynamic App Name and User ID

For multi-tenant applications, use the `Func` variants to derive values from
the HTTP request (e.g., from headers or JWT claims):

```go
package main

import (
	"log"
	"net/http"

	"github.com/ieshan/adk-go-pkg/agui"
	"github.com/ieshan/adk-go-pkg/aguiadk"
	"google.golang.org/adk/agent"
)

func main() {
	myAgent := agent.New("assistant", nil)

	handler, err := aguiadk.Handler(
		aguiadk.Config{
			Agent: myAgent,
			AppNameFunc: func(r *http.Request) string {
				return r.Header.Get("X-App-Name")
			},
			UserIDFunc: func(r *http.Request) string {
				return r.Header.Get("X-User-ID")
			},
		},
		agui.Config{},
	)
	if err != nil {
		log.Fatal(err)
	}

	http.Handle("/api/agent", handler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

## Event Translation Table

The bridge translates ADK session events into AG-UI events as follows:

| ADK Event | AG-UI Event(s) | Notes |
|-----------|---------------|-------|
| Text part (partial) | `TEXT_MESSAGE_START` + `TEXT_MESSAGE_CONTENT` | Delta computed from accumulated text |
| Text part (final) | `TEXT_MESSAGE_CONTENT` + `TEXT_MESSAGE_END` | Closes the message |
| Thought part | `REASONING_START` + `REASONING_MESSAGE_START` + `REASONING_MESSAGE_CONTENT` + `REASONING_MESSAGE_END` + `REASONING_END` | Full reasoning sequence per thought |
| FunctionCall part | `TOOL_CALL_START` + `TOOL_CALL_ARGS` + `TOOL_CALL_END` | Args serialized as JSON |
| State delta | `STATE_DELTA` | Each key becomes a `replace` operation at `/<key>` |
| (run start) | `RUN_STARTED` | Emitted before the ADK runner starts |
| (run end) | `RUN_FINISHED` | Emitted after the ADK runner completes |
| (session state) | `STATE_SNAPSHOT` | Emitted at run start if `EmitStateSnapshot` is true |
| (session events) | `MESSAGES_SNAPSHOT` | Emitted at run end if `EmitMessagesSnapshot` is true |
| Runner error | `RUN_ERROR` | Error message included |

### Text Delta Computation

For streaming (partial) text events, the bridge computes deltas by comparing
each new text with the previously accumulated text. If the new text starts with
the old text, only the new suffix is emitted as `TEXT_MESSAGE_CONTENT`. This
avoids duplicate content when the ADK runner sends cumulative text.

## Session Management

### SessionManager

```go
type SessionManager struct { /* unexported */ }

func NewSessionManager(cfg SessionManagerConfig) *SessionManager
```

`SessionManager` maps AG-UI thread IDs to ADK session IDs. It is safe for
concurrent use.

### SessionManagerConfig

```go
type SessionManagerConfig struct {
    Service         session.Service
    SessionTimeout  time.Duration // Default: 20 minutes
    CleanupInterval time.Duration // Default: 5 minutes
}
```

### Behavior

1. **First request for a thread:** Creates a new ADK session via the configured
   `session.Service`. The session state includes metadata keys
   `_ag_ui_thread_id`, `_ag_ui_app_name`, and `_ag_ui_user_id`.
2. **Subsequent requests:** Looks up the existing session by thread ID and
   refreshes it from the service.
3. **Expiry:** A background goroutine runs every `CleanupInterval` and deletes
   sessions that have been idle longer than `SessionTimeout`.
4. **Concurrency:** Uses double-checked locking with `sync.RWMutex` for safe
   concurrent access.

### Methods

| Method | Description |
|--------|-------------|
| `Resolve(ctx, threadID, appName, userID) (session.Session, error)` | Get or create an ADK session for a thread |
| `Stop()` | Stop the background cleanup goroutine |

The bridge creates and manages the `SessionManager` internally -- you do not
need to interact with it directly unless building custom integrations.

## Client Tool Proxy

### ProxyToolset

```go
type ProxyToolset struct { /* unexported */ }

func NewProxyToolset(
    tools []types.Tool,
    emitter *agui.EventEmitter,
    resultHandler *agui.ToolResultHandler,
    timeout time.Duration,
) (*ProxyToolset, error)
```

`ProxyToolset` wraps AG-UI client tool definitions as ADK `tool.Tool`
instances. When the ADK agent invokes one of these tools, the proxy:

1. Emits `TOOL_CALL_START`, `TOOL_CALL_ARGS`, `TOOL_CALL_END` over SSE
2. Blocks waiting for the client to POST a result to the `ToolResultEndpoint`
3. Returns the result to the ADK agent as a function response

Each proxied tool is created with `IsLongRunning: true` in the ADK
`functiontool.Config`.

### Methods

| Method | Description |
|--------|-------------|
| `Tools() []tool.Tool` | Returns the wrapped ADK tools |

### Usage

```go
package main

import (
	"time"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
	"github.com/ieshan/adk-go-pkg/aguiadk"
)

func main() {
	ch := make(chan interface{}, 64) // placeholder
	_ = ch

	emitter := agui.NewEventEmitter(make(chan interface{}, 64))
	resultHandler := agui.NewToolResultHandler()

	clientTools := []types.Tool{
		{Name: "get_weather", Description: "Get current weather for a city"},
		{Name: "search_web", Description: "Search the web"},
	}

	proxy, err := aguiadk.NewProxyToolset(clientTools, emitter, resultHandler, 5*time.Minute)
	if err != nil {
		panic(err)
	}

	// proxy.Tools() can be added to an ADK agent's tool list.
	_ = proxy.Tools()
}
```

> **Note:** `ProxyToolset` is an advanced feature for inline tool mode. For the
> simpler next-run mode, the bridge emits tool call events and the frontend
> handles execution between runs.

## Convenience Handler

```go
func Handler(cfg Config, agCfg agui.Config) (http.Handler, error)
```

`Handler` combines `New` (which creates the ADK-to-AG-UI bridge) with
`agui.Handler` (which serves the SSE endpoint) into a single call. It:

1. Calls `New(cfg)` to create the bridge agent
2. Sets `agCfg.Agent` to the bridge agent
3. Calls `agui.Handler(agCfg)` to create the HTTP handler

This is the recommended entry point for most applications.

### New

```go
func New(cfg Config) (agui.Agent, error)
```

For advanced use cases where you need to compose the bridge agent with custom
middleware or additional logic, use `New` directly:

```go
package main

import (
	"log"
	"net/http"

	"github.com/ieshan/adk-go-pkg/agui"
	"github.com/ieshan/adk-go-pkg/aguiadk"
	"google.golang.org/adk/agent"
)

func main() {
	myAgent := agent.New("assistant", nil)

	bridgeAgent, err := aguiadk.New(aguiadk.Config{
		Agent:   myAgent,
		AppName: "my-app",
		UserID:  "user-1",
	})
	if err != nil {
		log.Fatal(err)
	}

	handler, err := agui.Handler(agui.Config{
		Agent:       bridgeAgent,
		Middlewares: []agui.Middleware{ /* your middlewares */ },
		OnError: func(err error) {
			log.Printf("error: %v", err)
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	http.Handle("/api/agent", handler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

### WithHTTPRequest

```go
func WithHTTPRequest(ctx context.Context, r *http.Request) context.Context
```

Stores an `*http.Request` in the context so that `AppNameFunc` and
`UserIDFunc` can access it. This is used internally by the handler but is
exported for custom integrations.

## Full Example

A complete program with an ADK agent served via AG-UI, including state
snapshots and message history:

```go
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/ieshan/adk-go-pkg/agui"
	"github.com/ieshan/adk-go-pkg/aguiadk"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/session"
)

func main() {
	// Create an ADK agent with instructions.
	myAgent := agent.New("travel-assistant", nil,
		agent.WithInstruction("You are a helpful travel assistant. Help users plan trips."),
	)

	// Use a custom session service (or nil for in-memory).
	sessSvc := session.InMemoryService()

	// Configure the bridge.
	bridgeCfg := aguiadk.Config{
		Agent:          myAgent,
		AppName:        "travel-planner",
		UserID:         "demo-user",
		SessionService: sessSvc,
		SessionTimeout: 30 * time.Minute,

		EmitMessagesSnapshot: true, // send full message history after each run
		// EmitStateSnapshot defaults to true
	}

	// Configure the AG-UI handler with middleware and error handling.
	aguiCfg := agui.Config{
		OnError: func(err error) {
			log.Printf("AG-UI error: %v", err)
		},
	}

	handler, err := aguiadk.Handler(bridgeCfg, aguiCfg)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/agent", handler)

	log.Println("ADK + AG-UI server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
```
