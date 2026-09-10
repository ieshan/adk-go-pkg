// Package aguiadk bridges ADK-Go agents to the AG-UI protocol.
//
// The bridge translates ADK session events into AG-UI events, enabling
// any ADK-Go agent to serve AG-UI-compatible frontends.
package aguiadk

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"

	"github.com/ieshan/adk-go-pkg/agui"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/artifact"
	"google.golang.org/adk/v2/memory"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

// Config configures the ADK-to-AG-UI bridge.
type Config struct {
	// Agent is the root ADK agent (required).
	Agent agent.Agent

	// AppName is a static application name. Mutually exclusive with AppNameFunc.
	// If neither AppName nor AppNameFunc is set, defaults to "default".
	AppName string
	// AppNameFunc derives the application name from the HTTP request.
	// Mutually exclusive with AppName.
	AppNameFunc func(r *http.Request) string

	// UserID is a static user ID. Mutually exclusive with UserIDFunc.
	// If neither UserID nor UserIDFunc is set, defaults to "anonymous".
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
	// at the start of the run. Default: true.
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
	// When set, the bridge calls it per-request. If it returns true, the
	// bridge skips the approval interrupt and finishes the run normally —
	// the client executes the tool and starts a new run with the result
	// (same as HandBack mode). If false, long-running tools interrupt for
	// human approval. Requires the HTTP request to be stored in context
	// via WithHTTPRequest (done automatically by Handler).
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
	// content (images are always forwarded). Empty means no gating (all content
	// types forwarded). Default: "".
	Provider string
}

// ToolToStateMapper converts a tool call into JSON Patch operations for
// suppressed tool mode. The boolean return value indicates whether the
// tool call should be suppressed (true) or emitted as normal tool call
// events (false). When suppress is true and ops is non-nil, the patch
// operations are emitted as a STATE_DELTA. When suppress is true and ops
// is nil, the tool call is silently swallowed with no events emitted.
type ToolToStateMapper func(toolName string, args map[string]any) ([]events.JSONPatchOperation, bool)

// emitStateSnapshot returns whether state snapshots should be emitted.
func (c Config) emitStateSnapshot() bool {
	if c.EmitStateSnapshot == nil {
		return true // default
	}
	return *c.EmitStateSnapshot
}

// Validate checks the Config for errors.
func (c Config) Validate() error {
	if c.Agent == nil {
		return fmt.Errorf("aguiadk: Agent is required")
	}
	if c.AppName != "" && c.AppNameFunc != nil {
		return fmt.Errorf("aguiadk: AppName and AppNameFunc are mutually exclusive")
	}
	if c.UserID != "" && c.UserIDFunc != nil {
		return fmt.Errorf("aguiadk: UserID and UserIDFunc are mutually exclusive")
	}
	return nil
}

// New creates an agui.Agent that bridges ADK-Go to AG-UI.
// The returned agent translates AG-UI RunAgentInput into ADK runner calls
// and emits AG-UI events from the resulting ADK session events.
//
// If Config.ClientTools is set, the user must also add the ClientToolset
// to their llmagent.Config.Toolsets at agent construction time. Use
// NewClientToolset() to create the instance.
func New(cfg Config) (agui.Agent, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	sessSvc := cfg.SessionService
	if sessSvc == nil {
		sessSvc = session.InMemoryService()
	}

	r, err := runner.New(runner.Config{
		AppName:           cfg.AppName, // may be empty if AppNameFunc is used
		Agent:             cfg.Agent,
		SessionService:    sessSvc,
		ArtifactService:   cfg.ArtifactService,
		MemoryService:     cfg.MemoryService,
		AutoCreateSession: true,
	})
	if err != nil {
		return nil, fmt.Errorf("aguiadk: failed to create runner: %w", err)
	}

	timeout := cfg.SessionTimeout
	if timeout == 0 {
		timeout = 20 * time.Minute
	}

	sm := NewSessionManager(SessionManagerConfig{
		Service:        sessSvc,
		SessionTimeout: timeout,
	})

	b := &bridge{
		cfg:      cfg,
		runner:   r,
		sessMgr:  sm,
		sessSvc:  sessSvc,
		runStore: cfg.RunStore,
	}
	return b, nil
}

// bridge holds the runtime state for the ADK-AG-UI bridge.
type bridge struct {
	cfg          Config
	runner       *runner.Runner
	sessMgr      *SessionManager
	sessSvc      session.Service
	runStore     *RunStore
	runStoreMu   sync.Mutex
	ownsRunStore bool
}

// runStoreFor returns the bridge's RunStore, creating one lazily if none was
// configured. The lazy store is owned by the bridge and is stopped by Stop.
// Callers wanting lifecycle control should set Config.RunStore directly.
func (b *bridge) runStoreFor() *RunStore {
	b.runStoreMu.Lock()
	defer b.runStoreMu.Unlock()
	if b.runStore != nil {
		return b.runStore
	}
	b.runStore = NewRunStore()
	b.ownsRunStore = true
	return b.runStore
}

// Stop releases resources owned by the bridge, including any lazily created
// RunStore. It is safe to call multiple times. If Config.RunStore was provided
// by the caller, the caller manages its lifecycle and Stop does not stop it.
func (b *bridge) Stop() {
	b.runStoreMu.Lock()
	rs := b.runStore
	owns := b.ownsRunStore
	b.runStoreMu.Unlock()
	if owns && rs != nil {
		rs.Stop()
	}
}

// Stop releases resources associated with an agent created by New, including
// any lazily created RunStore. It is safe to call multiple times. If the agent
// was not created by New (e.g., it's a middleware wrapper), Stop is a no-op.
// If Config.RunStore was provided by the caller, the caller manages its
// lifecycle and Stop does not stop it.
func Stop(a agui.Agent) {
	if b, ok := a.(*bridge); ok {
		b.Stop()
	}
}

// resolveAppName returns the app name for the current request.
func (b *bridge) resolveAppName(ctx context.Context) string {
	if b.cfg.AppNameFunc != nil {
		if r, ok := ctx.Value(httpRequestKey{}).(*http.Request); ok {
			return b.cfg.AppNameFunc(r)
		}
	}
	if b.cfg.AppName != "" {
		return b.cfg.AppName
	}
	return "default"
}

// resolveUserID returns the user ID for the current request.
func (b *bridge) resolveUserID(ctx context.Context) string {
	if b.cfg.UserIDFunc != nil {
		if r, ok := ctx.Value(httpRequestKey{}).(*http.Request); ok {
			return b.cfg.UserIDFunc(r)
		}
	}
	if b.cfg.UserID != "" {
		return b.cfg.UserID
	}
	return "anonymous"
}

// resolveApprovalMode returns whether long-running tools should auto-approve
// for the current request. If ApprovalModeFunc is not set, returns false
// (default: require approval).
func (b *bridge) resolveApprovalMode(ctx context.Context) bool {
	if b.cfg.ApprovalModeFunc != nil {
		if r, ok := ctx.Value(httpRequestKey{}).(*http.Request); ok {
			return b.cfg.ApprovalModeFunc(r)
		}
	}
	return false
}

// emitStatusDelta emits a STATE_DELTA setting the "status" field when
// EmitStateStatus is enabled. It is a no-op otherwise.
func (b *bridge) emitStatusDelta(emitter *agui.EventEmitter, status string) {
	if b.cfg.EmitStateStatus {
		_ = emitter.StateDelta([]events.JSONPatchOperation{
			{Op: "replace", Path: "/status", Value: status},
		})
	}
}

