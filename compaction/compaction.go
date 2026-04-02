// Package compaction provides utilities for managing event history size in
// ADK-Go sessions. It defines a [Strategy] interface for pluggable compaction
// algorithms, a [Config] struct for threshold-based triggering, and helper
// functions for applying compaction to both event slices and raw content slices.
//
// # Basic usage
//
// Create a [Config] with a strategy and thresholds, then call [Apply]:
//
//	tr := compaction.NewTruncation(10)
//	cfg := compaction.Config{
//	    Strategy:   tr,
//	    MaxEvents:  50,
//	    KeepRecent: 10,
//	}
//
//	compacted, err := compaction.Apply(ctx, cfg, events)
//
// # Working with raw Contents
//
// When you have []*genai.Content (e.g. the slice sent directly to an LLM), use
// [ApplyToContents], which wraps each Content in a temporary Event, runs the
// strategy, then extracts the Content back:
//
//	compacted, err := compaction.ApplyToContents(ctx, cfg, contents)
package compaction

import (
	"context"

	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// Strategy defines how a list of events is compacted.
//
// Implementations must be safe for concurrent use from a single goroutine
// (i.e. they do not need to be goroutine-safe themselves, but they must not
// retain mutable references to the input slice).
//
// Example — a custom strategy that drops every other event:
//
//	type everyOther struct{}
//
//	func (everyOther) Compact(_ context.Context, events []*session.Event) ([]*session.Event, error) {
//	    out := make([]*session.Event, 0, len(events)/2)
//	    for i, e := range events {
//	        if i%2 == 0 {
//	            out = append(out, e)
//	        }
//	    }
//	    return out, nil
//	}
type Strategy interface {
	Compact(ctx context.Context, events []*session.Event) ([]*session.Event, error)
}

// Config controls when and how compaction is triggered.
//
// Compaction fires when either the number of events exceeds MaxEvents or the
// estimated token count (total characters / 4) exceeds MaxTokens. A zero value
// for MaxEvents or MaxTokens disables that threshold check.
//
//	cfg := compaction.Config{
//	    Strategy:   compaction.NewTruncation(20),
//	    MaxEvents:  100,
//	    MaxTokens:  4000,
//	    KeepRecent: 20,
//	}
type Config struct {
	// Strategy is called when compaction is triggered.
	Strategy Strategy
	// MaxEvents is the maximum number of events allowed before compaction.
	// Zero means unlimited (threshold disabled).
	MaxEvents int
	// MaxTokens is the maximum estimated token count before compaction.
	// Tokens are estimated from text content only (~4 chars/token); binary
	// content (InlineData) is not included in the count.
	// Zero means unlimited (threshold disabled).
	MaxTokens int
	// KeepRecent is a hint to strategies about how many recent events to
	// preserve. It is not enforced by Apply itself; enforcement is the
	// responsibility of the Strategy.
	KeepRecent int
}

// Apply checks whether compaction is needed based on cfg and, if so, calls
// cfg.Strategy.Compact. If no threshold is exceeded the original slice is
// returned unchanged.
//
// A nil or empty events slice is returned as-is.
//
// Example:
//
//	compacted, err := compaction.Apply(ctx, cfg, compaction.CollectEvents(sess.Events()))
//	if err != nil {
//	    log.Fatal(err)
//	}
func Apply(ctx context.Context, cfg Config, events []*session.Event) ([]*session.Event, error) {
	if !needsCompaction(cfg, events) {
		return events, nil
	}

	// Split into old (to compact) and recent (to preserve verbatim).
	keepRecent := cfg.KeepRecent
	if keepRecent >= len(events) {
		return events, nil
	}
	old := events[:len(events)-keepRecent]
	recent := events[len(events)-keepRecent:]

	compacted, err := cfg.Strategy.Compact(ctx, old)
	if err != nil {
		return nil, err
	}
	return append(compacted, recent...), nil
}

// ApplyToContents works on a []*genai.Content slice — the representation sent
// directly to an LLM. Each Content is wrapped in a temporary [session.Event],
// the strategy is applied, and Content is extracted back from the results.
//
// This allows the same [Strategy] implementations to be reused for both
// event-level and content-level compaction.
//
// Example:
//
//	compacted, err := compaction.ApplyToContents(ctx, cfg, req.Contents)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	req.Contents = compacted
func ApplyToContents(ctx context.Context, cfg Config, contents []*genai.Content) ([]*genai.Content, error) {
	// Wrap each Content into a temporary Event.
	events := make([]*session.Event, len(contents))
	for i, c := range contents {
		events[i] = &session.Event{
			LLMResponse: model.LLMResponse{Content: c},
		}
	}

	compacted, err := Apply(ctx, cfg, events)
	if err != nil {
		return nil, err
	}

	// Extract Content back from the compacted events.
	out := make([]*genai.Content, 0, len(compacted))
	for _, e := range compacted {
		if e.Content != nil {
			out = append(out, e.Content)
		}
	}
	return out, nil
}

// CollectEvents converts the [session.Events] interface into a plain
// []*session.Event slice, preserving iteration order.
//
// Example:
//
//	events := compaction.CollectEvents(sess.Events())
//	compacted, err := compaction.Apply(ctx, cfg, events)
func CollectEvents(events session.Events) []*session.Event {
	out := make([]*session.Event, 0, events.Len())
	for e := range events.All() {
		out = append(out, e)
	}
	return out
}

// needsCompaction returns true when at least one enabled threshold is exceeded.
func needsCompaction(cfg Config, events []*session.Event) bool {
	if cfg.MaxEvents > 0 && len(events) > cfg.MaxEvents {
		return true
	}
	if cfg.MaxTokens > 0 && estimateTokens(events) > cfg.MaxTokens {
		return true
	}
	return false
}

// estimateTokens approximates the total token count for the given events by
// summing the character lengths of all text parts and dividing by 4.
// This matches the common rule-of-thumb used by many LLM providers.
//
// NOTE: only text content (Part.Text) is counted. InlineData (binary content
// such as images or files) is excluded from the estimate.
func estimateTokens(events []*session.Event) int {
	total := 0
	for _, e := range events {
		if e.Content == nil {
			continue
		}
		for _, p := range e.Content.Parts {
			total += len(p.Text)
		}
	}
	return total / 4
}
