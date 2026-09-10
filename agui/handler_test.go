package agui_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ieshan/adk-go-pkg/agui"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

// parseSSEEvents reads SSE data lines from the response body and returns
// the parsed event type strings.
func parseSSEEvents(t *testing.T, body io.Reader) []string {
	t.Helper()
	var eventTypes []string
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		// Unescape embedded newlines from SSE framing.
		data = strings.ReplaceAll(data, "\\n", "\n")
		data = strings.ReplaceAll(data, "\\r", "\r")
		var m map[string]any
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Logf("skipping non-JSON data line: %s", data)
			continue
		}
		if typ, ok := m["type"].(string); ok {
			eventTypes = append(eventTypes, typ)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	return eventTypes
}

// parseSSEEventsRaw reads SSE data lines and returns the raw JSON maps.
func parseSSEEventsRaw(t *testing.T, body io.Reader) []map[string]any {
	t.Helper()
	var result []map[string]any
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		data = strings.ReplaceAll(data, "\\n", "\n")
		data = strings.ReplaceAll(data, "\\r", "\r")
		var m map[string]any
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Logf("skipping non-JSON data line: %s", data)
			continue
		}
		result = append(result, m)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	return result
}

func TestHandler_BasicRun(t *testing.T) {
	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		return func(yield func(events.Event, error) bool) {
			if !yield(events.NewRunStartedEvent(input.ThreadID, input.RunID), nil) {
				return
			}
			yield(events.NewRunFinishedEvent(input.ThreadID, input.RunID), nil)
		}
	})

	h, err := agui.Handler(agui.Config{Agent: agent})
	if err != nil {
		t.Fatal(err)
	}

	input := types.RunAgentInput{ThreadID: "t1", RunID: "r1"}
	body, _ := json.Marshal(input)

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("nil response")
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("got %s, want Content-Type text/event-stream", ct)
	}

	eventTypes := parseSSEEvents(t, resp.Body)
	if len(eventTypes) < 2 {
		t.Fatalf("got %d events, want at least 2", len(eventTypes))
	}
	if eventTypes[0] != "RUN_STARTED" {
		t.Errorf("event 0: got %s, want RUN_STARTED", eventTypes[0])
	}
	if eventTypes[len(eventTypes)-1] != "RUN_FINISHED" {
		t.Errorf("last event: got %s, want RUN_FINISHED", eventTypes[len(eventTypes)-1])
	}
}

func TestHandler_StreamsTextMessage(t *testing.T) {
	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		return func(yield func(events.Event, error) bool) {
			if !yield(events.NewRunStartedEvent(input.ThreadID, input.RunID), nil) {
				return
			}
			if !yield(events.NewTextMessageStartEvent("msg1"), nil) {
				return
			}
			if !yield(events.NewTextMessageContentEvent("msg1", "Hello"), nil) {
				return
			}
			if !yield(events.NewTextMessageEndEvent("msg1"), nil) {
				return
			}
			yield(events.NewRunFinishedEvent(input.ThreadID, input.RunID), nil)
		}
	})

	h, err := agui.Handler(agui.Config{Agent: agent})
	if err != nil {
		t.Fatal(err)
	}

	input := types.RunAgentInput{ThreadID: "t1", RunID: "r1"}
	body, _ := json.Marshal(input)

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp == nil {
		t.Fatal("nil response")
	}

	eventTypes := parseSSEEvents(t, resp.Body)
	expected := []string{
		"RUN_STARTED",
		"TEXT_MESSAGE_START",
		"TEXT_MESSAGE_CONTENT",
		"TEXT_MESSAGE_END",
		"RUN_FINISHED",
	}
	if len(eventTypes) != len(expected) {
		t.Fatalf("got %d events, want %d: %v", len(eventTypes), len(expected), eventTypes)
	}
	for i, want := range expected {
		if eventTypes[i] != want {
			t.Errorf("event %d: got %s, want %s", i, eventTypes[i], want)
		}
	}
}