// httpRequestKey is a context key for storing the HTTP request.
type httpRequestKey struct{}

// WithHTTPRequest stores an *http.Request in the context so that
// AppNameFunc and UserIDFunc can access it.
func WithHTTPRequest(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, httpRequestKey{}, r)
}

// Run implements the agui.Agent interface.
func (b *bridge) Run(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
	return func(yield func(events.Event, error) bool) {
		// runCtx is cancelled when the consumer stops iterating (via the
		// deferred cancel below) or when the parent ctx is cancelled. Binding
		// the emitter to runCtx ensures runInternal's emit calls unblock
		// promptly instead of leaking a goroutine blocked on a full channel.
		runCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		ch := make(chan events.Event, 64)
		emitter := agui.NewEventEmitterWithContext(runCtx, ch)

		go func() {
			defer close(ch)
			defer func() {
				if r := recover(); r != nil {
					_ = emitter.RunErrorWithOptions(
						fmt.Sprintf("internal panic: %v", r),
						events.WithRunID(input.RunID),
					)
				}
			}()
			b.runInternal(runCtx, input, emitter)
		}()

		for ev, err := range agui.ChanToIter(runCtx, ch) {
			if !yield(ev, err) {
				return
			}
		}
	}
}

// runInternal performs the actual bridge logic, emitting events via the emitter.
func (b *bridge) runInternal(ctx context.Context, input types.RunAgentInput, emitter *agui.EventEmitter) {
	appName := b.resolveAppName(ctx)
	userID := b.resolveUserID(ctx)

	// Resolve or create an ADK session for this AG-UI thread.
	adkSession, err := b.sessMgr.Resolve(ctx, input.ThreadID, appName, userID)
	if err != nil {
		_ = emitter.RunErrorWithOptions(
			fmt.Sprintf("session resolve failed: %v", err),
			events.WithRunID(input.RunID),
		)
		return
	}

	// Emitter errors from event methods are intentionally not checked for
	// content events — a closed channel means the client disconnected and
	// there is no one to receive the events. Lifecycle events (RunStarted,
	// RunFinished) check errors since they indicate structural problems.

	// Emit RUN_STARTED.
	if err = emitter.RunStarted(input.ThreadID, input.RunID); err != nil {
		return
	}
	b.emitStatusDelta(emitter, "running")

	// Emit STATE_SNAPSHOT if configured.
	if b.cfg.emitStateSnapshot() {
		snapshot := sessionStateToMap(adkSession.State())
		_ = emitter.StateSnapshot(snapshot)
	}

	// Convert the full inbound message history to genai.Content for the
	// current turn and prior session events for multi-turn context. This
	// preserves prior user/assistant/system/developer messages and converts
	// RoleTool messages to FunctionResponse parts.
	msg, priorEvents, err := convertInboundMessages(input.Messages, b.cfg.Provider)
	if err != nil {
		_ = emitter.RunErrorWithOptions(
			fmt.Sprintf("inbound message conversion failed: %v", err),
			events.WithRunID(input.RunID),
		)
		return
	}

	// Seed prior events into the session so multi-turn history survives.
	// Only seed when the session is freshly created (has no events yet) to
	// avoid duplicating history on subsequent runs within the same thread.
	if len(priorEvents) > 0 && adkSession.Events().Len() == 0 {
		for _, ev := range priorEvents {
			if appendErr := b.sessSvc.AppendEvent(ctx, adkSession, ev); appendErr != nil {
				_ = emitter.RunErrorWithOptions(
					fmt.Sprintf("session history seed failed: %v", appendErr),
					events.WithRunID(input.RunID),
				)
				return
			}
		}
	}

	// Process resume entries: re-emit the paused run's tool proposals, settle
	// each against the user's approval decision, and include FunctionResponse
	// parts in the user message so the ADK runner's buildResumeResponses
	// detects them and calls wf.Resume instead of wf.Run.
	// savedState captures the paused run's session state for restoration
	// on resume. It is nil for non-resume runs.
	var savedState map[string]any

	if len(input.Resume) > 0 {
		key := RunKey(input.ThreadID, input.RunID)
		store := b.runStoreFor()

		// Peek (non-destructive) so a malformed/partial resume can be retried.
		saved, ok := store.Load(key)
		if !ok {
			_ = emitter.RunErrorWithOptions(
				"cannot resume: no paused run found for this thread/run "+
					"(it may have expired, already been resumed, or the server restarted)",
				events.WithRunID(input.RunID),
			)
			return
		}

		approvals := approvalsFromResume(input.Resume)
		// Validate: every pending tool call needs an explicit decision.
		undecided := 0
		for _, p := range saved.Pending {
			if _, decided := approvals[p.ID]; !decided {
				undecided++
			}
		}
		if undecided > 0 {
			msg := "resume entries do not match any pending tool call for this run"
			if undecided < len(saved.Pending) {
				// Some but not all are undecided — identify the first one.
				for _, p := range saved.Pending {
					if _, decided := approvals[p.ID]; !decided {
						msg = fmt.Sprintf("resume did not address pending tool call %q", p.ID)
						break
					}
				}
			}
			_ = emitter.RunErrorWithOptions(msg, events.WithRunID(input.RunID))
			return
		}

		// Claim atomically so two concurrent resumes cannot both execute.
		if _, claimed := store.LoadAndDelete(key); !claimed {
			_ = emitter.RunErrorWithOptions(
				"cannot resume: the paused run was claimed by a concurrent resume",
				events.WithRunID(input.RunID),
			)
			return
		}
		savedState = saved.State

		// Re-surface the proposals in this new stream so a client rendering
		// tool cards has the call to attach the result to — the original
		// proposal was emitted in the prior (interrupted) response.
		for _, p := range saved.Pending {
			argsJSON, _ := json.Marshal(p.Args)
			_ = emitter.ToolCallStart(p.ID, p.Name, nil)
			_ = emitter.ToolCallArgs(p.ID, string(argsJSON))
			_ = emitter.ToolCallEnd(p.ID)
			// For approved calls, emit a tool_use activity snapshot (matching
			// the example server's settlePendingToolCalls.
			// Denied calls get no tool_use snapshot — they don't execute.
			if approvals[p.ID] {
				_ = emitter.ActivitySnapshot(
					emitter.GenerateMessageID(), "tool_use",
					map[string]any{"text": fmt.Sprintf("Running %s(%s)", p.Name, string(argsJSON))},
					nil,
				)
			}

			// Settle: emit TOOL_CALL_RESULT reflecting the user's decision.
			msgID := "result-" + p.ID
			if approvals[p.ID] {
				_ = emitter.ToolCallResult(msgID, p.ID, string(argsJSON))
			} else {
				_ = emitter.ToolCallResult(msgID, p.ID,
					`{"denied":true,"reason":"user did not approve this tool call"}`)
			}
		}

		// Build FunctionResponse parts so the ADK runner resumes the paused
		// workflow. Approved calls carry the original args as the result so
		// the tool can execute; denied calls carry a denial marker.
		var resumeParts []*genai.Part
		for _, p := range saved.Pending {
			resp := map[string]any{"result": p.Args}
			if !approvals[p.ID] {
				resp = map[string]any{"denied": true, "reason": "user did not approve this tool call"}
			}
			resumeParts = append(resumeParts, &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					ID:       p.ID,
					Name:     p.Name,
					Response: resp,
				},
			})
		}
		if msg == nil {
			msg = &genai.Content{Role: "user"}
		}
		msg.Parts = append(resumeParts, msg.Parts...)
	}

	// Build run options from input.State, merged with the paused run's
	// saved state (if resuming). The saved state was captured at interrupt
	// time and restores any session state that may have changed between the
	// pause and resume. input.State takes precedence (the resume request's
	// state is the most recent user-supplied value).
	var runOpts []runner.RunOption
	mergedState := map[string]any{}
	if len(savedState) > 0 {
		for k, v := range savedState {
			mergedState[k] = v
		}
	}
	if input.State != nil {
		if stateMap, ok := input.State.(map[string]any); ok {
			for k, v := range stateMap {
				mergedState[k] = v
			}
		} else {
			if data, err := json.Marshal(input.State); err == nil {
				var stateMap map[string]any
				if json.Unmarshal(data, &stateMap) == nil {
					for k, v := range stateMap {
						mergedState[k] = v
					}
				}
			}
		}
	}
	if len(mergedState) > 0 {
		runOpts = append(runOpts, runner.WithStateDelta(mergedState))
	}

	// Propagate AG-UI protocol envelope fields (ParentRunID, Context,
	// ForwardedProps) into the runner context and session state so downstream
	// agents, tools, and callbacks can read them via RunEnvelopeFrom or the
	// typed helpers.
	envelope := RunEnvelope{
		ParentRunID:    input.ParentRunID,
		Context:        input.Context,
		ForwardedProps: input.ForwardedProps,
	}
	if envelope.ParentRunID != nil || len(envelope.Context) > 0 || envelope.ForwardedProps != nil {
		ctx = WithRunEnvelope(ctx, envelope)
		envelopeState := map[string]any{}
		if envelope.ParentRunID != nil {
			envelopeState[stateKeyAGUIParentRunID] = *envelope.ParentRunID
		}
		if len(envelope.Context) > 0 {
			envelopeState[stateKeyAGUIContext] = envelope.Context
		}
		if envelope.ForwardedProps != nil {
			envelopeState[stateKeyAGUIForwardedProps] = envelope.ForwardedProps
		}
		runOpts = append(runOpts, runner.WithStateDelta(envelopeState))
	}

	// Run the ADK agent.
	runCfg := agent.RunConfig{
		StreamingMode: agent.StreamingModeSSE,
	}

	// Inject client tools into context for ClientToolset.Tools to read.
	if b.cfg.ClientTools != nil {
		timeout := b.cfg.ClientTools.Timeout
		if timeout == 0 {
			timeout = 5 * time.Minute
		}
		ctx = WithClientTools(ctx, clientToolsCtx{
			tools:         input.Tools,
			emitter:       emitter,
			resultHandler: b.cfg.ClientTools.ResultHandler,
			mode:          b.cfg.ClientTools.Mode,
			timeout:       timeout,
		})
	}

	emitSteps := b.cfg.EmitStepEvents != nil && *b.cfg.EmitStepEvents

	// Build the client-delegated tool name set from input.Tools so the
	// translator can disambiguate server-side tools from client-delegated
	// tools. When ClientTools is configured and the client declared
	// tools via input.Tools, function calls NOT in this set are server-side
	// and emit ACTIVITY_SNAPSHOT instead of TOOL_CALL_START/ARGS/END.
	var clientToolNames map[string]struct{}
	if b.cfg.ClientTools != nil && len(input.Tools) > 0 {
		clientToolNames = make(map[string]struct{}, len(input.Tools))
		for _, t := range input.Tools {
			if t.Name != "" {
				clientToolNames[t.Name] = struct{}{}
			}
		}
	}
	translator := newEventTranslatorWithClientTools(
		emitter, b.cfg.SuppressToolEvents, b.cfg.ToolToStateMapper,
		emitSteps, b.cfg.EmitActivityDeltas, b.cfg.Agent.Name(), clientToolNames,
	)

	// turnCount tracks completed model turns (non-partial events with
	// TurnComplete=true) for MaxIterations enforcement. Only the final event
	// of a model response sets TurnComplete, so this counts one event per turn
	// regardless of how many function-call or function-response events the
	// turn produces.
	var turnCount int
	for adkEvent, err := range b.runner.Run(ctx, userID, adkSession.ID(), msg, runCfg, runOpts...) {
		if err != nil {
			translator.closeOpenMessage()
			translator.closeOpenSubagent()
			translator.closeOpenStep()
			translator.closeStreamedToolCalls()
			b.emitStatusDelta(emitter, "error")
			_ = emitter.RunErrorWithUsage(
				fmt.Sprintf("agent error: %v", err),
				translator.collectedUsage(),
				events.WithRunID(input.RunID),
			)
			return
		}
		if adkEvent == nil {
			continue
		}

		translator.translate(adkEvent)

		if !adkEvent.Partial && adkEvent.TurnComplete {
			turnCount++
			if b.cfg.MaxIterations > 0 && turnCount > b.cfg.MaxIterations {
				translator.closeOpenMessage()
				translator.closeOpenSubagent()
				translator.closeOpenStep()
				translator.closeStreamedToolCalls()
				b.emitStatusDelta(emitter, "error")
				if b.cfg.EmitMessagesSnapshot {
					refreshed, rerr := b.sessMgr.Resolve(ctx, input.ThreadID, appName, userID)
					if rerr == nil {
						msgs := sessionEventsToMessages(refreshed.Events())
						_ = emitter.MessagesSnapshot(msgs)
					}
				}
				_ = emitter.RunErrorWithUsage(
					fmt.Sprintf("agent did not converge within %d model turns", b.cfg.MaxIterations),
					translator.collectedUsage(),
					events.WithRunID(input.RunID),
				)
				return
			}
		}

		// Check for long-running tool interrupts.
		if len(adkEvent.LongRunningToolIDs) > 0 {
			// Hand-back mode: plain RUN_FINISHED with optional MESSAGES_SNAPSHOT,
			// no interrupt outcome, no RunStore save.
			if b.cfg.ClientTools != nil && b.cfg.ClientTools.Mode == ClientToolModeHandBack {
				translator.closeOpenMessage()
				translator.closeOpenStep()
				translator.closeStreamedToolCalls()
				if b.cfg.EmitMessagesSnapshot {
					refreshed, rerr := b.sessMgr.Resolve(ctx, input.ThreadID, appName, userID)
					if rerr == nil {
						msgs := sessionEventsToMessages(refreshed.Events())
						_ = emitter.MessagesSnapshot(msgs)
					}
				}
				b.emitStatusDelta(emitter, "done")
				_ = emitter.RunFinishedWithUsage(input.ThreadID, input.RunID, translator.collectedUsage())
				return
			}

			autoApprove := b.resolveApprovalMode(ctx)
			if autoApprove {
				// Auto-approve: skip the approval interrupt and finish the
				// run. The ADK runner has parked on LongRunningToolIDs and
				// the iterator ends here. The client executes the tool and
				// starts a new run with the result (same as HandBack mode).
				translator.closeOpenMessage()
				translator.closeOpenStep()
				translator.closeStreamedToolCalls()
				if b.cfg.EmitMessagesSnapshot {
					refreshed, rerr := b.sessMgr.Resolve(ctx, input.ThreadID, appName, userID)
					if rerr == nil {
						msgs := sessionEventsToMessages(refreshed.Events())
						_ = emitter.MessagesSnapshot(msgs)
					}
				}
				b.emitStatusDelta(emitter, "done")
				_ = emitter.RunFinishedWithUsage(input.ThreadID, input.RunID, translator.collectedUsage())
				return
			}

			translator.closeOpenMessage()
			translator.closeOpenStep()
			translator.closeStreamedToolCalls()

			pending := extractPendingToolCalls(adkEvent)
			approvalSchema := map[string]any{
				"type":       "object",
				"properties": map[string]any{"approved": map[string]any{"type": "boolean"}},
				"required":   []any{"approved"},
			}

			var interrupts []types.Interrupt
			for _, p := range pending {
				argsJSON, _ := json.Marshal(p.Args)
				_ = emitter.ActivitySnapshot(
					emitter.GenerateMessageID(), "approval_request",
					map[string]any{"text": fmt.Sprintf("Agent wants to call %s with %s — approve?", p.Name, string(argsJSON))},
					nil,
				)
				interrupts = append(interrupts, types.Interrupt{
					ID:             p.ID,
					Reason:         "tool_call",
					Message:        fmt.Sprintf("Approve %s(%s)?", p.Name, string(argsJSON)),
					ToolCallID:     p.ID,
					ResponseSchema: approvalSchema,
				})
			}

			// Persist the paused run so a later resume can re-emit the
			// proposals and settle them against the user's decisions.
			store := b.runStoreFor()
			store.Save(RunKey(input.ThreadID, input.RunID), &PausedRun{
				ThreadID:  input.ThreadID,
				RunID:     input.RunID,
				SessionID: adkSession.ID(),
				Pending:   pending,
				State:     sessionStateToMap(adkSession.State()),
			})

			b.emitStatusDelta(emitter, "awaiting_approval")

			if b.cfg.EmitMessagesSnapshot {
				refreshed, rerr := b.sessMgr.Resolve(ctx, input.ThreadID, appName, userID)
				if rerr == nil {
					msgs := sessionEventsToMessages(refreshed.Events())
					_ = emitter.MessagesSnapshot(msgs)
				}
			}

			_ = emitter.RunFinishedWithUsage(
				input.ThreadID, input.RunID,
				translator.collectedUsage(),
				events.WithInterruptOutcome(interrupts),
			)
			return
		}
	}

	// Close any open text message, step, streamed tool calls, and sub-agent.
	translator.closeOpenMessage()
	translator.closeOpenSubagent()
	translator.closeOpenStep()
	translator.closeStreamedToolCalls()
	b.emitStatusDelta(emitter, "done")

	// Emit custom events if configured.
	if b.cfg.CustomEventEmitter != nil {
		_ = b.cfg.CustomEventEmitter(emitter, len(translator.toolCallDetails))
	}

	// Emit MESSAGES_SNAPSHOT if configured.
	if b.cfg.EmitMessagesSnapshot {
		// Re-fetch the session to get updated events.
		refreshed, err := b.sessMgr.Resolve(ctx, input.ThreadID, appName, userID)
		if err == nil {
			msgs := sessionEventsToMessages(refreshed.Events())
			_ = emitter.MessagesSnapshot(msgs)
		}
	}

	// Emit RUN_FINISHED with aggregated token usage telemetry.
	if err := emitter.RunFinishedWithUsage(input.ThreadID, input.RunID, translator.collectedUsage()); err != nil {
		return
	}
}

