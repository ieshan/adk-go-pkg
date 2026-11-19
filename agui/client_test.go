package agui_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func TestClientAgent_RunStreamsEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = fmt.Fprintf(w, "event: RUN_STARTED\ndata: {\"type\":\"RUN_STARTED\",\"threadId\":\"t1\",\"runId\":\"r1\"}\n\n")
		flusher.Flush()
		_, _ = fmt.Fprintf(w, "event: RUN_FINISHED\ndata: {\"type\":\"RUN_FINISHED\",\"threadId\":\"t1\",\"runId\":\"r1\"}\n\n")
		flusher.Flush()
	}))
	t.Cleanup(server.Close)

	client := agui.NewClientAgent(agui.ClientConfig{Endpoint: server.URL})
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
		t.Fatalf("got %d events, want 2: %+v", len(received), received)
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
		_, _ = fmt.Fprintf(w, "event: RUN_STARTED\ndata: {\"type\":\"RUN_STARTED\",\"threadId\":\"t1\",\"runId\":\"r1\"}\n\n")
		flusher.Flush()
		_, _ = fmt.Fprintf(w, "event: TEXT_MESSAGE_START\ndata: {\"type\":\"TEXT_MESSAGE_START\",\"messageId\":\"m1\",\"role\":\"assistant\"}\n\n")
		flusher.Flush()
		_, _ = fmt.Fprintf(w, "event: TEXT_MESSAGE_CONTENT\ndata: {\"type\":\"TEXT_MESSAGE_CONTENT\",\"messageId\":\"m1\",\"delta\":\"Hello from remote!\"}\n\n")
		flusher.Flush()
		_, _ = fmt.Fprintf(w, "event: TEXT_MESSAGE_END\ndata: {\"type\":\"TEXT_MESSAGE_END\",\"messageId\":\"m1\"}\n\n")
		flusher.Flush()
		_, _ = fmt.Fprintf(w, "event: RUN_FINISHED\ndata: {\"type\":\"RUN_FINISHED\",\"threadId\":\"t1\",\"runId\":\"r1\"}\n\n")
		flusher.Flush()
	}))
	t.Cleanup(server.Close)

	client := agui.NewClientAgent(agui.ClientConfig{Endpoint: server.URL})
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
		t.Fatalf("got %q, want 'Hello from remote!'", text)
	}
}

func TestClientAgent_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		// Emit one event, then stall.
		_, _ = fmt.Fprintf(w, "event: RUN_STARTED\ndata: {\"type\":\"RUN_STARTED\",\"threadId\":\"t1\",\"runId\":\"r1\"}\n\n")
		flusher.Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := agui.NewClientAgent(agui.ClientConfig{Endpoint: server.URL})
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
		t.Fatalf("got %d events, want 1 (before cancellation)", count)
	}
}

func TestClientAgent_DeclaresProtocolVersion(t *testing.T) {
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: {\"type\":\"RUN_FINISHED\",\"threadId\":\"t1\",\"runId\":\"r1\"}\n\n")
	}))
	t.Cleanup(server.Close)

	client := agui.NewClientAgent(agui.ClientConfig{Endpoint: server.URL})
	for _, err := range client.Run(context.Background(), types.RunAgentInput{ThreadID: "t1", RunID: "r1"}) {
		if err != nil {
			t.Fatalf("unexpected stream error: %v", err)
		}
	}

	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if got := m["protocolVersion"]; got != agui.ProtocolVersion {
		t.Errorf("protocolVersion = %v, want %q", got, agui.ProtocolVersion)
	}
	if m["threadId"] != "t1" || m["runId"] != "r1" {
		t.Errorf("unexpected body: %s", string(body))
	}
}

