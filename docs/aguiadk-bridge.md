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
  FunctionTools in three modes: NextRun (hand-back via interrupt), Inline
  (wait for result on the same connection), and HandBack (clean finish with
  no interrupt — the client receives the tool call and starts a new run with
  the result). See [ClientToolset](#clienttoolset) and
  [ProxyToolset](#proxytoolset).
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

> **Companion docs:** This package builds on the generic [`agui`](agui-server.md)
> server library. Refer to it for [`agui.Handler`](agui-server.md#http-handler),
> [`agui.EventEmitter`](agui-server.md#eventemitter),
> [`agui.ToolResultHandler`](agui-server.md#toolresulthandler-api),
> [`agui.Config`](agui-server.md#config), [CORS](agui-server.md#cors),
> [error handling](agui-server.md#error-handling), and
> [production notes](agui-server.md#production-notes). For MCP integration, see
> [AG-UI MCP Support](agui-mcp.md).

## Table of Contents

- [Overview](#overview)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Config](#config)
- [Event Translation Table](#event-translation-table)
- [Session Management](#session-management)
- [Client Tool Proxy](#client-tool-proxy)
- [ClientToolset](#clienttoolset)
- [RunStore](#runstore)
- [Suppressed Tool Mode](#suppressed-tool-mode)
- [Presets](#presets)
- [Convenience Handler](#convenience-handler)
- [MCP Server Toolsets](#mcp-server-toolsets)
- [Capabilities Inference](#capabilities-inference)
- [Run Envelope](#run-envelope)
- [Remote Agent](#remote-agent)
- [Stop](#stop)
- [Resource Lifecycle](#resource-lifecycle)
- [Full Example](#full-example)

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
    // each finalized tool call; if it returns (ops, true), the patch
    // operations are emitted as a STATE_DELTA instead of tool call events.
    // If the mapper returns (nil, false) for a given tool, normal tool call
    // events are emitted. This enables generative-UI patterns where tool
    // calls become state mutations rather than visible tool invocations.
    SuppressToolEvents bool

    // ToolToStateMapper maps a tool call (name + args) to a set of JSON Patch
    // operations to apply as a state delta and a boolean indicating whether
    // the tool call should be suppressed. Only used when SuppressToolEvents
    // is true. Return false as the second value to emit normal tool call
    // events for this tool. Return (nil, true) to suppress the tool call
    // without emitting any state delta.
    ToolToStateMapper ToolToStateMapper

    // EmitStepEvents controls whether STEP_STARTED/STEP_FINISHED events are
    // emitted around LLM and tool-execution phases. Default: false (off).
    EmitStepEvents *bool

    // MaxIterations caps the number of completed model turns per run. A model
    // turn is counted when an ADK event has Partial=false and
    // TurnComplete=true (the final event of a model response, including
    // function-call responses). If the agent exceeds this without producing a
    // final response, the bridge emits a RUN_ERROR with a descriptive message.
    // Default: 0 (unlimited).
    MaxIterations int

    // CustomEventEmitter is an optional callback invoked after the runner
    // loop completes successfully but before MESSAGES_SNAPSHOT and
    // RUN_FINISHED. It receives the emitter and the count of tool calls
    // made during the run.
    CustomEventEmitter func(emitter *agui.EventEmitter, toolCallCount int) error

    // ApprovalModeFunc derives the approval mode from the HTTP request.
    // When set, the bridge calls it per-request. If it returns true,
    // long-running tools auto-execute (no interrupt); if false, they
    // interrupt for human approval. Requires the HTTP request to be stored
    // in context via WithHTTPRequest (done automatically by Handler).
    ApprovalModeFunc func(r *http.Request) bool

    // EmitStateStatus controls whether the bridge emits STATE_DELTA events
    // with a "status" field at key lifecycle transitions: "running" at
    // start, "awaiting_approval" on interrupt, "done" on success, "error"
    // on failure. Default: false.
    EmitStateStatus bool

    // EmitActivityDeltas controls whether ACTIVITY_DELTA events are emitted
    // during streaming tool calls to progressively update tool_use
    // activities with argument deltas. When enabled, an ACTIVITY_SNAPSHOT
    // is emitted at TOOL_CALL_START time and ACTIVITY_DELTA patches follow
    // each args delta. When disabled (default), ACTIVITY_SNAPSHOT is emitted
    // at tool execution time (FunctionResponse) only.
    EmitActivityDeltas bool

    // Provider names the LLM provider for multimodal content gating. When
    // set, inputContentsToGenaiParts filters content types by provider
    // capability: "openai" receives image, audio, video, and document parts;
    // other non-empty providers get text-only fallback for audio/video/document
    // content (images are always forwarded regardless of provider). Empty
    // means no gating (all content types forwarded). Default: "".
    Provider string
}
```

### Validate

```go
func (c Config) Validate() error
```

`Validate` performs pre-flight validation of the `Config` before the bridge is
constructed. It checks that required fields are set (e.g., `Agent`) and that
mutually exclusive fields are not both populated (e.g., `AppName` vs
`AppNameFunc`, `UserID` vs `UserIDFunc`). `Handler` and `New` call it
internally, but it is exported so callers can validate a config early — for
example, at startup or in tests — before constructing the bridge.

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
| `SuppressToolEvents` | `false` | Replace `TOOL_CALL_*` with `STATE_DELTA` via `ToolToStateMapper`. Mapper returns `(ops, suppress)`; suppress=true emits STATE_DELTA, suppress=false emits normal tool events. See [Suppressed Tool Mode](#suppressed-tool-mode). |
| `ToolToStateMapper` | nil | Maps a tool call to `(JSONPatchOps, suppress bool)`; only used when `SuppressToolEvents` is true |
| `EmitStepEvents` | `false` | Emit `STEP_STARTED`/`STEP_FINISHED` around LLM and tool-execution phases |
| `MaxIterations` | `0` (unlimited) | Cap on completed model turns per run (a turn is counted when an ADK event has `Partial=false` and `TurnComplete=true`); bridge emits `RUN_ERROR` if exceeded |
| `CustomEventEmitter` | nil | Optional callback after the runner loop, before `MESSAGES_SNAPSHOT`/`RUN_FINISHED`. The `toolCallCount` argument excludes suppressed tool calls |
| `ApprovalModeFunc` | nil | Per-request approval mode from the HTTP request; `true` = auto-approve, `false` = interrupt for HITL |
| `EmitStateStatus` | `false` | Emit `STATE_DELTA` with a `status` field at lifecycle transitions (`running`, `awaiting_approval`, `done`, `error`) |
| `EmitActivityDeltas` | `false` | Emit `ACTIVITY_DELTA` during streaming tool calls for progressive `tool_use` updates |
| `Provider` | `""` | LLM provider name for multimodal content gating (`"openai"` gets image/audio/video/document parts; other non-empty providers get text-only fallback for audio/video/document while images are always forwarded; empty = no gating, all content types forwarded) |

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
| Thought part | `REASONING_START` + `REASONING_MESSAGE_START` + `REASONING_MESSAGE_CONTENT` + `REASONING_MESSAGE_END` + `REASONING_END` | Full reasoning sequence per thought. When a thought part carries a `ThoughtSignature`, a `REASONING_ENCRYPTED_VALUE` event is also emitted. |
| FunctionCall part (partial) | `TOOL_CALL_START` + one `TOOL_CALL_ARGS` per `PartialArgs[].StringValue` | Streaming deltas; AG-UI clients concatenate deltas |
| FunctionCall part (final, streaming) | `TOOL_CALL_ARGS` (remaining accumulated `fc.Args`) + `TOOL_CALL_END` | Closes the streaming tool call |
| FunctionCall part (non-streaming) | `TOOL_CALL_START` + `TOOL_CALL_ARGS` + `TOOL_CALL_END` | All-at-once with accumulated `fc.Args` |
| FunctionCall part (malformed) | `TOOL_CALL_RESULT` with `{"error": "..."}` | Empty name or bad JSON args emit an error result instead of `TOOL_CALL_*`. Empty ID gets a synthetic ID via `GenerateToolCallID()` and proceeds normally. |
| FunctionResponse part | `ACTIVITY_SNAPSHOT` (`tool_use`) + `TOOL_CALL_RESULT` | Activity snapshot content: `{"text": "Running <name>(<args>)"}`; snapshot emitted first, then the result |
| State delta | `STATE_DELTA` | Each key becomes a `replace` operation at `/<key>` |
| (run start) | `RUN_STARTED` | Emitted before the ADK runner starts |
| (run end) | `RUN_FINISHED` | Emitted after the ADK runner completes; `closeStreamedToolCalls()` synthesizes `TOOL_CALL_END` for any streamed calls that never got a final event |
| Long-running tool IDs | `RUN_FINISHED` (with `WithInterruptOutcome`) + `ACTIVITY_SNAPSHOT` (`approval_request`) per pending call | Run ends with interrupts (each carrying `ResponseSchema` and `Message`); paused run saved to `RunStore`. Client resumes with `Resume` entries. Skipped when `ApprovalModeFunc` returns `true` (auto-approve). |
| Long-running tool IDs (hand-back mode) | `RUN_FINISHED` (plain, no interrupt) + optional `MESSAGES_SNAPSHOT` | When `ClientTools.Mode` is `ClientToolModeHandBack`, the run ends with a clean finish — no interrupt outcome, no `RunStore` entry. The client receives the tool call and starts a new run with the result. |
| Sub-agent author transition | `SUBAGENT_STARTED` / `SUBAGENT_FINISHED` | Emitted when the ADK event author changes from the root agent to a sub-agent (or back). Streamed events between lifecycle boundaries carry `subagentRunId` for attribution. |
| (suppressed tool mode) | `STATE_DELTA` | When `SuppressToolEvents` is true and `ToolToStateMapper` returns `suppress=true`, `TOOL_CALL_*` events are replaced with `STATE_DELTA` carrying the mapper's patch ops (if non-nil). Partial events are skipped. Suppressed calls never enter `toolCallIDs`, so their `FunctionResponse` is naturally skipped. Non-suppressed calls (`suppress=false`) get normal `TOOL_CALL_*` + `TOOL_CALL_RESULT`. |
| (session state) | `STATE_SNAPSHOT` | Emitted at run start if `EmitStateSnapshot` is true |
| (session events) | `MESSAGES_SNAPSHOT` | Emitted at run end if `EmitMessagesSnapshot` is true; `EncryptedValue`/`EncryptedContent` scrubbed |
| Runner error | `RUN_ERROR` | Error message included; aggregated token usage attached when available |
| (run end with telemetry) | `RUN_FINISHED` (with `usage` field) | When token usage telemetry was collected, `RUN_FINISHED` carries an aggregated `usage` array. Falls back to plain `RUN_FINISHED` when no telemetry is available. |

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

### Server-Side Tool Disambiguation

When `Config.ClientTools` is set and `input.Tools` contains a per-request
client tool list, the bridge distinguishes client tools from server-side
tools by name. Function-call names that appear in `input.Tools` are treated
as client tools and emit the normal `TOOL_CALL_*` event sequence. Function-call
names **not** in that list are treated as server-side tools: instead of
`TOOL_CALL_*` events, they emit an `ACTIVITY_SNAPSHOT` (type `tool_use`) so
the frontend can still observe the invocation. This is significant runtime
behavior — it lets a single agent mix client-provided and server-resident
tools in the same run without the frontend expecting to fulfill server-side
calls.

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
     with `{"denied":true,...}`; else emits an `ACTIVITY_SNAPSHOT` (`tool_use`)
     and then a `TOOL_CALL_RESULT` with the approval
   - Appends `FunctionResponse` parts to the current turn's user message so the
     ADK runner can resume the paused workflow
   - Restores `PausedRun.State` on resume by merging it into the runner's state
     delta, with `input.State` taking precedence

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
| `Stop()` | Stop the background cleanup goroutine. Safe to call multiple times (uses `sync.Once`) |

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
`functiontool.Config`. The tool's `InputSchema` is set from the AG-UI tool's
`Parameters` field via JSON schema conversion.

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
    ClientToolModeNextRun  ClientToolMode = iota
    ClientToolModeInline
    ClientToolModeHandBack
)
```

| Mode | Behavior |
|------|----------|
| `ClientToolModeNextRun` | `IsLongRunning=true`; handler returns `(nil, nil)` so ADK pauses the run and emits `LongRunningToolIDs`. The bridge's interrupt path emits `RUN_FINISHED` with interrupts. The client fulfills the tool calls and starts a new run with the results. |
| `ClientToolModeInline` | `IsLongRunning=false`; handler blocks on `ToolResultHandler.Wait` until the client POSTs a result to `/tool-result`. The SSE connection stays open. Default timeout: 5 minutes. |
| `ClientToolModeHandBack` | `IsLongRunning=true`; handler returns `(nil, nil)` so ADK pauses the run. The bridge emits a plain `RUN_FINISHED` (no interrupt outcome) with optional `MESSAGES_SNAPSHOT`. No `RunStore` entry is saved and no interrupt schema is emitted — a clean finish that the client interprets as "your turn." |

### ClientToolConfig

```go
type ClientToolConfig struct {
    Mode          ClientToolMode
    ResultHandler *agui.ToolResultHandler // Required for Inline; auto-created by Handler if nil
    Timeout       time.Duration           // Max wait for inline results. Default: 5 minutes.
}
```

### Usage

ADK agents receive their toolsets at construction time (`llmagent.Config.Toolsets`),
so `ClientToolset` must be added there — the bridge cannot inject it into an
already-constructed agent. Instead, the bridge injects the per-request tool
definitions via context before `runner.Run`, and `ClientToolset.Tools` reads
them at runtime. The same agent instance can therefore serve different clients
with different tool sets on each request.

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
`aguiadk.Handler` routes `/tool-result` by path suffix (matching any path
ending in `/tool-result`), so it works correctly when mounted at sub-paths
like `/api/agent/`. The examples that mount at `/api/agent` therefore work
for inline mode as well.

### WithClientToolsContext

```go
func WithClientToolsContext(ctx context.Context, tools []types.Tool, emitter *agui.EventEmitter, cfg *ClientToolConfig) context.Context
```

A public helper for tests and integrations that invoke `ClientToolset.Tools`
directly without going through the bridge's `runInternal` (which sets the
context itself via the unexported `WithClientTools`). It now propagates
`cfg.ResultHandler` into the context, so it can be used for inline-mode tests
that need the tool result handler wired up. Production code should not need
this — the bridge handles context injection automatically when
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
func NewRunStoreWithMaxEntries(ttl time.Duration, maxEntries int) *RunStore
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
returns `(ops, true)` for a finalized tool call, the bridge emits a `STATE_DELTA`
with the mapper's patch operations instead of `TOOL_CALL_START`/`ARGS`/`END`/
`RESULT` events. Partial events are skipped — only the final (non-partial)
event produces a `STATE_DELTA`. If the mapper returns `(nil, false)` for a
given tool, normal tool call events are emitted (including `TOOL_CALL_RESULT`
when a `FunctionResponse` arrives). If the mapper returns `(nil, true)`, the
tool call is silently swallowed with no events emitted at all.

Suppressed tool calls never enter the `toolCallIDs` map (the bridge returns
early before populating it), so their `FunctionResponse` parts naturally hit
the "no matching tool call, skip" path — no blanket suppression check is
needed in `emitFunctionResponse`.

This enables generative-UI patterns where tool invocations become state
mutations in the UI rather than visible tool calls (matching the AG-UI example
server's `applyRecipeChanges` / `validateToolCallsQuiet` pattern).

```go
type ToolToStateMapper func(toolName string, args map[string]any) ([]events.JSONPatchOperation, bool)
```

### Example

```go
mapper := func(toolName string, args map[string]any) ([]events.JSONPatchOperation, bool) {
	if toolName == "set_theme" {
		return []events.JSONPatchOperation{
			{Op: "replace", Path: "/theme", Value: args["theme"]},
		}, true
	}
	return nil, false // fall back to normal tool call events for other tools
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
func HandBackPreset(base Config) Config
func PredictiveStatePreset(base Config) Config
func AgenticGenerativeUIPreset(base Config, mapper ToolToStateMapper) Config
```

| Preset | ClientTools | RunStore | SuppressToolEvents | Notes |
|--------|-------------|----------|--------------------|-------|
| `AgenticChatPreset` | NextRun | — | — | State snapshots on; 20m session timeout |
| `GenerativeUIPreset` | NextRun | — | — | Currently an alias for `AgenticChatPreset` — serves as a semantic marker for generative-UI agents (no extra configuration). `AgenticGenerativeUIPreset` is the one that maps tool calls to `STATE_DELTA` events. |
| `HumanInTheLoopPreset` | NextRun | auto (if `!autoApprove`); `ApprovalModeFunc` set if `autoApprove` | — | 30m session timeout; approval interrupts for consequential actions; `autoApprove=true` sets `ApprovalModeFunc` to always return `true` |
| `SharedStatePreset` | — | — | yes (with `mapper`) | Tool calls become `STATE_DELTA` via mapper; collaborative document editing |
| `InlineToolsPreset` | Inline (5m timeout) | — | — | Keeps SSE connection open for inline tool results; `Handler` mounts `/tool-result` automatically |
| `HandBackPreset` | HandBack | — | — | Ends run with plain `RUN_FINISHED` on client tool invocation; messages snapshot enabled |
| `PredictiveStatePreset` | — | — | — | Activity deltas + step events for ghosted `/_predictive` state streaming |
| `AgenticGenerativeUIPreset` | — | — | yes (with `mapper`) | Tool calls become `STATE_DELTA` via mapper; step events + messages snapshot enabled |

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

```go
cfg := aguiadk.HumanInTheLoopPreset(base, true) // autoApprove=true — tools execute without interrupt
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
6. If inline tool mode is in use, wraps the handler in an `http.HandlerFunc` that also
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

## Capabilities Inference

```go
func InferCapabilities(a agent.Agent, cfg Config) *agui.AgentCapabilities
```

`InferCapabilities` inspects an ADK agent and bridge configuration to produce
an `AgentCapabilities` descriptor for AG-UI discovery. It checks the agent's
immediate sub-agents, the bridge's client-tool configuration, and the HITL/interrupt
configuration to produce an accurate capabilities snapshot. Pass the result to
`agui.Config.Capabilities` so `GET /capabilities` serves it.

### What It Sets

| Capability | Always Set | Condition |
|------------|------------|-----------|
| `Identity` | yes — `Name`, `Type: "adk-go"`, `Description` from the agent | — |
| `Transport` | yes — `Streaming: true` | — |
| `State` | yes — `Snapshots: true`, `Deltas: true` | — |
| `Messages` | yes — `Snapshots` mirrors `EmitMessagesSnapshot`, `StreamingText: true` | — |
| `Tools` | yes — `Supported: true`, `ServerTools: true`, `Streaming: true` | `ClientTools: true` when `Config.ClientTools` is set |
| `Reasoning` | yes — `Supported: true`, `Streaming: true`, `Encrypted: true` | — |
| `HumanInTheLoop` | no | `Interrupts: true` when `ClientTools.Mode == ClientToolModeNextRun` |
| `Activities` | no | `Snapshots: true`, `Deltas: EmitActivityDeltas` when the agent has sub-agents (`len(a.SubAgents()) > 0`) |

Override the returned descriptor if you need finer control (e.g., to suppress a capability you don't want exposed).

```go
caps := aguiadk.InferCapabilities(myAgent, bridgeCfg)
handler, err := aguiadk.Handler(bridgeCfg, agui.Config{Capabilities: caps})
```

## Run Envelope

```go
type RunEnvelope struct {
    ParentRunID    *string          `json:"parentRunId,omitempty"`
    Context        []types.Context  `json:"context,omitempty"`
    ForwardedProps any              `json:"forwardedProps,omitempty"`
}

func WithRunEnvelope(ctx context.Context, env RunEnvelope) context.Context
func RunEnvelopeFrom(ctx context.Context) (env RunEnvelope, ok bool)
func ContextFrom(ctx agent.ReadonlyContext) []types.Context
func ForwardedPropsFrom[T any](ctx agent.ReadonlyContext) (T, bool)
```

`RunEnvelope` carries AG-UI protocol envelope fields (`ParentRunID`, `Context`,
`ForwardedProps`) through Go contexts and ADK agent contexts so they survive
the boundary between the AG-UI HTTP request and the ADK runner. The bridge
attaches the envelope to the context passed to `runner.Run` and also persists
it into the ADK session state under well-known keys so downstream agents,
tools, and callbacks can read it via `RunEnvelopeFrom` or the typed helpers
`ContextFrom` and `ForwardedPropsFrom`.

**Lookup semantics:** `RunEnvelopeFrom` reads **only** from the Go
`context.Context`. `ContextFrom` and `ForwardedPropsFrom` first check the
context envelope; if none is present, they fall back to ADK session state
entries persisted by the bridge under well-known keys (see below).

### Well-Known State Keys

The envelope is persisted into session state under these keys (defined in
`aguiadk/context.go`):

| Key | Type | Contents |
|-----|------|----------|
| `_ag_ui_parent_run_id` | `string` | The `ParentRunID` from the envelope, if set |
| `_ag_ui_context` | `[]types.Context` | The `Context` slice from the envelope |
| `_ag_ui_forwarded_props` | `any` | The `ForwardedProps` from the envelope |

## Remote Agent

```go
type RemoteAgentConfig struct {
    Name                 string
    Description          string
    Endpoint             string
    Client               agui.Agent
    BeforeAgentCallbacks []agent.BeforeAgentCallback
    AfterAgentCallbacks  []agent.AfterAgentCallback
    SubAgents            []agent.Agent
}

func NewRemoteAgent(cfg RemoteAgentConfig) (agent.Agent, error)
```

`NewRemoteAgent` creates an ADK agent that delegates to a remote AG-UI endpoint.
It maps ADK invocation context (user content + session history) into AG-UI
`RunAgentInput.Messages`, streams AG-UI events from the remote endpoint, and
maps a fixed set of AG-UI event types back into ADK `*session.Event` instances.
The handled event types are:

- `TextMessageContent` and `TextMessageEnd` → ADK text parts
- `ToolCallStart` and `ToolCallArgs` → ADK `FunctionCall` parts. Note these
  are split across two ADK events: `ToolCallStart` produces a `FunctionCall`
  with the name but no args, and `ToolCallArgs` produces one with the args but
  no name — downstream consumers must correlate them by tool call ID.
- `StateSnapshot` → ADK state delta
- `RunError` → ADK error

It does **not** handle `TEXT_MESSAGE_START`, `TOOL_CALL_END`, `STATE_DELTA`,
`MESSAGES_SNAPSHOT`, `RUN_FINISHED`, or other event types — those are ignored.
This enables composing remote AG-UI agents as sub-agents within an ADK agent
tree, but the mapping is intentionally limited to the core streaming event
types.

### Field Details

| Field | Required | Description |
|-------|----------|-------------|
| `Name` | yes | ADK agent name |
| `Description` | no | ADK agent description |
| `Endpoint` | yes* | Remote AG-UI SSE URL. Ignored when `Client` is set. |
| `Client` | no | Optional pre-configured `agui.Agent` (e.g., a `*agui.ClientAgent` with custom auth headers, or a fake in tests). When set, `Endpoint` is ignored and this client is used directly. When nil, a `ClientAgent` is created from `Endpoint`. |
| `BeforeAgentCallbacks` | no | Run before the remote call |
| `AfterAgentCallbacks` | no | Run after the remote call completes |
| `SubAgents` | no | Child ADK agents |

\* At least one of `Endpoint` or `Client` must be set.

## Stop

```go
func Stop(a agui.Agent)
```

`Stop` releases resources associated with an agent created by `New` —
specifically, any lazily created `RunStore`. It is safe to call multiple times.
If the agent was not created by `New` (e.g., it's a middleware wrapper), `Stop`
is a no-op. If `Config.RunStore` was provided by the caller, the caller manages
its lifecycle and `Stop` does not stop it. The internal `SessionManager`'s
cleanup goroutine is not stopped by `Stop` — see [Resource Lifecycle](#resource-lifecycle)
for details.

## Resource Lifecycle

The bridge and its collaborators start background goroutines and own resources
that must be released to avoid leaks. This section consolidates the lifecycle
rules.

### What Owns What

| Component | Created By | Background Goroutine | Who Stops It |
|-----------|------------|----------------------|--------------|
| `bridge` | `New(cfg)` | No (per-request goroutines only) | `Stop(bridge)` — stops the lazy RunStore only |
| `SessionManager` | `New(cfg)` (internally) | Yes — cleanup loop every `CleanupInterval` | **Not stopped by `bridge.Stop()`** — see note below |
| `RunStore` (caller-provided) | Caller | Yes — cleanup loop every `ttl/4` | **Caller** — `Stop` does not stop it |
| `RunStore` (lazy) | `bridge.runStoreFor()` on first interrupt | Yes — cleanup loop every `ttl/4` | `Stop(bridge)` stops it |
| `aguiadk.Handler` | `Handler(cfg, agCfg)` | No | Does **not** call `Stop` — caller must arrange this |

> **Note (SessionManager goroutine):** `bridge.Stop()` currently stops only the
> lazily created `RunStore`. The internal `SessionManager`'s cleanup goroutine
> is **not** stopped by `Stop`. For long-running processes this is harmless
> (the goroutine sleeps on a ticker and a `done` channel that is never closed),
> but for tests or short-lived servers that create many bridges, the leaked
> goroutines accumulate. If this matters for your use case, prefer a single
> long-lived bridge instance, or share a `session.Service` across bridge
> instances so the cleanup goroutine is amortized.

### Important: `Handler` Does Not Call `Stop`

`aguiadk.Handler` returns an `http.Handler` but does not wire up bridge
shutdown. For long-running processes this is usually fine (the process exit
reclaims everything), but for tests, short-lived servers, or graceful shutdown
you must call `Stop` yourself:

```go
bridge, err := aguiadk.New(cfg)
if err != nil { /* handle */ }
defer aguiadk.Stop(bridge) // releases the lazy RunStore (SessionManager is not stopped by Stop)

handler, err := agui.Handler(agui.Config{Agent: bridge})
// ... serve ...
```

If you use `aguiadk.Handler` (the convenience entry point) and need graceful
shutdown, capture the bridge first via `New`, then build the handler from it:

```go
bridge, err := aguiadk.New(cfg)
if err != nil { /* handle */ }
defer aguiadk.Stop(bridge)

// Wire inline tool mode if needed.
agCfg := agui.Config{ /* ... */ }
if cfg.ClientTools != nil && cfg.ClientTools.Mode == ClientToolModeInline {
    if cfg.ClientTools.ResultHandler == nil {
        cfg.ClientTools.ResultHandler = agui.NewToolResultHandler()
    }
    agCfg.ToolResultHandler = cfg.ClientTools.ResultHandler
    agCfg.ToolMode = agui.ToolModeInline
}
agCfg.Agent = bridge

handler, err := agui.Handler(agCfg)
if err != nil { /* handle */ }
// ... serve ...
```

### Caller-Provided `RunStore`

When you set `Config.RunStore`, you own its lifecycle. The bridge will not stop
it. This is the recommended pattern for production:

```go
runStore := aguiadk.NewRunStore()
defer runStore.Stop()

cfg := aguiadk.Config{
    Agent:    myAgent,
    RunStore: runStore,
    // ...
}
```

### `SessionManager` (Advanced)

The bridge creates and manages a `SessionManager` internally. You do not need
to interact with it directly unless you are building custom integrations that
share session state across bridge instances. In that case, construct a
`SessionManager` and reuse it — but note that the bridge does not expose a way
to inject one; you would need to share the underlying `session.Service`
instead.

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