func TestHandler_RunError(t *testing.T) {
	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		return func(yield func(events.Event, error) bool) {
			if !yield(events.NewRunStartedEvent(input.ThreadID, input.RunID), nil) {
				return
			}
			yield(nil, fmt.Errorf("something went wrong"))
		}
	})

	var gotErr error
	h, err := agui.Handler(agui.Config{
		Agent:   agent,
		OnError: func(err error) { gotErr = err },
	})
	if err != nil {
		t.Fatal(err)
	}

	input := types.RunAgentInput{ThreadID: "t1", RunID: "r1"}
	body, _ := json.Marshal(input)

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp == nil {
		t.Fatal("nil response")
	}

	rawEvents := parseSSEEventsRaw(t, resp.Body)

	// Should contain RUN_STARTED and RUN_ERROR.
	hasRunError := false
	for _, m := range rawEvents {
		if m["type"] == "RUN_ERROR" {
			hasRunError = true
			if runID, _ := m["runId"].(string); runID != "r1" {
				t.Errorf("RUN_ERROR runId = %q, want %q", runID, "r1")
			}
		}
	}
	if !hasRunError {
		t.Errorf("got %v, want RUN_ERROR event in stream", rawEvents)
	}
	if gotErr == nil {
		t.Error("got nil from OnError callback, want it to be called")
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		return func(yield func(events.Event, error) bool) {}
	})

	h, err := agui.Handler(agui.Config{Agent: agent})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("nil response")
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	// GET / without configured capabilities returns 404 (discovery is
	// opt-in). Previously returned 405 before capabilities discovery.
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("got %d, want 404", resp.StatusCode)
	}
}

func TestHandler_InvalidBody(t *testing.T) {
	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		return func(yield func(events.Event, error) bool) {}
	})

	h, err := agui.Handler(agui.Config{Agent: agent})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL, "application/json", strings.NewReader("{invalid"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp == nil {
		t.Fatal("nil response")
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("got %d, want 400", resp.StatusCode)
	}
}

func TestHandler_NilAgent(t *testing.T) {
	_, err := agui.Handler(agui.Config{})
	if err == nil {
		t.Fatal("got nil error, want error for nil Agent")
	}
}

func TestHandler_CORS(t *testing.T) {
	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent(input.ThreadID, input.RunID), nil)
			yield(events.NewRunFinishedEvent(input.ThreadID, input.RunID), nil)
		}
	})

	h, err := agui.Handler(agui.Config{
		Agent: agent,
		CORS:  &agui.CORSConfig{},
	})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	t.Run("POST with CORS default", func(t *testing.T) {
		input := types.RunAgentInput{ThreadID: "t1", RunID: "r1"}
		body, _ := json.Marshal(input)
		resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("Allow-Origin = %q, want *", got)
		}
	})

	t.Run("OPTIONS preflight", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodOptions, srv.URL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("got %d, want 204", resp.StatusCode)
		}
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("Allow-Origin = %q, want *", got)
		}
		if got := resp.Header.Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
			t.Errorf("Allow-Methods = %q, want to contain POST", got)
		}
	})

	t.Run("specific origin", func(t *testing.T) {
		h2, err := agui.Handler(agui.Config{
			Agent: agent,
			CORS:  &agui.CORSConfig{AllowOrigins: []string{"https://myapp.com"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		srv2 := httptest.NewServer(h2)
		t.Cleanup(srv2.Close)

		input := types.RunAgentInput{ThreadID: "t1", RunID: "r1"}
		body, _ := json.Marshal(input)
		resp, err := http.Post(srv2.URL, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://myapp.com" {
			t.Errorf("Allow-Origin = %q, want https://myapp.com", got)
		}
	})
}

func TestHandler_CORSWithCustomHeaders(t *testing.T) {
	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent(input.ThreadID, input.RunID), nil)
			yield(events.NewRunFinishedEvent(input.ThreadID, input.RunID), nil)
		}
	})

	h, err := agui.Handler(agui.Config{
		Agent: agent,
		CORS: &agui.CORSConfig{
			AllowOrigins:     []string{"https://app.example.com"},
			AllowMethods:     []string{"POST", "OPTIONS", "GET"},
			AllowHeaders:     []string{"Content-Type", "Authorization", "X-Request-ID"},
			ExposeHeaders:    []string{"X-Response-ID", "X-Trace-ID"},
			AllowCredentials: true,
			MaxAge:           600 * time.Second,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	t.Run("OPTIONS preflight with custom headers", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodOptions, srv.URL, nil)
		req.Header.Set("Origin", "https://app.example.com")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })

		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("got %d, want 204", resp.StatusCode)
		}
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
			t.Errorf("Allow-Origin = %q, want https://app.example.com", got)
		}
		if got := resp.Header.Get("Access-Control-Allow-Methods"); !strings.Contains(got, "GET") {
			t.Errorf("Allow-Methods = %q, want to contain GET", got)
		}
		if got := resp.Header.Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Authorization") {
			t.Errorf("Allow-Headers = %q, want to contain Authorization", got)
		}
		if got := resp.Header.Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-Request-ID") {
			t.Errorf("Allow-Headers = %q, want to contain X-Request-ID", got)
		}
		if got := resp.Header.Get("Access-Control-Expose-Headers"); !strings.Contains(got, "X-Response-ID") {
			t.Errorf("Expose-Headers = %q, want to contain X-Response-ID", got)
		}
		if got := resp.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Errorf("Allow-Credentials = %q, want true", got)
		}
		if got := resp.Header.Get("Access-Control-Max-Age"); got != "600" {
			t.Errorf("Max-Age = %q, want 600", got)
		}
	})

	t.Run("POST with custom CORS headers", func(t *testing.T) {
		input := types.RunAgentInput{ThreadID: "t1", RunID: "r1"}
		body, _ := json.Marshal(input)
		resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })

		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
			t.Errorf("Allow-Origin = %q, want https://app.example.com", got)
		}
		if got := resp.Header.Get("Access-Control-Expose-Headers"); !strings.Contains(got, "X-Trace-ID") {
			t.Errorf("Expose-Headers = %q, want to contain X-Trace-ID", got)
		}
		if got := resp.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Errorf("Allow-Credentials = %q, want true", got)
		}
	})
}

