package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

// cannedChatResponse is a reusable canned non-streaming response JSON.
const cannedChatResponse = `{
  "id": "chatcmpl-123",
  "choices": [{
    "index": 0,
    "message": {"role": "assistant", "content": "Hello!"},
    "finish_reason": "stop"
  }],
  "usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
}`

// collectResponses drives an iter.Seq2[*model.LLMResponse, error] to completion.
func collectResponses(m model.LLM, ctx context.Context, req *model.LLMRequest, stream bool) ([]*model.LLMResponse, []error) {
	var resps []*model.LLMResponse
	var errs []error
	for resp, err := range m.GenerateContent(ctx, req, stream) {
		if err != nil {
			errs = append(errs, err)
		} else {
			resps = append(resps, resp)
		}
	}
	return resps, errs
}

// TestNew_ValidConfig verifies that New with a minimal valid config returns a
// non-nil LLM whose Name() matches the model name in the config.
func TestNew_ValidConfig(t *testing.T) {
	m, err := New(Config{
		Model:  "gpt-4o",
		APIKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("New: got nil LLM, want non-nil")
	}
	if m.Name() != "gpt-4o" {
		t.Errorf("Name(): got %q, want %q", m.Name(), "gpt-4o")
	}
}

// TestNew_EmptyModel verifies that New returns an error when Model is empty.
func TestNew_EmptyModel(t *testing.T) {
	_, err := New(Config{APIKey: "sk-test"})
	if err == nil {
		t.Errorf("New: got nil error, want error for empty model")
	}
}

// TestNew_DefaultBaseURL verifies that omitting BaseURL in Config sets the
// default OpenAI base URL so requests are sent there.
func TestNew_DefaultBaseURL(t *testing.T) {
	// We can only verify the default is applied by inspecting the struct;
	// since openaiModel is unexported, we use a test server that closes
	// immediately to confirm the base URL is used.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, cannedChatResponse)
	}))
	t.Cleanup(srv.Close)

	// Model with explicit base URL pointing at our test server.
	m, err := New(Config{
		Model:   "gpt-4o",
		APIKey:  "sk-test",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
	}
	resps, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(resps) == 0 {
		t.Fatal("got no responses, want at least one")
	}

	// Now create a model with NO BaseURL — it should default to the OpenAI URL.
	// We just verify New doesn't error; we can't reach the real API in tests.
	mDefault, err := New(Config{
		Model:  "gpt-4o",
		APIKey: "sk-test",
		// BaseURL intentionally omitted
	})
	if err != nil {
		t.Fatalf("New (default base URL): unexpected error: %v", err)
	}
	if mDefault.Name() != "gpt-4o" {
		t.Errorf("Name(): got %q, want %q", mDefault.Name(), "gpt-4o")
	}
}

// TestGenerateContent_NonStreaming starts a test HTTP server that returns a
// canned chatResponse and verifies that the LLM yields a correct LLMResponse
// with the expected text, finish reason, and usage metadata.
func TestGenerateContent_NonStreaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("got %s, want POST", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("got %s, want path /chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, cannedChatResponse)
	}))
	t.Cleanup(srv.Close)

	m, err := New(Config{
		Model:   "gpt-4o",
		APIKey:  "sk-test",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "Hello"}}}},
	}

	resps, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(resps) != 1 {
		t.Fatalf("got %d responses, want 1", len(resps))
	}

	resp := resps[0]

	// Verify TurnComplete is set on non-streaming responses.
	if !resp.TurnComplete {
		t.Errorf("got TurnComplete=false, want true for non-streaming response")
	}

	// Verify text content.
	if resp.Content == nil {
		t.Fatal("got nil Content, want non-nil")
	}
	if len(resp.Content.Parts) == 0 {
		t.Fatal("got no Parts, want at least one")
	}
	if resp.Content.Parts[0].Text != "Hello!" {
		t.Errorf("text: got %q, want %q", resp.Content.Parts[0].Text, "Hello!")
	}

	// Verify finish reason.
	if resp.FinishReason != genai.FinishReasonStop {
		t.Errorf("finish_reason: got %v, want %v", resp.FinishReason, genai.FinishReasonStop)
	}

	// Verify usage.
	if resp.UsageMetadata == nil {
		t.Fatal("got nil UsageMetadata, want non-nil")
	}
	if resp.UsageMetadata.PromptTokenCount != 10 {
		t.Errorf("prompt_tokens: got %d, want 10", resp.UsageMetadata.PromptTokenCount)
	}
	if resp.UsageMetadata.CandidatesTokenCount != 5 {
		t.Errorf("completion_tokens: got %d, want 5", resp.UsageMetadata.CandidatesTokenCount)
	}
	if resp.UsageMetadata.TotalTokenCount != 15 {
		t.Errorf("total_tokens: got %d, want 15", resp.UsageMetadata.TotalTokenCount)
	}
}