// eventTranslator holds per-run state for translating ADK events to AG-UI events.
type eventTranslator struct {
	emitter            *agui.EventEmitter // base emitter for lifecycle events
	activeEmitter      *agui.EventEmitter // current emitter for stream events (base or subagent-attributed)
	currentMsgID       string
	msgOpen            bool
	prevText           string            // accumulated text for computing deltas
	reasoningOpen      bool              // whether a reasoning block is currently open
	currentReasoningID string            // message ID for the open reasoning block
	prevThoughtText    string            // accumulated thought text for computing reasoning deltas
	toolCallIDs        map[string]string // ADK function call ID/name → AG-UI tool call ID
	toolCallDetails    map[string]toolCallDetail
	startedTools       map[string]bool // tool call IDs that have emitted TOOL_CALL_START
	openTools          map[string]bool // tool call IDs with START emitted but END pending
	hadPartialArgs     map[string]bool // tool call IDs that received partial args deltas (skip final full-args re-send)
	suppressTools      bool
	toolToStateMapper  ToolToStateMapper
	emitSteps          bool                // whether to emit STEP_STARTED/STEP_FINISHED events
	stepOpen           bool                // whether a step is currently open
	stepName           string              // name of the currently open step
	emitActivityDeltas bool                // whether to emit ACTIVITY_DELTA during streaming tool calls
	activityMsgIDs     map[string]string   // tool call ID → activity message ID for deltas
	clientTools        map[string]struct{} // when non-nil, only these tool names are client-delegated
	usageEntries       []agui.TokenUsage   // collected token usage telemetry for terminal events
	currentAuthor      string              // ADK event author for the current sub-agent context
	activeSubagentRun  string              // subagentRunId for the currently active sub-agent
	subagentEmitted    bool                // whether SUBAGENT_STARTED has been emitted for currentAuthor
	rootAgentName      string              // name of the root agent, excluded from sub-agent lifecycle
}

