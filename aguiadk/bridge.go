// Package aguiadk bridges ADK-Go agents to the AG-UI protocol.
//
// The bridge translates ADK session events into AG-UI events, enabling
// any ADK-Go agent to serve AG-UI-compatible frontends.
package aguiadk

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"time"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"

	"github.com/ieshan/adk-go-pkg/agui"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/artifact"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
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
}

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
// NOTE: AG-UI input.Tools (client-side tools) are not yet wired into the
// ADK runner because the ADK-Go runner does not support dynamic tool
// injection at run time. Tools must be configured on the agent at creation
// time. This limitation will be addressed when the ADK runner API adds
// support for per-run tool overrides.
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
		cfg:     cfg,
		runner:  r,
		sessMgr: sm,
	}
	return agui.AgentFunc(b.run), nil
}

// bridge holds the runtime state for the ADK-AG-UI bridge.
type bridge struct {
	cfg     Config
	runner  *runner.Runner
	sessMgr *SessionManager
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

// httpRequestKey is a context key for storing the HTTP request.
type httpRequestKey struct{}

// WithHTTPRequest stores an *http.Request in the context so that
// AppNameFunc and UserIDFunc can access it.
func WithHTTPRequest(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, httpRequestKey{}, r)
}

// run implements the agui.Agent interface.
func (b *bridge) run(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
	ch := make(chan events.Event, 64)
	emitter := agui.NewEventEmitter(ch)

	go func() {
		defer close(ch)
		b.runInternal(ctx, input, emitter)
	}()

	return agui.ChanToIter(ctx, ch)
}

