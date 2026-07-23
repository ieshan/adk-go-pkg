package eval

import (
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/genai"
)

// SessionInput represents the initial session state for an eval case.
type SessionInput struct {
	// AppName is the name of the app.
	AppName string `json:"appName,omitempty"`

	// UserID is the user ID for the session.
	UserID string `json:"userId,omitempty"`

	// State is the initial state for the session.
	State map[string]any `json:"state,omitempty"`
}

// SessionState is a type alias for session state maps.
type SessionState = map[string]any

// InvocationEvent represents a single event within an invocation.
type InvocationEvent struct {
	// Author is the name of the event's author.
	Author string `json:"author,omitempty"`

	// Content is the content of the event.
	Content *genai.Content `json:"content,omitempty"`
}

// IntermediateData is an interface representing the intermediate data
// (tool calls, responses, events) of an invocation. It has two
// implementations:
//   - LegacyIntermediateData: the older format with separate tool_uses and
//     intermediate_responses fields.
//   - InvocationEventsData: the newer format with a list of InvocationEvent.
//
// Custom JSON marshal/unmarshal handles both formats transparently.
type IntermediateData interface {
	// IsIntermediateData marks the interface.
	IsIntermediateData()

	// GetInvocationEvents returns the events if this is InvocationEventsData,
	// or nil otherwise.
	GetInvocationEvents() []InvocationEvent

	// GetToolUses returns the tool calls if this is LegacyIntermediateData,
	// or nil otherwise.
	GetToolUses() []genai.FunctionCall

	// GetToolResponses returns the tool responses if this is
	// LegacyIntermediateData, or nil otherwise.
	GetToolResponses() []genai.FunctionResponse
}

// LegacyIntermediateData is the older format for intermediate data with
// separate tool uses and intermediate responses.
type LegacyIntermediateData struct {
	// ToolUses is the list of tool calls made during the invocation.
	ToolUses []genai.FunctionCall `json:"toolUses,omitempty"`

	// ToolResponses is the list of tool responses received.
	ToolResponses []genai.FunctionResponse `json:"toolResponses,omitempty"`

	// IntermediateResponses is a list of (author, parts) pairs for
	// intermediate agent responses.
	IntermediateResponses []IntermediateResponse `json:"intermediateResponses,omitempty"`
}

// IntermediateResponse represents an intermediate response from an agent,
// consisting of an author and a list of parts.
type IntermediateResponse struct {
	Author string        `json:"author,omitempty"`
	Parts  []*genai.Part `json:"parts,omitempty"`
}

// IsIntermediateData marks LegacyIntermediateData as implementing IntermediateData.
func (*LegacyIntermediateData) IsIntermediateData() {}

// GetInvocationEvents returns nil for legacy format.
func (l *LegacyIntermediateData) GetInvocationEvents() []InvocationEvent { return nil }

// GetToolUses returns the tool calls.
func (l *LegacyIntermediateData) GetToolUses() []genai.FunctionCall { return l.ToolUses }

// GetToolResponses returns the tool responses.
func (l *LegacyIntermediateData) GetToolResponses() []genai.FunctionResponse { return l.ToolResponses }

// InvocationEventsData is the newer format for intermediate data, storing
// a list of InvocationEvent objects.
type InvocationEventsData struct {
	// Events is the list of invocation events.
	Events []InvocationEvent `json:"invocationEvents,omitempty"`
}

// IsIntermediateData marks InvocationEventsData as implementing IntermediateData.
func (*InvocationEventsData) IsIntermediateData() {}

// GetInvocationEvents returns the events.
func (i *InvocationEventsData) GetInvocationEvents() []InvocationEvent { return i.Events }

// GetToolUses returns nil for events format.
func (i *InvocationEventsData) GetToolUses() []genai.FunctionCall { return nil }

// GetToolResponses returns nil for events format.
func (i *InvocationEventsData) GetToolResponses() []genai.FunctionResponse { return nil }

// MarshalIntermediateData implements custom JSON marshaling for IntermediateData.
// It detects the concrete type and marshals accordingly.
func MarshalIntermediateData(data IntermediateData) ([]byte, error) {
	switch v := data.(type) {
	case *LegacyIntermediateData:
		return json.Marshal(v)
	case *InvocationEventsData:
		// Wrap in an object with the invocationEvents key.
		return json.Marshal(struct {
			InvocationEvents []InvocationEvent `json:"invocationEvents,omitempty"`
		}{InvocationEvents: v.Events})
	case nil:
		return []byte("null"), nil
	default:
		return nil, fmt.Errorf("unknown IntermediateData type %T", data)
	}
}

// UnmarshalIntermediateData implements custom JSON unmarshaling for
// IntermediateData. It detects the format by checking for the
// "invocationEvents" key (new format) vs "toolUses" key (old format).
func UnmarshalIntermediateData(data []byte) (IntermediateData, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}

	// Peek at the raw JSON to detect format.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal intermediate data: %w", err)
	}

	if _, hasEvents := raw["invocationEvents"]; hasEvents {
		var eventsData InvocationEventsData
		if err := json.Unmarshal(data, &eventsData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal invocation events: %w", err)
		}
		return &eventsData, nil
	}

	// Legacy format.
	var legacy LegacyIntermediateData
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("failed to unmarshal legacy intermediate data: %w", err)
	}
	return &legacy, nil
}

