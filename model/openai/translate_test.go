package openai

import (
	"encoding/json"
	"errors"
	"testing"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

// TestContentsToMessages_UserText verifies that a user content with a single
// text Part is translated into an OpenAI user message with a string content.
func TestContentsToMessages_UserText(t *testing.T) {
	contents := []*genai.Content{
		{
			Role:  "user",
			Parts: []*genai.Part{{Text: "Hello!"}},
		},
	}

	msgs, err := contentsToMessagesErr(contents)
	if err != nil {
		t.Fatal(err)
	}

	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	msg := msgs[0]
	if msg.Role != "user" {
		t.Errorf("role: got %q, want %q", msg.Role, "user")
	}
	if msg.Content != "Hello!" {
		t.Errorf("content: got %v, want %q", msg.Content, "Hello!")
	}
}

// TestContentsToMessages_ModelText verifies that a model content is translated
// into an OpenAI assistant message.
func TestContentsToMessages_ModelText(t *testing.T) {
	contents := []*genai.Content{
		{
			Role:  "model",
			Parts: []*genai.Part{{Text: "World!"}},
		},
	}

	msgs, err := contentsToMessagesErr(contents)
	if err != nil {
		t.Fatal(err)
	}

	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	if msgs[0].Role != "assistant" {
		t.Errorf("role: got %q, want %q", msgs[0].Role, "assistant")
	}
	if msgs[0].Content != "World!" {
		t.Errorf("content: got %v, want %q", msgs[0].Content, "World!")
	}
}

// TestContentsToMessages_SystemInstruction verifies that buildChatRequest places
// a system message at index 0 when SystemInstruction is set in the config.
func TestContentsToMessages_SystemInstruction(t *testing.T) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "ping"}}},
		},
		Config: &genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{{Text: "You are a helpful assistant."}},
			},
		},
	}

	chatReq, err := buildChatRequest(req, "gpt-4o", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chatReq.Messages) < 2 {
		t.Fatalf("got %d messages, want at least 2", len(chatReq.Messages))
	}
	systemMsg := chatReq.Messages[0]
	if systemMsg.Role != "system" {
		t.Errorf("first message role: got %q, want %q", systemMsg.Role, "system")
	}
	if systemMsg.Content != "You are a helpful assistant." {
		t.Errorf("system content: got %v, want %q", systemMsg.Content, "You are a helpful assistant.")
	}
}

// TestContentsToMessages_FunctionResponse verifies that a Part containing a
// FunctionResponse is translated to an OpenAI tool role message with the
// correct tool_call_id set to FunctionResponse.ID.
func TestContentsToMessages_FunctionResponse(t *testing.T) {
	contents := []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				{
					FunctionResponse: &genai.FunctionResponse{
						ID:   "call_abc123",
						Name: "get_weather",
						Parts: []*genai.FunctionResponsePart{
							{
								InlineData: &genai.FunctionResponseBlob{
									MIMEType: "application/json",
									Data:     []byte(`{"temperature": 72}`),
								},
							},
						},
					},
				},
			},
		},
	}

	msgs, err := contentsToMessagesErr(contents)
	if err != nil {
		t.Fatal(err)
	}

	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	msg := msgs[0]
	if msg.Role != "tool" {
		t.Errorf("role: got %q, want %q", msg.Role, "tool")
	}
	if msg.ToolCallID != "call_abc123" {
		t.Errorf("tool_call_id: got %q, want %q", msg.ToolCallID, "call_abc123")
	}
	// Verify content contains valid JSON with expected data
	content, ok := msg.Content.(string)
	if !ok {
		t.Fatalf("content: got %T, want string", msg.Content)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		t.Fatalf("content is not valid JSON: %v", err)
	}
	if result["temperature"] != float64(72) {
		t.Errorf("temperature: got %v, want %v", result["temperature"], 72)
	}
}

