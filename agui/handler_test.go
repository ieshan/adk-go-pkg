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
	defer srv.Close()

	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", ct)
	}

	eventTypes := parseSSEEvents(t, resp.Body)
	if len(eventTypes) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(eventTypes))
	}
	if eventTypes[0] != "RUN_STARTED" {
		t.Errorf("event 0: expected RUN_STARTED, got %s", eventTypes[0])
	}
	if eventTypes[len(eventTypes)-1] != "RUN_FINISHED" {
		t.Errorf("last event: expected RUN_FINISHED, got %s", eventTypes[len(eventTypes)-1])
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
	defer srv.Close()

	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	eventTypes := parseSSEEvents(t, resp.Body)
	expected := []string{
		"RUN_STARTED",
		"TEXT_MESSAGE_START",
		"TEXT_MESSAGE_CONTENT",
		"TEXT_MESSAGE_END",
		"RUN_FINISHED",
	}
	if len(eventTypes) != len(expected) {
		t.Fatalf("expected %d events, got %d: %v", len(expected), len(eventTypes), eventTypes)
	}
	for i, want := range expected {
		if eventTypes[i] != want {
			t.Errorf("event %d: expected %s, got %s", i, want, eventTypes[i])
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
	defer srv.Close()

	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

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
		t.Errorf("expected RUN_ERROR event in stream, got: %v", rawEvents)
	}
	if gotErr == nil {
		t.Error("expected OnError callback to be called")
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
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
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
	defer srv.Close()

	resp, err := http.Post(srv.URL, "application/json", strings.NewReader("{invalid"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandler_NilAgent(t *testing.T) {
	_, err := agui.Handler(agui.Config{})
	if err == nil {
		t.Fatal("expected error for nil Agent")
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
	defer srv.Close()

	t.Run("POST with CORS default", func(t *testing.T) {
		input := types.RunAgentInput{ThreadID: "t1", RunID: "r1"}
		body, _ := json.Marshal(input)
		resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
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
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("expected 204, got %d", resp.StatusCode)
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
		defer srv2.Close()

		input := types.RunAgentInput{ThreadID: "t1", RunID: "r1"}
		body, _ := json.Marshal(input)
		resp, err := http.Post(srv2.URL, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
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
	defer srv.Close()

	t.Run("OPTIONS preflight with custom headers", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodOptions, srv.URL, nil)
		req.Header.Set("Origin", "https://app.example.com")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("expected 204, got %d", resp.StatusCode)
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
		defer func() { _ = resp.Body.Close() }()

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

func TestHandler_Keepalive(t *testing.T) {
	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		return func(yield func(events.Event, error) bool) {
			time.Sleep(200 * time.Millisecond)
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
	defer srv.Close()

	input := types.RunAgentInput{ThreadID: "t1", RunID: "r1"}
	body, _ := json.Marshal(input)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	scanner := bufio.NewScanner(resp.Body)
	hasPing := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, ": ping") {
			hasPing = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	if !hasPing {
		t.Error("expected at least one keepalive ping before events")
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
	defer srv.Close()

	// Body > 100 bytes.
	bigBody := strings.Repeat("x", 200)
	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(bigBody))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
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
	defer srv.Close()

	input := types.RunAgentInput{ThreadID: "t1", RunID: "r1"}
	body, _ := json.Marshal(input)

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	cancel()

	select {
	case <-ctxCancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("agent context was not cancelled after client disconnect")
	}

	_ = resp.Body.Close()
}