// TestHandler_CORSCredentialsWithWildcardOrigin verifies that when
// AllowCredentials is true and AllowOrigins defaults to ["*"], the server
// reflects the request Origin header instead of sending "*" — the W3C CORS
// spec forbids "*" with credentials and browsers reject that combination.
func TestHandler_CORSCredentialsWithWildcardOrigin(t *testing.T) {
	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		return func(yield func(events.Event, error) bool) {
			yield(events.NewRunStartedEvent(input.ThreadID, input.RunID), nil)
			yield(events.NewRunFinishedEvent(input.ThreadID, input.RunID), nil)
		}
	})

	h, err := agui.Handler(agui.Config{
		Agent: agent,
		CORS: &agui.CORSConfig{
			AllowCredentials: true,
			// AllowOrigins left empty → defaults to ["*"]
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	t.Run("OPTIONS reflects request origin", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodOptions, srv.URL, nil)
		req.Header.Set("Origin", "http://localhost:3000")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })

		origin := resp.Header.Get("Access-Control-Allow-Origin")
		if origin == "*" {
			t.Fatal("W3C CORS violation: Access-Control-Allow-Origin cannot be '*' when Allow-Credentials is true")
		}
		if origin != "http://localhost:3000" {
			t.Errorf("got %q, want origin 'http://localhost:3000'", origin)
		}
		if resp.Header.Get("Vary") != "Origin" {
			t.Errorf("got %q, want Vary: Origin header", resp.Header.Get("Vary"))
		}
		if resp.Header.Get("Access-Control-Allow-Credentials") != "true" {
			t.Errorf("got %q, want Allow-Credentials: true", resp.Header.Get("Access-Control-Allow-Credentials"))
		}
	})

	t.Run("POST reflects request origin", func(t *testing.T) {
		input := types.RunAgentInput{ThreadID: "t1", RunID: "r1"}
		body, _ := json.Marshal(input)
		req, _ := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost:3000")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })

		origin := resp.Header.Get("Access-Control-Allow-Origin")
		if origin == "*" {
			t.Fatal("W3C CORS violation: Access-Control-Allow-Origin cannot be '*' when Allow-Credentials is true")
		}
		if origin != "http://localhost:3000" {
			t.Errorf("got %q, want origin 'http://localhost:3000'", origin)
		}
	})

	t.Run("no Origin header still gets wildcard", func(t *testing.T) {
		// Non-browser clients without an Origin header should still get a
		// permissive response.
		input := types.RunAgentInput{ThreadID: "t1", RunID: "r1"}
		body, _ := json.Marshal(input)
		resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })

		origin := resp.Header.Get("Access-Control-Allow-Origin")
		if origin != "*" {
			t.Errorf("got %q, want wildcard for no-Origin request", origin)
		}
	})
}

