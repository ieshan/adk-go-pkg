package aguiadk

import (
	"context"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"

	"google.golang.org/adk/v2/agent"
)

// RunEnvelope carries AG-UI protocol envelope fields (ParentRunID, Context,
// ForwardedProps) through Go contexts and ADK agent contexts so they survive
// the boundary between the AG-UI HTTP request and the ADK runner.
//
// The envelope is attached to the context passed to runner.Run and is also
// persisted into the ADK session state under well-known keys so downstream
// agents, tools, and callbacks can read it via RunEnvelopeFrom or the
// typed helpers.
type RunEnvelope struct {
	// ParentRunID is the identifier of the run that spawned this run, when
	// this run is a sub-run of a larger workflow.
	ParentRunID *string `json:"parentRunId,omitempty"`

	// Context is the list of AG-UI Context entries forwarded by the client
	// (e.g. selected code, open file URIs).
	Context []types.Context `json:"context,omitempty"`

	// ForwardedProps is an arbitrary bag of additional properties forwarded
	// from the client to the agent.
	ForwardedProps any `json:"forwardedProps,omitempty"`
}

// envelopeKey is the context key for RunEnvelope values.
type envelopeKey struct{}

// WithRunEnvelope returns a copy of ctx carrying the given RunEnvelope. The
// envelope can later be retrieved with RunEnvelopeFrom.
func WithRunEnvelope(ctx context.Context, env RunEnvelope) context.Context {
	return context.WithValue(ctx, envelopeKey{}, env)
}

// RunEnvelopeFrom retrieves the RunEnvelope stored in ctx, if any. The ok
// return value is false when no envelope was attached.
func RunEnvelopeFrom(ctx context.Context) (env RunEnvelope, ok bool) {
	v, exists := ctx.Value(envelopeKey{}).(RunEnvelope)
	return v, exists
}

// ContextFrom extracts AG-UI Context entries from an ADK agent.ReadonlyContext.
// It first checks for a RunEnvelope attached via WithRunEnvelope; if none is
// present it falls back to session state entries previously persisted by the
// bridge under stateKeyAGUIContext.
func ContextFrom(ctx agent.ReadonlyContext) []types.Context {
	if env, ok := RunEnvelopeFrom(ctx); ok {
		return env.Context
	}
	if rs := ctx.ReadonlyState(); rs != nil {
		if v, err := rs.Get(stateKeyAGUIContext); err == nil {
			if items, ok := v.([]types.Context); ok {
				return items
			}
		}
	}
	return nil
}

// ForwardedPropsFrom extracts typed ForwardedProps from an ADK
// agent.ReadonlyContext. It first checks for a RunEnvelope attached via
// WithRunEnvelope; if none is present it falls back to session state entries
// previously persisted by the bridge under stateKeyAGUIForwardedProps. The
// generic parameter T lets callers decode the props into a concrete type.
func ForwardedPropsFrom[T any](ctx agent.ReadonlyContext) (T, bool) {
	var zero T
	if env, ok := RunEnvelopeFrom(ctx); ok {
		if env.ForwardedProps == nil {
			return zero, false
		}
		if v, ok := env.ForwardedProps.(T); ok {
			return v, true
		}
		return zero, false
	}
	if rs := ctx.ReadonlyState(); rs != nil {
		if v, err := rs.Get(stateKeyAGUIForwardedProps); err == nil {
			if v, ok := v.(T); ok {
				return v, true
			}
		}
	}
	return zero, false
}

// stateKeyAGUIParentRunID is the session state key under which the bridge
// persists the AG-UI ParentRunID envelope field.
const stateKeyAGUIParentRunID = "_ag_ui_parent_run_id"

// stateKeyAGUIContext is the session state key under which the bridge
// persists the AG-UI Context envelope field.
const stateKeyAGUIContext = "_ag_ui_context"

// stateKeyAGUIForwardedProps is the session state key under which the bridge
// persists the AG-UI ForwardedProps envelope field.
const stateKeyAGUIForwardedProps = "_ag_ui_forwarded_props"