func TestClientAgent_SSEFraming(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		// Comment line and event: line are ignored; data split across two
		// data: lines concatenates with "\n".
		_, _ = fmt.Fprint(w, ": a comment\n")
		_, _ = fmt.Fprint(w, "event: RUN_FINISHED\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"RUN_FINISHED\",\n")
		_, _ = fmt.Fprint(w, "data: \"threadId\":\"t1\",\"runId\":\"r1\"}\n")
		_, _ = fmt.Fprint(w, "\n")
		flusher.Flush()
		// Trailing block without a blank line still yields.
		_, _ = fmt.Fprint(w, "data: {\"type\":\"RUN_STARTED\",\"threadId\":\"t1\",\"runId\":\"r1\"}")
		flusher.Flush()
	}))
	t.Cleanup(server.Close)

	client := agui.NewClientAgent(agui.ClientConfig{Endpoint: server.URL})
	var got []events.EventType
	for ev, err := range client.Run(context.Background(), types.RunAgentInput{ThreadID: "t1", RunID: "r1"}) {
		if err != nil {
			t.Fatalf("unexpected stream error: %v", err)
		}
		if ev != nil {
			got = append(got, ev.Type())
		}
	}
	want := []events.EventType{events.EventTypeRunFinished, events.EventTypeRunStarted}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestClientAgent_TruncatedStreamErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		// Write a partial block and abort the connection mid-stream.
		_, _ = fmt.Fprint(w, "data: {\"type\":\"RUN_STARTED\"")
		flusher.Flush()
		panic(http.ErrAbortHandler)
	}))
	t.Cleanup(server.Close)

	client := agui.NewClientAgent(agui.ClientConfig{Endpoint: server.URL})
	var sawErr bool
	for _, err := range client.Run(context.Background(), types.RunAgentInput{ThreadID: "t1", RunID: "r1"}) {
		if err != nil {
			sawErr = true
		}
	}
	if !sawErr {
		t.Fatal("got no error for truncated stream, want error")
	}
}

func TestClientAgent_CustomAuthHeader(t *testing.T) {
	tests := []struct {
		name       string
		apiKey     string
		authHeader string
		authScheme string
		wantHeader string
		wantValue  string
	}{
		{
			name:       "default authorization with bearer",
			apiKey:     "secret",
			wantHeader: "Authorization",
			wantValue:  "Bearer secret",
		},
		{
			name:       "custom scheme on authorization",
			apiKey:     "secret",
			authScheme: "Token",
			wantHeader: "Authorization",
			wantValue:  "Token secret",
		},
		{
			name:       "custom header carries raw key",
			apiKey:     "secret",
			authHeader: "X-API-Key",
			authScheme: "Bearer",
			wantHeader: "X-API-Key",
			wantValue:  "secret",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Get(tc.wantHeader)
				if tc.authHeader != "" {
					// The scheme must not leak into a custom header.
					if r.Header.Get("Authorization") != "" {
						t.Errorf("unexpected Authorization header: %q", r.Header.Get("Authorization"))
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintf(w, "data: {\"type\":\"RUN_FINISHED\",\"threadId\":\"t1\",\"runId\":\"r1\"}\n\n")
			}))
			t.Cleanup(server.Close)

			client := agui.NewClientAgent(agui.ClientConfig{
				Endpoint:   server.URL,
				APIKey:     tc.apiKey,
				AuthHeader: tc.authHeader,
				AuthScheme: tc.authScheme,
			})
			for _, err := range client.Run(context.Background(), types.RunAgentInput{ThreadID: "t1", RunID: "r1"}) {
				if err != nil {
					t.Fatalf("unexpected stream error: %v", err)
				}
			}
			if got != tc.wantValue {
				t.Errorf("%s = %q, want %q", tc.wantHeader, got, tc.wantValue)
			}
		})
	}
}

func TestClientAgent_NonSSEContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, "not an event stream")
	}))
	t.Cleanup(server.Close)

	client := agui.NewClientAgent(agui.ClientConfig{Endpoint: server.URL})
	var gotErr error
	for _, err := range client.Run(context.Background(), types.RunAgentInput{ThreadID: "t1", RunID: "r1"}) {
		if err != nil {
			gotErr = err
		}
	}
	if gotErr == nil {
		t.Fatal("got no error, want content-type error")
	}
	if !strings.Contains(gotErr.Error(), "text/plain") {
		t.Errorf("error = %q, want it to name the content type", gotErr)
	}
}