// Invocation represents a single user-agent interaction within an eval case.
type Invocation struct {
	// InvocationID is the unique identifier for this invocation.
	InvocationID string `json:"invocationId,omitempty"`

	// UserContent is the user's input content for this invocation.
	UserContent *genai.Content `json:"userContent,omitempty"`

	// FinalResponse is the agent's final response content.
	FinalResponse *genai.Content `json:"finalResponse,omitempty"`

	// IntermediateData contains tool calls, responses, and intermediate events.
	IntermediateData IntermediateData `json:"-"`

	// CreationTimestamp is the Unix timestamp of the invocation.
	CreationTimestamp float64 `json:"creationTimestamp,omitempty"`

	// Rubrics are rubrics specific to this invocation.
	Rubrics []Rubric `json:"rubrics,omitempty"`

	// AppDetails contains details about the agents involved.
	AppDetails *AppDetails `json:"appDetails,omitempty"`
}

// MarshalJSON implements custom JSON marshaling for Invocation to handle
// the IntermediateData interface.
func (inv Invocation) MarshalJSON() ([]byte, error) {
	type alias Invocation
	aux := struct {
		alias
		IntermediateData json.RawMessage `json:"intermediateData,omitempty"`
	}{}

	aux.alias = alias(inv)

	if inv.IntermediateData != nil {
		data, err := MarshalIntermediateData(inv.IntermediateData)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal intermediate data: %w", err)
		}
		aux.IntermediateData = data
	}

	return json.Marshal(aux)
}

// UnmarshalJSON implements custom JSON unmarshaling for Invocation to handle
// the IntermediateData interface.
func (inv *Invocation) UnmarshalJSON(data []byte) error {
	type alias Invocation
	aux := struct {
		alias
		IntermediateData json.RawMessage `json:"intermediateData,omitempty"`
	}{}

	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to unmarshal invocation: %w", err)
	}

	*inv = Invocation(aux.alias)

	if len(aux.IntermediateData) > 0 && string(aux.IntermediateData) != "null" {
		idata, err := UnmarshalIntermediateData(aux.IntermediateData)
		if err != nil {
			return fmt.Errorf("failed to unmarshal invocation intermediate data: %w", err)
		}
		inv.IntermediateData = idata
	}

	return nil
}

// EvalCase represents a single test case within an eval set.
type EvalCase struct {
	// EvalID is the unique identifier for this eval case.
	EvalID string `json:"evalId"`

	// Conversation is the static list of invocations (mutually exclusive
	// with ConversationScenario).
	Conversation []Invocation `json:"conversation,omitempty"`

	// ConversationScenario is the scenario for dynamic conversation
	// (mutually exclusive with Conversation).
	ConversationScenario *ConversationScenario `json:"conversationScenario,omitempty"`

	// SessionInput is the initial session state for this eval case.
	SessionInput *SessionInput `json:"sessionInput,omitempty"`

	// CreationTimestamp is the Unix timestamp of the eval case.
	CreationTimestamp float64 `json:"creationTimestamp,omitempty"`

	// Rubrics are rubrics applied to all invocations in this eval case.
	Rubrics []Rubric `json:"rubrics,omitempty"`

	// FinalSessionState is the expected final session state.
	FinalSessionState SessionState `json:"finalSessionState,omitempty"`
}

// UnmarshalJSON implements custom JSON unmarshaling for EvalCase to validate
// that exactly one of Conversation or ConversationScenario is set.
func (ec *EvalCase) UnmarshalJSON(data []byte) error {
	type alias EvalCase
	var aux alias
	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to unmarshal eval case: %w", err)
	}

	hasConversation := len(aux.Conversation) > 0
	hasScenario := aux.ConversationScenario != nil

	if hasConversation && hasScenario {
		return fmt.Errorf("both conversation and conversationScenario provided in EvalCase; provide exactly one")
	}
	if !hasConversation && !hasScenario {
		return fmt.Errorf("neither conversation nor conversationScenario provided in EvalCase; provide exactly one")
	}

	*ec = EvalCase(aux)
	return nil
}

// EvalSet represents a collection of eval cases.
type EvalSet struct {
	// EvalSetID is the unique identifier for this eval set.
	EvalSetID string `json:"evalSetId"`

	// Name is a human-readable name for the eval set.
	Name string `json:"name,omitempty"`

	// Description provides additional context about the eval set.
	Description string `json:"description,omitempty"`

	// EvalCases is the list of eval cases in this set.
	EvalCases []EvalCase `json:"evalCases,omitempty"`

	// CreationTimestamp is the Unix timestamp of the eval set.
	CreationTimestamp float64 `json:"creationTimestamp,omitempty"`
}

// NewEvalSet creates a new empty EvalSet with the given ID and current timestamp.
func NewEvalSet(evalSetID string) *EvalSet {
	return &EvalSet{
		EvalSetID:         evalSetID,
		Name:              evalSetID,
		EvalCases:         []EvalCase{},
		CreationTimestamp: float64(time.Now().Unix()),
	}
}