// toolCallDetail records the name and args of a tool call so the tool_use
// activity snapshot can be emitted at execution time (FunctionResponse),
// matching the example server's settlePendingToolCalls.
type toolCallDetail struct {
	name string
	args map[string]any
}

func newEventTranslator(emitter *agui.EventEmitter, suppressTools bool, mapper ToolToStateMapper, emitSteps bool, emitActivityDeltas bool, rootAgentName string) *eventTranslator {
	return &eventTranslator{
		emitter:            emitter,
		activeEmitter:      emitter,
		toolCallIDs:        make(map[string]string),
		toolCallDetails:    make(map[string]toolCallDetail),
		startedTools:       make(map[string]bool),
		openTools:          make(map[string]bool),
		hadPartialArgs:     make(map[string]bool),
		suppressTools:      suppressTools,
		toolToStateMapper:  mapper,
		emitSteps:          emitSteps,
		emitActivityDeltas: emitActivityDeltas,
		activityMsgIDs:     make(map[string]string),
		rootAgentName:      rootAgentName,
	}
}

// newEventTranslatorWithClientTools is like newEventTranslator but configures
// client-tool disambiguation. When clientTools is non-nil, function calls
// whose name is NOT in the set are treated as server-side tools: they emit an
// ACTIVITY_SNAPSHOT instead of TOOL_CALL_START/ARGS/END so the client is not
// asked to execute them. Client-delegated tools emit normal TOOL_CALL_* events.
func newEventTranslatorWithClientTools(emitter *agui.EventEmitter, suppressTools bool, mapper ToolToStateMapper, emitSteps bool, emitActivityDeltas bool, rootAgentName string, clientTools map[string]struct{}) *eventTranslator {
	t := newEventTranslator(emitter, suppressTools, mapper, emitSteps, emitActivityDeltas, rootAgentName)
	t.clientTools = clientTools
	return t
}

