package anthropic_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ieshan/adk-go-pkg/model/anthropic"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

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

// cannedTextResponse is a reusable non-streaming response with a single text block.
const cannedTextResponse = `{
  "id": "msg_01",
  "type": "message",
  "role": "assistant",
  "content": [{"type": "text", "text": "Hello!"}],
  "model": "claude-opus-4",
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "usage": {"input_tokens": 10, "output_tokens": 5}
}`

// TestNew_ValidConfig verifies that New with a minimal valid config returns a
// non-nil LLM whose Name() matches the model name.
func TestNew_ValidConfig(t *testing.T) {
	t.Parallel()
	m, err := anthropic.New(anthropic.Config{
		Model:  "claude-opus-4",
		APIKey: "sk-ant-test",
	})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("New: got nil LLM, want non-nil")
	}
	if m.Name() != "claude-opus-4" {
		t.Errorf("Name(): got %q, want %q", m.Name(), "claude-opus-4")
	}
}

// TestNew_EmptyModel verifies that New returns an error when Model is empty.
func TestNew_EmptyModel(t *testing.T) {
	t.Parallel()
	_, err := anthropic.New(anthropic.Config{APIKey: "sk-ant-test"})
	if err == nil {
		t.Errorf("New: got nil error, want error for empty model")
	}
}

// TestGenerateContent_TextOnly verifies non-streaming response parsing,
// finish reason mapping, and usage metadata.
func TestGenerateContent_TextOnly(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("got %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("got %s, want path /v1/messages", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, cannedTextResponse)
	}))
	t.Cleanup(srv.Close)

	m, err := anthropic.New(anthropic.Config{
		Model:   "claude-opus-4",
		APIKey:  "sk-ant-test",
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
	if !resp.TurnComplete {
		t.Errorf("got TurnComplete=false, want true for non-streaming response")
	}
	if resp.Content == nil || len(resp.Content.Parts) == 0 {
		t.Fatal("got nil Content or no Parts, want non-nil Content with at least one Part")
	}
	if resp.Content.Parts[0].Text != "Hello!" {
		t.Errorf("text: got %q, want %q", resp.Content.Parts[0].Text, "Hello!")
	}
	if resp.FinishReason != genai.FinishReasonStop {
		t.Errorf("finish_reason: got %v, want %v", resp.FinishReason, genai.FinishReasonStop)
	}
	if resp.UsageMetadata == nil {
		t.Fatal("got nil UsageMetadata, want non-nil")
	}
	if resp.UsageMetadata.PromptTokenCount != 10 {
		t.Errorf("prompt_tokens: got %d, want 10", resp.UsageMetadata.PromptTokenCount)
	}
	if resp.UsageMetadata.CandidatesTokenCount != 5 {
		t.Errorf("output_tokens: got %d, want 5", resp.UsageMetadata.CandidatesTokenCount)
	}
	if resp.UsageMetadata.TotalTokenCount != 15 {
		t.Errorf("total_tokens: got %d, want 15", resp.UsageMetadata.TotalTokenCount)
	}
}

// TestGenerateContent_SystemInstruction captures the request body to verify
// that SystemInstruction is emitted as the top-level system field.
func TestGenerateContent_SystemInstruction(t *testing.T) {
	t.Parallel()
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, cannedTextResponse)
	}))
	t.Cleanup(srv.Close)

	m, err := anthropic.New(anthropic.Config{
		Model:   "claude-opus-4",
		APIKey:  "sk-ant-test",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "Hello"}}},
		},
		Config: &genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{{Text: "You are helpful."}},
			},
		},
	}

	_, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	if capturedBody["system"] != "You are helpful." {
		t.Errorf("system: got %v, want %q", capturedBody["system"], "You are helpful.")
	}
}