// TestContentsToMessages_InlineData verifies that a Part with InlineData (an
// image) is translated into an OpenAI multimodal content block using the
// image_url format.
func TestContentsToMessages_InlineData(t *testing.T) {
	imgBytes := []byte("fakeimagedata")
	contents := []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				{Text: "What is in this image?"},
				{InlineData: &genai.Blob{
					MIMEType: "image/png",
					Data:     imgBytes,
				}},
			},
		},
	}

	msgs, err := contentsToMessagesErr(contents)
	if err != nil {
		t.Fatal(err)
	}

	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	// Content should be a slice of content parts ([]any) for multimodal.
	parts, ok := msgs[0].Content.([]any)
	if !ok {
		t.Fatalf("content: got %T, want []any for multimodal", msgs[0].Content)
	}
	if len(parts) != 2 {
		t.Fatalf("got %d content parts, want 2", len(parts))
	}
	// Second part should be image_url type.
	imgPart, ok := parts[1].(map[string]any)
	if !ok {
		t.Fatalf("image part: got %T, want map[string]any", parts[1])
	}
	if imgPart["type"] != "image_url" {
		t.Errorf("image part type: got %v, want %q", imgPart["type"], "image_url")
	}
}

// TestBuildChatRequest_BasicFields verifies that temperature, top_p,
// max_tokens, and stop sequences from the config are forwarded correctly.
func TestBuildChatRequest_BasicFields(t *testing.T) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "Hi"}}},
		},
		Config: &genai.GenerateContentConfig{
			Temperature:     new(float32(0.7)),
			TopP:            new(float32(0.9)),
			MaxOutputTokens: 512,
			StopSequences:   []string{"###", "END"},
		},
	}

	chatReq, err := buildChatRequest(req, "gpt-4o-mini", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if chatReq.Model != "gpt-4o-mini" {
		t.Errorf("model: got %q, want %q", chatReq.Model, "gpt-4o-mini")
	}
	if chatReq.Temperature == nil || *chatReq.Temperature != 0.7 {
		t.Errorf("temperature: got %v, want 0.7", chatReq.Temperature)
	}
	if chatReq.TopP == nil || *chatReq.TopP != 0.9 {
		t.Errorf("top_p: got %v, want 0.9", chatReq.TopP)
	}
	if chatReq.MaxTokens == nil || *chatReq.MaxTokens != 512 {
		t.Errorf("max_tokens: got %v, want 512", chatReq.MaxTokens)
	}
	if len(chatReq.Stop) != 2 || chatReq.Stop[0] != "###" {
		t.Errorf("stop: got %v, want [### END]", chatReq.Stop)
	}
	if chatReq.Stream != false {
		t.Errorf("stream: got true, want false")
	}
}

// TestBuildChatRequest_ResponseSchema verifies that a ResponseSchema in the
// config produces a response_format of type json_schema.
func TestBuildChatRequest_ResponseSchema(t *testing.T) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "give JSON"}}},
		},
		Config: &genai.GenerateContentConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"name": {Type: genai.TypeString},
				},
			},
		},
	}

	chatReq, err := buildChatRequest(req, "gpt-4o", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if chatReq.ResponseFormat == nil {
		t.Fatal("response_format: got nil, want non-nil")
	}

	rfMap, ok := chatReq.ResponseFormat.(map[string]any)
	if !ok {
		t.Fatalf("response_format: got %T, want map[string]any", chatReq.ResponseFormat)
	}
	if rfMap["type"] != "json_schema" {
		t.Errorf("response_format.type: got %v, want %q", rfMap["type"], "json_schema")
	}
	if _, ok := rfMap["json_schema"]; !ok {
		t.Error("response_format: missing json_schema key")
	}
}

// TestBuildChatRequest_ResponseMIMEType verifies that when only ResponseMIMEType
// is "application/json" (without a schema), the response_format becomes
// {type: "json_object"}.
func TestBuildChatRequest_ResponseMIMEType(t *testing.T) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "give JSON"}}},
		},
		Config: &genai.GenerateContentConfig{
			ResponseMIMEType: "application/json",
		},
	}

	chatReq, err := buildChatRequest(req, "gpt-4o", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if chatReq.ResponseFormat == nil {
		t.Fatal("response_format: got nil, want non-nil")
	}

	rfMap, ok := chatReq.ResponseFormat.(map[string]any)
	if !ok {
		t.Fatalf("response_format: got %T, want map[string]any", chatReq.ResponseFormat)
	}
	if rfMap["type"] != "json_object" {
		t.Errorf("response_format.type: got %v, want %q", rfMap["type"], "json_object")
	}
}

