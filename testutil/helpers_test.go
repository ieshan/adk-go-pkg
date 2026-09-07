package testutil_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

var errCollectEventsBoom = errors.New("boom")

func TestNewContent(t *testing.T) {
	c := testutil.NewContent("hello", genai.RoleUser)
	if c.Parts[0].Text != "hello" {
		t.Errorf("text = %q, want %q", c.Parts[0].Text, "hello")
	}
	if c.Role != string(genai.RoleUser) {
		t.Errorf("role = %q, want %q", c.Role, genai.RoleUser)
	}
}

func TestNewUserContent(t *testing.T) {
	c := testutil.NewUserContent("hi")
	if c.Role != string(genai.RoleUser) {
		t.Errorf("role = %q, want user", c.Role)
	}
}

func TestNewModelContent(t *testing.T) {
	c := testutil.NewModelContent("response")
	if c.Role != string(genai.RoleModel) {
		t.Errorf("role = %q, want model", c.Role)
	}
}

func TestNewTextPart(t *testing.T) {
	p := testutil.NewTextPart("hello")
	if p.Text != "hello" {
		t.Errorf("Text = %q, want %q", p.Text, "hello")
	}
}

func TestNewInlineDataPart(t *testing.T) {
	p := testutil.NewInlineDataPart("image/png", []byte{1, 2, 3})
	if p.InlineData.MIMEType != "image/png" {
		t.Errorf("MIMEType = %q, want %q", p.InlineData.MIMEType, "image/png")
	}
}

func TestNewEvent(t *testing.T) {
	e := testutil.NewEvent(context.Background(), "user", testutil.NewUserContent("hi"))
	if e.Author != "user" {
		t.Errorf("Author = %q, want %q", e.Author, "user")
	}
	if e.Content.Parts[0].Text != "hi" {
		t.Errorf("text = %q, want %q", e.Content.Parts[0].Text, "hi")
	}
	if e.ID == "" {
		t.Error("ID should be auto-generated")
	}
}

func TestNewTextEvent(t *testing.T) {
	e := testutil.NewTextEvent(context.Background(), "model", "hello")
	if e.Author != "model" {
		t.Errorf("Author = %q, want %q", e.Author, "model")
	}
}

func TestNewFunctionCallEvent(t *testing.T) {
	fc := testutil.NewFunctionCall("search", map[string]any{"query": "test"})
	e := testutil.NewFunctionCallEvent(context.Background(), "model", fc)
	if e.Author != "model" {
		t.Errorf("Author = %q, want %q", e.Author, "model")
	}
	if e.Content.Parts[0].FunctionCall.Name != "search" {
		t.Errorf("FunctionCall name = %q, want %q", e.Content.Parts[0].FunctionCall.Name, "search")
	}
}

func TestNewFunctionResponseEvent(t *testing.T) {
	fr := testutil.NewFunctionResponseForCall("search", map[string]any{"result": "found"})
	e := testutil.NewFunctionResponseEvent(context.Background(), "user", fr)
	if e.Content.Parts[0].FunctionResponse.Name != "search" {
		t.Errorf("FunctionResponse name = %q, want %q", e.Content.Parts[0].FunctionResponse.Name, "search")
	}
}

func TestNewTransferEvent(t *testing.T) {
	e := testutil.NewTransferEvent(context.Background(), "model", "sub-agent")
	if e.Actions.TransferToAgent != "sub-agent" {
		t.Errorf("TransferToAgent = %q, want %q", e.Actions.TransferToAgent, "sub-agent")
	}
}

func TestNewTextResponse(t *testing.T) {
	r := testutil.NewTextResponse("hello")
	if r.Content.Parts[0].Text != "hello" {
		t.Errorf("text = %q, want %q", r.Content.Parts[0].Text, "hello")
	}
	if !r.TurnComplete {
		t.Error("TurnComplete should be true")
	}
	if r.Partial {
		t.Error("Partial should be false")
	}
}

func TestNewPartialTextResponse(t *testing.T) {
	r := testutil.NewPartialTextResponse("chunk")
	if !r.Partial {
		t.Error("Partial should be true")
	}
	if r.TurnComplete {
		t.Error("TurnComplete should be false")
	}
}

