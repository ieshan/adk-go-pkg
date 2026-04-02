package compaction_test

import (
	"context"
	"testing"

	"github.com/ieshan/adk-go-pkg/compaction"
)

// TestTruncation_RetainCount verifies that Compact returns the last RetainCount
// events when there are more events than RetainCount.
func TestTruncation_RetainCount(t *testing.T) {
	const total = 20
	const retain = 5

	events := makeEvents(total, "msg")
	// Replace last 'retain' events with distinctly tagged text.
	for i := total - retain; i < total; i++ {
		events[i] = makeEvent("last")
	}

	tr := compaction.NewTruncation(retain)
	got, err := tr.Compact(context.Background(), events)
	if err != nil {
		t.Fatalf("Compact returned error: %v", err)
	}
	if len(got) != retain {
		t.Fatalf("expected %d events, got %d", retain, len(got))
	}
	for i, e := range got {
		if e.Content == nil || len(e.Content.Parts) == 0 {
			t.Fatalf("event %d has no content", i)
		}
		if e.Content.Parts[0].Text != "last" {
			t.Errorf("event %d: expected %q, got %q", i, "last", e.Content.Parts[0].Text)
		}
	}
}

// TestTruncation_FewerThanRetain verifies that Compact returns all events
// unchanged when there are fewer events than RetainCount.
func TestTruncation_FewerThanRetain(t *testing.T) {
	events := makeEvents(3, "hello")
	tr := compaction.NewTruncation(5)

	got, err := tr.Compact(context.Background(), events)
	if err != nil {
		t.Fatalf("Compact returned error: %v", err)
	}
	if len(got) != len(events) {
		t.Errorf("expected %d events (all), got %d", len(events), len(got))
	}
	for i, e := range got {
		if e != events[i] {
			t.Errorf("event %d: pointer mismatch", i)
		}
	}
}

// TestTruncation_RetainZero verifies that Compact returns an empty slice when
// RetainCount is 0.
func TestTruncation_RetainZero(t *testing.T) {
	events := makeEvents(10, "hello")
	tr := compaction.NewTruncation(0)

	got, err := tr.Compact(context.Background(), events)
	if err != nil {
		t.Fatalf("Compact returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 events, got %d", len(got))
	}
}
