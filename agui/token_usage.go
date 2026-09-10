package agui

import (
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// AggregateTokenUsage merges a slice of events.TokenUsage entries by
// (Provider, Model) key, summing each token-count field. Entries with nil
// counts contribute zero to the corresponding aggregate. The returned slice
// has stable ordering by first appearance of each key.
func AggregateTokenUsage(entries []events.TokenUsage) []events.TokenUsage {
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

	out := make([]events.TokenUsage, 0, len(order))
	for _, key := range order {
		a, ok := aggs[key]
		if !ok || a == nil {
			continue
		}
		// Recover provider/model from the key by splitting on the separator.
		provider, model := splitUsageKey(key)
		u := events.TokenUsage{Provider: provider, Model: model}
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

// RunFinishedWithUsage emits a RUN_FINISHED event carrying aggregated token
// usage telemetry. When usage is empty or nil, it falls back to the plain
// RunFinishedWithOptions emitter so the wire format stays canonical for runs
// without telemetry.
func (e *EventEmitter) RunFinishedWithUsage(threadID, runID string, usage []events.TokenUsage, opts ...events.RunFinishedOption) error {
	if len(usage) == 0 {
		return e.RunFinishedWithOptions(threadID, runID, opts...)
	}
	allOpts := make([]events.RunFinishedOption, len(opts)+1)
	copy(allOpts, opts)
	allOpts[len(opts)] = events.WithUsage(usage)
	return e.RunFinishedWithOptions(threadID, runID, allOpts...)
}

// RunErrorWithUsage emits a RUN_ERROR event carrying aggregated token usage
// telemetry. When usage is empty or nil, it falls back to the plain
// RunErrorWithOptions emitter.
func (e *EventEmitter) RunErrorWithUsage(message string, usage []events.TokenUsage, opts ...events.RunErrorOption) error {
	if len(usage) == 0 {
		return e.RunErrorWithOptions(message, opts...)
	}
	allOpts := make([]events.RunErrorOption, len(opts)+1)
	copy(allOpts, opts)
	allOpts[len(opts)] = events.WithErrorUsage(usage)
	return e.RunErrorWithOptions(message, allOpts...)
}