func TestNewFunctionCallResponse(t *testing.T) {
	fc := testutil.NewFunctionCall("search", map[string]any{"q": "test"})
	r := testutil.NewFunctionCallResponse(fc)
	if r.Content.Parts[0].FunctionCall.Name != "search" {
		t.Errorf("FunctionCall name = %q, want %q", r.Content.Parts[0].FunctionCall.Name, "search")
	}
	if !r.TurnComplete {
		t.Error("TurnComplete should be true")
	}
}

func TestNewErrorResponse(t *testing.T) {
	r := testutil.NewErrorResponse("RATE_LIMIT", "too many requests")
	if r.ErrorCode != "RATE_LIMIT" {
		t.Errorf("ErrorCode = %q, want %q", r.ErrorCode, "RATE_LIMIT")
	}
	if r.ErrorMessage != "too many requests" {
		t.Errorf("ErrorMessage = %q, want %q", r.ErrorMessage, "too many requests")
	}
}

func TestNewFunctionCall(t *testing.T) {
	fc := testutil.NewFunctionCall("search", map[string]any{"query": "test"})
	if fc.Name != "search" {
		t.Errorf("Name = %q, want %q", fc.Name, "search")
	}
	if fc.Args["query"] != "test" {
		t.Errorf("Args[query] = %v, want test", fc.Args["query"])
	}
}

func TestNewFunctionCallWithID(t *testing.T) {
	fc := testutil.NewFunctionCallWithID("search", "fc-123", nil)
	if fc.ID != "fc-123" {
		t.Errorf("ID = %q, want %q", fc.ID, "fc-123")
	}
}

func TestNewFunctionResponseForCall(t *testing.T) {
	fr := testutil.NewFunctionResponseForCall("search", map[string]any{"result": "found"})
	if fr.Name != "search" {
		t.Errorf("Name = %q, want %q", fr.Name, "search")
	}
	if fr.Response["result"] != "found" {
		t.Errorf("Response[result] = %v, want found", fr.Response["result"])
	}
}

func TestNewFunctionResponseWithID(t *testing.T) {
	fr := testutil.NewFunctionResponseWithID("search", "fc-123", nil)
	if fr.ID != "fc-123" {
		t.Errorf("ID = %q, want %q", fr.ID, "fc-123")
	}
}

func TestCollectEvents(t *testing.T) {
	e1 := testutil.NewTextEvent(context.Background(), "user", "hello")
	e2 := testutil.NewTextEvent(context.Background(), "model", "hi")

	seq := func(yield func(*session.Event, error) bool) {
		yield(e1, nil)
		yield(e2, nil)
	}

	events, err := testutil.CollectEvents(seq)
	if err != nil {
		t.Fatalf("CollectEvents() error = %v", err)
	}
	if len(events) != 2 {
		t.Errorf("CollectEvents() count = %d, want 2", len(events))
	}
}

func TestCollectEvents_WithError(t *testing.T) {
	seq := func(yield func(*session.Event, error) bool) {
		yield(testutil.NewTextEvent(context.Background(), "user", "hi"), nil)
		yield(nil, errCollectEventsBoom)
	}

	events, err := testutil.CollectEvents(seq)
	if !errors.Is(err, errCollectEventsBoom) {
		t.Errorf("CollectEvents() error = %v, want errCollectEventsBoom", err)
	}
	if len(events) != 1 {
		t.Errorf("CollectEvents() count before error = %d, want 1", len(events))
	}
}

func TestCollectFinalEvents(t *testing.T) {
	e1 := testutil.NewTextEvent(context.Background(), "model", "partial")
	e1.Partial = true
	e2 := testutil.NewTextEvent(context.Background(), "model", "final")

	seq := func(yield func(*session.Event, error) bool) {
		yield(e1, nil)
		yield(e2, nil)
	}

	events, err := testutil.CollectFinalEvents(seq)
	if err != nil {
		t.Fatalf("CollectFinalEvents() error = %v", err)
	}
	// Only e2 should be final (no Partial, no function calls/responses).
	if len(events) != 1 {
		t.Errorf("CollectFinalEvents() count = %d, want 1", len(events))
	}
}

