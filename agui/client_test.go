package agui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

func TestClientAgent_RunStreamsEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "event: RUN_STARTED\ndata: {\"type\":\"RUN_STARTED\",\"threadId\":\"t1\",\"runId\":\"r1\"}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: RUN_FINISHED\ndata: {\"type\":\"RUN_FINISHED\",\"threadId\":\"t1\",\"runId\":\"r1\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	client := NewClientAgent(ClientConfig{Endpoint: server.URL})
	var received []events.Event
	for ev, err := range client.Run(context.Background(), types.RunAgentInput{ThreadID: "t1", RunID: "r1"}) {
		if err != nil {
			t.Fatalf("unexpected stream error: %v", err)
		}
		if ev != nil {
			received = append(received, ev)
		}
	}

	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d: %+v", len(received), received)
	}
	if received[0].Type() != events.EventTypeRunStarted {
		t.Errorf("event[0] type = %v, want RUN_STARTED", received[0].Type())
	}
	if received[1].Type() != events.EventTypeRunFinished {
		t.Errorf("event[1] type = %v, want RUN_FINISHED", received[1].Type())
	}
}

func TestClientAgent_TextMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "event: RUN_STARTED\ndata: {\"type\":\"RUN_STARTED\",\"threadId\":\"t1\",\"runId\":\"r1\"}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: TEXT_MESSAGE_START\ndata: {\"type\":\"TEXT_MESSAGE_START\",\"messageId\":\"m1\",\"role\":\"assistant\"}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: TEXT_MESSAGE_CONTENT\ndata: {\"type\":\"TEXT_MESSAGE_CONTENT\",\"messageId\":\"m1\",\"delta\":\"Hello from remote!\"}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: TEXT_MESSAGE_END\ndata: {\"type\":\"TEXT_MESSAGE_END\",\"messageId\":\"m1\"}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: RUN_FINISHED\ndata: {\"type\":\"RUN_FINISHED\",\"threadId\":\"t1\",\"runId\":\"r1\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	client := NewClientAgent(ClientConfig{Endpoint: server.URL})
	var text string
	for ev, err := range client.Run(context.Background(), types.RunAgentInput{ThreadID: "t1", RunID: "r1"}) {
		if err != nil {
			t.Fatalf("unexpected stream error: %v", err)
		}
		if ev == nil {
			continue
		}
		if contentEv, ok := ev.(*events.TextMessageContentEvent); ok {
			text += contentEv.Delta
		}
	}

	if text != "Hello from remote!" {
		t.Fatalf("expected 'Hello from remote!', got %q", text)
	}
}

func TestClientAgent_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		// Emit one event, then stall.
		fmt.Fprintf(w, "event: RUN_STARTED\ndata: {\"type\":\"RUN_STARTED\",\"threadId\":\"t1\",\"runId\":\"r1\"}\n\n")
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := NewClientAgent(ClientConfig{Endpoint: server.URL})
	count := 0
	for ev, err := range client.Run(ctx, types.RunAgentInput{ThreadID: "t1", RunID: "r1"}) {
		if err != nil {
			break
		}
		if ev != nil {
			count++
			if count == 1 {
				cancel()
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 event before cancellation, got %d", count)
	}
}