// translate converts a single ADK event into one or more AG-UI events.
func (t *eventTranslator) translate(ev *session.Event) {
	// Collect token usage telemetry from non-partial events. Partial
	// events carry intermediate counts that the final non-partial event
	// supersedes, so we only record the latter to avoid double counting.
	if !ev.Partial && ev.UsageMetadata != nil {
		t.recordUsage(ev.UsageMetadata, ev.ModelVersion)
	}

	// Handle state delta.
	if len(ev.Actions.StateDelta) > 0 {
		t.emitStateDelta(ev.Actions.StateDelta)
	}

	// Determine whether this event contains function calls or responses
	// for step event management.
	hasFunctionCall := false
	hasFunctionResponse := false
	if ev.Content != nil {
		for _, part := range ev.Content.Parts {
			if part == nil {
				continue
			}
			if part.FunctionCall != nil {
				hasFunctionCall = true
			}
			if part.FunctionResponse != nil {
				hasFunctionResponse = true
			}
		}
	}

	// Manage step events based on ADK event boundaries.
	if t.emitSteps {
		if hasFunctionResponse {
			// Tool execution finished; next LLM step begins.
			t.closeOpenStep()
			t.startStep("llm")
		} else if hasFunctionCall && !ev.Partial {
			// LLM step finished; tool execution begins.
			t.closeOpenStep()
			t.startStep("tools")
		} else if !t.stepOpen && !ev.Partial {
			// First non-partial event starts the first LLM step.
			t.startStep("llm")
		}
	}

	// No content means no message-level events.
	if ev.Content == nil || len(ev.Content.Parts) == 0 {
		return
	}

	// Track sub-agent author transitions and emit SUBAGENT_STARTED/FINISHED
	// lifecycle events so frontends can attribute streamed events to the
	// emitting sub-agent. The root agent author ("", "user", "model",
	// or the configured agent name) does not trigger sub-agent events.
	t.maybeEmitSubagentLifecycle(ev)

	for _, part := range ev.Content.Parts {
		if part == nil {
			continue
		}

		switch {
		case part.Thought && part.Text != "":
			t.emitThought(part, ev.Partial)

		case part.FunctionCall != nil:
			t.closeOpenMessage()
			t.emitFunctionCall(part.FunctionCall, ev.Partial)

		case part.FunctionResponse != nil:
			t.closeOpenMessage()
			t.emitFunctionResponse(part.FunctionResponse)

		case part.Text != "":
			t.emitText(part.Text, ev.Partial, ev.Author)

		case part.FileData != nil:
			// Non-text artifact fallback: surface file URIs as
			// readable text so generated files reach the user instead of
			// being silently dropped.
			t.emitText(fmt.Sprintf("[File: %s](%s)", part.FileData.MIMEType, part.FileData.FileURI), ev.Partial, ev.Author)

		case part.InlineData != nil:
			// Non-text artifact fallback: surface inline binary
			// data as a readable text summary.
			t.emitText(fmt.Sprintf("[Binary data: %s, %d bytes]", part.InlineData.MIMEType, len(part.InlineData.Data)), ev.Partial, ev.Author)
		}
	}
}

// maybeEmitSubagentLifecycle detects sub-agent author transitions from ADK
// events and emits SUBAGENT_FINISHED for the previous sub-agent (if any)
// followed by SUBAGENT_STARTED for the new one. The root agent (identified by
// the configured rootAgentName) and canonical roles ("", "user", "model") do
// not count as sub-agents. When the author transitions back to the root agent,
// the active sub-agent is finished.
func (t *eventTranslator) maybeEmitSubagentLifecycle(ev *session.Event) {
	author := ev.Author
	if t.isRootAuthor(author) {
		if t.activeSubagentRun != "" {
			_ = t.emitter.SubagentFinished(t.activeSubagentRun, agui.WithSubagentName(t.currentAuthor), agui.WithSubagentSuccessOutcome())
			t.activeSubagentRun = ""
			t.currentAuthor = ""
			t.subagentEmitted = false
			// Restore the base emitter so root-agent events are not tagged.
			t.activeEmitter = t.emitter
		}
		return
	}
	if author != t.currentAuthor {
		if t.activeSubagentRun != "" {
			_ = t.emitter.SubagentFinished(t.activeSubagentRun, agui.WithSubagentName(t.currentAuthor), agui.WithSubagentSuccessOutcome())
		}
		t.currentAuthor = author
		t.activeSubagentRun = t.emitter.GenerateToolCallID()
		t.subagentEmitted = false
		// Immediately switch to a sub-agent-attributing emitter so all
		// subsequent stream events (including partials) carry subagentRunId.
		t.activeEmitter = t.emitter.ForSubagent(t.activeSubagentRun)
	}
	if !t.subagentEmitted {
		_ = t.emitter.SubagentStarted(t.activeSubagentRun, author)
		t.subagentEmitted = true
	}
}

// isRootAuthor reports whether the given ADK event author represents the root
// agent or a canonical role rather than a delegated sub-agent.
func (t *eventTranslator) isRootAuthor(author string) bool {
	switch author {
	case "", "user", "model":
		return true
	case t.rootAgentName:
		return true
	default:
		return false
	}
}

// subagentNameForEvent returns the author name to attach to a TEXT_MESSAGE_START
// event for sub-agent attribution. Root authors return "" so the message
// carries no Name field; sub-agent author names are preserved.
func subagentNameForEvent(author, rootAgentName string) string {
	if author == "" || author == "user" || author == "model" || author == rootAgentName {
		return ""
	}
	return author
}

// startStep emits STEP_STARTED and tracks the open step.
func (t *eventTranslator) startStep(name string) {
	_ = t.activeEmitter.StepStarted(name)
	t.stepOpen = true
	t.stepName = name
}

// closeOpenStep emits STEP_FINISHED for the currently open step, if any.
func (t *eventTranslator) closeOpenStep() {
	if t.stepOpen {
		_ = t.activeEmitter.StepFinished(t.stepName)
		t.stepOpen = false
	}
}

// emitText handles text parts, managing the message lifecycle. The author
// parameter is the ADK event author; when it identifies a sub-agent it is
// attached to the TextMessageStart event as the name field so frontends can
// attribute the message to the emitting sub-agent.
func (t *eventTranslator) emitText(text string, partial bool, author string) {
	// Close any open reasoning block before emitting text (protocol:
	// reasoning and text blocks must not overlap).
	t.closeOpenReasoning()

	if !t.msgOpen {
		t.currentMsgID = t.activeEmitter.GenerateMessageID()
		role := new("assistant")
		name := subagentNameForEvent(author, t.rootAgentName)
		_ = t.activeEmitter.TextMessageStartWithID(t.currentMsgID, role, name)
		t.msgOpen = true
		t.prevText = ""
	}

	if partial {
		// Streaming mode: compute delta from previous accumulated text.
		delta := text
		if strings.HasPrefix(text, t.prevText) {
			delta = text[len(t.prevText):]
		}
		if delta != "" {
			_ = t.activeEmitter.TextMessageContent(t.currentMsgID, delta)
		}
		t.prevText += delta
	} else {
		// Final (non-partial) event: emit remaining content and close.
		delta := text
		if strings.HasPrefix(text, t.prevText) {
			delta = text[len(t.prevText):]
		}
		if delta != "" {
			_ = t.activeEmitter.TextMessageContent(t.currentMsgID, delta)
		}
		_ = t.activeEmitter.TextMessageEnd(t.currentMsgID)
		t.msgOpen = false
		t.prevText = ""
	}
}

// closeOpenMessage ends an open text message if one is active, and also
// closes any open reasoning block (defensive — ensures clean state at all
// terminal paths).
func (t *eventTranslator) closeOpenMessage() {
	if t.msgOpen {
		_ = t.activeEmitter.TextMessageEnd(t.currentMsgID)
		t.msgOpen = false
		t.prevText = ""
	}
}