func TestHandler_Keepalive(t *testing.T) {
	// The agent blocks on a channel until the test signals it to proceed.
	// This keeps the SSE stream open so the keepalive ticker can fire.
	// The test reads the stream for a ping, then releases the agent.
	agentProceed := make(chan struct{})
	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		return func(yield func(events.Event, error) bool) {
			// Block until the test signals us to proceed. The keepalive
			// ticker should fire while we're blocked here.
			select {
			case <-agentProceed:
			case <-ctx.Done():
				return
			}
			if !yield(events.NewRunStartedEvent(input.ThreadID, input.RunID), nil) {
				return
			}
			yield(events.NewRunFinishedEvent(input.ThreadID, input.RunID), nil)
		}
	})

	h, err := agui.Handler(agui.Config{
		Agent:             agent,
		KeepaliveInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	input := types.RunAgentInput{ThreadID: "t1", RunID: "r1"}
	body, _ := json.Marshal(input)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, bytes.NewReader(body))
	if req == nil {
		t.Fatal("nil request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("nil response")
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	scanner := bufio.NewScanner(resp.Body)
	hasPing := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, ": ping") {
			hasPing = true
			// Release the agent so it can emit its events and the stream
			// can complete cleanly.
			close(agentProceed)
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	if !hasPing {
		t.Error("got no keepalive ping, want at least one before events")
	}
}

func TestHandler_MaxBodySize(t *testing.T) {
	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		return func(yield func(events.Event, error) bool) {}
	})

	h, err := agui.Handler(agui.Config{
		Agent:       agent,
		MaxBodySize: 100,
	})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	// Body > 100 bytes.
	bigBody := strings.Repeat("x", 200)
	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(bigBody))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp == nil {
		t.Fatal("nil response")
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("got %d, want 400", resp.StatusCode)
	}
}

func TestHandler_DisconnectCancelsAgent(t *testing.T) {
	ctxCancelled := make(chan struct{})

	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		return func(yield func(events.Event, error) bool) {
			if !yield(events.NewRunStartedEvent(input.ThreadID, input.RunID), nil) {
				return
			}
			<-ctx.Done()
			close(ctxCancelled)
		}
	})

	h, err := agui.Handler(agui.Config{Agent: agent})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	input := types.RunAgentInput{ThreadID: "t1", RunID: "r1"}
	body, _ := json.Marshal(input)

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, bytes.NewReader(body))
	if req == nil {
		t.Fatal("nil request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("nil response")
	}

	cancel()

	select {
	case <-ctxCancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("agent context was not cancelled after client disconnect")
	}

	_ = resp.Body.Close()
}

func TestHandler_AcceptNegotiation(t *testing.T) {
	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		return func(yield func(events.Event, error) bool) {}
	})

	h, err := agui.Handler(agui.Config{Agent: agent})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	tests := []struct {
		name       string
		accept     string
		wantStatus int
	}{
		{"protobuf rejected", "application/vnd.ag-ui.event+proto", http.StatusNotAcceptable},
		{"x-protobuf rejected", "application/x-protobuf", http.StatusNotAcceptable},
		{"google protobuf rejected", "application/vnd.google.protobuf", http.StatusNotAcceptable},
		{"SSE accepted", "text/event-stream", http.StatusOK},
		{"JSON accepted", "application/json", http.StatusOK},
		{"empty accepted", "", http.StatusOK},
		{"wildcard accepted", "*/*", http.StatusOK},
	}

	body, _ := json.Marshal(types.RunAgentInput{
		ThreadID: "t1",
		RunID:    "r1",
		Messages: []types.Message{{ID: "m1", Role: types.RoleUser, Content: "hi"}},
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(body))
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = resp.Body.Close() })
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("got %d, want %d", resp.StatusCode, tt.wantStatus)
			}
		})
	}
}
