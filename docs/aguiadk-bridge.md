# ADK-Go AG-UI Bridge (`aguiadk/`)

Package `aguiadk` bridges ADK-Go agents to the AG-UI protocol.

## Overview

The `aguiadk` package translates ADK-Go session events into AG-UI events,
enabling any ADK-Go agent to serve AG-UI-compatible frontends like CopilotKit
or AG-UI Vue. It sits between the generic [`agui`](agui-server.md) server
library and the ADK-Go runner, handling:

- **Event translation** -- ADK text, function calls (streaming and
  all-at-once), thoughts, and state deltas are mapped to AG-UI events
- **Session management** -- AG-UI thread IDs are mapped to ADK sessions with
  automatic creation and expiry
- **Client tool proxy** -- AG-UI client tools can be exposed as ADK
  FunctionTools in two modes: NextRun (hand-back via interrupt) and Inline
  (wait for result on the same connection). See [ClientToolset](#clienttoolset)
  and [ProxyToolset](#proxytoolset).
- **Streaming tool calls** -- Progressive `TOOL_CALL_START`/`ARGS`/`END` from
  partial `FunctionCall` parts, with delta fragments from `PartialArgs`
- **HITL runstore & resume** -- Paused runs are persisted in a `RunStore` for
  the interrupt/resume cycle, with atomic claim to prevent double-execution.
  See [RunStore](#runstore).
- **Tool call validation** -- Empty IDs get synthetic IDs; empty names and bad
  JSON args emit corrective `TOOL_CALL_RESULT` events
- **Activity snapshots** -- `tool_use` snapshots on tool execution and
  `approval_request` snapshots on HITL interrupts
- **Suppressed tool mode** -- Tool calls can be mapped to `STATE_DELTA` events
  instead of `TOOL_CALL_*` for generative-UI patterns. See [Suppressed Tool
  Mode](#suppressed-tool-mode).
- **Preset configurations** -- Builders for common AG-UI patterns (agentic
  chat, HITL, generative UI, shared state, inline tools). See
  [Presets](#presets).

## Installation

```bash
go get github.com/ieshan/adk-go-pkg/aguiadk
```

## Quick Start

```go
package main

import (
	"iter"
	"log"
	"net/http"

	"github.com/ieshan/adk-go-pkg/agui"
	"github.com/ieshan/adk-go-pkg/aguiadk"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

func main() {
	myAgent, err := agent.New(agent.Config{
		Name: "greeter",
		Run: func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			return func(yield func(*session.Event, error) bool) {
				content := genai.NewContentFromText("Hello from ADK!", genai.RoleModel)
				yield(&session.Event{Author: "greeter", Content: content}, nil)
			}
		},
	})
	if err != nil {
		log.Fatal(err)
	}

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

    // ClientTools configures client tool handling. When set, the bridge
    // injects per-request client tools via context so the ClientToolset
    // (which must be added to llmagent.Config.Toolsets) can resolve them.
    ClientTools *ClientToolConfig

    // RunStore persists paused runs for the HITL interrupt/resume cycle. If
    // nil and a long-running tool interrupt is encountered, an in-memory
    // RunStore with a 30-minute TTL is created lazily. Callers that want
    // explicit lifecycle control should set this field and call Stop on it
    // when done.
    RunStore *RunStore

    // SuppressToolEvents replaces TOOL_CALL_START/ARGS/END/RESULT events
    // with STATE_DELTA events. When true, ToolToStateMapper is called for
    // each finalized tool call; if it returns non-nil, the patch operations
    // are emitted as a STATE_DELTA instead of tool call events. If the
    // mapper returns nil for a given tool, normal tool call events are
    // emitted. This enables generative-UI patterns where tool calls become
    // state mutations rather than visible tool invocations.
    SuppressToolEvents bool

    // ToolToStateMapper maps a tool call (name + args) to a set of JSON Patch
    // operations to apply as a state delta. Only used when SuppressToolEvents
    // is true. Return nil to emit normal tool call events for this tool.
    ToolToStateMapper ToolToStateMapper
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
| `EmitMessagesSnapshot` | `false` | Emit `MESSAGES_SNAPSHOT` after run completes (encrypted fields scrubbed) |
| `EmitStateSnapshot` | `true` | Emit `STATE_SNAPSHOT` at run start |
| `SessionTimeout` | 20 min | Idle timeout before sessions are cleaned up |
| `ClientTools` | nil | Client tool config (Mode, ResultHandler, Timeout). See [ClientToolset](#clienttoolset). |
| `RunStore` | nil (lazy) | Paused-run store for HITL. If nil, an in-memory store with 30m TTL is created lazily. See [RunStore](#runstore). |
| `SuppressToolEvents` | `false` | Replace `TOOL_CALL_*` with `STATE_DELTA` via `ToolToStateMapper`. See [Suppressed Tool Mode](#suppressed-tool-mode). |
| `ToolToStateMapper` | nil | Maps a tool call to JSON Patch ops; only used when `SuppressToolEvents` is true |

### Dynamic App Name and User ID

For multi-tenant applications, use the `Func` variants to derive values from
the HTTP request (e.g., from headers or JWT claims):

```go
package main

import (
	"iter"
	"log"
	"net/http"

	"github.com/ieshan/adk-go-pkg/agui"
	"github.com/ieshan/adk-go-pkg/aguiadk"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

func main() {
	myAgent, err := agent.New(agent.Config{
		Name: "assistant",
		Run: func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			return func(yield func(*session.Event, error) bool) {
				content := genai.NewContentFromText("Hello!", genai.RoleModel)
				yield(&session.Event{Author: "assistant", Content: content}, nil)
			}
		},
	})
	if err != nil {
		log.Fatal(err)
	}

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
| FunctionCall part (partial) | `TOOL_CALL_START` + one `TOOL_CALL_ARGS` per `PartialArgs[].StringValue` | Streaming deltas; AG-UI clients concatenate deltas |
| FunctionCall part (final, streaming) | `TOOL_CALL_ARGS` (remaining accumulated `fc.Args`) + `TOOL_CALL_END` | Closes the streaming tool call |
| FunctionCall part (non-streaming) | `TOOL_CALL_START` + `TOOL_CALL_ARGS` + `TOOL_CALL_END` | All-at-once with accumulated `fc.Args` |
| FunctionCall part (malformed) | `TOOL_CALL_RESULT` with `{"error": "..."}` | Empty name or bad JSON args emit an error result instead of `TOOL_CALL_*`. Empty ID gets a synthetic ID via `GenerateToolCallID()` and proceeds normally. |
| FunctionResponse part | `TOOL_CALL_RESULT` + `ACTIVITY_SNAPSHOT` (`tool_use`) | Activity snapshot content: `{"text": "Running <name>(<args>)"}` |
| State delta | `STATE_DELTA` | Each key becomes a `replace` operation at `/<key>` |
| (run start) | `RUN_STARTED` | Emitted before the ADK runner starts |
| (run end) | `RUN_FINISHED` | Emitted after the ADK runner completes; `closeStreamedToolCalls()` synthesizes `TOOL_CALL_END` for any streamed calls that never got a final event |
| Long-running tool IDs | `RUN_FINISHED` (with `WithInterruptOutcome`) + `ACTIVITY_SNAPSHOT` (`approval_request`) per pending call | Run ends with interrupts (each carrying `ResponseSchema` and `Message`); paused run saved to `RunStore`. Client resumes with `Resume` entries. |
| (suppressed tool mode) | `STATE_DELTA` | When `SuppressToolEvents` is true and `ToolToStateMapper` returns non-nil, `TOOL_CALL_*` events are replaced with `STATE_DELTA` carrying the mapper's patch ops. Partial events are skipped. |
| (session state) | `STATE_SNAPSHOT` | Emitted at run start if `EmitStateSnapshot` is true |
| (session events) | `MESSAGES_SNAPSHOT` | Emitted at run end if `EmitMessagesSnapshot` is true; `EncryptedValue`/`EncryptedContent` scrubbed |
| Runner error | `RUN_ERROR` | Error message included |

### Text Delta Computation

For streaming (partial) text events, the bridge computes deltas by comparing
each new text with the previously accumulated text. If the new text starts with
the old text, only the new suffix is emitted as `TEXT_MESSAGE_CONTENT`. This
avoids duplicate content when the ADK runner sends cumulative text.

### Streaming Tool Call Delta Computation

For streaming (partial) `FunctionCall` parts, ADK's `streamingResponseAggregator`
marks streamed chunks `Partial=true` and populates `fc.PartialArgs` (the delta
fragments), leaving `fc.Args` nil until the final flush. The bridge emits one
`TOOL_CALL_ARGS` per `fc.PartialArgs[].StringValue` (the delta string), matching
the AG-UI example server's `loop.go` which emits `tc.Function.Arguments`
fragments. AG-UI clients concatenate deltas, so emitting the accumulated
`fc.Args` on each partial would break incremental rendering. The final
non-partial event carries the accumulated `fc.Args` and emits `TOOL_CALL_END`.

### Tool Call Validation

The bridge validates model-emitted tool calls before emitting AG-UI events,
matching the AG-UI example server's `validateToolCalls`:

- **Empty `fc.ID`**: Generates a synthetic ID via `emitter.GenerateToolCallID()`
  and proceeds normally (the SDK rejects empty `toolCallId`).
- **Empty `fc.Name`**: Skips `TOOL_CALL_*` emission and emits a
  `TOOL_CALL_RESULT` with `{"error":"tool call had an empty function name"}`.
- **`fc.Args` not JSON-serializable**: Skips `TOOL_CALL_*` emission and emits a
  `TOOL_CALL_RESULT` with
  `{"error":"tool arguments for \"<name>\" were not valid JSON"}`.
- **Streaming partials**: Validation is deferred until the final (non-partial)
  event.

### Resume and Interrupt Handling

The bridge supports AG-UI's interrupt/resume flow for long-running tools and
human-in-the-loop approval:

1. **Interrupt detection:** When an ADK event carries `LongRunningToolIDs`,
   the bridge:
   - Closes any open text message and streamed tool calls
   - Extracts pending tool call details (ID, name, args) from the last ADK
     event's `FunctionCall` parts
   - Saves a `PausedRun` to the `RunStore` (keyed by `threadID|runID`) with
     the pending calls and current state
   - Builds `Interrupt` events, each carrying the tool call ID, reason
     `"tool_call"`, `ToolCallID`, a `ResponseSchema`
     (`{"type":"object","properties":{"approved":{"type":"boolean"}},"required":["approved"]}`),
     and a `Message` (`"Approve <name>(<args>)?"`)
   - Emits an `ACTIVITY_SNAPSHOT` with type `approval_request` for each
     pending call
   - Emits `RUN_FINISHED` with `events.WithInterruptOutcome(interrupts)`

2. **Resume processing:** On the next `RunAgentInput`, the client sends
   `Resume` entries (one per resolved interrupt). The bridge:
   - `Load`s (non-destructive) the paused run and validates that every pending
     tool call has a resume entry
   - `LoadAndDelete`s (atomic claim) — prevents double-execution on concurrent
     resume
   - Re-emits `TOOL_CALL_START`/`ARGS`/`END` for each pending tool call
   - Parses the resume payload: `approved: false` → emits `TOOL_CALL_RESULT`
     with `{"denied":true,...}`; else emits `TOOL_CALL_RESULT` with the
     approval and an `ACTIVITY_SNAPSHOT` (`tool_use`)
   - Appends `FunctionResponse` events to the ADK session so the agent can
     continue from where it left off

This allows human-in-the-loop workflows where a tool requires user confirmation
or input before proceeding.

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

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
	"github.com/ieshan/adk-go-pkg/aguiadk"
)

func main() {
	ch := make(chan events.Event, 64)
	_ = ch

	emitter := agui.NewEventEmitter(make(chan events.Event, 64))
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

> **Note:** `ProxyToolset` is an advanced feature for inline tool mode where
> the tool list is known at construction time. For per-request client tools
> (where the frontend sends the tool list in each `RunAgentInput`), use
> [`ClientToolset`](#clienttoolset) instead.

## ClientToolset

`ClientToolset` implements `tool.Toolset` and wraps AG-UI client tool
definitions (sent per-request in `RunAgentInput.Tools`) as ADK `FunctionTool`
instances. Per-request tool definitions are injected via context by the bridge
before `runner.Run`, so the same agent can serve different clients with
different tool sets.

```go
type ClientToolset struct { /* unexported */ }

func NewClientToolset() *ClientToolset
```

### ClientToolMode

```go
type ClientToolMode int

const (
    ClientToolModeNextRun ClientToolMode = iota
    ClientToolModeInline
)
```

| Mode | Behavior |
|------|----------|
| `ClientToolModeNextRun` | `IsLongRunning=true`; handler returns `(nil, nil)` so ADK pauses the run and emits `LongRunningToolIDs`. The bridge's interrupt path emits `RUN_FINISHED` with interrupts. The client fulfills the tool calls and starts a new run with the results. |
| `ClientToolModeInline` | `IsLongRunning=false`; handler blocks on `ToolResultHandler.Wait` until the client POSTs a result to `/tool-result`. The SSE connection stays open. Default timeout: 5 minutes. |

### ClientToolConfig

```go
type ClientToolConfig struct {
    Mode          ClientToolMode
    ResultHandler *agui.ToolResultHandler // Required for Inline; auto-created by Handler if nil
    Timeout       time.Duration           // Max wait for inline results. Default: 5 minutes.
}
```

### Usage

Because `llminternal` is internal to ADK-Go, the `ClientToolset` must be added
to `llmagent.Config.Toolsets` at agent construction time — the bridge cannot
inject it into an already-constructed agent. The bridge sets the context key
before `runner.Run` so `ClientToolset.Tools` can read the per-request tool
definitions.

```go
package main

import (
	"log"
	"net/http"

	"github.com/ieshan/adk-go-pkg/agui"
	"github.com/ieshan/adk-go-pkg/aguiadk"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/tool"
)

func main() {
	clientToolset := aguiadk.NewClientToolset()

	// Build your LLM agent and add the ClientToolset to its toolsets.
	myAgent, err := llmagent.New(llmagent.Config{
		Name:     "chat",
		Model:    /* your model */,
		Toolsets: []tool.Toolset{clientToolset},
	})
	if err != nil {
		log.Fatal(err)
	}

	handler, err := aguiadk.Handler(
		aguiadk.Config{
			Agent:       myAgent,
			AppName:     "my-chatbot",
			UserID:      "default-user",
			ClientTools: &aguiadk.ClientToolConfig{Mode: aguiadk.ClientToolModeNextRun},
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

When `ClientToolModeInline` is set, `aguiadk.Handler` automatically mounts the
`/tool-result` endpoint and shares the `ToolResultHandler` between the bridge
and the agui handler so client submissions reach the waiting tool handlers.

### WithClientToolsContext

```go
func WithClientToolsContext(ctx context.Context, tools []types.Tool, emitter *agui.EventEmitter, cfg *ClientToolConfig) context.Context
```

A public helper for tests and integrations that invoke `ClientToolset.Tools`
directly without going through the bridge's `runInternal` (which sets the
context itself via the unexported `WithClientTools`). Production code should
not need this — the bridge handles context injection automatically when
`Config.ClientTools` is set.

## RunStore

`RunStore` is a concurrency-safe in-memory map of paused runs keyed by
`threadID|runID`, with lazy TTL expiry and a bounded size (oldest evicted on
overflow). It is the resume primitive for HITL: a paused run is saved on
interrupt and claimed atomically on resume so two concurrent resumes cannot
both execute the pending tool calls.

```go
type RunStore struct { /* unexported */ }

func NewRunStore() *RunStore               // 30-minute TTL, 1024-entry bound
func NewRunStoreWithTTL(ttl time.Duration) *RunStore
func RunKey(threadID, runID string) string  // returns threadID + "|" + runID
```

### Methods

| Method | Description |
|--------|-------------|
| `Save(key string, run *PausedRun)` | Stores a paused run; copies caller's `Pending` slice and `State` map. Purges expired entries first, then evicts oldest if at capacity. |
| `Load(key string) (*PausedRun, bool)` | Non-destructive peek; for validation before claiming. Expired entries are treated as a miss and deleted. |
| `LoadAndDelete(key string) (*PausedRun, bool)` | Atomic claim — exactly one caller wins. Use this on resume to prevent double-execution. |
| `Delete(key string)` | Explicit removal. |
| `Stop()` | Signals the background cleanup goroutine to exit and waits (with a 5-second timeout). Safe to call multiple times via `sync.Once`. |

### PausedRun

```go
type PausedRun struct {
    ThreadID  string
    RunID     string
    SessionID string
    Pending   []PendingToolCall
    State     map[string]any
}

type PendingToolCall struct {
    ID   string
    Name string
    Args map[string]any
}
```

> **Note:** `RunStore` is deliberately process-local and non-durable. For
> multi-tenant production use, gate the endpoint behind auth and namespace
> keys by the authenticated principal.

If `Config.RunStore` is nil and a long-running tool interrupt is encountered,
the bridge creates an in-memory `RunStore` with a 30-minute TTL lazily.
Callers that want explicit lifecycle control should set this field and call
`Stop` on it when done.

## Suppressed Tool Mode

When `Config.SuppressToolEvents` is true and `Config.ToolToStateMapper`
returns non-nil for a finalized tool call, the bridge emits a `STATE_DELTA`
with the mapper's patch operations instead of `TOOL_CALL_START`/`ARGS`/`END`/
`RESULT` events. Partial events are skipped — only the final (non-partial)
event produces a `STATE_DELTA`. If the mapper returns nil for a given tool,
normal tool call events are emitted.

This enables generative-UI patterns where tool invocations become state
mutations in the UI rather than visible tool calls (matching the AG-UI example
server's `applyRecipeChanges` / `validateToolCallsQuiet` pattern).

```go
type ToolToStateMapper func(toolName string, args map[string]any) []events.JSONPatchOperation
```

### Example

```go
mapper := func(toolName string, args map[string]any) []events.JSONPatchOperation {
	if toolName == "set_theme" {
		return []events.JSONPatchOperation{
			{Op: "replace", Path: "/theme", Value: args["theme"]},
		}
	}
	return nil // fall back to normal tool call events for other tools
}

handler, err := aguiadk.Handler(
	aguiadk.Config{
		Agent:             myAgent,
		AppName:           "my-app",
		SuppressToolEvents: true,
		ToolToStateMapper: mapper,
	},
	agui.Config{},
)
```

## Presets

Preset builders return a `Config` with sensible defaults for common AG-UI
patterns. Each takes a base `Config` (typically providing `Agent` and services)
and overrides only the preset-specific fields, preserving caller-supplied
values.

```go
func AgenticChatPreset(base Config) Config
func GenerativeUIPreset(base Config) Config
func HumanInTheLoopPreset(base Config, autoApprove bool) Config
func SharedStatePreset(base Config, mapper ToolToStateMapper) Config
func InlineToolsPreset(base Config) Config
```

| Preset | ClientTools | RunStore | SuppressToolEvents | Notes |
|--------|-------------|----------|--------------------|-------|
| `AgenticChatPreset` | NextRun | — | — | State snapshots on; 20m session timeout |
| `GenerativeUIPreset` | NextRun | — | — | State snapshots on; structured tool calls over prose |
| `HumanInTheLoopPreset` | NextRun | auto (if `!autoApprove`) | — | 30m session timeout; approval interrupts for consequential actions |
| `SharedStatePreset` | — | — | yes (with `mapper`) | Tool calls become `STATE_DELTA` via mapper; collaborative document editing |
| `InlineToolsPreset` | Inline (5m timeout) | — | — | Keeps SSE connection open for inline tool results; `Handler` mounts `/tool-result` automatically |

### Example: Human-in-the-Loop

```go
base := aguiadk.Config{
	Agent:   myAgent,
	AppName: "approval-agent",
	UserID:  "user-1",
}
cfg := aguiadk.HumanInTheLoopPreset(base, false) // autoApprove=false
handler, err := aguiadk.Handler(cfg, agui.Config{})
```

## Convenience Handler

```go
func Handler(cfg Config, agCfg agui.Config) (http.Handler, error)
```

`Handler` combines `New` (which creates the ADK-to-AG-UI bridge) with
`agui.Handler` (which serves the SSE endpoint) into a single call. It:

1. Pre-populates `agCfg.ToolResultHandler` for inline tool mode (if not set)
2. Shares the `ToolResultHandler` between the bridge (`Config.ClientTools`)
   and the agui handler when `ClientToolModeInline` is set
3. Calls `New(cfg)` to create the bridge agent
4. Sets `agCfg.Agent` to the bridge agent
5. Calls `agui.Handler(agCfg)` to create the HTTP handler
6. If inline tool mode is in use, wraps the handler in a `ServeMux` that also
   mounts `POST /tool-result` (via `agui.ToolResultEndpoint`)

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
	"iter"
	"log"
	"net/http"

	"github.com/ieshan/adk-go-pkg/agui"
	"github.com/ieshan/adk-go-pkg/aguiadk"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

func main() {
	myAgent, err := agent.New(agent.Config{
		Name: "assistant",
		Run: func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			return func(yield func(*session.Event, error) bool) {
				content := genai.NewContentFromText("Hello!", genai.RoleModel)
				yield(&session.Event{Author: "assistant", Content: content}, nil)
			}
		},
	})
	if err != nil {
		log.Fatal(err)
	}

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

## MCP Server Toolsets

```go
func BuildMCPServerToolsets(servers []agui.MCPClientConfig) ([]tool.Toolset, error)
```

`BuildMCPServerToolsets` creates ADK `mcptoolset.Toolset` instances from MCP
server configs. Each config is converted to an MCP transport via
`agui.BuildMCPTransport` and wrapped in ADK-Go's native `mcptoolset.New`.
Add the returned toolsets to `llmagent.Config.Toolsets` at agent construction
time so the ADK runner resolves MCP tools natively.

```go
toolsets, err := aguiadk.BuildMCPServerToolsets([]agui.MCPClientConfig{
    {Type: "http", URL: "https://example.com/mcp", ServerID: "srv1"},
})
if err != nil { /* handle */ }

agent, err := llmagent.New(llmagent.Config{
    Name:     "my-agent",
    Model:    model,
    Toolsets: toolsets,
})
```

See [AG-UI MCP Support](agui-mcp.md) for the full MCP integration guide,
including `MCPMiddleware` and `MCPAppsMiddleware` for the generic AG-UI server.

## Full Example

A complete program with an ADK agent served via AG-UI, including state
snapshots and message history:

```go
package main

import (
	"iter"
	"log"
	"net/http"
	"time"

	"github.com/ieshan/adk-go-pkg/agui"
	"github.com/ieshan/adk-go-pkg/aguiadk"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

func main() {
	// Create a minimal ADK agent.
	myAgent, err := agent.New(agent.Config{
		Name:        "travel-assistant",
		Description: "Helps users plan trips.",
		Run: func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			return func(yield func(*session.Event, error) bool) {
				content := genai.NewContentFromText("I'd be happy to help plan your trip!", genai.RoleModel)
				yield(&session.Event{Author: "travel-assistant", Content: content}, nil)
			}
		},
	})
	if err != nil {
		log.Fatal(err)
	}

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
