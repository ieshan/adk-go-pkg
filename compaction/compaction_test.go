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

// mockEvents implements session.Events for testing.
type mockEvents struct {
	events []*session.Event
}

func (m *mockEvents) All() iter.Seq[*session.Event] {
	return func(yield func(*session.Event) bool) {
		for _, e := range m.events {
			if !yield(e) {
				return
			}
		}
	}
}

func (m *mockEvents) Len() int { return len(m.events) }

func (m *mockEvents) At(i int) *session.Event { return m.events[i] }

// makeEvent creates a session.Event with a text part.
func makeEvent(text string) *session.Event {
	return &session.Event{
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{
				Role:  "user",
				Parts: []*genai.Part{{Text: text}},
			},
		},
	}
}

// makeEvents creates n events each with the given text.
func makeEvents(n int, text string) []*session.Event {
	events := make([]*session.Event, n)
	for i := range events {
		events[i] = makeEvent(text)
	}
	return events
}

// identityStrategy is a no-op Strategy used in tests.
type identityStrategy struct{}

func (identityStrategy) Compact(_ context.Context, events []*session.Event) ([]*session.Event, error) {
	return events, nil
}

// truncateStrategy keeps the last N events.
type truncateStrategy struct{ n int }

func (t truncateStrategy) Compact(_ context.Context, events []*session.Event) ([]*session.Event, error) {
	if len(events) <= t.n {
		return events, nil
	}
	return events[len(events)-t.n:], nil
}

// TestApply_BelowThreshold verifies that Apply does not invoke compaction when
// the event count is below MaxEvents and estimated tokens are below MaxTokens.
func TestApply_BelowThreshold(t *testing.T) {
	events := makeEvents(10, "hello")
	cfg := compaction.Config{
		Strategy:  identityStrategy{},
		MaxEvents: 50,
		MaxTokens: 10000,
	}

	got, err := compaction.Apply(context.Background(), cfg, events)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if len(got) != len(events) {
		t.Errorf("expected %d events, got %d", len(events), len(got))
	}
}

// TestApply_ExceedsMaxEvents verifies that Apply triggers compaction when the
// event count exceeds MaxEvents.
func TestApply_ExceedsMaxEvents(t *testing.T) {
	events := makeEvents(60, "hello")
	cfg := compaction.Config{
		Strategy:   truncateStrategy{n: 10},
		MaxEvents:  50,
		KeepRecent: 10,
	}

	got, err := compaction.Apply(context.Background(), cfg, events)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if len(got) >= len(events) {
		t.Errorf("expected compaction to reduce events; got %d (original %d)", len(got), len(events))
	}
}

// TestApply_ExceedsMaxTokens verifies that Apply triggers compaction when the
// estimated token count exceeds MaxTokens.
func TestApply_ExceedsMaxTokens(t *testing.T) {
	// Each event has 400-char text, so ~100 tokens per event; 5 events = ~500 tokens.
	longText := strings.Repeat("a", 400)
	events := makeEvents(5, longText)
	cfg := compaction.Config{
		Strategy:  truncateStrategy{n: 2},
		MaxTokens: 100, // Below ~500 estimated tokens
	}

	got, err := compaction.Apply(context.Background(), cfg, events)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if len(got) >= len(events) {
		t.Errorf("expected compaction to reduce events; got %d (original %d)", len(got), len(events))
	}
}

// TestApply_KeepRecent verifies that Apply splits events into old (compacted)
// and recent (preserved verbatim), then returns compacted + recent.
func TestApply_KeepRecent(t *testing.T) {
	const total = 60
	const keepRecent = 5

	events := make([]*session.Event, total)
	for i := range events {
		text := "old"
		if i >= total-keepRecent {
			text = "recent"
		}
		events[i] = makeEvent(text)
	}

	// Strategy that keeps only the last 3 of whatever it receives.
	// Apply will pass it the old portion (55 events), so it returns 3 "old".
	// Then Apply appends the 5 "recent" events. Result: 3 old + 5 recent = 8.
	cfg := compaction.Config{
		Strategy:   truncateStrategy{n: 3},
		MaxEvents:  50,
		KeepRecent: keepRecent,
	}

	got, err := compaction.Apply(context.Background(), cfg, events)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if len(got) != 3+keepRecent {
		t.Fatalf("expected %d events, got %d", 3+keepRecent, len(got))
	}

	// First 3 are compacted old events.
	for i := 0; i < 3; i++ {
		if got[i].Content.Parts[0].Text != "old" {
			t.Errorf("event %d: expected 'old', got %q", i, got[i].Content.Parts[0].Text)
		}
	}

	// Last 5 are preserved recent events.
	for i := 3; i < len(got); i++ {
		if got[i].Content.Parts[0].Text != "recent" {
			t.Errorf("event %d: expected 'recent', got %q", i, got[i].Content.Parts[0].Text)
		}
	}
}

// TestCollectEvents verifies that CollectEvents converts a session.Events
// interface into a []*session.Event slice with the same length and order.
func TestCollectEvents(t *testing.T) {
	src := makeEvents(7, "test")
	mock := &mockEvents{events: src}

	got := compaction.CollectEvents(mock)
	if len(got) != len(src) {
		t.Fatalf("expected %d events, got %d", len(src), len(got))
	}
	for i, e := range got {
		if e != src[i] {
			t.Errorf("event %d: pointer mismatch", i)
		}
	}
}

// TestApplyToContents_BelowThreshold verifies that ApplyToContents returns the
// original contents unchanged when thresholds are not exceeded.
func TestApplyToContents_BelowThreshold(t *testing.T) {
	contents := make([]*genai.Content, 5)
	for i := range contents {
		contents[i] = &genai.Content{
			Role:  "user",
			Parts: []*genai.Part{{Text: "hello"}},
		}
	}

	cfg := compaction.Config{
		Strategy:  identityStrategy{},
		MaxEvents: 50,
		MaxTokens: 10000,
	}

	got, err := compaction.ApplyToContents(context.Background(), cfg, contents)
	if err != nil {
		t.Fatalf("ApplyToContents returned error: %v", err)
	}
	if len(got) != len(contents) {
		t.Errorf("expected %d contents, got %d", len(contents), len(got))
	}
}

// TestApplyToContents_ExceedsThreshold verifies that ApplyToContents triggers
// compaction and returns fewer contents when MaxEvents is exceeded.
func TestApplyToContents_ExceedsThreshold(t *testing.T) {
	const total = 60
	const keepRecent = 10

	contents := make([]*genai.Content, total)
	for i := range contents {
		contents[i] = &genai.Content{
			Role:  "user",
			Parts: []*genai.Part{{Text: "hello"}},
		}
	}

	cfg := compaction.Config{
		Strategy:   truncateStrategy{n: keepRecent},
		MaxEvents:  50,
		KeepRecent: keepRecent,
	}

	got, err := compaction.ApplyToContents(context.Background(), cfg, contents)
	if err != nil {
		t.Fatalf("ApplyToContents returned error: %v", err)
	}
	if len(got) >= total {
		t.Errorf("expected fewer than %d contents after compaction; got %d", total, len(got))
	}
}