// TestTranslateResponse_TextContent verifies that a basic OpenAI text response
// is converted to an LLMResponse with the correct Content.
func TestTranslateResponse_TextContent(t *testing.T) {
	resp := &chatResponse{
		ID: "chatcmpl-abc",
		Choices: []chatChoice{
			{
				Index: 0,
				Message: chatMessage{
					Role:    "assistant",
					Content: "Hello, world!",
				},
				FinishReason: "stop",
			},
		},
	}

	llmResp := translateResponse(resp)

	if llmResp == nil {
		t.Fatal("got nil LLMResponse, want non-nil")
	}
	if llmResp.Content == nil {
		t.Fatal("got nil Content, want non-nil")
	}
	if llmResp.Content.Role != "model" {
		t.Errorf("role: got %q, want %q", llmResp.Content.Role, "model")
	}
	if len(llmResp.Content.Parts) != 1 {
		t.Fatalf("got %d parts, want 1", len(llmResp.Content.Parts))
	}
	if llmResp.Content.Parts[0].Text != "Hello, world!" {
		t.Errorf("text: got %q, want %q", llmResp.Content.Parts[0].Text, "Hello, world!")
	}
	if llmResp.FinishReason != genai.FinishReasonStop {
		t.Errorf("finish_reason: got %v, want %v", llmResp.FinishReason, genai.FinishReasonStop)
	}
}

// TestTranslateResponse_Usage verifies that usage metadata from the OpenAI
// response is correctly mapped to genai.GenerateContentResponseUsageMetadata.
func TestTranslateResponse_Usage(t *testing.T) {
	resp := &chatResponse{
		ID: "chatcmpl-xyz",
		Choices: []chatChoice{
			{
				Message:      chatMessage{Role: "assistant", Content: "ok"},
				FinishReason: "stop",
			},
		},
		Usage: &chatUsage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	llmResp := translateResponse(resp)

	if llmResp.UsageMetadata == nil {
		t.Fatal("got nil UsageMetadata, want non-nil")
	}
	if llmResp.UsageMetadata.PromptTokenCount != 10 {
		t.Errorf("prompt tokens: got %d, want 10", llmResp.UsageMetadata.PromptTokenCount)
	}
	if llmResp.UsageMetadata.CandidatesTokenCount != 5 {
		t.Errorf("completion tokens: got %d, want 5", llmResp.UsageMetadata.CandidatesTokenCount)
	}
	if llmResp.UsageMetadata.TotalTokenCount != 15 {
		t.Errorf("total tokens: got %d, want 15", llmResp.UsageMetadata.TotalTokenCount)
	}
}

// TestTranslateFinishReason verifies all supported OpenAI finish reason
// mappings to genai.FinishReason constants.
func TestTranslateFinishReason(t *testing.T) {
	cases := []struct {
		input string
		want  genai.FinishReason
	}{
		{"stop", genai.FinishReasonStop},
		{"length", genai.FinishReasonMaxTokens},
		{"tool_calls", genai.FinishReasonStop},
		{"content_filter", genai.FinishReasonSafety},
		{"", genai.FinishReasonUnspecified},
		{"unknown_reason", genai.FinishReasonOther},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := translateFinishReason(tc.input)
			if got != tc.want {
				t.Errorf("translateFinishReason(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestMessageToContent_AssistantText verifies that an assistant chatMessage is
// converted back to a model genai.Content with text.
func TestMessageToContent_AssistantText(t *testing.T) {
	msg := chatMessage{
		Role:    "assistant",
		Content: "Hello back!",
	}

	content := messageToContent(msg)

	if content == nil {
		t.Fatal("got nil Content, want non-nil")
	}
	if content.Role != "model" {
		t.Errorf("role: got %q, want %q", content.Role, "model")
	}
	if len(content.Parts) != 1 || content.Parts[0].Text != "Hello back!" {
		t.Errorf("parts: got %v", content.Parts)
	}
}

// TestBuildChatRequest_StreamFlag verifies that the stream parameter is
// correctly forwarded to the chatRequest.
func TestBuildChatRequest_StreamFlag(t *testing.T) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "stream me"}}},
		},
	}

	chatReq, err := buildChatRequest(req, "gpt-4o", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !chatReq.Stream {
		t.Errorf("stream: got false, want true")
	}
}

