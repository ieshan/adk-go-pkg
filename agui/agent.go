// Package agui provides a generic AG-UI protocol server for Go applications.
//
// The agui package implements the AG-UI (Agent-User Interaction) protocol,
// enabling any Go application to serve AG-UI-compatible frontends like
// CopilotKit or AG-UI Vue. It handles HTTP/SSE streaming, event emission,
// state management, client tool orchestration, and middleware composition.
//
// This package has zero dependency on ADK-Go. For ADK-Go integration,
// use the aguiadk package.
//
// Basic usage:
//
//	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
//	    ch := make(chan events.Event, 64)
//	    emitter := agui.NewEventEmitter(ch)
//	    go func() {
//	        defer close(ch)
//	        emitter.RunStarted(input.ThreadID, input.RunID)
//	        msgID := emitter.GenerateMessageID()
//	        role := "assistant"
//	        emitter.TextMessageStart(msgID, &role)
//	        emitter.TextMessageContent(msgID, "Hello from AG-UI!")
//	        emitter.TextMessageEnd(msgID)
//	        emitter.RunFinished(input.ThreadID, input.RunID)
//	    }()
//	    return agui.ChanToIter(ctx, ch)
//	})
//
//	handler, _ := agui.Handler(agui.Config{Agent: agent})
//	http.ListenAndServe(":8080", handler)
package agui

import (
	"context"
	"iter"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

// Agent is the interface for anything that processes AG-UI runs.
// Implement this to plug any backend (LLM, workflow engine, custom logic)
// into the AG-UI protocol.
type Agent interface {
	// Run processes a single agent run and yields AG-UI events.
	// The implementation should emit events in protocol order:
	// RUN_STARTED → (messages, tool calls, state, steps, etc.) → RUN_FINISHED or RUN_ERROR.
	Run(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error]
}

// AgentFunc is an adapter to allow use of ordinary functions as Agents.
type AgentFunc func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error]

// Run calls f(ctx, input).
func (f AgentFunc) Run(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
	return f(ctx, input)
}

// ChanToIter converts a channel of events into an iter.Seq2.
// The iterator yields events from the channel until it is closed.
// If ctx is cancelled, yields a context cancellation error and stops.
func ChanToIter(ctx context.Context, ch <-chan events.Event) iter.Seq2[events.Event, error] {
	return func(yield func(events.Event, error) bool) {
		for {
			select {
			case <-ctx.Done():
				yield(nil, ctx.Err())
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				if !yield(ev, nil) {
					return
				}
			}
		}
	}
}