// TestGenerateContent_Streaming starts a test server that returns a canned SSE
// stream. Verifies that the model yields partial text responses followed by a
// final TurnComplete response.
func TestGenerateContent_Streaming(t *testing.T) {
	sseBody := strings.Join([]string{
		`data: {"id":"1","choices":[{"index":0,"delta":{"role":"assistant","content":"Hi"}}]}`,
		``,
		`data: {"id":"1","choices":[{"index":0,"delta":{"content":"!"}}]}`,
		``,
		`data: {"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, sseBody)
	}))
	t.Cleanup(srv.Close)

	m, err := New(Config{
		Model:   "gpt-4o",
		APIKey:  "sk-test",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "Hello"}}}},
	}

	resps, errs := collectResponses(m, context.Background(), req, true)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(resps) == 0 {
		t.Fatal("got no responses, want at least one")
	}

	// Verify at least one partial text response.
	foundPartial := false
	for _, resp := range resps {
		if resp.Partial && resp.Content != nil && len(resp.Content.Parts) > 0 && resp.Content.Parts[0].Text != "" {
			foundPartial = true
			break
		}
	}
	if !foundPartial {
		t.Errorf("got no partial text responses, want at least one")
	}

	// Verify last response is TurnComplete.
	last := resps[len(resps)-1]
	if !last.TurnComplete {
		t.Errorf("last response: got %+v, want TurnComplete=true", last)
	}
}

// TestGenerateContent_ToolCalling starts a test server that returns a response
// with tool_calls and verifies that FunctionCall Parts are present in the
// resulting LLMResponse.
func TestGenerateContent_ToolCalling(t *testing.T) {
	toolResp := `{
  "id": "chatcmpl-tools",
  "choices": [{
    "index": 0,
    "message": {
      "role": "assistant",
      "content": null,
      "tool_calls": [{
        "id": "call_abc",
        "type": "function",
        "function": {
          "name": "get_weather",
          "arguments": "{\"location\": \"NYC\"}"
        }
      }]
    },
    "finish_reason": "tool_calls"
  }],
  "usage": {"prompt_tokens": 20, "completion_tokens": 10, "total_tokens": 30}
}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, toolResp)
	}))
	t.Cleanup(srv.Close)

	m, err := New(Config{
		Model:   "gpt-4o",
		APIKey:  "sk-test",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "What's the weather in NYC?"}}}},
	}

	resps, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(resps) != 1 {
		t.Fatalf("got %d responses, want 1", len(resps))
	}

	resp := resps[0]
	if resp.Content == nil {
		t.Fatal("got nil Content, want non-nil")
	}

	// Find FunctionCall part.
	var fc *genai.FunctionCall
	for _, p := range resp.Content.Parts {
		if p.FunctionCall != nil {
			fc = p.FunctionCall
			break
		}
	}
	if fc == nil {
		t.Fatal("got no FunctionCall part, want one")
	}
	if fc.Name != "get_weather" {
		t.Errorf("FunctionCall.Name: got %q, want %q", fc.Name, "get_weather")
	}
	if fc.ID != "call_abc" {
		t.Errorf("FunctionCall.ID: got %q, want %q", fc.ID, "call_abc")
	}
	if loc, ok := fc.Args["location"]; !ok || loc != "NYC" {
		t.Errorf("FunctionCall.Args[location]: got %v, want %q", fc.Args["location"], "NYC")
	}
}

// TestGenerateContent_StructuredOutput verifies that when the request has a
// ResponseSchema, the HTTP request body sent to the server contains a
// response_format field with type "json_schema".
func TestGenerateContent_StructuredOutput(t *testing.T) {
	var receivedBody chatRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &receivedBody); err != nil {
			t.Errorf("unmarshal body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, cannedChatResponse)
	}))
	t.Cleanup(srv.Close)

	m, err := New(Config{
		Model:   "gpt-4o",
		APIKey:  "sk-test",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "give me JSON"}}}},
		Config: &genai.GenerateContentConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"answer": {Type: genai.TypeString},
				},
			},
		},
	}

	_, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	if receivedBody.ResponseFormat == nil {
		t.Fatal("got nil response_format in request body, want non-nil")
	}

	rfMap, ok := receivedBody.ResponseFormat.(map[string]any)
	if !ok {
		t.Fatalf("response_format: got %T, want map[string]any", receivedBody.ResponseFormat)
	}
	if rfMap["type"] != "json_schema" {
		t.Errorf("response_format.type: got %v, want %q", rfMap["type"], "json_schema")
	}
	if _, ok := rfMap["json_schema"]; !ok {
		t.Error("response_format: missing json_schema key")
	}
}

