package aguiadk_test

import (
	"bytes"
	"context"
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

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

func TestHandler_Success(t *testing.T) {
	adkAgent := testutil.MustNewFakeAgent("test-handler")

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
		t.Fatal("got nil handler, want non-nil")
	}
}

func TestHandler_InvalidConfig(t *testing.T) {
	// Missing Agent should cause an error.
	h, err := aguiadk.Handler(
		aguiadk.Config{},
		agui.Config{},
	)
	if err == nil {
		t.Fatal("got nil error, want error for missing Agent")
	}
	if h != nil {
		t.Fatal("got non-nil handler on error, want nil")
	}
}

func TestHandler_E2E_SSE(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	// Create a mock ADK agent that returns a simple text response.
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "e2e-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: "Hello from handler!"}},
		},
		Partial: false,
	}

	adkAgent := testutil.MustNewFakeAgent("e2e-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
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
	t.Cleanup(srv.Close)

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
	if resp == nil {
		t.Fatal("nil response")
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("got %q, want text/event-stream", ct)
	}

	// Read the full SSE response.
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	sseBody := string(data)

	// Verify RUN_STARTED and RUN_FINISHED events are present.
	if !strings.Contains(sseBody, "RUN_STARTED") {
		t.Error("got no RUN_STARTED event in SSE response, want one")
	}
	if !strings.Contains(sseBody, "RUN_FINISHED") {
		t.Error("got no RUN_FINISHED event in SSE response, want one")
	}
	if !strings.Contains(sseBody, "TEXT_MESSAGE_START") {
		t.Error("got no TEXT_MESSAGE_START event in SSE response, want one")
	}
	if !strings.Contains(sseBody, "Hello from handler!") {
		t.Error("got no text content in SSE response, want one")
	}
}

func TestHandler_InlineToolMode(t *testing.T) {
	adkAgent := testutil.MustNewFakeAgent("inline-agent")

	h, err := aguiadk.Handler(
		aguiadk.Config{
			Agent:   adkAgent,
			AppName: "inline-app",
			UserID:  "user-1",
		},
		agui.Config{
			ToolMode: agui.ToolModeInline,
		},
	)
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	// POST a tool result — the handler should accept it at /tool-result.
	resultBody, _ := json.Marshal(map[string]string{
		"toolCallId": "tc-test-1",
		"content":    "result data",
	})
	resp, err := http.Post(srv.URL+"/tool-result", "application/json", bytes.NewReader(resultBody))
	if err != nil {
		t.Fatalf("POST /tool-result: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp == nil {
		t.Fatal("nil response")
	}

	// 404 is expected since no agent run is waiting for this tool call ID,
	// but the endpoint should exist and respond (not 405 or connection refused).
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("got %d, want 404 (no pending tool call)", resp.StatusCode)
	}

	// Verify GET to /tool-result returns 405 (method not allowed via Go 1.22+ pattern).
	getResp, err := http.Get(srv.URL + "/tool-result")
	if err != nil {
		t.Fatalf("GET /tool-result: %v", err)
	}
	if getResp == nil {
		t.Fatal("nil response")
	}
	t.Cleanup(func() { _ = getResp.Body.Close() })
	if getResp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /tool-result: got %d, want 405", getResp.StatusCode)
	}
}

func TestHandler_PerRequestApproval(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ev := session.NewEvent(context.Background(), "inv-1")
	ev.Author = "approval-agent"
	ev.LLMResponse = model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
			Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "fc-1",
					Name: "approve",
					Args: map[string]any{"action": "delete"},
				},
			}},
		},
		Partial: false,
	}
	ev.LongRunningToolIDs = []string{"fc-1"}

	adkAgent := testutil.MustNewFakeAgent("approval-agent").WithRunFunc(func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
		return func(yield func(*session.Event, error) bool) {
			if !yield(ev, nil) {
				return
			}
		}
	})

	var sawRequest bool
	h, err := aguiadk.Handler(
		aguiadk.Config{
			Agent:   adkAgent,
			AppName: "approval-app",
			UserID:  "user-1",
			ApprovalModeFunc: func(r *http.Request) bool {
				sawRequest = true
				if r == nil {
					t.Error("got nil *http.Request in ApprovalModeFunc, want non-nil")
				}
				return r.Header.Get("X-AG-Approval") == "auto"
			},
		},
		agui.Config{},
	)
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	input := types.RunAgentInput{
		ThreadID: "approval-thread",
		RunID:    "approval-run",
		Messages: []types.Message{
			{ID: "msg-1", Role: types.RoleUser, Content: "do it"},
		},
	}
	body, _ := json.Marshal(input)

	req, _ := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(body))
	if req == nil {
		t.Fatal("nil request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AG-Approval", "auto")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	if resp == nil {
		t.Fatal("nil response")
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}

	data, _ := io.ReadAll(resp.Body)
	sseBody := string(data)

	if !sawRequest {
		t.Fatal("got ApprovalModeFunc not called with HTTP request, want called")
	}

	if strings.Contains(sseBody, `"interrupt"`) {
		t.Error("got interrupt outcome with auto-approval via X-AG-Approval header, want none")
	}
	if !strings.Contains(sseBody, "RUN_FINISHED") {
		t.Error("got no RUN_FINISHED in SSE response, want one")
	}
}
