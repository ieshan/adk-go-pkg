package agui

import (
	"encoding/json"
	"fmt"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// Sub-agent lifecycle event types. These extend the AG-UI protocol with
// explicit sub-agent boundaries so frontends can attribute streamed events to
// the emitting sub-agent and render sub-agent activity.
const (
	EventTypeSubagentStarted  events.EventType = "SUBAGENT_STARTED"
	EventTypeSubagentFinished events.EventType = "SUBAGENT_FINISHED"
	EventTypeSubagentError    events.EventType = "SUBAGENT_ERROR"
)

// SubagentStartedEvent marks the start of a sub-agent run. It carries the
// sub-agent's run ID, name, and optional metadata describing the delegation.
type SubagentStartedEvent struct {
	*events.BaseEvent
	SubagentRunID       string  `json:"subagentRunId"`
	Name                string  `json:"name"`
	Description         string  `json:"description,omitempty"`
	ParentSubagentRunID *string `json:"parentSubagentRunId,omitempty"`
	ParentToolCallID    *string `json:"parentToolCallId,omitempty"`
	ParentMessageID     *string `json:"parentMessageId,omitempty"`
}

// NewSubagentStartedEvent creates a SUBAGENT_STARTED event with optional
// configuration applied via the supplied options.
func NewSubagentStartedEvent(subagentRunID, name string, opts ...SubagentStartedOption) *SubagentStartedEvent {
	e := &SubagentStartedEvent{
		BaseEvent:     events.NewBaseEvent(EventTypeSubagentStarted),
		SubagentRunID: subagentRunID,
		Name:          name,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// SubagentStartedOption configures a SubagentStartedEvent.
type SubagentStartedOption func(*SubagentStartedEvent)

// WithSubagentDescription sets the description on a SubagentStartedEvent.
func WithSubagentDescription(desc string) SubagentStartedOption {
	return func(e *SubagentStartedEvent) { e.Description = desc }
}

// WithParentSubagentRunID sets the parent sub-agent run ID on a SubagentStartedEvent.
func WithParentSubagentRunID(id string) SubagentStartedOption {
	return func(e *SubagentStartedEvent) { e.ParentSubagentRunID = &id }
}

// WithParentToolCallID sets the parent tool call ID on a SubagentStartedEvent.
func WithParentToolCallID(id string) SubagentStartedOption {
	return func(e *SubagentStartedEvent) { e.ParentToolCallID = &id }
}

// WithParentMessageID sets the parent message ID on a SubagentStartedEvent.
func WithParentMessageID(id string) SubagentStartedOption {
	return func(e *SubagentStartedEvent) { e.ParentMessageID = &id }
}

// Validate validates the sub-agent started event. It skips the SDK's
// BaseEvent type-check (which only knows canonical AG-UI types) since
// SUBAGENT_STARTED is a protocol extension.
func (e *SubagentStartedEvent) Validate() error {
	if e.BaseEvent == nil || e.EventType == "" {
		return fmt.Errorf("SubagentStartedEvent validation failed: type field is required")
	}
	if e.SubagentRunID == "" {
		return fmt.Errorf("SubagentStartedEvent validation failed: subagentRunId field is required")
	}
	if e.Name == "" {
		return fmt.Errorf("SubagentStartedEvent validation failed: name field is required")
	}
	return nil
}

// ToJSON serializes the event to JSON.
func (e *SubagentStartedEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// SubagentFinishedEvent marks the successful completion (or suspension) of a
// sub-agent run. Result carries the sub-agent's final output (if any);
// Outcome discriminates success vs. suspended.
type SubagentFinishedEvent struct {
	*events.BaseEvent
	SubagentRunID string                   `json:"subagentRunId"`
	Name          string                   `json:"name,omitempty"`
	Result        any                      `json:"result,omitempty"`
	Outcome       *SubagentFinishedOutcome `json:"outcome,omitempty"`
}

// SubagentFinishedOutcome discriminates between success and suspended outcomes.
type SubagentFinishedOutcome struct {
	Type       SubagentFinishedOutcomeType `json:"type"`
	Interrupts []any                       `json:"interrupts,omitempty"`
}

// SubagentFinishedOutcomeType enumerates the outcome variants.
type SubagentFinishedOutcomeType string

const (
	// SubagentFinishedOutcomeTypeSuccess indicates the sub-agent completed normally.
	SubagentFinishedOutcomeTypeSuccess SubagentFinishedOutcomeType = "success"
	// SubagentFinishedOutcomeTypeSuspended indicates the sub-agent paused on an interrupt.
	SubagentFinishedOutcomeTypeSuspended SubagentFinishedOutcomeType = "suspended"
)

// NewSubagentFinishedEvent creates a SUBAGENT_FINISHED event.
func NewSubagentFinishedEvent(subagentRunID string, opts ...SubagentFinishedOption) *SubagentFinishedEvent {
	e := &SubagentFinishedEvent{
		BaseEvent:     events.NewBaseEvent(EventTypeSubagentFinished),
		SubagentRunID: subagentRunID,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// SubagentFinishedOption configures a SubagentFinishedEvent.
type SubagentFinishedOption func(*SubagentFinishedEvent)

// WithSubagentName sets the name on a SubagentFinishedEvent.
func WithSubagentName(name string) SubagentFinishedOption {
	return func(e *SubagentFinishedEvent) { e.Name = name }
}

// WithSubagentResult sets the result payload on a SubagentFinishedEvent.
func WithSubagentResult(result any) SubagentFinishedOption {
	return func(e *SubagentFinishedEvent) { e.Result = result }
}

// WithSubagentSuccessOutcome sets a success outcome on a SubagentFinishedEvent.
func WithSubagentSuccessOutcome() SubagentFinishedOption {
	return func(e *SubagentFinishedEvent) {
		e.Outcome = &SubagentFinishedOutcome{Type: SubagentFinishedOutcomeTypeSuccess}
	}
}

// WithSubagentSuspendedOutcome sets a suspended outcome on a SubagentFinishedEvent.
func WithSubagentSuspendedOutcome(interrupts []any) SubagentFinishedOption {
	return func(e *SubagentFinishedEvent) {
		e.Outcome = &SubagentFinishedOutcome{Type: SubagentFinishedOutcomeTypeSuspended, Interrupts: interrupts}
	}
}

// Validate validates the sub-agent finished event. It skips the SDK's
// BaseEvent type-check since SUBAGENT_FINISHED is a protocol extension.
func (e *SubagentFinishedEvent) Validate() error {
	if e.BaseEvent == nil || e.EventType == "" {
		return fmt.Errorf("SubagentFinishedEvent validation failed: type field is required")
	}
	if e.SubagentRunID == "" {
		return fmt.Errorf("SubagentFinishedEvent validation failed: subagentRunId field is required")
	}
	return nil
}

// ToJSON serializes the event to JSON.
func (e *SubagentFinishedEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// SubagentErrorEvent marks the failure of a sub-agent run.
type SubagentErrorEvent struct {
	*events.BaseEvent
	SubagentRunID string  `json:"subagentRunId"`
	Name          string  `json:"name,omitempty"`
	Message       string  `json:"message"`
	Code          *string `json:"code,omitempty"`
}

// NewSubagentErrorEvent creates a SUBAGENT_ERROR event.
func NewSubagentErrorEvent(subagentRunID, message string, opts ...SubagentErrorOption) *SubagentErrorEvent {
	e := &SubagentErrorEvent{
		BaseEvent:     events.NewBaseEvent(EventTypeSubagentError),
		SubagentRunID: subagentRunID,
		Message:       message,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// SubagentErrorOption configures a SubagentErrorEvent.
type SubagentErrorOption func(*SubagentErrorEvent)

// WithSubagentErrorName sets the name on a SubagentErrorEvent.
func WithSubagentErrorName(name string) SubagentErrorOption {
	return func(e *SubagentErrorEvent) { e.Name = name }
}

// WithSubagentErrorCode sets the error code on a SubagentErrorEvent.
func WithSubagentErrorCode(code string) SubagentErrorOption {
	return func(e *SubagentErrorEvent) { e.Code = &code }
}

// Validate validates the sub-agent error event. It skips the SDK's
// BaseEvent type-check since SUBAGENT_ERROR is a protocol extension.
func (e *SubagentErrorEvent) Validate() error {
	if e.BaseEvent == nil || e.EventType == "" {
		return fmt.Errorf("SubagentErrorEvent validation failed: type field is required")
	}
	if e.SubagentRunID == "" {
		return fmt.Errorf("SubagentErrorEvent validation failed: subagentRunId field is required")
	}
	if e.Message == "" {
		return fmt.Errorf("SubagentErrorEvent validation failed: message field is required")
	}
	return nil
}

// ToJSON serializes the event to JSON.
func (e *SubagentErrorEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// Compile-time checks that sub-agent events satisfy events.Event.
var (
	_ events.Event = (*SubagentStartedEvent)(nil)
	_ events.Event = (*SubagentFinishedEvent)(nil)
	_ events.Event = (*SubagentErrorEvent)(nil)
)
