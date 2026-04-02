package agui_test

import (
	"context"
	"iter"
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func TestChain_Empty(t *testing.T) {
	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
		}
	})

	chained := agui.Chain()(base)

	var count int
	for ev, err := range chained.Run(context.Background(), types.RunAgentInput{}) {
		if err != nil {
			t.Fatal(err)
		}
		if ev.Type() != events.EventTypeRunStarted {
			t.Errorf("unexpected event type: %s", ev.Type())
		}
		count++
	}
	if count != 1 {
		t.Fatalf("expected 1 event, got %d", count)
	}
}

func TestChain_Single(t *testing.T) {
	var order []string

	mw := func(next agui.Agent) agui.Agent {
		return agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
			order = append(order, "mw-before")
			seq := next.Run(ctx, input)
			return func(yield func(events.Event, error) bool) {
				for ev, err := range seq {
					if !yield(ev, err) {
						return
					}
				}
				order = append(order, "mw-after")
			}
		})
	}

	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		order = append(order, "base")
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
		}
	})

	chained := agui.Chain(mw)(base)
	for range chained.Run(context.Background(), types.RunAgentInput{}) {
	}

	if len(order) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(order), order)
	}
	if order[0] != "mw-before" || order[1] != "base" || order[2] != "mw-after" {
		t.Errorf("unexpected order: %v", order)
	}
}

func TestChain_Ordering(t *testing.T) {
	var order []string

	makeMW := func(name string) agui.Middleware {
		return func(next agui.Agent) agui.Agent {
			return agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
				order = append(order, name+"-before")
				seq := next.Run(ctx, input)
				return func(yield func(events.Event, error) bool) {
					for ev, err := range seq {
						if !yield(ev, err) {
							return
						}
					}
					order = append(order, name+"-after")
				}
			})
		}
	}

	base := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		order = append(order, "base")
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent("t1", "r1"), nil)
		}
	})

	// Chain(a, b, c)(agent) = a(b(c(agent)))
	// Execution: a-before, b-before, c-before, base, c-after, b-after, a-after
	chained := agui.Chain(makeMW("a"), makeMW("b"), makeMW("c"))(base)
	for range chained.Run(context.Background(), types.RunAgentInput{}) {
	}

	expected := []string{
		"a-before", "b-before", "c-before", "base",
		"c-after", "b-after", "a-after",
	}
	if len(order) != len(expected) {
		t.Fatalf("expected %d entries, got %d: %v", len(expected), len(order), order)
	}
	for i, want := range expected {
		if order[i] != want {
			t.Errorf("order[%d] = %q, want %q", i, order[i], want)
		}
	}
}