// closeOpenSubagent emits SUBAGENT_FINISHED for any active sub-agent. Called
// at terminal run boundaries so the sub-agent lifecycle is balanced.
func (t *eventTranslator) closeOpenSubagent() {
	if t.activeSubagentRun != "" {
		_ = t.emitter.SubagentFinished(t.activeSubagentRun, agui.WithSubagentName(t.currentAuthor), agui.WithSubagentSuccessOutcome())
		t.activeSubagentRun = ""
		t.currentAuthor = ""
		t.subagentEmitted = false
		t.activeEmitter = t.emitter
	}
}

// closeOpenReasoning ends an open reasoning block if one is active.
func (t *eventTranslator) closeOpenReasoning() {
	if t.reasoningOpen {
		_ = t.activeEmitter.ReasoningMessageEnd(t.currentReasoningID)
		_ = t.activeEmitter.ReasoningEnd(t.currentReasoningID)
		t.reasoningOpen = false
		t.prevThoughtText = ""
		t.currentReasoningID = ""
	}
}

// closeStreamedToolCalls emits TOOL_CALL_END for any tool calls that were
// started during streaming but never received a final (non-partial) event.
// This prevents protocol violations where TOOL_CALL_START is emitted without
// a matching TOOL_CALL_END.
func (t *eventTranslator) closeStreamedToolCalls() {
	for id := range t.openTools {
		_ = t.activeEmitter.ToolCallEnd(id)
		delete(t.openTools, id)
		delete(t.hadPartialArgs, id)
	}
}

// emitFunctionCall translates an ADK FunctionCall to AG-UI tool call events.
//
// In streaming mode (partial=true), ADK's streamingResponseAggregator
// marks streamed chunks Partial=true and populates
// fc.PartialArgs with the delta fragments, leaving fc.Args nil until the final
// flush. We emit TOOL_CALL_START for new calls and one TOOL_CALL_ARGS per
// PartialArg.StringValue (the delta string the client appends), matching the
// example server which emits tc.Function.Arguments fragments.
// TOOL_CALL_END is deferred to the final (non-partial) event.
//
// In non-streaming mode, the final event carries the accumulated fc.Args and
// we emit START + ARGS + END in one shot.
//
// Malformed calls (empty name or invalid JSON args) on non-partial events emit
// a TOOL_CALL_RESULT with an error payload instead of TOOL_CALL_START/ARGS/END,
// matching the example server's validateToolCalls behavior.
func (t *eventTranslator) emitFunctionCall(fc *genai.FunctionCall, partial bool) {
	// Suppressed tool mode: replace tool call events with state deltas.
	// Partial events are buffered (skipped); only the final event produces
	// a STATE_DELTA. If the mapper returns nil for this tool, fall through
	// to normal emission.
	if t.suppressTools && t.toolToStateMapper != nil {
		if partial {
			return
		}
		if ops, suppress := t.toolToStateMapper(fc.Name, fc.Args); suppress {
			if ops != nil {
				_ = t.activeEmitter.StateDelta(ops)
			}
			return
		}
	}

	// Generate or reuse the AG-UI tool call ID. The ADK function call ID is
	// used when available so the client can correlate TOOL_CALL events with
	// interrupts and submit inline tool results using the same ID the tool
	// handler waits on (ctx.FunctionCallID()).
	toolCallID := fc.ID
	if toolCallID == "" {
		toolCallID = t.activeEmitter.GenerateToolCallID()
	}

	// Client/server tool disambiguation: when client tools are configured,
	// tools NOT in the client set are server-side. They execute in the
	// backend, so we emit an ACTIVITY_SNAPSHOT (informing the UI) instead of
	// TOOL_CALL_START/ARGS/END (which would ask the client to execute them).
	// We still record the ID mapping so FunctionResponse correlates correctly
	// and emits a deterministic TOOL_CALL_RESULT.
	if t.clientTools != nil {
		if _, isClient := t.clientTools[fc.Name]; !isClient {
			if fc.ID != "" {
				t.toolCallIDs[fc.ID] = toolCallID
			}
			if fc.Name != "" {
				t.toolCallIDs[fc.Name] = toolCallID
			}
			if fc.Name != "" || fc.Args != nil {
				t.toolCallDetails[toolCallID] = toolCallDetail{name: fc.Name, args: fc.Args}
			}
			if !partial {
				t.emitToolUseActivity(toolCallID, fc.Name, fc.Args)
			}
			return
		}
	}

	// Validate on non-partial events only; partial events may have incomplete data.
	if !partial {
		if fc.Name == "" {
			msgID := t.activeEmitter.GenerateMessageID()
			_ = t.activeEmitter.ToolCallResult(msgID, toolCallID,
				`{"error":"tool call had an empty function name"}`)
			return
		}
		if fc.Args != nil {
			if _, err := json.Marshal(fc.Args); err != nil {
				msgID := t.activeEmitter.GenerateMessageID()
				_ = t.activeEmitter.ToolCallResult(msgID, toolCallID,
					fmt.Sprintf(`{"error":"tool arguments for %q were not valid JSON"}`, fc.Name))
				return
			}
		}
	}

	// Map ADK function call ID and name to AG-UI tool call ID for later
	// correlation with FunctionResponse parts. Record name+args so the
	// tool_use activity snapshot can be emitted at execution time.
	if fc.ID != "" {
		t.toolCallIDs[fc.ID] = toolCallID
	}
	if fc.Name != "" {
		t.toolCallIDs[fc.Name] = toolCallID
	}
	if fc.Name != "" || fc.Args != nil {
		t.toolCallDetails[toolCallID] = toolCallDetail{name: fc.Name, args: fc.Args}
	}

	if partial {
		// Streaming: emit START for new tool calls, then ARGS per PartialArg
		// delta. ADK populates fc.PartialArgs (not fc.Args) on Partials.
		if !t.startedTools[toolCallID] {
			_ = t.activeEmitter.ToolCallStart(toolCallID, fc.Name, nil)
			t.startedTools[toolCallID] = true
			t.openTools[toolCallID] = true
			// Emit an initial ACTIVITY_SNAPSHOT for progressive tool_use tracking.
			if t.emitActivityDeltas {
				actMsgID := t.activeEmitter.GenerateMessageID()
				t.activityMsgIDs[toolCallID] = actMsgID
				_ = t.activeEmitter.ActivitySnapshot(actMsgID, "tool_use",
					map[string]any{"text": fmt.Sprintf("Running %s...", fc.Name)}, nil)
			}
		}
		for _, pa := range fc.PartialArgs {
			if pa.StringValue != "" {
				_ = t.activeEmitter.ToolCallArgs(toolCallID, pa.StringValue)
				t.hadPartialArgs[toolCallID] = true
				// Emit ACTIVITY_DELTA with the args delta for progressive UI updates.
				if t.emitActivityDeltas {
					if actMsgID, ok := t.activityMsgIDs[toolCallID]; ok {
						_ = t.activeEmitter.ActivityDelta(actMsgID, "tool_use", []events.JSONPatchOperation{
							{Op: "add", Path: "/args", Value: pa.StringValue},
						})
					}
				}
			}
		}
		return
	}

	// Non-partial: final event for this model response.
	if t.startedTools[toolCallID] {
		// Already started during streaming. If partial deltas were emitted,
		// the client has already accumulated them — do NOT re-emit the full
		// args (that would duplicate/corrupt the client's concatenated args).
		// Only emit the full args if no partial deltas were seen (covers
		// non-streaming mode and the case where PartialArgs was empty).
		if fc.Args != nil && !t.hadPartialArgs[toolCallID] {
			argsJSON, err := json.Marshal(fc.Args)
			if err == nil {
				_ = t.activeEmitter.ToolCallArgs(toolCallID, string(argsJSON))
			}
		}
		_ = t.activeEmitter.ToolCallEnd(toolCallID)
		delete(t.openTools, toolCallID)
		delete(t.hadPartialArgs, toolCallID)
		return
	}

	// Non-streaming: emit START + ARGS + END in one shot.
	_ = t.activeEmitter.ToolCallStart(toolCallID, fc.Name, nil)
	if fc.Args != nil {
		argsJSON, err := json.Marshal(fc.Args)
		if err == nil {
			_ = t.activeEmitter.ToolCallArgs(toolCallID, string(argsJSON))
		}
	}
	_ = t.activeEmitter.ToolCallEnd(toolCallID)
}