func TestFindEventsByAuthor(t *testing.T) {
	events := []*session.Event{
		testutil.NewTextEvent(context.Background(), "user", "a"),
		testutil.NewTextEvent(context.Background(), "model", "b"),
		testutil.NewTextEvent(context.Background(), "user", "c"),
	}
	filtered := testutil.FindEventsByAuthor(events, "user")
	if len(filtered) != 2 {
		t.Errorf("FindEventsByAuthor(user) = %d, want 2", len(filtered))
	}
}

func TestFindFunctionCallEvents(t *testing.T) {
	events := []*session.Event{
		testutil.NewTextEvent(context.Background(), "user", "hello"),
		testutil.NewFunctionCallEvent(context.Background(), "model", testutil.NewFunctionCall("search", nil)),
		testutil.NewTextEvent(context.Background(), "model", "results"),
	}
	filtered := testutil.FindFunctionCallEvents(events)
	if len(filtered) != 1 {
		t.Errorf("FindFunctionCallEvents() = %d, want 1", len(filtered))
	}
}

func TestFindFunctionResponseEvents(t *testing.T) {
	events := []*session.Event{
		testutil.NewFunctionCallEvent(context.Background(), "model", testutil.NewFunctionCall("search", nil)),
		testutil.NewFunctionResponseEvent(context.Background(), "user", testutil.NewFunctionResponseForCall("search", nil)),
	}
	filtered := testutil.FindFunctionResponseEvents(events)
	if len(filtered) != 1 {
		t.Errorf("FindFunctionResponseEvents() = %d, want 1", len(filtered))
	}
}

func TestExtractTextFromEvents(t *testing.T) {
	events := []*session.Event{
		testutil.NewTextEvent(context.Background(), "user", "hello"),
		testutil.NewTextEvent(context.Background(), "model", "world"),
	}
	text := testutil.ExtractTextFromEvents(events)
	if text == "" {
		t.Error("ExtractTextFromEvents() returned empty string")
	}
}

func TestNewMemoryEntry(t *testing.T) {
	e := testutil.NewMemoryEntry("e1", "hello world", "model")
	if e.ID != "e1" {
		t.Errorf("ID = %q, want %q", e.ID, "e1")
	}
	if e.Author != "model" {
		t.Errorf("Author = %q, want %q", e.Author, "model")
	}
}

func TestNewLLMRequest(t *testing.T) {
	req := testutil.NewLLMRequest(testutil.NewUserContent("hi"))
	if len(req.Contents) != 1 {
		t.Errorf("Contents count = %d, want 1", len(req.Contents))
	}
}

func TestNewLLMRequestWithConfig(t *testing.T) {
	cfg := &genai.GenerateContentConfig{Temperature: float32Ptr(0.5)}
	req := testutil.NewLLMRequestWithConfig(cfg, testutil.NewUserContent("hi"))
	if req.Config == nil || req.Config.Temperature == nil {
		t.Error("Config should be set")
	}
}

func float32Ptr(v float32) *float32 { return &v }

func TestNewContentWithParts(t *testing.T) {
	p1 := testutil.NewTextPart("hello")
	p2 := testutil.NewTextPart("world")
	c := testutil.NewContentWithParts("user", p1, p2)
	if len(c.Parts) != 2 {
		t.Errorf("Parts count = %d, want 2", len(c.Parts))
	}
}

func TestNewEventWithInvocationID(t *testing.T) {
	e := testutil.NewEventWithInvocationID(context.Background(), "inv-42", "user", testutil.NewUserContent("hi"))
	if e.InvocationID != "inv-42" {
		t.Errorf("InvocationID = %q, want %q", e.InvocationID, "inv-42")
	}
}

func TestNewFunctionResponseLLMResponse(t *testing.T) {
	fr := testutil.NewFunctionResponseForCall("search", map[string]any{"result": "ok"})
	r := testutil.NewFunctionResponseLLMResponse(fr)
	if r.Content.Parts[0].FunctionResponse.Name != "search" {
		t.Errorf("FunctionResponse name = %q, want %q", r.Content.Parts[0].FunctionResponse.Name, "search")
	}
}

// Compile-time check that model.LLMResponse fields we use exist.
var _ = model.LLMResponse{}.Partial
var _ = model.LLMResponse{}.TurnComplete
