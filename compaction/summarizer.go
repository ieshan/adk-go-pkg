package compaction

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

const defaultSummaryInstruction = "Summarize the following conversation concisely, preserving key decisions, facts, and action items."

// Summarizer is a [Strategy] that compacts a set of events by asking an LLM
// to produce a concise summary of them. The resulting compacted history is a
// single [session.Event] authored by "system" whose Content carries the
// summary text.
//
// Summarizer is well-suited for long-running conversations where the complete
// event history would exceed an LLM's context window, but key information must
// still be preserved for future turns.
//
// Example:
//
//	s := compaction.NewSummarizer(myLLM,
//	    compaction.WithInstruction("Summarize in three bullet points."),
//	)
//	cfg := compaction.Config{
//	    Strategy:  s,
//	    MaxEvents: 100,
//	}
//	compacted, err := compaction.Apply(ctx, cfg, events)
type Summarizer struct {
	// Model is the LLM used to generate the summary.
	Model model.LLM
	// SummaryInstruction is the system prompt sent to the model.
	// If empty, [defaultSummaryInstruction] is used.
	SummaryInstruction string
}

// SummarizerOption is a functional option for configuring a [Summarizer].
type SummarizerOption func(*Summarizer)

// WithInstruction returns a [SummarizerOption] that replaces the default
// summary prompt with the given instruction.
//
// Example:
//
//	s := compaction.NewSummarizer(m, compaction.WithInstruction("Be very brief."))
func WithInstruction(instruction string) SummarizerOption {
	return func(s *Summarizer) {
		s.SummaryInstruction = instruction
	}
}

// NewSummarizer creates a [Summarizer] backed by the given [model.LLM].
// Zero or more [SummarizerOption] values may be passed to customise behaviour.
//
// The returned *Summarizer implements [Strategy] and can be used directly in a
// [Config].
func NewSummarizer(m model.LLM, opts ...SummarizerOption) *Summarizer {
	s := &Summarizer{
		Model:              m,
		SummaryInstruction: defaultSummaryInstruction,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Compact summarises the given events using the configured LLM and returns a
// single [session.Event] containing the summary.
//
// Events whose Content field is nil are silently skipped; only text parts are
// included in the conversation transcript sent to the model.
//
// An empty (non-nil) input slice is returned as-is without calling the model.
//
// The returned event has:
//   - Author set to "system"
//   - Content containing the summary text returned by the LLM
func (s *Summarizer) Compact(ctx context.Context, events []*session.Event) ([]*session.Event, error) {
	if len(events) == 0 {
		return []*session.Event{}, nil
	}

	// Build a concatenated transcript of all event texts.
	var sb strings.Builder
	for _, e := range events {
		if e.Content == nil {
			continue
		}
		for _, p := range e.Content.Parts {
			if p.Text != "" {
				sb.WriteString(p.Text)
				sb.WriteString("\n")
			}
		}
	}

	instruction := s.SummaryInstruction
	if instruction == "" {
		instruction = defaultSummaryInstruction
	}

	req := &model.LLMRequest{
		Model: s.Model.Name(),
		Contents: []*genai.Content{
			genai.NewContentFromText(sb.String(), "user"),
		},
		Config: &genai.GenerateContentConfig{
			SystemInstruction: genai.NewContentFromText(instruction, "system"),
		},
	}

	// Call the model (non-streaming) and consume the iterator to get the final
	// response.
	var finalResponse *model.LLMResponse
	for resp, err := range s.Model.GenerateContent(ctx, req, false) {
		if err != nil {
			return nil, fmt.Errorf("compaction/summarizer: LLM call failed: %w", err)
		}
		finalResponse = resp
	}

	// Build the summary event.
	summaryEvent := session.NewEvent("")
	summaryEvent.Author = "system"
	if finalResponse != nil && finalResponse.Content != nil {
		summaryEvent.LLMResponse = model.LLMResponse{
			Content: finalResponse.Content,
		}
	}

	return []*session.Event{summaryEvent}, nil
}