// runInternal performs the actual bridge logic, emitting events via the emitter.
func (b *bridge) runInternal(ctx context.Context, input types.RunAgentInput, emitter *agui.EventEmitter) {
	appName := b.resolveAppName(ctx)
	userID := b.resolveUserID(ctx)

	// Resolve or create an ADK session for this AG-UI thread.
	adkSession, err := b.sessMgr.Resolve(ctx, input.ThreadID, appName, userID)
	if err != nil {
		_ = emitter.RunError(fmt.Sprintf("session resolve failed: %v", err), nil)
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

	// Emit STATE_SNAPSHOT if configured.
	if b.cfg.emitStateSnapshot() {
		snapshot := sessionStateToMap(adkSession.State())
		_ = emitter.StateSnapshot(snapshot)
	}

	// Translate the last user message to genai.Content.
	msg := lastUserMessage(input.Messages)

	// Run the ADK agent.
	runCfg := agent.RunConfig{
		StreamingMode: agent.StreamingModeSSE,
	}

	translator := newEventTranslator(emitter)

	for adkEvent, err := range b.runner.Run(ctx, userID, adkSession.ID(), msg, runCfg) {
		if err != nil {
			_ = emitter.RunError(fmt.Sprintf("agent error: %v", err), nil)
			return
		}
		if adkEvent == nil {
			continue
		}

		translator.translate(adkEvent)
	}

	// Close any open text message.
	translator.closeOpenMessage()

	// Emit MESSAGES_SNAPSHOT if configured.
	if b.cfg.EmitMessagesSnapshot {
		// Re-fetch the session to get updated events.
		refreshed, err := b.sessMgr.Resolve(ctx, input.ThreadID, appName, userID)
		if err == nil {
			msgs := sessionEventsToMessages(refreshed.Events())
			_ = emitter.MessagesSnapshot(msgs)
		}
	}

	// Emit RUN_FINISHED.
	if err := emitter.RunFinished(input.ThreadID, input.RunID); err != nil {
		return
	}
}

// eventTranslator holds per-run state for translating ADK events to AG-UI events.
type eventTranslator struct {
	emitter      *agui.EventEmitter
	currentMsgID string
	msgOpen      bool
	prevText     string // accumulated text for computing deltas
}

func newEventTranslator(emitter *agui.EventEmitter) *eventTranslator {
	return &eventTranslator{emitter: emitter}
}

// translate converts a single ADK event into one or more AG-UI events.
func (t *eventTranslator) translate(ev *session.Event) {
	// Handle state delta.
	if len(ev.Actions.StateDelta) > 0 {
		t.emitStateDelta(ev.Actions.StateDelta)
	}

	// No content means no message-level events.
	if ev.Content == nil || len(ev.Content.Parts) == 0 {
		return
	}

	for _, part := range ev.Content.Parts {
		if part == nil {
			continue
		}

		switch {
		case part.Thought && part.Text != "":
			t.emitThought(part.Text)

		case part.FunctionCall != nil:
			t.closeOpenMessage()
			t.emitFunctionCall(part.FunctionCall)

		case part.Text != "":
			t.emitText(part.Text, ev.Partial)

			// FunctionResponse parts are internal to ADK, skip them.
		}
	}
}

// emitText handles text parts, managing the message lifecycle.
func (t *eventTranslator) emitText(text string, partial bool) {
	if !t.msgOpen {
		t.currentMsgID = t.emitter.GenerateMessageID()
		_ = t.emitter.TextMessageStart(t.currentMsgID, new("assistant"))
		t.msgOpen = true
		t.prevText = ""
	}

	if partial {
		// Streaming mode: compute delta from previous accumulated text.
		delta := text
		if len(text) > len(t.prevText) && text[:len(t.prevText)] == t.prevText {
			delta = text[len(t.prevText):]
		}
		if delta != "" {
			_ = t.emitter.TextMessageContent(t.currentMsgID, delta)
		}
		t.prevText = text
	} else {
		// Final (non-partial) event: emit remaining content and close.
		delta := text
		if len(text) > len(t.prevText) && text[:len(t.prevText)] == t.prevText {
			delta = text[len(t.prevText):]
		}
		if delta != "" {
			_ = t.emitter.TextMessageContent(t.currentMsgID, delta)
		}
		_ = t.emitter.TextMessageEnd(t.currentMsgID)
		t.msgOpen = false
		t.prevText = ""
	}
}

// closeOpenMessage ends an open text message if one is active.
func (t *eventTranslator) closeOpenMessage() {
	if t.msgOpen {
		_ = t.emitter.TextMessageEnd(t.currentMsgID)
		t.msgOpen = false
		t.prevText = ""
	}
}

// emitFunctionCall translates an ADK FunctionCall to AG-UI tool call events.
func (t *eventTranslator) emitFunctionCall(fc *genai.FunctionCall) {
	toolCallID := t.emitter.GenerateToolCallID()
	_ = t.emitter.ToolCallStart(toolCallID, fc.Name, nil)

	// Serialize arguments as JSON.
	if fc.Args != nil {
		argsJSON, err := json.Marshal(fc.Args)
		if err == nil {
			_ = t.emitter.ToolCallArgs(toolCallID, string(argsJSON))
		}
	}

	_ = t.emitter.ToolCallEnd(toolCallID)
}

// emitThought translates a thought part to AG-UI reasoning events.
func (t *eventTranslator) emitThought(text string) {
	t.closeOpenMessage()

	msgID := t.emitter.GenerateMessageID()
	_ = t.emitter.ReasoningStart(msgID)
	_ = t.emitter.ReasoningMessageStart(msgID, "assistant")
	_ = t.emitter.ReasoningMessageContent(msgID, text)
	_ = t.emitter.ReasoningMessageEnd(msgID)
	_ = t.emitter.ReasoningEnd(msgID)
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
		_ = t.emitter.StateDelta(ops)
	}
}

// --- Helper functions ---

// lastUserMessage extracts the last user message from AG-UI messages and
// converts it to a genai.Content suitable for the ADK runner.
func lastUserMessage(messages []types.Message) *genai.Content {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != types.RoleUser {
			continue
		}
		text, ok := msg.ContentString()
		if !ok {
			continue
		}
		return &genai.Content{
			Role:  "user",
			Parts: []*genai.Part{{Text: text}},
		}
	}
	return &genai.Content{
		Role:  "user",
		Parts: []*genai.Part{{Text: ""}},
	}
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

// sessionEventsToMessages converts ADK session events to AG-UI messages
// for the MESSAGES_SNAPSHOT event.
func sessionEventsToMessages(evts session.Events) []types.Message {
	if evts == nil {
		return nil
	}
	var msgs []types.Message
	for ev := range evts.All() {
		if ev.Content == nil || len(ev.Content.Parts) == 0 {
			continue
		}

		role := types.RoleAssistant
		if ev.Author == "user" {
			role = types.RoleUser
		}

		// Collect text from parts.
		var text string
		for _, part := range ev.Content.Parts {
			if part != nil && part.Text != "" && !part.Thought {
				text += part.Text
			}
		}

		if text != "" {
			msgs = append(msgs, types.Message{
				ID:      ev.ID,
				Role:    role,
				Content: text,
			})
		}
	}
	return msgs
}
