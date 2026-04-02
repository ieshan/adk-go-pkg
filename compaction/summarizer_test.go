package compaction_test

import (
	"context"
	"iter"
	"strings"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	"github.com/ieshan/adk-go-pkg/compaction"
)

// mockLLM is a local in-test implementation of model.LLM that returns a
// fixed response string. It also records the last request it received so
// tests can inspect what was sent to the model.
type mockLLM struct {
	// response is the text that GenerateContent yields.
	response string
	// lastReq is set to the most recent *model.LLMRequest passed to GenerateContent.
	lastReq *model.LLMRequest
}

func (m *mockLLM) Name() string { return "mock" }

func (m *mockLLM) GenerateContent(_ context.Context, req *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	m.lastReq = req
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{
			Content:      genai.NewContentFromText(m.response, "model"),
			TurnComplete: true,
		}, nil)
	}
}

// TestSummarizer_Compact verifies that Compact with 20 text events produces a
// single summary event whose Author is "system" and whose Content contains
// the mock model's response text.
func TestSummarizer_Compact(t *testing.T) {
	const eventCount = 20
	const summaryText = "This is the summary."

	mock := &mockLLM{response: summaryText}
	s := compaction.NewSummarizer(mock)

	events := makeEvents(eventCount, "some event text")
	got, err := s.Compact(context.Background(), events)
	if err != nil {
		t.Fatalf("Compact returned unexpected error: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 summary event, got %d", len(got))
	}

	ev := got[0]
	if ev.Author != "system" {
		t.Errorf("expected Author %q, got %q", "system", ev.Author)
	}
	if ev.Content == nil {
		t.Fatal("summary event has nil Content")
	}
	if len(ev.Content.Parts) == 0 {
		t.Fatal("summary event Content has no Parts")
	}
	if ev.Content.Parts[0].Text != summaryText {
		t.Errorf("expected summary text %q, got %q", summaryText, ev.Content.Parts[0].Text)
	}
}

// TestSummarizer_CustomInstruction verifies that WithInstruction overrides the
// default summary prompt and that the custom instruction is passed to the model
// via the request's Config.SystemInstruction.
func TestSummarizer_CustomInstruction(t *testing.T) {
	const customInstruction = "Be very brief. One sentence only."

	mock := &mockLLM{response: "brief summary"}
	s := compaction.NewSummarizer(mock, compaction.WithInstruction(customInstruction))

	events := makeEvents(5, "hello world")
	_, err := s.Compact(context.Background(), events)
	if err != nil {
		t.Fatalf("Compact returned unexpected error: %v", err)
	}

	if mock.lastReq == nil {
		t.Fatal("expected GenerateContent to have been called, but lastReq is nil")
	}
	if mock.lastReq.Config == nil {
		t.Fatal("expected LLMRequest.Config to be non-nil")
	}
	si := mock.lastReq.Config.SystemInstruction
	if si == nil {
		t.Fatal("expected Config.SystemInstruction to be non-nil")
	}
	if len(si.Parts) == 0 {
		t.Fatal("SystemInstruction has no Parts")
	}
	if !strings.Contains(si.Parts[0].Text, customInstruction) {
		t.Errorf("expected SystemInstruction to contain %q, got %q", customInstruction, si.Parts[0].Text)
	}
}

// TestSummarizer_EmptyEvents verifies that Compact returns an empty slice
// (not an error) when the input event list is empty.
func TestSummarizer_EmptyEvents(t *testing.T) {
	mock := &mockLLM{response: "should not be called"}
	s := compaction.NewSummarizer(mock)

	got, err := s.Compact(context.Background(), []*session.Event{})
	if err != nil {
		t.Fatalf("Compact returned unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 events for empty input, got %d", len(got))
	}

	// The model should NOT have been called for empty input.
	if mock.lastReq != nil {
		t.Error("expected GenerateContent not to be called for empty input, but it was")
	}
}