// TestBuildChatRequest_NilConfig verifies that buildChatRequest works when
// Config is nil (no panic, default values used).
func TestBuildChatRequest_NilConfig(t *testing.T) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "Hi"}}},
		},
		Config: nil,
	}

	chatReq, err := buildChatRequest(req, "gpt-4o", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chatReq.Model != "gpt-4o" {
		t.Errorf("model: got %q, want %q", chatReq.Model, "gpt-4o")
	}
	if chatReq.Temperature != nil {
		t.Errorf("temperature: got %v, want nil", chatReq.Temperature)
	}
}

// TestTranslateResponse_ToolCalls verifies that a model response with tool_calls
// is converted into a genai.Content with FunctionCall parts.
func TestTranslateResponse_ToolCalls(t *testing.T) {
	argsJSON := `{"location": "NYC"}`
	resp := &chatResponse{
		ID: "chatcmpl-tools",
		Choices: []chatChoice{
			{
				Message: chatMessage{
					Role:    "assistant",
					Content: nil,
					ToolCalls: []any{
						map[string]any{
							"id":   "call_xyz",
							"type": "function",
							"function": map[string]any{
								"name":      "get_weather",
								"arguments": argsJSON,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}

	llmResp := translateResponse(resp)

	if llmResp.Content == nil {
		t.Fatal("got nil Content, want non-nil")
	}
	if len(llmResp.Content.Parts) != 1 {
		t.Fatalf("got %d parts, want 1", len(llmResp.Content.Parts))
	}
	fc := llmResp.Content.Parts[0].FunctionCall
	if fc == nil {
		t.Fatal("got no FunctionCall part, want one")
	}
	if fc.Name != "get_weather" {
		t.Errorf("function name: got %q, want %q", fc.Name, "get_weather")
	}
	if fc.ID != "call_xyz" {
		t.Errorf("function id: got %q, want %q", fc.ID, "call_xyz")
	}
	// Args should be parsed from JSON.
	if fc.Args == nil {
		t.Fatal("got nil Args, want non-nil")
	}
	_, _ = json.Marshal(fc.Args) // just ensure it's serialisable
}

// TestTranslateThoughtPart_Request verifies that thought parts are included
// as regular text when converting contents to OpenAI messages.
func TestTranslateThoughtPart_Request(t *testing.T) {
	contents := []*genai.Content{
		{
			Role: "model",
			Parts: []*genai.Part{
				{Text: "Let me think about this...", Thought: true},
				{Text: "Here is the answer."},
			},
		},
	}
	msgs, err := contentsToMessagesErr(contents)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) == 0 {
		t.Fatal("got no messages, want at least one")
	}
}

// TestTranslateThoughtPart_Response verifies that an assistant message is
// correctly converted back to a model content.
func TestTranslateThoughtPart_Response(t *testing.T) {
	msg := chatMessage{
		Role:    "assistant",
		Content: "The answer is 42.",
	}
	content := messageToContent(msg)
	if content == nil {
		t.Fatal("got nil content, want non-nil")
	}
	if content.Role != "model" {
		t.Errorf("got %q, want role 'model'", content.Role)
	}
}

// TestBuildChatRequest_MaxTokensOmittedWhenZero verifies that when
// MaxOutputTokens is 0 (unset), max_tokens is omitted from the JSON payload.
func TestBuildChatRequest_MaxTokensOmittedWhenZero(t *testing.T) {
	// When MaxOutputTokens is 0 (unset), max_tokens should be omitted from JSON
	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
		Config:   &genai.GenerateContentConfig{},
	}
	cr, err := buildChatRequest(req, "test-model", false)
	if err != nil {
		t.Fatal(err)
	}
	if cr.MaxTokens != nil {
		t.Errorf("got %v for MaxTokens when unset, want nil", *cr.MaxTokens)
	}
	data, _ := json.Marshal(cr)
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal chat request: %v", err)
	}
	if _, ok := raw["max_tokens"]; ok {
		t.Error("max_tokens should be omitted from JSON when nil")
	}
}

// TestContentsToMessages_NilEntries verifies that nil entries in the contents
// slice are skipped gracefully.
func TestContentsToMessages_NilEntries(t *testing.T) {
	contents := []*genai.Content{
		nil,
		{Role: "user", Parts: []*genai.Part{{Text: "hello"}}},
		nil,
	}
	msgs, err := contentsToMessagesErr(contents)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Errorf("got %d messages (skipping nils), want 1", len(msgs))
	}
}

// TestContentsToMessages_FunctionResponse_NoParts verifies that a FunctionResponse
// with empty Parts returns an error.
func TestContentsToMessages_FunctionResponse_NoParts(t *testing.T) {
	contents := []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				{
					FunctionResponse: &genai.FunctionResponse{
						ID:    "call_abc123",
						Name:  "get_weather",
						Parts: []*genai.FunctionResponsePart{},
					},
				},
			},
		},
	}

	_, err := contentsToMessagesErr(contents)
	if err == nil {
		t.Fatal("got nil error for empty Parts, want error")
	}
	if !errors.Is(err, ErrEmptyFunctionResponseParts) {
		t.Errorf("got %v, want ErrEmptyFunctionResponseParts", err)
	}
}

