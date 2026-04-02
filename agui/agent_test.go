package agui_test

import (
	"context"
	"iter"
	"testing"

	"github.com/ieshan/adk-go-pkg/agui"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

func TestAgentFunc_ImplementsAgent(t *testing.T) {
	var _ agui.Agent = agui.AgentFunc(nil)
}

func TestAgentFunc_Run(t *testing.T) {
	called := false
	fn := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		return func(yield func(events.Event, error) bool) {
			called = true
			ev := events.NewRunStartedEvent(input.ThreadID, input.RunID)
			yield(ev, nil)
		}
	})

	ctx := context.Background()
	input := types.RunAgentInput{ThreadID: "t1", RunID: "r1"}
	for ev, err := range fn.Run(ctx, input) {
		if err != nil {
			t.Fatal(err)
		}
		if ev.Type() != events.EventTypeRunStarted {
			t.Errorf("expected RUN_STARTED, got %s", ev.Type())
		}
	}
	if !called {
		t.Error("AgentFunc was not called")
	}
}

func TestChanToIter(t *testing.T) {
	ctx := context.Background()
	ch := make(chan events.Event, 3)

	ch <- events.NewRunStartedEvent("t1", "r1")
	ch <- events.NewTextMessageStartEvent("msg1")
	ch <- events.NewRunFinishedEvent("t1", "r1")
	close(ch)

	var collected []events.EventType
	for ev, err := range agui.ChanToIter(ctx, ch) {
		if err != nil {
			t.Fatal(err)
		}
		collected = append(collected, ev.Type())
	}

	if len(collected) != 3 {
		t.Fatalf("expected 3 events, got %d", len(collected))
	}
	if collected[0] != events.EventTypeRunStarted {
		t.Errorf("event 0: expected RUN_STARTED, got %s", collected[0])
	}
	if collected[2] != events.EventTypeRunFinished {
		t.Errorf("event 2: expected RUN_FINISHED, got %s", collected[2])
	}
}

func TestChanToIter_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan events.Event)

	cancel()

	var gotErr error
	for _, err := range agui.ChanToIter(ctx, ch) {
		if err != nil {
			gotErr = err
			break
		}
	}
	if gotErr == nil {
		t.Error("expected context cancellation error")
	}
}