// TestGenerateContent_Images captures the request body to verify InlineData
// parts are emitted as image blocks with base64 source.
func TestGenerateContent_Images(t *testing.T) {
	t.Parallel()
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, cannedTextResponse)
	}))
	t.Cleanup(srv.Close)

	m, err := anthropic.New(anthropic.Config{
		Model:   "claude-opus-4",
		APIKey:  "sk-ant-test",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{
				Role: "user",
				Parts: []*genai.Part{
					{Text: "What is this?"},
					{InlineData: &genai.Blob{MIMEType: "image/png", Data: []byte("fakeimage")}},
				},
			},
		},
	}

	_, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	msgs, ok := capturedBody["messages"].([]any)
	if !ok || len(msgs) == 0 {
		t.Fatal("got non-array or empty messages, want messages array")
	}
	content, ok := msgs[0].(map[string]any)["content"].([]any)
	if !ok || len(content) != 2 {
		t.Fatalf("got %v content blocks, want 2", content)
	}
	imgBlock, ok := content[1].(map[string]any)
	if !ok {
		t.Fatal("got non-map image block, want map")
	}
	if imgBlock["type"] != "image" {
		t.Errorf("type: got %v, want %q", imgBlock["type"], "image")
	}
	source, ok := imgBlock["source"].(map[string]any)
	if !ok {
		t.Fatal("got non-map source, want map")
	}
	if source["type"] != "base64" {
		t.Errorf("source.type: got %v, want %q", source["type"], "base64")
	}
	if source["media_type"] != "image/png" {
		t.Errorf("source.media_type: got %v, want %q", source["media_type"], "image/png")
	}
}

// TestGenerateContent_Tools captures the request body to verify tools are
// emitted with input_schema.
func TestGenerateContent_Tools(t *testing.T) {
	t.Parallel()
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, cannedTextResponse)
	}))
	t.Cleanup(srv.Close)

	m, err := anthropic.New(anthropic.Config{
		Model:   "claude-opus-4",
		APIKey:  "sk-ant-test",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "What's the weather?"}}},
		},
		Config: &genai.GenerateContentConfig{
			Tools: []*genai.Tool{
				{
					FunctionDeclarations: []*genai.FunctionDeclaration{
						{
							Name:        "get_weather",
							Description: "Get weather",
							Parameters: &genai.Schema{
								Type: genai.TypeObject,
								Properties: map[string]*genai.Schema{
									"location": {Type: genai.TypeString},
								},
							},
						},
					},
				},
			},
		},
	}

	_, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	tools, ok := capturedBody["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("got %v tools, want 1", capturedBody["tools"])
	}
	toolMap, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatal("got non-map tool, want map")
	}
	if toolMap["name"] != "get_weather" {
		t.Errorf("name: got %v, want %q", toolMap["name"], "get_weather")
	}
	if _, hasParams := toolMap["parameters"]; hasParams {
		t.Errorf("got 'parameters' key, want it absent; Anthropic uses 'input_schema'")
	}
	schema, ok := toolMap["input_schema"].(map[string]any)
	if !ok {
		t.Fatal("got non-map input_schema, want map")
	}
	if schema["type"] != "object" {
		t.Errorf("input_schema.type: got %v, want %q", schema["type"], "object")
	}
}

