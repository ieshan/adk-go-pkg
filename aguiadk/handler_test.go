package aguiadk_test

import (
	"bytes"
	"encoding/json"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"

	"github.com/ieshan/adk-go-pkg/agui"
	"github.com/ieshan/adk-go-pkg/aguiadk"
	"github.com/ieshan/adk-go-pkg/testutil"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

func TestHandler_Success(t *testing.T) {
	adkAgent := testutil.NewFakeAgent("test-handler")

	h, err := aguiadk.Handler(
		aguiadk.Config{
			Agent:   adkAgent,
			AppName: "test-app",
			UserID:  "user-1",
		},
		agui.Config{},
	)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestHandler_InvalidConfig(t *testing.T) {
	// Missing Agent should cause an error.
	h, err := aguiadk.Handler(
		aguiadk.Config{},
		agui.Config{},
	)
	if err == nil {
		t.Fatal("expected error for missing Agent")
	}
	if h != nil {
		t.Fatal("expected nil handler on error")
	}
}

func TestHandler_E2E_SSE(t *testing.T) {
	// Create a mock ADK agent that returns a simple text response.
	ev := session.NewEvent("inv-1")
	ev.Author = "e2e-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "Hello from handler!"}},
		},
		Partial: false,
	}

	adkAgent := testutil.NewFakeAgent("e2e-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			if !yield(ev, nil) {
				return
			}
		}
	})

	h, err := aguiadk.Handler(
		aguiadk.Config{
			Agent:   adkAgent,
			AppName: "e2e-app",
			UserID:  "e2e-user",
		},
		agui.Config{},
	)
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}

	srv := httptest.NewServer(h)
	defer srv.Close()

	// Build RunAgentInput payload.
	input := types.RunAgentInput{
		ThreadID: "e2e-thread",
		RunID:    "e2e-run",
		Messages: []types.Message{
			{
				ID:      "msg-1",
				Role:    types.RoleUser,
				Content: "Hello",
			},
		},
	}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}

	// Read the full SSE response.
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	sseBody := string(data)

	// Verify RUN_STARTED and RUN_FINISHED events are present.
	if !strings.Contains(sseBody, "RUN_STARTED") {
		t.Error("expected RUN_STARTED event in SSE response")
	}
	if !strings.Contains(sseBody, "RUN_FINISHED") {
		t.Error("expected RUN_FINISHED event in SSE response")
	}
	if !strings.Contains(sseBody, "TEXT_MESSAGE_START") {
		t.Error("expected TEXT_MESSAGE_START event in SSE response")
	}
	if !strings.Contains(sseBody, "Hello from handler!") {
		t.Error("expected text content in SSE response")
	}
}