// emitToolUseActivity emits an ACTIVITY_SNAPSHOT with type "tool_use" and a
// human-readable content payload, matching the example server's
// settlePendingToolCalls. It is emitted when the tool
// actually executes (on FunctionResponse for server tools, or on resume for
// approved HITL calls) — NOT at TOOL_CALL_END time, because a long-running
// tool may be paused for approval after the proposal and never execute.
func (t *eventTranslator) emitToolUseActivity(toolCallID, toolName string, args map[string]any) {
	argsJSON := "{}"
	if args != nil {
		if b, err := json.Marshal(args); err == nil {
			argsJSON = string(b)
		}
	}
	_ = t.activeEmitter.ActivitySnapshot(
		t.activeEmitter.GenerateMessageID(), "tool_use",
		map[string]any{"text": fmt.Sprintf("Running %s(%s)", toolName, argsJSON)},
		nil,
	)
}

// emitFunctionResponse translates an ADK FunctionResponse to a AG-UI
// TOOL_CALL_RESULT event, correlating it to the earlier FunctionCall
// via the toolCallIDs map. It also emits a tool_use ACTIVITY_SNAPSHOT
// (matching the example server's settlePendingToolCalls)
// since a FunctionResponse means the tool actually executed.
func (t *eventTranslator) emitFunctionResponse(fr *genai.FunctionResponse) {
	toolCallID, ok := t.toolCallIDs[fr.ID]
	if !ok {
		toolCallID, ok = t.toolCallIDs[fr.Name]
	}
	if !ok {
		return // no matching tool call, skip
	}
	// Emit the tool_use activity snapshot now that the tool has executed.
	if detail, has := t.toolCallDetails[toolCallID]; has {
		t.emitToolUseActivity(toolCallID, detail.name, detail.args)
	}
	content, _ := json.Marshal(fr.Response)
	msgID := "result-" + toolCallID
	_ = t.activeEmitter.ToolCallResult(msgID, toolCallID, string(content))
}

// emitThought translates a thought part to AG-UI reasoning events.
//
// Across a streaming turn, ADK emits multiple partial events whose Thought
// Text accumulates the full reasoning. To avoid spawning one reasoning card
// per chunk (which flickers frontends), a single reasoning block is opened on
// the first thought chunk and reused for subsequent chunks. Incremental
// deltas are computed by comparing the new text against the previously
// accumulated text. The block is closed when a non-partial thought arrives
// (the final chunk of the turn) or when text/terminal paths call
// closeOpenReasoning.
//
// If the part carries a ThoughtSignature, a REASONING_ENCRYPTED_VALUE event
// is also emitted so clients that need encrypted reasoning can access it.
func (t *eventTranslator) emitThought(part *genai.Part, partial bool) {
	// Close any open text message, but do NOT close an open reasoning block
	// here — the reasoning lifecycle spans multiple partial thought chunks.
	// Calling closeOpenMessage() would close the reasoning block prematurely
	// via its closeOpenReasoning() tail.
	if t.msgOpen {
		_ = t.activeEmitter.TextMessageEnd(t.currentMsgID)
		t.msgOpen = false
		t.prevText = ""
	}

	if !t.reasoningOpen {
		t.currentReasoningID = t.activeEmitter.GenerateMessageID()
		_ = t.activeEmitter.ReasoningStart(t.currentReasoningID)
		_ = t.activeEmitter.ReasoningMessageStart(t.currentReasoningID, "reasoning")
		t.reasoningOpen = true
		t.prevThoughtText = ""
	}

	// Compute incremental delta from the accumulated thought text.
	delta := part.Text
	if strings.HasPrefix(part.Text, t.prevThoughtText) {
		delta = part.Text[len(t.prevThoughtText):]
	}
	if delta != "" {
		_ = t.activeEmitter.ReasoningMessageContent(t.currentReasoningID, delta)
	}
	t.prevThoughtText += delta

	// Emit encrypted reasoning value if the part carries a thought signature.
	if len(part.ThoughtSignature) > 0 {
		_ = t.activeEmitter.ReasoningEncryptedValue(
			events.ReasoningEncryptedValueSubtypeMessage,
			t.currentReasoningID,
			base64.StdEncoding.EncodeToString(part.ThoughtSignature),
		)
	}

	if !partial {
		t.closeOpenReasoning()
	}
}

// emitStateDelta converts ADK state delta to AG-UI STATE_DELTA event.
func (t *eventTranslator) emitStateDelta(delta map[string]any) {
	var ops []events.JSONPatchOperation
	for k, v := range delta {
		ops = append(ops, events.JSONPatchOperation{
			Op:    "replace",
			Path:  "/" + k,
			Value: v,
		})
	}
	if len(ops) > 0 {
		_ = t.activeEmitter.StateDelta(ops)
	}
}

// recordUsage converts a genai usage metadata block into an agui.TokenUsage
// entry and appends it to the translator's collection. The model version is
// used as the Model field; the provider is inferred from the model prefix
// when possible (e.g. "gemini-*" → "google") and left empty otherwise so the
// aggregator can still group by model.
func (t *eventTranslator) recordUsage(meta *genai.GenerateContentResponseUsageMetadata, modelVersion string) {
	if meta == nil {
		return
	}
	u := agui.TokenUsage{
		Provider: inferProviderFromModel(modelVersion),
		Model:    modelVersion,
	}
	if meta.PromptTokenCount > 0 {
		v := int64(meta.PromptTokenCount)
		u.InputTokens = &v
	}
	if meta.CandidatesTokenCount > 0 {
		v := int64(meta.CandidatesTokenCount)
		u.OutputTokens = &v
	}
	if meta.TotalTokenCount > 0 {
		v := int64(meta.TotalTokenCount)
		u.TotalTokens = &v
	}
	if meta.ThoughtsTokenCount > 0 {
		v := int64(meta.ThoughtsTokenCount)
		u.ReasoningTokens = &v
	}
	if meta.CachedContentTokenCount > 0 {
		v := int64(meta.CachedContentTokenCount)
		u.CachedInputTokens = &v
	}
	t.usageEntries = append(t.usageEntries, u)
}

// collectedUsage returns the aggregated token usage entries for emission on
// terminal events. Returns nil when no usage was recorded, so callers can
// pass the slice directly to RunFinishedWithUsage / RunErrorWithUsage and
// get the canonical plain event when there is no telemetry.
func (t *eventTranslator) collectedUsage() []agui.TokenUsage {
	return agui.AggregateTokenUsage(t.usageEntries)
}

