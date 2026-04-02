package agui

import (
	"fmt"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

// EventEmitter provides typed methods for emitting AG-UI events.
type EventEmitter struct {
	out chan<- events.Event
}

// NewEventEmitter creates an emitter that writes to the given channel.
func NewEventEmitter(out chan<- events.Event) *EventEmitter {
	return &EventEmitter{out: out}
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
			err = fmt.Errorf("agui: emit failed (channel closed): %v", r)
		}
	}()
	e.out <- ev
	return nil
}

// RunStarted emits a RUN_STARTED event.
func (e *EventEmitter) RunStarted(threadID, runID string) error {
	return e.emit(events.NewRunStartedEvent(threadID, runID))
}

// RunFinished emits a RUN_FINISHED event.
func (e *EventEmitter) RunFinished(threadID, runID string) error {
	return e.emit(events.NewRunFinishedEvent(threadID, runID))
}

// RunError emits a RUN_ERROR event.
func (e *EventEmitter) RunError(message string, code *string) error {
	opts := []events.RunErrorOption{}
	if code != nil {
		opts = append(opts, events.WithErrorCode(*code))
	}
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
func (e *EventEmitter) MessagesSnapshot(messages []types.Message) error {
	return e.emit(events.NewMessagesSnapshotEvent(messages))
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
