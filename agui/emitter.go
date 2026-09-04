package agui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

// ErrTransport indicates a transport-level error (e.g. the client disconnected
// and the event channel is closed). Callers can use errors.Is to distinguish
// transport errors from encoding errors: a transport error means the client is
// gone and the run should be cancelled; an encoding error means the event
// content is malformed and the event should be dropped while keeping the stream
// alive for subsequent events.
var ErrTransport = errors.New("agui: transport error")

// EventEmitter provides typed methods for emitting AG-UI events.
//
// When constructed with NewEventEmitterWithContext, emit blocks are bounded by
// the context lifecycle: if the context is cancelled (e.g. the consumer stops
// iterating or the client disconnects), pending and future emit calls return
// promptly with an error wrapping ErrTransport instead of blocking forever.
// This prevents goroutine leaks when the downstream consumer breaks out of a
// stream early.
type EventEmitter struct {
	out chan<- events.Event
	ctx context.Context // nil for the legacy context-unaware emitter

	// eventWrapper, when non-nil, transforms each event before it is sent
	// to the channel. ForSubagent uses this to inject subagentRunId into
	// stream events for sub-agent attribution.
	eventWrapper func(events.Event) events.Event
}

// NewEventEmitter creates an emitter that writes to the given channel.
//
// The returned emitter is not bound to any context: emit calls block until the
// channel accepts the event. If the channel is closed, emit recovers the panic
// and returns an error wrapping ErrTransport. Prefer
// NewEventEmitterWithContext for streaming runs where the consumer may stop
// iterating before the producer finishes.
func NewEventEmitter(out chan<- events.Event) *EventEmitter {
	return &EventEmitter{out: out}
}

// NewEventEmitterWithContext creates an emitter bounded by the context
// lifecycle. When ctx is cancelled, emit calls unblock and return an error
// wrapping ErrTransport and the context's error (ctx.Err()), preventing
// goroutine leaks on early stream termination or client disconnect.
func NewEventEmitterWithContext(ctx context.Context, out chan<- events.Event) *EventEmitter {
	return &EventEmitter{out: out, ctx: ctx}
}

// GenerateMessageID returns a new unique message ID.
func (e *EventEmitter) GenerateMessageID() string {
	return events.GenerateMessageID()
}

// GenerateToolCallID returns a new unique tool call ID.
func (e *EventEmitter) GenerateToolCallID() string {
	return events.GenerateToolCallID()
}

func (e *EventEmitter) emit(ev events.Event) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrTransport, r)
		}
	}()
	if e.eventWrapper != nil {
		ev = e.eventWrapper(ev)
	}
	if e.ctx != nil {
		select {
		case <-e.ctx.Done():
			return fmt.Errorf("%w: %w", ErrTransport, e.ctx.Err())
		case e.out <- ev:
			return nil
		}
	}
	e.out <- ev
	return nil
}

// RunStarted emits a RUN_STARTED event.
func (e *EventEmitter) RunStarted(threadID, runID string) error {
	return e.emit(events.NewRunStartedEvent(threadID, runID))
}

// RunFinishedWithOptions emits a RUN_FINISHED event with optional configuration
// (e.g., events.WithSuccessOutcome, events.WithInterruptOutcome).
func (e *EventEmitter) RunFinishedWithOptions(threadID, runID string, opts ...events.RunFinishedOption) error {
	return e.emit(events.NewRunFinishedEventWithOptions(threadID, runID, opts...))
}

// RunErrorWithOptions emits a RUN_ERROR event with optional configuration
// (e.g., events.WithRunID, events.WithErrorCode).
func (e *EventEmitter) RunErrorWithOptions(message string, opts ...events.RunErrorOption) error {
	return e.emit(events.NewRunErrorEvent(message, opts...))
}

// TextMessageStart emits a TEXT_MESSAGE_START event.
func (e *EventEmitter) TextMessageStart(messageID string, role *string) error {
	opts := []events.TextMessageStartOption{}
	if role != nil {
		opts = append(opts, events.WithRole(*role))
	}
	return e.emit(events.NewTextMessageStartEvent(messageID, opts...))
}

// TextMessageStartWithID emits a TEXT_MESSAGE_START event with an optional
// role and sub-agent name. The name is attached via WithName so frontends can
// attribute the message to the emitting sub-agent.
func (e *EventEmitter) TextMessageStartWithID(messageID string, role *string, name string) error {
	opts := []events.TextMessageStartOption{}
	if role != nil {
		opts = append(opts, events.WithRole(*role))
	}
	if name != "" {
		opts = append(opts, events.WithName(name))
	}
	return e.emit(events.NewTextMessageStartEvent(messageID, opts...))
}

// TextMessageContent emits a TEXT_MESSAGE_CONTENT event with a text delta.
func (e *EventEmitter) TextMessageContent(messageID, delta string) error {
	return e.emit(events.NewTextMessageContentEvent(messageID, delta))
}

