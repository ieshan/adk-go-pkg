package agui

import (
	"fmt"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// predictiveNamespace is the JSON Patch path prefix marking a delta as a
// prediction (ghosted) rather than committed state. Clients that drop every
// delta under this path still end in the correct committed state.
const predictiveNamespace = "/_predictive"

// PredictiveStateTracker streams ghosted state deltas under the /_predictive
// namespace, then commits the final value to the real path and clears the
// prediction. This lets a UI render an optimistic preview while the agent is
// still generating, then settle to the committed value on completion.
//
// The tracker composes a StateManager (for applying patches) with an
// EventEmitter (for streaming STATE_DELTA events). It is safe for concurrent
// use only if the underlying StateManager and EventEmitter are — which they
// are by construction.
type PredictiveStateTracker struct {
	state   *StateManager
	emitter *EventEmitter
}

// NewPredictiveStateTracker creates a tracker over the given state and emitter.
func NewPredictiveStateTracker(state *StateManager, emitter *EventEmitter) *PredictiveStateTracker {
	return &PredictiveStateTracker{state: state, emitter: emitter}
}

// State returns the underlying StateManager. Callers can use it to inspect
// the current state (including the /_predictive namespace) for testing or
// debugging.
func (p *PredictiveStateTracker) State() *StateManager {
	return p.state
}

// PredictiveDelta applies the given patch operations under the /_predictive
// namespace and emits a STATE_DELTA with the prefixed paths. The caller's
// operations should use paths relative to the predictive namespace root
// (e.g. "/draft" becomes "/_predictive/draft").
//
// The namespace is created lazily on first use. Subsequent calls preserve
// existing predictive state — only the paths in the given patch are modified.
func (p *PredictiveStateTracker) PredictiveDelta(patch []events.JSONPatchOperation) error {
	if len(patch) == 0 {
		return nil
	}

	prefixed := prefixPatchPaths(patch, predictiveNamespace)

	// Try applying directly first — works when /_predictive already exists.
	// This preserves any prior predictive state.
	if err := p.state.Apply(prefixed); err != nil {
		// Namespace doesn't exist yet — create it, then retry. Using "add"
		// on a non-existent root key creates it without disturbing siblings.
		ensureOps := []events.JSONPatchOperation{
			{Op: "add", Path: predictiveNamespace, Value: map[string]any{}},
		}
		if err2 := p.state.Apply(ensureOps); err2 != nil {
			return fmt.Errorf("agui: ensure predictive namespace: %w", err2)
		}
		if err = p.state.Apply(prefixed); err != nil {
			return fmt.Errorf("agui: apply predictive patch: %w", err)
		}
	}

	if err := p.emitter.StateDelta(prefixed); err != nil {
		return fmt.Errorf("agui: emit predictive delta: %w", err)
	}
	return nil
}

// Commit applies a value to the real state path and emits a STATE_DELTA with
// the real (un-prefixed) path. The committed value is independent of any
// prediction — a dropped or garbled prediction cannot corrupt it.
//
// If the target path's parent does not exist, Commit creates the intermediate
// objects so the caller does not need to pre-seed the state tree.
func (p *PredictiveStateTracker) Commit(path string, value any) error {
	emitOps := []events.JSONPatchOperation{
		{Op: "add", Path: path, Value: value},
	}

	// Try a direct add first — works when the parent already exists.
	if err := p.state.Apply(emitOps); err != nil {
		// Fall back to creating intermediate objects via a nested add on the
		// top-level key. The emitted delta still uses the original path so the
		// client sees the logical commit, not the storage workaround.
		nestedOps := ensureParentPath(emitOps)
		if err2 := p.state.Apply(nestedOps); err2 != nil {
			return fmt.Errorf("agui: apply commit: %w", err)
		}
	}

	if err := p.emitter.StateDelta(emitOps); err != nil {
		return fmt.Errorf("agui: emit commit: %w", err)
	}
	return nil
}

// ensureParentPath rewrites a patch op targeting a deep path into an op
// targeting the top-level key with a nested value, so "add" succeeds even
// when intermediate objects don't exist yet.
func ensureParentPath(ops []events.JSONPatchOperation) []events.JSONPatchOperation {
	out := make([]events.JSONPatchOperation, len(ops))
	for i, op := range ops {
		out[i] = op
		if op.Op != "add" || op.Path == "" || op.Path == "/" {
			continue
		}
		trimmed := op.Path[1:] // strip leading /
		segIdx := -1
		for j := 0; j < len(trimmed); j++ {
			if trimmed[j] == '/' {
				segIdx = j
				break
			}
		}
		if segIdx <= 0 {
			continue // single-segment path, no parent to ensure
		}
		topKey := trimmed[:segIdx]
		rest := trimmed[segIdx:] // includes leading /
		nested := buildNestedValue(rest, op.Value)
		out[i] = events.JSONPatchOperation{
			Op:    "add",
			Path:  "/" + topKey,
			Value: nested,
		}
	}
	return out
}

// buildNestedValue turns a path suffix like "/steps" and a value into
// {"steps": value}. For deeper paths like "/a/b" it produces {"a": {"b": value}}.
func buildNestedValue(pathSuffix string, value any) any {
	if pathSuffix == "" || pathSuffix == "/" {
		return value
	}
	segs := splitPath(pathSuffix)
	if len(segs) == 0 {
		return value
	}
	cur := value
	for i := len(segs) - 1; i >= 0; i-- {
		cur = map[string]any{segs[i]: cur}
	}
	return cur
}

func splitPath(p string) []string {
	var segs []string
	start := 0
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			if i > start {
				segs = append(segs, p[start:i])
			}
			start = i + 1
		}
	}
	if start < len(p) {
		segs = append(segs, p[start:])
	}
	return segs
}

// Clear removes the /_predictive namespace and emits a STATE_DELTA for the
// removal. Clients that ignored predictions never created the namespace and
// will simply ignore this too.
func (p *PredictiveStateTracker) Clear() error {
	ops := []events.JSONPatchOperation{
		{Op: "remove", Path: predictiveNamespace},
	}
	// Apply best-effort: if the namespace doesn't exist, the patch fails but
	// we still emit the delta so clients that did track it can clean up.
	_ = p.state.Apply(ops)
	if err := p.emitter.StateDelta(ops); err != nil {
		return fmt.Errorf("agui: emit clear: %w", err)
	}
	return nil
}

// prefixPatchPaths returns a copy of the patch with every path prefixed by
// the given namespace. A path of "/" becomes the namespace itself; any other
// path is concatenated as namespace + path.
func prefixPatchPaths(patch []events.JSONPatchOperation, namespace string) []events.JSONPatchOperation {
	out := make([]events.JSONPatchOperation, len(patch))
	for i, op := range patch {
		out[i] = op
		if op.Path == "" || op.Path == "/" {
			out[i].Path = namespace
		} else {
			out[i].Path = namespace + op.Path
		}
		if op.From == "/" || op.From == "" {
			// From is only used for move/copy; leave empty as-is.
		} else {
			out[i].From = namespace + op.From
		}
	}
	return out
}
