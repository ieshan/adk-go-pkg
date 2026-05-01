package testutil

import (
	"iter"
	"strings"
	"time"

	"google.golang.org/adk/memory"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// ---------------------------------------------------------------------------
// Content Builders
// ---------------------------------------------------------------------------

// NewContent creates a genai.Content with text and role.
func NewContent(text string, role genai.Role) *genai.Content {
	return genai.NewContentFromText(text, role)
}

// NewUserContent creates a genai.Content with role "user".
func NewUserContent(text string) *genai.Content {
	return genai.NewContentFromText(text, genai.RoleUser)
}

// NewModelContent creates a genai.Content with role "model".
func NewModelContent(text string) *genai.Content {
	return genai.NewContentFromText(text, genai.RoleModel)
}

// NewContentWithParts creates content with multiple parts and the given role.
func NewContentWithParts(role string, parts ...*genai.Part) *genai.Content {
	return &genai.Content{
		Role:  role,
		Parts: parts,
	}
}

// NewTextPart creates a text part.
func NewTextPart(text string) *genai.Part {
	return &genai.Part{Text: text}
}

// NewInlineDataPart creates an inline data part with the given MIME type and
// raw bytes.
func NewInlineDataPart(mimeType string, data []byte) *genai.Part {
	return &genai.Part{
		InlineData: &genai.Blob{MIMEType: mimeType, Data: data},
	}
}

// ---------------------------------------------------------------------------
// Event Builders
// ---------------------------------------------------------------------------

// NewEvent creates a session.Event with the given author and content.
// The event ID and timestamp are auto-generated.
func NewEvent(author string, content *genai.Content) *session.Event {
	e := session.NewEvent("")
	e.Author = author
	e.LLMResponse = model.LLMResponse{Content: content}
	return e
}

// NewEventWithInvocationID creates a session.Event with a specific invocation
// ID, author, and content.
func NewEventWithInvocationID(invID, author string, content *genai.Content) *session.Event {
	e := session.NewEvent(invID)
	e.Author = author
	e.LLMResponse = model.LLMResponse{Content: content}
	return e
}

// NewTextEvent creates a session.Event with a text content for the given author.
func NewTextEvent(author, text string) *session.Event {
	return NewEvent(author, genai.NewContentFromText(text, genai.Role(author)))
}

// NewFunctionCallEvent creates a session.Event containing function calls.
func NewFunctionCallEvent(author string, calls ...*genai.FunctionCall) *session.Event {
	parts := make([]*genai.Part, len(calls))
	for i, fc := range calls {
		parts[i] = &genai.Part{FunctionCall: fc}
	}
	return NewEvent(author, &genai.Content{Role: author, Parts: parts})
}

// NewFunctionResponseEvent creates a session.Event containing function responses.
func NewFunctionResponseEvent(author string, responses ...*genai.FunctionResponse) *session.Event {
	parts := make([]*genai.Part, len(responses))
	for i, fr := range responses {
		parts[i] = &genai.Part{FunctionResponse: fr}
	}
	return NewEvent(author, &genai.Content{Role: author, Parts: parts})
}

// NewTransferEvent creates a session.Event that transfers to another agent.
func NewTransferEvent(author, targetAgent string) *session.Event {
	e := NewEvent(author, genai.NewContentFromText("", genai.Role(author)))
	e.Actions.TransferToAgent = targetAgent
	return e
}

// ---------------------------------------------------------------------------
// LLM Response Builders
// ---------------------------------------------------------------------------

// NewTextResponse creates a simple text LLMResponse with TurnComplete=true.
func NewTextResponse(text string) model.LLMResponse {
	return model.LLMResponse{
		Content:      genai.NewContentFromText(text, genai.RoleModel),
		TurnComplete: true,
	}
}

// NewPartialTextResponse creates a partial text LLMResponse for streaming.
func NewPartialTextResponse(text string) model.LLMResponse {
	return model.LLMResponse{
		Content: genai.NewContentFromText(text, genai.RoleModel),
		Partial: true,
	}
}

// NewFunctionCallResponse creates an LLMResponse containing function calls.
func NewFunctionCallResponse(calls ...*genai.FunctionCall) model.LLMResponse {
	parts := make([]*genai.Part, len(calls))
	for i, fc := range calls {
		parts[i] = &genai.Part{FunctionCall: fc}
	}
	return model.LLMResponse{
		Content:      &genai.Content{Role: genai.RoleModel, Parts: parts},
		TurnComplete: true,
	}
}

// NewFunctionResponseLLMResponse creates an LLMResponse containing function
// response parts.
func NewFunctionResponseLLMResponse(responses ...*genai.FunctionResponse) model.LLMResponse {
	parts := make([]*genai.Part, len(responses))
	for i, fr := range responses {
		parts[i] = &genai.Part{FunctionResponse: fr}
	}
	return model.LLMResponse{
		Content:      &genai.Content{Role: genai.RoleUser, Parts: parts},
		TurnComplete: true,
	}
}

// NewErrorResponse creates an LLMResponse with error code and message.
func NewErrorResponse(code, message string) model.LLMResponse {
	return model.LLMResponse{
		ErrorCode:    code,
		ErrorMessage: message,
		TurnComplete: true,
	}
}

// ---------------------------------------------------------------------------
// LLM Request Builders
// ---------------------------------------------------------------------------

// NewLLMRequest creates an LLM request with contents.
func NewLLMRequest(contents ...*genai.Content) *model.LLMRequest {
	return &model.LLMRequest{
		Contents: contents,
	}
}

// NewLLMRequestWithConfig creates an LLM request with contents and config.
func NewLLMRequestWithConfig(config *genai.GenerateContentConfig, contents ...*genai.Content) *model.LLMRequest {
	return &model.LLMRequest{
		Contents: contents,
		Config:   config,
	}
}

// ---------------------------------------------------------------------------
// Function Call/Response Builders
// ---------------------------------------------------------------------------

// NewFunctionCall creates a genai.FunctionCall with a name and args.
func NewFunctionCall(name string, args map[string]any) *genai.FunctionCall {
	return &genai.FunctionCall{
		Name: name,
		Args: args,
	}
}

// NewFunctionCallWithID creates a genai.FunctionCall with a specific ID.
func NewFunctionCallWithID(name, id string, args map[string]any) *genai.FunctionCall {
	fc := NewFunctionCall(name, args)
	fc.ID = id
	return fc
}

// NewFunctionResponseForCall creates a genai.FunctionResponse for a function
// call by name.
func NewFunctionResponseForCall(name string, result map[string]any) *genai.FunctionResponse {
	return &genai.FunctionResponse{
		Name:     name,
		Response: result,
	}
}

// NewFunctionResponseWithID creates a genai.FunctionResponse with a specific
// ID.
func NewFunctionResponseWithID(name, id string, result map[string]any) *genai.FunctionResponse {
	fr := NewFunctionResponseForCall(name, result)
	fr.ID = id
	return fr
}

// ---------------------------------------------------------------------------
// Event Collection Helpers
// ---------------------------------------------------------------------------

// CollectEvents collects all events from an iterator, returning them as a
// slice. It returns the first error encountered, or nil if all events were
// yielded successfully.
func CollectEvents(seq iter.Seq2[*session.Event, error]) ([]*session.Event, error) {
	var events []*session.Event
	for event, err := range seq {
		if err != nil {
			return events, err
		}
		if event != nil {
			events = append(events, event)
		}
	}
	return events, nil
}

// CollectFinalEvents collects only final (IsFinalResponse=true) events from an
// iterator.
func CollectFinalEvents(seq iter.Seq2[*session.Event, error]) ([]*session.Event, error) {
	var events []*session.Event
	for event, err := range seq {
		if err != nil {
			return events, err
		}
		if event != nil && event.IsFinalResponse() {
			events = append(events, event)
		}
	}
	return events, nil
}

// FindEventsByAuthor filters events by author name.
func FindEventsByAuthor(events []*session.Event, author string) []*session.Event {
	var result []*session.Event
	for _, e := range events {
		if e.Author == author {
			result = append(result, e)
		}
	}
	return result
}

// FindFunctionCallEvents returns events that contain function calls.
func FindFunctionCallEvents(events []*session.Event) []*session.Event {
	var result []*session.Event
	for _, e := range events {
		if e.Content == nil {
			continue
		}
		for _, p := range e.Content.Parts {
			if p.FunctionCall != nil {
				result = append(result, e)
				break
			}
		}
	}
	return result
}

// FindFunctionResponseEvents returns events that contain function responses.
func FindFunctionResponseEvents(events []*session.Event) []*session.Event {
	var result []*session.Event
	for _, e := range events {
		if e.Content == nil {
			continue
		}
		for _, p := range e.Content.Parts {
			if p.FunctionResponse != nil {
				result = append(result, e)
				break
			}
		}
	}
	return result
}

// ExtractTextFromEvents extracts all text content from events, joining with
// newlines.
func ExtractTextFromEvents(events []*session.Event) string {
	var texts []string
	for _, e := range events {
		if e.Content == nil {
			continue
		}
		for _, p := range e.Content.Parts {
			if p.Text != "" {
				texts = append(texts, p.Text)
			}
		}
	}
	return strings.Join(texts, "\n")
}

// NewMemoryEntry creates a memory.Entry for use with FakeMemoryService.
func NewMemoryEntry(id, content, author string) memory.Entry {
	return memory.Entry{
		ID:        id,
		Content:   genai.NewContentFromText(content, genai.Role(author)),
		Author:    author,
		Timestamp: time.Now(),
	}
}