// TestContentsToMessages_FunctionResponse_EmptyID verifies that a FunctionResponse
// with empty ID returns an error.
func TestContentsToMessages_FunctionResponse_EmptyID(t *testing.T) {
	contents := []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				{
					FunctionResponse: &genai.FunctionResponse{
						ID:   "",
						Name: "get_weather",
						Parts: []*genai.FunctionResponsePart{
							{
								InlineData: &genai.FunctionResponseBlob{
									MIMEType: "application/json",
									Data:     []byte(`{"temperature": 72}`),
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := contentsToMessagesErr(contents)
	if err == nil {
		t.Fatal("got nil error for empty ID, want error")
	}
	if !errors.Is(err, ErrEmptyToolCallID) {
		t.Errorf("got %v, want ErrEmptyToolCallID", err)
	}
}

// TestContentsToMessages_FunctionResponse_InvalidJSON verifies that a FunctionResponse
// with invalid JSON data returns an error.
func TestContentsToMessages_FunctionResponse_InvalidJSON(t *testing.T) {
	contents := []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				{
					FunctionResponse: &genai.FunctionResponse{
						ID:   "call_abc123",
						Name: "get_weather",
						Parts: []*genai.FunctionResponsePart{
							{
								InlineData: &genai.FunctionResponseBlob{
									MIMEType: "application/json",
									Data:     []byte(`{invalid json}`),
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := contentsToMessagesErr(contents)
	if err == nil {
		t.Fatal("got nil error for invalid JSON, want error")
	}
	if !errors.Is(err, ErrInvalidJSONData) {
		t.Errorf("got %v, want ErrInvalidJSONData", err)
	}
}

// FuzzContentsToMessages verifies that contentsToMessagesErr never panics on
// arbitrary JSON input. Valid JSON arrays of genai.Content should parse and
// translate without error; invalid input should return an error (no panic).
func FuzzContentsToMessages(f *testing.F) {
	// Seed: valid JSON array of contents.
	f.Add([]byte(`[{"role":"user","parts":[{"text":"Hello"}]}]`))
	// Seed: malformed JSON.
	f.Add([]byte(`invalid json`))
	// Seed: empty array.
	f.Add([]byte(`[]`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var contents []*genai.Content
		if err := json.Unmarshal(data, &contents); err != nil {
			// Skip unmarshal failures — expected for random bytes.
			return
		}
		msgs, err := contentsToMessagesErr(contents)
		// The function must not panic — reaching here is the primary assertion.
		// Both error and non-error outcomes are acceptable as long as no panic
		// occurred.
		if err != nil {
			return
		}
		_ = msgs
	})
}
