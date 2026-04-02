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
	return eventTypes
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

	eventTypes := parseSSEEvents(t, resp.Body)

	// Should contain RUN_STARTED and RUN_ERROR.
	hasRunError := false
	for _, et := range eventTypes {
		if et == "RUN_ERROR" {
			hasRunError = true
		}
	}
	if !hasRunError {
		t.Errorf("expected RUN_ERROR event in stream, got: %v", eventTypes)
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