// TestGenerateContent_ToolUseResponse verifies that a response containing
// tool_use blocks is parsed into FunctionCall parts.
func TestGenerateContent_ToolUseResponse(t *testing.T) {
	t.Parallel()
	respJSON := `{
  "id": "msg_02",
  "type": "message",
  "role": "assistant",
  "content": [
    {"type": "tool_use", "id": "toolu_01E", "name": "get_weather", "input": {"location": "Berlin"}}
  ],
  "model": "claude-opus-4",
  "stop_reason": "tool_use",
  "usage": {"input_tokens": 15, "output_tokens": 20}
}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, respJSON)
	}))
	t.Cleanup(srv.Close)

	m, err := anthropic.New(anthropic.Config{
		Model:   "claude-opus-4",
		APIKey:  "sk-ant-test",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "Weather in Berlin?"}}}},
	}

	resps, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(resps) != 1 {
		t.Fatalf("got %d responses, want 1", len(resps))
	}

	resp := resps[0]
	if resp.Content == nil || len(resp.Content.Parts) != 1 {
		t.Fatalf("got %d parts, want 1", len(resp.Content.Parts))
	}
	fc := resp.Content.Parts[0].FunctionCall
	if fc == nil {
		t.Fatal("got no FunctionCall part, want one")
	}
	if fc.ID != "toolu_01E" {
		t.Errorf("ID: got %q, want %q", fc.ID, "toolu_01E")
	}
	if fc.Name != "get_weather" {
		t.Errorf("Name: got %q, want %q", fc.Name, "get_weather")
	}
	if fc.Args["location"] != "Berlin" {
		t.Errorf("Args.location: got %v, want %q", fc.Args["location"], "Berlin")
	}
}

// TestGenerateContent_StructuredOutput captures the request body to verify
// ResponseSchema maps to output_config with format "json" and json_schema.
func TestGenerateContent_StructuredOutput(t *testing.T) {
	t.Parallel()
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, cannedTextResponse)
	}))
	t.Cleanup(srv.Close)

	m, err := anthropic.New(anthropic.Config{
		Model:   "claude-opus-4",
		APIKey:  "sk-ant-test",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "Extract info"}}},
		},
		Config: &genai.GenerateContentConfig{
			ResponseSchema: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"name": {Type: genai.TypeString},
				},
				Required: []string{"name"},
			},
		},
	}

	_, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	outCfg, ok := capturedBody["output_config"].(map[string]any)
	if !ok {
		t.Fatal("got non-map output_config, want map")
	}
	if outCfg["format"] != "json" {
		t.Errorf("format: got %v, want %q", outCfg["format"], "json")
	}
	if _, hasSchema := outCfg["json_schema"]; !hasSchema {
		t.Errorf("got no json_schema in output_config, want it present")
	}
}

// TestGenerateContent_HTTPError verifies that non-2xx responses propagate as
// errors.
func TestGenerateContent_HTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error": {"type": "invalid_request_error", "message": "max_tokens is required"}}`)
	}))
	t.Cleanup(srv.Close)

	m, err := anthropic.New(anthropic.Config{
		Model:   "claude-opus-4",
		APIKey:  "sk-ant-test",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "Hi"}}}},
	}

	resps, errs := collectResponses(m, context.Background(), req, false)
	if len(resps) != 0 {
		t.Errorf("got %d responses, want 0", len(resps))
	}
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1", len(errs))
	}
	if errs[0] == nil {
		t.Fatal("got nil error, want non-nil")
	}
	var httpErr *anthropic.HTTPError
	if !errors.As(errs[0], &httpErr) {
		t.Fatalf("got %T, want *anthropic.HTTPError", errs[0])
	}
	if httpErr.StatusCode != 400 {
		t.Errorf("StatusCode: got %d, want 400", httpErr.StatusCode)
	}
}

// TestGenerateContent_MaxTokensDefault captures the request body to verify
// that max_tokens defaults to 4096 when MaxOutputTokens is zero.
func TestGenerateContent_MaxTokensDefault(t *testing.T) {
	t.Parallel()
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, cannedTextResponse)
	}))
	t.Cleanup(srv.Close)

	m, err := anthropic.New(anthropic.Config{
		Model:   "claude-opus-4",
		APIKey:  "sk-ant-test",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "Hi"}}},
		},
		Config: &genai.GenerateContentConfig{
			MaxOutputTokens: 0,
		},
	}

	_, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	if capturedBody["max_tokens"] != float64(4096) {
		t.Errorf("max_tokens: got %v, want 4096", capturedBody["max_tokens"])
	}
}

// TestGenerateContent_AutomaticCaching captures the request body to verify
// that Config.CacheControl is emitted as a top-level cache_control field.
func TestGenerateContent_AutomaticCaching(t *testing.T) {
	t.Parallel()
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, cannedTextResponse)
	}))
	t.Cleanup(srv.Close)

	m, err := anthropic.New(anthropic.Config{
		Model:        "claude-opus-4",
		APIKey:       "sk-ant-test",
		BaseURL:      srv.URL,
		CacheControl: map[string]any{"type": "ephemeral"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "Hi"}}}},
	}

	_, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	cc, ok := capturedBody["cache_control"].(map[string]any)
	if !ok {
		t.Fatal("got non-map top-level cache_control, want map")
	}
	if cc["type"] != "ephemeral" {
		t.Errorf("cache_control.type: got %v, want %q", cc["type"], "ephemeral")
	}
}