// TextMessageEnd emits a TEXT_MESSAGE_END event.
func (e *EventEmitter) TextMessageEnd(messageID string) error {
	return e.emit(events.NewTextMessageEndEvent(messageID))
}

// ToolCallStart emits a TOOL_CALL_START event.
func (e *EventEmitter) ToolCallStart(toolCallID, toolCallName string, parentMessageID *string) error {
	opts := []events.ToolCallStartOption{}
	if parentMessageID != nil {
		opts = append(opts, events.WithParentMessageID(*parentMessageID))
	}
	return e.emit(events.NewToolCallStartEvent(toolCallID, toolCallName, opts...))
}

// ToolCallArgs emits a TOOL_CALL_ARGS event.
func (e *EventEmitter) ToolCallArgs(toolCallID, delta string) error {
	return e.emit(events.NewToolCallArgsEvent(toolCallID, delta))
}

// ToolCallEnd emits a TOOL_CALL_END event.
func (e *EventEmitter) ToolCallEnd(toolCallID string) error {
	return e.emit(events.NewToolCallEndEvent(toolCallID))
}

// ToolCallResult emits a TOOL_CALL_RESULT event.
func (e *EventEmitter) ToolCallResult(messageID, toolCallID, content string) error {
	return e.emit(events.NewToolCallResultEvent(messageID, toolCallID, content))
}

// StateSnapshot emits a STATE_SNAPSHOT event.
func (e *EventEmitter) StateSnapshot(snapshot any) error {
	return e.emit(events.NewStateSnapshotEvent(snapshot))
}

// StateDelta emits a STATE_DELTA event with JSON Patch operations.
func (e *EventEmitter) StateDelta(delta []events.JSONPatchOperation) error {
	return e.emit(events.NewStateDeltaEvent(delta))
}

// MessagesSnapshot emits a MESSAGES_SNAPSHOT event.
// EncryptedValue and EncryptedContent fields are scrubbed from messages
// before emission to prevent leaking ciphertext to the client.
func (e *EventEmitter) MessagesSnapshot(messages []types.Message) error {
	return e.emit(events.NewMessagesSnapshotEvent(scrubEncryptedValues(messages)))
}

// scrubEncryptedValues zeroes EncryptedValue and EncryptedContent fields
// in messages. If no scrubbing is needed, it returns the original slice
// without allocation.
func scrubEncryptedValues(msgs []types.Message) []types.Message {
	needsScrub := false
	for i := range msgs {
		if msgs[i].EncryptedValue != "" || msgs[i].EncryptedContent != "" {
			needsScrub = true
			break
		}
	}
	if !needsScrub {
		return msgs
	}
	out := make([]types.Message, len(msgs))
	copy(out, msgs)
	for i := range out {
		out[i].EncryptedValue = ""
		out[i].EncryptedContent = ""
	}
	return out
}

// StepStarted emits a STEP_STARTED event.
func (e *EventEmitter) StepStarted(stepName string) error {
	return e.emit(events.NewStepStartedEvent(stepName))
}

// StepFinished emits a STEP_FINISHED event.
func (e *EventEmitter) StepFinished(stepName string) error {
	return e.emit(events.NewStepFinishedEvent(stepName))
}

// ActivitySnapshot emits an ACTIVITY_SNAPSHOT event.
func (e *EventEmitter) ActivitySnapshot(messageID, activityType string, content any, replace *bool) error {
	ev := events.NewActivitySnapshotEvent(messageID, activityType, content)
	if replace != nil {
		ev.WithReplace(*replace)
	}
	return e.emit(ev)
}

// ActivityDelta emits an ACTIVITY_DELTA event.
func (e *EventEmitter) ActivityDelta(messageID, activityType string, patch []events.JSONPatchOperation) error {
	return e.emit(events.NewActivityDeltaEvent(messageID, activityType, patch))
}

// ReasoningStart emits a REASONING_START event.
func (e *EventEmitter) ReasoningStart(messageID string) error {
	return e.emit(events.NewReasoningStartEvent(messageID))
}

// ReasoningMessageStart emits a REASONING_MESSAGE_START event.
func (e *EventEmitter) ReasoningMessageStart(messageID, role string) error {
	return e.emit(events.NewReasoningMessageStartEvent(messageID, role))
}

// ReasoningMessageContent emits a REASONING_MESSAGE_CONTENT event.
func (e *EventEmitter) ReasoningMessageContent(messageID, delta string) error {
	return e.emit(events.NewReasoningMessageContentEvent(messageID, delta))
}

// ReasoningMessageEnd emits a REASONING_MESSAGE_END event.
func (e *EventEmitter) ReasoningMessageEnd(messageID string) error {
	return e.emit(events.NewReasoningMessageEndEvent(messageID))
}

// ReasoningEnd emits a REASONING_END event.
func (e *EventEmitter) ReasoningEnd(messageID string) error {
	return e.emit(events.NewReasoningEndEvent(messageID))
}

