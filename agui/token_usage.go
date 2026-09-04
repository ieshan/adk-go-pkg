package agui

import (
	"encoding/json"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// TokenUsage mirrors the AG-UI TokenUsageSchema. Pointer fields preserve the
// protocol's optional semantics: a missing count is encoded as null/omitted,
// not zero, so consumers can distinguish "no telemetry" from "zero tokens".
//
// JSON tags match the canonical schema field names.
type TokenUsage struct {
	Provider          string `json:"provider,omitempty"`
	Model             string `json:"model,omitempty"`
	InputTokens       *int64 `json:"inputTokens,omitempty"`
	OutputTokens      *int64 `json:"outputTokens,omitempty"`
	TotalTokens       *int64 `json:"totalTokens,omitempty"`
	ReasoningTokens   *int64 `json:"reasoningTokens,omitempty"`
	CachedInputTokens *int64 `json:"cachedInputTokens,omitempty"`
}

// AggregateTokenUsage merges a slice of TokenUsage entries by (Provider, Model)
// key, summing each token-count field. Entries with nil counts contribute zero
// to the corresponding aggregate. The returned slice has stable ordering by
// first appearance of each key.
func AggregateTokenUsage(entries []TokenUsage) []TokenUsage {
	if len(entries) == 0 {
		return nil
	}
	type agg struct {
		in, out, tot, reasoning, cached int64
		hasIn, hasOut, hasTot           bool
		hasReasoning, hasCached         bool
	}
	order := make([]string, 0, len(entries))
	aggs := make(map[string]*agg, len(entries))
	for _, e := range entries {
		key := e.Provider + "\x00" + e.Model
		a, ok := aggs[key]
		if !ok {
			a = &agg{}
			aggs[key] = a
			order = append(order, key)
		}
		if e.InputTokens != nil {
			a.in += *e.InputTokens
			a.hasIn = true
		}
		if e.OutputTokens != nil {
			a.out += *e.OutputTokens
			a.hasOut = true
		}
		if e.TotalTokens != nil {
			a.tot += *e.TotalTokens
			a.hasTot = true
		}
		if e.ReasoningTokens != nil {
			a.reasoning += *e.ReasoningTokens
			a.hasReasoning = true
		}
		if e.CachedInputTokens != nil {
			a.cached += *e.CachedInputTokens
			a.hasCached = true
		}
	}

	out := make([]TokenUsage, 0, len(order))
	for _, key := range order {
		// Recover provider/model from the key by splitting on the separator.
		provider, model := splitUsageKey(key)
		u := TokenUsage{Provider: provider, Model: model}
		a := aggs[key]
		if a.hasIn {
			v := a.in
			u.InputTokens = &v
		}
		if a.hasOut {
			v := a.out
			u.OutputTokens = &v
		}
		if a.hasTot {
			v := a.tot
			u.TotalTokens = &v
		}
		if a.hasReasoning {
			v := a.reasoning
			u.ReasoningTokens = &v
		}
		if a.hasCached {
			v := a.cached
			u.CachedInputTokens = &v
		}
		out = append(out, u)
	}
	return out
}

// splitUsageKey reverses the "provider\x00model" composite key built by
// AggregateTokenUsage.
func splitUsageKey(key string) (provider, model string) {
	for i := 0; i < len(key); i++ {
		if key[i] == 0 {
			return key[:i], key[i+1:]
		}
	}
	return key, ""
}

// RunFinishedWithUsageEvent is a RUN_FINISHED event carrying token usage
// telemetry. It embeds the canonical RunFinishedEvent so it satisfies the
// events.Event interface and serializes with the standard RUN_FINISHED shape,
// adding a top-level "usage" field.
type RunFinishedWithUsageEvent struct {
	*events.RunFinishedEvent
	Usage []TokenUsage `json:"usage,omitempty"`
}

// ToJSON serializes the event including the usage field. Overrides the
// embedded RunFinishedEvent.ToJSON so the Usage field is included.
func (e *RunFinishedWithUsageEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// RunErrorWithUsageEvent is a RUN_ERROR event carrying token usage telemetry.
// It embeds the canonical RunErrorEvent so it satisfies events.Event and
// serializes with the standard RUN_ERROR shape, adding a top-level "usage"
// field.
type RunErrorWithUsageEvent struct {
	*events.RunErrorEvent
	Usage []TokenUsage `json:"usage,omitempty"`
}

// ToJSON serializes the event including the usage field. Overrides the
// embedded RunErrorEvent.ToJSON so the Usage field is included.
func (e *RunErrorWithUsageEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// RunFinishedWithUsage emits a RUN_FINISHED event carrying aggregated token
// usage telemetry. When usage is empty or nil, it falls back to the plain
// RunFinishedWithOptions emitter so the wire format stays canonical for runs
// without telemetry.
func (e *EventEmitter) RunFinishedWithUsage(threadID, runID string, usage []TokenUsage, opts ...events.RunFinishedOption) error {
	if len(usage) == 0 {
		return e.RunFinishedWithOptions(threadID, runID, opts...)
	}
	base := events.NewRunFinishedEventWithOptions(threadID, runID, opts...)
	return e.emit(&RunFinishedWithUsageEvent{
		RunFinishedEvent: base,
		Usage:            usage,
	})
}

// RunErrorWithUsage emits a RUN_ERROR event carrying aggregated token usage
// telemetry. When usage is empty or nil, it falls back to the plain
// RunErrorWithOptions emitter.
func (e *EventEmitter) RunErrorWithUsage(message string, usage []TokenUsage, opts ...events.RunErrorOption) error {
	if len(usage) == 0 {
		return e.RunErrorWithOptions(message, opts...)
	}
	base := events.NewRunErrorEvent(message, opts...)
	return e.emit(&RunErrorWithUsageEvent{
		RunErrorEvent: base,
		Usage:         usage,
	})
}

// Compile-time checks that the with-usage events satisfy events.Event.
var (
	_ events.Event = (*RunFinishedWithUsageEvent)(nil)
	_ events.Event = (*RunErrorWithUsageEvent)(nil)
)