// TestGenerateContent_CacheUsageInResponse verifies that cache-specific usage
// fields are stored in CustomMetadata.
func TestGenerateContent_CacheUsageInResponse(t *testing.T) {
	t.Parallel()
	respJSON := `{
  "id": "msg_03",
  "type": "message",
  "role": "assistant",
  "content": [{"type": "text", "text": "Done"}],
  "model": "claude-opus-4",
  "stop_reason": "end_turn",
  "usage": {
    "input_tokens": 10,
    "output_tokens": 5,
    "cache_creation_input_tokens": 100,
    "cache_read_input_tokens": 200
  }
}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, respJSON)
	}))
	t.Cleanup(srv.Close)

	m, err := anthropic.New(anthropic.Config{
		Model:   "claude-opus-4",
		APIKey:  "sk-ant-test",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "Hi"}}}},
	}

	resps, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(resps) != 1 {
		t.Fatalf("got %d responses, want 1", len(resps))
	}

	resp := resps[0]
	if resp.UsageMetadata == nil {
		t.Fatal("got nil UsageMetadata, want non-nil")
	}
	if resp.UsageMetadata.CachedContentTokenCount != 200 {
		t.Errorf("CachedContentTokenCount: got %d, want 200", resp.UsageMetadata.CachedContentTokenCount)
	}
	if resp.CustomMetadata == nil {
		t.Fatal("got nil CustomMetadata, want non-nil")
	}
	if resp.CustomMetadata["cache_creation_input_tokens"] != int32(100) {
		t.Errorf("cache_creation: got %v, want 100", resp.CustomMetadata["cache_creation_input_tokens"])
	}
	if resp.CustomMetadata["cache_read_input_tokens"] != int32(200) {
		t.Errorf("cache_read: got %v, want 200", resp.CustomMetadata["cache_read_input_tokens"])
	}
}

// TestGenerateContent_Streaming verifies the streaming path through the
// public GenerateContent API.
func TestGenerateContent_Streaming(t *testing.T) {
	t.Parallel()
	stream := ""
	stream += "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_01\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-opus-4\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\n"
	stream += "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"
	stream += "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi\"}}\n\n"
	stream += "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" there\"}}\n\n"
	stream += "data: {\"type\":\"content_block_stop\",\"index\":0}\n\n"
	stream += "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":2}}\n\n"
	stream += "data: {\"type\":\"message_stop\"}\n\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, stream)
	}))
	t.Cleanup(srv.Close)

	m, err := anthropic.New(anthropic.Config{
		Model:   "claude-opus-4",
		APIKey:  "sk-ant-test",
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

	var partials []string
	var final *model.LLMResponse
	for _, resp := range resps {
		if resp.Partial {
			partials = append(partials, resp.Content.Parts[0].Text)
		}
		if resp.TurnComplete {
			final = resp
		}
	}
	if len(partials) != 2 {
		t.Fatalf("got %d partials, want 2: %v", len(partials), partials)
	}
	if partials[0] != "Hi" || partials[1] != " there" {
		t.Errorf("partials: got %v", partials)
	}
	if final == nil {
		t.Fatal("got no final TurnComplete response, want one")
	}
	if final.Content == nil || len(final.Content.Parts) != 1 {
		t.Fatalf("got %v final parts, want 1", final.Content)
	}
	if final.Content.Parts[0].Text != "Hi there" {
		t.Errorf("final text: got %q, want %q", final.Content.Parts[0].Text, "Hi there")
	}
}

// TestGenerateContent_AuthSchemeBearer verifies that when AuthScheme is
// "bearer", the Authorization: Bearer header is sent.
func TestGenerateContent_AuthSchemeBearer(t *testing.T) {
	t.Parallel()
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, cannedTextResponse)
	}))
	t.Cleanup(srv.Close)

	m, err := anthropic.New(anthropic.Config{
		Model:      "claude-opus-4",
		APIKey:     "gateway-token",
		BaseURL:    srv.URL,
		AuthScheme: "bearer",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "Hi"}}}},
	}

	_, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	if authHeader != "Bearer gateway-token" {
		t.Errorf("Authorization: got %q, want %q", authHeader, "Bearer gateway-token")
	}
}

// TestGenerateContent_CustomHeaders verifies that custom headers from
// Config.Headers are sent with the request.
func TestGenerateContent_CustomHeaders(t *testing.T) {
	t.Parallel()
	var tenantID, orgID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID = r.Header.Get("X-Tenant-ID")
		orgID = r.Header.Get("X-Org-ID")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, cannedTextResponse)
	}))
	t.Cleanup(srv.Close)

	m, err := anthropic.New(anthropic.Config{
		Model:   "claude-opus-4",
		APIKey:  "sk-ant-test",
		BaseURL: srv.URL,
		Headers: map[string]string{
			"X-Tenant-ID": "tenant-42",
			"X-Org-ID":    "org-99",
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "Hi"}}}},
	}

	_, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	if tenantID != "tenant-42" {
		t.Errorf("X-Tenant-ID: got %q, want %q", tenantID, "tenant-42")
	}
	if orgID != "org-99" {
		t.Errorf("X-Org-ID: got %q, want %q", orgID, "org-99")
	}
}

// TestGenerateContent_FileDataURLImage captures the request body to verify
// FileData with an HTTP URL maps to source.type "url".
func TestGenerateContent_FileDataURLImage(t *testing.T) {
	t.Parallel()
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, cannedTextResponse)
	}))
	t.Cleanup(srv.Close)

	m, err := anthropic.New(anthropic.Config{
		Model:   "claude-opus-4",
		APIKey:  "sk-ant-test",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{
				Role: "user",
				Parts: []*genai.Part{
					{Text: "What is this?"},
					{FileData: &genai.FileData{FileURI: "https://example.com/image.jpg", MIMEType: "image/jpeg"}},
				},
			},
		},
	}

	_, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	msgs, ok := capturedBody["messages"].([]any)
	if !ok || len(msgs) == 0 {
		t.Fatal("got non-array or empty messages, want messages array")
	}
	content, ok := msgs[0].(map[string]any)["content"].([]any)
	if !ok || len(content) != 2 {
		t.Fatalf("got %v content blocks, want 2", content)
	}
	imgBlock, ok := content[1].(map[string]any)
	if !ok {
		t.Fatal("got non-map image block, want map")
	}
	source, ok := imgBlock["source"].(map[string]any)
	if !ok {
		t.Fatal("got non-map source, want map")
	}
	if source["type"] != "url" {
		t.Errorf("source.type: got %v, want %q", source["type"], "url")
	}
	if source["url"] != "https://example.com/image.jpg" {
		t.Errorf("source.url: got %v, want %q", source["url"], "https://example.com/image.jpg")
	}
}

// TestGenerateContent_Non2xxErrorBodyClose verifies that the response body is
// closed even when io.ReadAll fails during non-2xx error handling. The server
// returns a 500 with a Content-Length that exceeds the actual body, causing
// io.ReadAll to fail with io.ErrUnexpectedEOF. A transport with
// MaxConnsPerHost: 1 ensures that if the body is not closed, a second request
// cannot obtain a connection and times out — proving the defer resp.Body.Close()
// releases the connection slot.
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

	m, err := anthropic.New(anthropic.Config{
		Model:      "claude-opus-4",
		APIKey:     "sk-ant-test",
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
		var httpErr *anthropic.HTTPError
		if errors.As(e, &httpErr) && httpErr.StatusCode == http.StatusInternalServerError {
			found500 = true
			break
		}
	}
	if !found500 {
		t.Errorf("second errors: got %v, want an HTTPError with status 500", errs2)
	}
}
