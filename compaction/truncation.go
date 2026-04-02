package compaction

import (
	"context"

	"google.golang.org/adk/session"
)

// Truncation is a [Strategy] that keeps only the last RetainCount events,
// discarding all earlier ones. It is the simplest compaction strategy and is
// useful when only recent context matters.
//
// When the event count is already at or below RetainCount, all events are
// returned unchanged.
//
// Example:
//
//	tr := compaction.NewTruncation(20)
//	cfg := compaction.Config{
//	    Strategy:  tr,
//	    MaxEvents: 100,
//	}
//	compacted, err := compaction.Apply(ctx, cfg, events)
type Truncation struct {
	// RetainCount is the maximum number of events to keep. Events are kept from
	// the end of the slice (i.e. the most recent events).
	RetainCount int
}

// NewTruncation returns a *Truncation that retains the last retainCount events.
//
// A retainCount of 0 causes Compact to always return an empty slice.
func NewTruncation(retainCount int) *Truncation {
	return &Truncation{RetainCount: retainCount}
}

// Compact returns the last RetainCount events from events. If len(events) <=
// RetainCount, the original slice is returned without copying.
//
// An empty (non-nil) slice is returned when RetainCount is 0.
func (t *Truncation) Compact(_ context.Context, events []*session.Event) ([]*session.Event, error) {
	if t.RetainCount == 0 {
		return []*session.Event{}, nil
	}
	if len(events) <= t.RetainCount {
		return events, nil
	}
	return events[len(events)-t.RetainCount:], nil
}