// inferProviderFromModel returns a best-effort provider string for the given
// model version. It recognises common Gemini, OpenAI, and Anthropic prefixes;
// unknown models return an empty string so the aggregator groups by model
// alone.
func inferProviderFromModel(model string) string {
	switch {
	case strings.HasPrefix(model, "gemini-"):
		return "google"
	case strings.HasPrefix(model, "gpt-"):
		return "openai"
	case strings.HasPrefix(model, "claude-"):
		return "anthropic"
	default:
		return ""
	}
}

// --- Helper functions ---

// inputContentsToGenaiParts converts AG-UI InputContent entries to genai.Part
// instances for the ADK runner. The provider string controls multimodal gating:
// when set to "openai", image, audio, video, and document parts are forwarded;
// other non-empty providers get text-only fallback for audio/video/document
// content (images are always forwarded). An empty provider means no gating
// (all content types forwarded).
func inputContentsToGenaiParts(contents []types.InputContent, provider string) []*genai.Part {
	var parts []*genai.Part
	for _, c := range contents {
		switch c.Type {
		case types.InputContentTypeText:
			if c.Text != "" {
				parts = append(parts, &genai.Part{Text: c.Text})
			}

		case types.InputContentTypeBinary:
			if c.Data != "" {
				if data, err := base64.StdEncoding.DecodeString(c.Data); err == nil {
					parts = append(parts, &genai.Part{
						InlineData: &genai.Blob{Data: data, MIMEType: c.MimeType},
					})
				}
			} else if c.URL != "" {
				parts = append(parts, &genai.Part{
					FileData: &genai.FileData{FileURI: c.URL, MIMEType: c.MimeType},
				})
			}

		case types.InputContentTypeImage:
			if c.Source != nil {
				switch c.Source.Type {
				case types.InputContentSourceTypeData:
					if data, err := base64.StdEncoding.DecodeString(c.Source.Value); err == nil {
						parts = append(parts, &genai.Part{
							InlineData: &genai.Blob{Data: data, MIMEType: c.Source.MimeType},
						})
					}
				case types.InputContentSourceTypeURL:
					parts = append(parts, &genai.Part{
						FileData: &genai.FileData{FileURI: c.Source.Value, MIMEType: c.Source.MimeType},
					})
				}
			}

		case types.InputContentTypeAudio, types.InputContentTypeVideo, types.InputContentTypeDocument:
			// Provider gating: OpenAI supports audio input; other providers may not.
			// When provider is set and doesn't match the content type, fall back to text.
			if provider == "" || provider == "openai" {
				if c.Source != nil {
					switch c.Source.Type {
					case types.InputContentSourceTypeData:
						if data, err := base64.StdEncoding.DecodeString(c.Source.Value); err == nil {
							parts = append(parts, &genai.Part{
								InlineData: &genai.Blob{Data: data, MIMEType: c.Source.MimeType},
							})
						}
					case types.InputContentSourceTypeURL:
						parts = append(parts, &genai.Part{
							FileData: &genai.FileData{FileURI: c.Source.Value, MIMEType: c.Source.MimeType},
						})
					}
				}
			} else {
				// Text-only fallback for non-supporting providers.
				if c.Text != "" {
					parts = append(parts, &genai.Part{Text: c.Text})
				}
			}
		}
	}
	return parts
}

// sessionStateToMap converts ADK session state to a plain map.
func sessionStateToMap(state session.State) map[string]any {
	m := make(map[string]any)
	if state == nil {
		return m
	}
	for k, v := range state.All() {
		m[k] = v
	}
	return m
}

// extractPendingToolCalls pulls the FunctionCall parts from an ADK event whose
// IDs appear in LongRunningToolIDs, returning the pending tool calls for the
// runstore. Calls not in LongRunningToolIDs are excluded.
func extractPendingToolCalls(ev *session.Event) []PendingToolCall {
	if ev == nil || ev.Content == nil || len(ev.Content.Parts) == 0 {
		return nil
	}
	longRunning := make(map[string]bool, len(ev.LongRunningToolIDs))
	for _, id := range ev.LongRunningToolIDs {
		longRunning[id] = true
	}
	var pending []PendingToolCall
	for _, part := range ev.Content.Parts {
		if part == nil || part.FunctionCall == nil {
			continue
		}
		fc := part.FunctionCall
		if !longRunning[fc.ID] {
			continue
		}
		args := make(map[string]any, len(fc.Args))
		for k, v := range fc.Args {
			args[k] = v
		}
		pending = append(pending, PendingToolCall{
			ID:   fc.ID,
			Name: fc.Name,
			Args: args,
		})
	}
	return pending
}

// sessionEventsToMessages converts ADK session events to AG-UI messages
// for the MESSAGES_SNAPSHOT event. It preserves full fidelity: text parts,
// tool calls (as ToolCalls on assistant messages), tool responses (as separate
// RoleTool messages), and the event author as the message Name so sub-agent
// attribution survives reconciliation.
func sessionEventsToMessages(evts session.Events) []types.Message {
	if evts == nil {
		return nil
	}
	var msgs []types.Message
	for ev := range evts.All() {
		if ev.Content == nil || len(ev.Content.Parts) == 0 {
			continue
		}

		role := roleForAuthor(ev.Author)
		name := authorNameForSnapshot(ev.Author)

		// Collect text and tool calls into the assistant/user message;
		// FunctionResponse parts become separate RoleTool messages so the
		// client can correlate them by ToolCallID.
		var text string
		var toolCalls []types.ToolCall
		for _, part := range ev.Content.Parts {
			if part == nil {
				continue
			}
			if part.Text != "" && !part.Thought {
				text += part.Text
			}
			if fc := part.FunctionCall; fc != nil {
				argsJSON := "{}"
				if fc.Args != nil {
					if b, err := json.Marshal(fc.Args); err == nil {
						argsJSON = string(b)
					}
				}
				toolCalls = append(toolCalls, types.ToolCall{
					ID:   fc.ID,
					Type: types.ToolCallTypeFunction,
					Function: types.FunctionCall{
						Name:      fc.Name,
						Arguments: argsJSON,
					},
				})
			}
			if fr := part.FunctionResponse; fr != nil {
				content := "{}"
				if b, err := json.Marshal(fr.Response); err == nil {
					content = string(b)
				}
				msgs = append(msgs, types.Message{
					ID:         "result-" + fr.ID,
					Role:       types.RoleTool,
					Content:    content,
					ToolCallID: fr.ID,
					Name:       fr.Name,
				})
			}
		}

		if text == "" && len(toolCalls) == 0 {
			continue
		}
		msg := types.Message{
			ID:        ev.ID,
			Role:      role,
			Name:      name,
			ToolCalls: toolCalls,
		}
		if text != "" {
			msg.Content = text
		}
		msgs = append(msgs, msg)
	}
	return msgs
}

// roleForAuthor maps an ADK event author to an AG-UI message role. The author
// "user" maps to RoleUser; any other author (model, sub-agent name) maps to
// RoleAssistant since ADK persists all agent-generated content under the
// agent's name rather than the canonical "model" role.
func roleForAuthor(author string) types.Role {
	if author == "user" {
		return types.RoleUser
	}
	return types.RoleAssistant
}

// authorNameForSnapshot returns the Name to attach to a snapshot message. The
// "user" and "model" authors are canonical roles and don't need a Name, so
// they're dropped to keep snapshots clean. Sub-agent author names are
// preserved so frontends can attribute messages to the emitting sub-agent.
func authorNameForSnapshot(author string) string {
	switch author {
	case "", "user", "model":
		return ""
	default:
		return author
	}
}