// TextMessageChunk emits a TEXT_MESSAGE_CHUNK convenience event.
func (e *EventEmitter) TextMessageChunk(messageID, role, delta *string) error {
	return e.emit(events.NewTextMessageChunkEvent(messageID, role, delta))
}

// ToolCallChunk emits a TOOL_CALL_CHUNK convenience event.
func (e *EventEmitter) ToolCallChunk(toolCallID, toolCallName, parentMessageID, delta *string) error {
	ev := events.NewToolCallChunkEvent()
	if toolCallID != nil {
		ev.WithToolCallChunkID(*toolCallID)
	}
	if toolCallName != nil {
		ev.WithToolCallChunkName(*toolCallName)
	}
	if parentMessageID != nil {
		ev.WithToolCallChunkParentMessageID(*parentMessageID)
	}
	if delta != nil {
		ev.WithToolCallChunkDelta(*delta)
	}
	return e.emit(ev)
}

// ReasoningMessageChunk emits a REASONING_MESSAGE_CHUNK convenience event.
func (e *EventEmitter) ReasoningMessageChunk(messageID, delta *string) error {
	return e.emit(events.NewReasoningMessageChunkEvent(messageID, delta))
}

// ReasoningEncryptedValue emits a REASONING_ENCRYPTED_VALUE event.
func (e *EventEmitter) ReasoningEncryptedValue(subtype events.ReasoningEncryptedValueSubtype, entityID, encryptedValue string) error {
	return e.emit(events.NewReasoningEncryptedValueEvent(subtype, entityID, encryptedValue))
}

// Custom emits a CUSTOM event.
func (e *EventEmitter) Custom(name string, value any) error {
	return e.emit(events.NewCustomEvent(name, events.WithValue(value)))
}

// Raw emits a RAW event.
func (e *EventEmitter) Raw(event any, source *string) error {
	opts := []events.RawEventOption{}
	if source != nil {
		opts = append(opts, events.WithSource(*source))
	}
	return e.emit(events.NewRawEvent(event, opts...))
}

// SubagentStarted emits a SUBAGENT_STARTED event marking the beginning of a
// sub-agent run.
func (e *EventEmitter) SubagentStarted(subagentRunID, name string, opts ...SubagentStartedOption) error {
	return e.emit(NewSubagentStartedEvent(subagentRunID, name, opts...))
}

// SubagentFinished emits a SUBAGENT_FINISHED event marking the completion of a
// sub-agent run.
func (e *EventEmitter) SubagentFinished(subagentRunID string, opts ...SubagentFinishedOption) error {
	return e.emit(NewSubagentFinishedEvent(subagentRunID, opts...))
}

// SubagentError emits a SUBAGENT_ERROR event marking the failure of a
// sub-agent run.
func (e *EventEmitter) SubagentError(subagentRunID, message string, opts ...SubagentErrorOption) error {
	return e.emit(NewSubagentErrorEvent(subagentRunID, message, opts...))
}

// ForSubagent returns a new EventEmitter sharing the same output channel and
// context as e, but wrapping every emitted event with a subagentRunId field
// for stream attribution. Events emitted through the returned emitter
// carry "subagentRunId" in their JSON payload so frontends can attribute
// streamed text, tool calls, state deltas, etc. to the emitting sub-agent.
//
// The returned emitter is lightweight — it does not allocate a new channel or
// goroutine. Lifecycle events (SubagentStarted/Finished/Error) should be
// emitted through the original emitter, not the sub-agent wrapper, since they
// carry subagentRunId as their own dedicated field.
func (e *EventEmitter) ForSubagent(subagentRunID string) *EventEmitter {
	return &EventEmitter{
		out: e.out,
		ctx: e.ctx,
		eventWrapper: func(ev events.Event) events.Event {
			return &subagentAttributedEvent{Event: ev, subagentRunID: subagentRunID}
		},
	}
}

// subagentAttributedEvent wraps an events.Event and injects "subagentRunId"
// into its JSON serialization. All interface methods delegate to the wrapped
// event; only ToJSON is overridden to merge the attribution field.
type subagentAttributedEvent struct {
	events.Event
	subagentRunID string
}

// ToJSON serializes the wrapped event and injects the subagentRunId field.
func (e *subagentAttributedEvent) ToJSON() ([]byte, error) {
	data, err := e.Event.ToJSON()
	if err != nil {
		return nil, err
	}
	// Unmarshal into a map, add the field, re-marshal. This preserves all
	// original fields and works for any event type without knowing its
	// concrete struct.
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		// Can't augment — return the original JSON as-is.
		return data, nil
	}
	m["subagentRunId"] = e.subagentRunID
	return json.Marshal(m)
}

// Compile-time check that subagentAttributedEvent satisfies events.Event.
var _ events.Event = (*subagentAttributedEvent)(nil)