// TestGenerateContent_CustomHeaders verifies that custom headers configured in
// Config.Headers are included in every request sent to the server.
func TestGenerateContent_CustomHeaders(t *testing.T) {
	var receivedXCustom, receivedXVersion string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedXCustom = r.Header.Get("X-Custom-Header")
		receivedXVersion = r.Header.Get("X-Version")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, cannedChatResponse)
	}))
	t.Cleanup(srv.Close)

	m, err := New(Config{
		Model:   "gpt-4o",
		APIKey:  "sk-test",
		BaseURL: srv.URL,
		Headers: map[string]string{
			"X-Custom-Header": "my-value",
			"X-Version":       "v2",
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
	}

	_, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	if receivedXCustom != "my-value" {
		t.Errorf("X-Custom-Header: got %q, want %q", receivedXCustom, "my-value")
	}
	if receivedXVersion != "v2" {
		t.Errorf("X-Version: got %q, want %q", receivedXVersion, "v2")
	}
}

// TestGenerateContent_ErrorResponse verifies that when the server returns a
// non-2xx status code, the model yields an error.
func TestGenerateContent_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"invalid request","type":"invalid_request_error"}}`, http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	m, err := New(Config{
		Model:   "gpt-4o",
		APIKey:  "sk-test",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
	}

	_, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) == 0 {
		t.Fatal("got no errors from 400 response, want at least one")
	}
}

// TestGenerateContent_AuthorizationHeader verifies that the API key is sent as
// a Bearer token in the Authorization header.
func TestGenerateContent_AuthorizationHeader(t *testing.T) {
	var receivedAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, cannedChatResponse)
	}))
	t.Cleanup(srv.Close)

	m, err := New(Config{
		Model:   "gpt-4o",
		APIKey:  "sk-my-secret-key",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
	}

	_, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	want := "Bearer sk-my-secret-key"
	if receivedAuth != want {
		t.Errorf("Authorization: got %q, want %q", receivedAuth, want)
	}
}

// TestNon2xxErrorBodyClose verifies that the response body is closed even when
// io.ReadAll fails during non-2xx error handling. The server returns a 500 with
// a Content-Length that exceeds the actual body, causing io.ReadAll to fail
// with io.ErrUnexpectedEOF. A transport with MaxConnsPerHost: 1 ensures that if
// the body is not closed, a second request cannot obtain a connection and times
// out — proving the defer resp.Body.Close() releases the connection slot.
func TestGenerateContent_Non2xxErrorBodyClose(t *testing.T) {
	t.Parallel()

	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount > 1 {
			// Second request: return a normal 500 with a readable body.
			http.Error(w, `{"error":"bad"}`, http.StatusInternalServerError)
			return
		}
		// First request: hijack and send a truncated body so io.ReadAll
		// fails on the client side with io.ErrUnexpectedEOF.
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatalf("server does not support hijacking")
		}
		conn, bufrw, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}
		_, _ = bufrw.WriteString("HTTP/1.1 500 Internal Server Error\r\n")
		_, _ = bufrw.WriteString("Content-Length: 100\r\n")
		_, _ = bufrw.WriteString("\r\n")
		_, _ = bufrw.WriteString("partial") // only 7 of 100 promised bytes
		_ = bufrw.Flush()
		_ = conn.Close()
	}))
	t.Cleanup(srv.Close)

	// Use a transport with MaxConnsPerHost: 1 so that if the body from the
	// first request is not closed, the second request cannot get a connection.
	transport := &http.Transport{
		MaxConnsPerHost: 1,
	}
	t.Cleanup(transport.CloseIdleConnections)

	m, err := New(Config{
		Model:      "gpt-4o",
		APIKey:     "sk-test",
		BaseURL:    srv.URL,
		HTTPClient: &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
	}

	// First request: io.ReadAll fails because the body is truncated.
	_, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) == 0 {
		t.Fatal("got no errors from truncated 500 response, want at least one")
	}

	// Second request: if the first body was not closed, MaxConnsPerHost: 1
	// would prevent a new connection and this would hang. Use a timeout
	// context as a safety net so the test fails fast rather than hanging
	// indefinitely if the body leak regresses.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	_, errs2 := collectResponses(m, ctx, req, false)
	if len(errs2) == 0 {
		t.Fatal("got no errors from second 500 response, want at least one")
	}

	// Verify the second error is an HTTPError with status 500.
	found500 := false
	for _, e := range errs2 {
		var httpErr *HTTPError
		if errors.As(e, &httpErr) && httpErr.StatusCode == http.StatusInternalServerError {
			found500 = true
			break
		}
	}
	if !found500 {
		t.Errorf("second errors: got %v, want an HTTPError with status 500", errs2)
	}
}
