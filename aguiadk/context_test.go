package aguiadk

import (
	"context"
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/testutil"

	"google.golang.org/adk/v2/agent"
)

// TestContextPropagation verifies that RunEnvelope values attached to a Go
// context are retrievable via RunEnvelopeFrom, and that the typed helpers
// ContextFrom and ForwardedPropsFrom fall back to the envelope when no
// session state is present.
func TestContextPropagation(t *testing.T) {
	ctx := context.Background()
	parentID := "run-parent-1"
	agContext := []types.Context{{Description: "selected_code", Value: "fmt.Println()"}}
	props := map[string]any{"workspaceId": "ws-123"}

	ctx = WithRunEnvelope(ctx, RunEnvelope{
		ParentRunID:    &parentID,
		Context:        agContext,
		ForwardedProps: props,
	})

	env, ok := RunEnvelopeFrom(ctx)
	if !ok {
		t.Fatal("got nil RunEnvelope from context, want non-nil")
	}
	if env.ParentRunID == nil || *env.ParentRunID != parentID {
		t.Fatalf("ParentRunID = %v, want %q", env.ParentRunID, parentID)
	}
	if len(env.Context) != 1 || env.Context[0].Description != "selected_code" {
		t.Fatalf("Context = %+v, want one entry with description 'selected_code'", env.Context)
	}

	// ForwardedPropsFrom should decode the props map from the envelope.
	rc := testutil.NewFakeReadonlyContext().WithContext(ctx)
	got, ok := ForwardedPropsFrom[map[string]any](rc)
	if !ok {
		t.Fatal("got nil from ForwardedPropsFrom, want envelope")
	}
	if got["workspaceId"] != "ws-123" {
		t.Errorf("ForwardedProps workspaceId = %v, want ws-123", got["workspaceId"])
	}

	// ContextFrom should return the envelope's Context entries.
	items := ContextFrom(rc)
	if len(items) != 1 || items[0].Value != "fmt.Println()" {
		t.Errorf("ContextFrom = %+v, want one entry with value 'fmt.Println()'", items)
	}
}

// TestRunEnvelopeFrom_NoEnvelope verifies the absent-envelope case.
func TestRunEnvelopeFrom_NoEnvelope(t *testing.T) {
	_, ok := RunEnvelopeFrom(context.Background())
	if ok {
		t.Fatal("got ok=true when no envelope is attached, want false")
	}
}

// TestContextFrom_FallsBackToState verifies that when no envelope is attached
// to the Go context, ContextFrom reads from session state populated by the
// bridge under stateKeyAGUIContext.
func TestContextFrom_FallsBackToState(t *testing.T) {
	agContext := []types.Context{{Description: "open_file", Value: "main.go"}}
	rc := testutil.NewFakeReadonlyContext().WithReadonlyState(
		testutil.NewFakeStateWithData(map[string]any{
			stateKeyAGUIContext: agContext,
		}),
	)
	items := ContextFrom(rc)
	if len(items) != 1 || items[0].Description != "open_file" {
		t.Errorf("ContextFrom = %+v, want one entry with description 'open_file'", items)
	}
}

// TestForwardedPropsFrom_FallsBackToState verifies the state fallback path for
// ForwardedPropsFrom.
func TestForwardedPropsFrom_FallsBackToState(t *testing.T) {
	props := map[string]any{"theme": "dark"}
	rc := testutil.NewFakeReadonlyContext().WithReadonlyState(
		testutil.NewFakeStateWithData(map[string]any{
			stateKeyAGUIForwardedProps: props,
		}),
	)
	got, ok := ForwardedPropsFrom[map[string]any](rc)
	if !ok {
		t.Fatal("got nil from ForwardedPropsFrom, want state-stored props")
	}
	if got["theme"] != "dark" {
		t.Errorf("theme = %v, want dark", got["theme"])
	}
}

// Compile-time check that FakeReadonlyContext satisfies agent.ReadonlyContext.
var _ agent.ReadonlyContext = (*testutil.FakeReadonlyContext)(nil)
