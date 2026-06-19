package anthropic

import (
	"encoding/json"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// TestContentsToMessages_RoleMapping verifies that genai role "model" maps to
// "assistant" and "user" stays "user".
func TestContentsToMessages_RoleMapping(t *testing.T) {
	contents := []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: "Hello"}}},
		{Role: "model", Parts: []*genai.Part{{Text: "Hi there"}}},
	}

	msgs, err := contentsToMessages(contents)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("msg[0].role: got %q, want %q", msgs[0].Role, "user")
	}
	if msgs[1].Role != "assistant" {
		t.Errorf("msg[1].role: got %q, want %q", msgs[1].Role, "assistant")
	}
}

// TestContentsToMessages_FunctionResponse verifies that a FunctionResponse
// with InlineData parts becomes a tool_result block in a user message.
func TestContentsToMessages_FunctionResponse(t *testing.T) {
	contents := []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				{
					FunctionResponse: &genai.FunctionResponse{
						ID:   "toolu_01A",
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

	msgs, err := contentsToMessages(contents)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	msg := msgs[0]
	if msg.Role != "user" {
		t.Errorf("role: got %q, want %q", msg.Role, "user")
	}

	blocks, ok := msg.Content.([]map[string]any)
	if !ok {
		t.Fatalf("content: expected []map[string]any, got %T", msg.Content)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0]["type"] != "tool_result" {
		t.Errorf("type: got %v, want %q", blocks[0]["type"], "tool_result")
	}
	if blocks[0]["tool_use_id"] != "toolu_01A" {
		t.Errorf("tool_use_id: got %v, want %q", blocks[0]["tool_use_id"], "toolu_01A")
	}
}

// TestContentsToMessages_FunctionCall verifies that a FunctionCall part
// becomes a tool_use block in an assistant message.
func TestContentsToMessages_FunctionCall(t *testing.T) {
	contents := []*genai.Content{
		{
			Role: "model",
			Parts: []*genai.Part{
				{
					FunctionCall: &genai.FunctionCall{
						ID:   "toolu_01B",
						Name: "get_time",
						Args: map[string]any{"timezone": "UTC"},
					},
				},
			},
		},
	}

	msgs, err := contentsToMessages(contents)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	msg := msgs[0]
	if msg.Role != "assistant" {
		t.Errorf("role: got %q, want %q", msg.Role, "assistant")
	}

	blocks, ok := msg.Content.([]map[string]any)
	if !ok {
		t.Fatalf("content: expected []map[string]any, got %T", msg.Content)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0]["type"] != "tool_use" {
		t.Errorf("type: got %v, want %q", blocks[0]["type"], "tool_use")
	}
	if blocks[0]["id"] != "toolu_01B" {
		t.Errorf("id: got %v, want %q", blocks[0]["id"], "toolu_01B")
	}
	if blocks[0]["name"] != "get_time" {
		t.Errorf("name: got %v, want %q", blocks[0]["name"], "get_time")
	}
}

// TestBuildMessageRequest_NoTemperature verifies that Temperature, TopP, and
// TopK are omitted from the Anthropic request even when set in the config.
func TestBuildMessageRequest_NoTemperature(t *testing.T) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "Hi"}}},
		},
		Config: &genai.GenerateContentConfig{
			Temperature:     ptr(float32(0.7)),
			TopP:            ptr(float32(0.9)),
			TopK:            ptr(float32(40)),
			MaxOutputTokens: 512,
		},
	}

	msgReq, err := buildMessageRequest(req, "claude-opus-4", false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify max_tokens is present.
	if msgReq.MaxTokens != 512 {
		t.Errorf("max_tokens: got %d, want 512", msgReq.MaxTokens)
	}

	// Marshal and verify temperature/top_p/top_k are absent.
	b, _ := json.Marshal(msgReq)
	var raw map[string]any
	_ = json.Unmarshal(b, &raw)
	if _, ok := raw["temperature"]; ok {
		t.Error("expected temperature to be omitted")
	}
	if _, ok := raw["top_p"]; ok {
		t.Error("expected top_p to be omitted")
	}
	if _, ok := raw["top_k"]; ok {
		t.Error("expected top_k to be omitted")
	}
}

// TestBuildMessageRequest_CacheControl verifies that explicit cache_control
// from PartMetadata is added to cacheable blocks and silently stripped from
// non-cacheable blocks (tool_use, tool_result).
func TestBuildMessageRequest_CacheControl(t *testing.T) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{
				Role: "user",
				Parts: []*genai.Part{
					{
						Text: "Context doc",
						PartMetadata: map[string]any{
							"cache_control": map[string]any{"type": "ephemeral"},
						},
					},
				},
			},
			{
				Role: "model",
				Parts: []*genai.Part{
					{
						FunctionCall: &genai.FunctionCall{
							ID:   "toolu_01C",
							Name: "calc",
							Args: map[string]any{},
						},
						PartMetadata: map[string]any{
							"cache_control": map[string]any{"type": "ephemeral"},
						},
					},
				},
			},
		},
	}

	msgReq, err := buildMessageRequest(req, "claude-opus-4", false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First message: text block should have cache_control.
	firstBlocks, ok := msgReq.Messages[0].Content.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any for first message content")
	}
	if len(firstBlocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(firstBlocks))
	}
	if _, hasCC := firstBlocks[0]["cache_control"]; !hasCC {
		t.Error("expected cache_control on text block")
	}

	// Second message: tool_use block should NOT have cache_control.
	secondBlocks, ok := msgReq.Messages[1].Content.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any for second message content")
	}
	if len(secondBlocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(secondBlocks))
	}
	if _, hasCC := secondBlocks[0]["cache_control"]; hasCC {
		t.Error("expected cache_control to be stripped from tool_use block")
	}
}

// TestTranslateFinishReason verifies all stop_reason mappings.
func TestTranslateFinishReason(t *testing.T) {
	tests := []struct {
		reason string
		want   genai.FinishReason
	}{
		{"end_turn", genai.FinishReasonStop},
		{"max_tokens", genai.FinishReasonMaxTokens},
		{"stop_sequence", genai.FinishReasonStop},
		{"tool_use", genai.FinishReasonStop},
		{"refusal", genai.FinishReasonSafety},
		{"", genai.FinishReasonUnspecified},
		{"unknown", genai.FinishReasonOther},
	}
	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			got := translateFinishReason(tt.reason)
			if got != tt.want {
				t.Errorf("translateFinishReason(%q): got %v, want %v", tt.reason, got, tt.want)
			}
		})
	}
}

// TestContentsToMessages_FileDataError verifies that a non-HTTP FileData URI
// returns an error.
func TestContentsToMessages_FileDataError(t *testing.T) {
	contents := []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				{FileData: &genai.FileData{FileURI: "gs://bucket/image.jpg"}},
			},
		},
	}
	_, err := contentsToMessages(contents)
	if err == nil {
		t.Fatal("expected error for non-HTTP FileData URI, got nil")
	}
}

// TestContentsToMessages_ThinkingBlock verifies that thought parts are emitted
// as thinking blocks.
func TestContentsToMessages_ThinkingBlock(t *testing.T) {
	contents := []*genai.Content{
		{
			Role: "assistant",
			Parts: []*genai.Part{
				{Text: "Let me think...", Thought: true, ThoughtSignature: []byte("sig123")},
			},
		},
	}
	msgs, err := contentsToMessages(contents)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	blocks, ok := msgs[0].Content.([]map[string]any)
	if !ok || len(blocks) != 1 {
		t.Fatalf("expected 1 thinking block, got %v", msgs[0].Content)
	}
	if blocks[0]["type"] != "thinking" {
		t.Errorf("type: got %v, want %q", blocks[0]["type"], "thinking")
	}
}

// TestBuildMessageRequest_ResponseJsonSchema verifies that ResponseJsonSchema
// maps to output_config.json_schema.
func TestBuildMessageRequest_ResponseJsonSchema(t *testing.T) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "Extract info"}}},
		},
		Config: &genai.GenerateContentConfig{
			ResponseJsonSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
			},
		},
	}
	msgReq, err := buildMessageRequest(req, "claude-opus-4", false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msgReq.OutputConfig == nil {
		t.Fatal("expected OutputConfig")
	}
	if msgReq.OutputConfig.Format != "json" {
		t.Errorf("format: got %q, want %q", msgReq.OutputConfig.Format, "json")
	}
	if msgReq.OutputConfig.JSONSchema == nil {
		t.Error("expected JSONSchema")
	}
}

// TestBuildMessageRequest_ThinkingConfig verifies that ThinkingConfig maps to
// the thinking request field.
func TestBuildMessageRequest_ThinkingConfig(t *testing.T) {
	budget := int32(1000)
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "Think deeply"}}},
		},
		Config: &genai.GenerateContentConfig{
			ThinkingConfig: &genai.ThinkingConfig{
				IncludeThoughts: true,
				ThinkingBudget:  &budget,
			},
		},
	}
	msgReq, err := buildMessageRequest(req, "claude-opus-4", false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msgReq.Thinking == nil {
		t.Fatal("expected Thinking config")
	}
	if msgReq.Thinking.Type != "enabled" {
		t.Errorf("type: got %q, want %q", msgReq.Thinking.Type, "enabled")
	}
	if msgReq.Thinking.BudgetTokens != 1000 {
		t.Errorf("budget_tokens: got %d, want 1000", msgReq.Thinking.BudgetTokens)
	}
}

// TestContentsToMessages_FunctionResponseWithResponseMap verifies that a
// FunctionResponse with Response map (not Parts) serialises to JSON string.
func TestContentsToMessages_FunctionResponseWithResponseMap(t *testing.T) {
	contents := []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				{
					FunctionResponse: &genai.FunctionResponse{
						ID:       "toolu_01E",
						Name:     "get_weather",
						Response: map[string]any{"temperature": 72},
					},
				},
			},
		},
	}
	msgs, err := contentsToMessages(contents)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	blocks, ok := msgs[0].Content.([]map[string]any)
	if !ok || len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %v", msgs[0].Content)
	}
	contentStr, ok := blocks[0]["content"].(string)
	if !ok {
		t.Fatalf("expected string content, got %T", blocks[0]["content"])
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(contentStr), &result); err != nil {
		t.Fatalf("content is not valid JSON: %v", err)
	}
	if result["temperature"] != float64(72) {
		t.Errorf("temperature: got %v, want 72", result["temperature"])
	}
}

// TestTranslateResponse_Mixed verifies response translation with text,
// tool_use, and thinking blocks.
func TestTranslateResponse_Mixed(t *testing.T) {
	resp := &messageResponse{
		ID:         "msg_01",
		Type:       "message",
		Role:       "assistant",
		StopReason: "tool_use",
		Content: []map[string]any{
			{"type": "text", "text": "Let me think..."},
			{"type": "thinking", "thinking": "I need to use a tool", "signature": "sig123"},
			{"type": "tool_use", "id": "toolu_01D", "name": "get_weather", "input": map[string]any{"location": "Paris"}},
		},
		Usage: &anthropicUsage{
			InputTokens:  20,
			OutputTokens: 15,
		},
	}

	llmResp := translateResponse(resp)
	if llmResp.FinishReason != genai.FinishReasonStop {
		t.Errorf("finish_reason: got %v, want %v", llmResp.FinishReason, genai.FinishReasonStop)
	}
	if llmResp.Content == nil {
		t.Fatal("expected non-nil Content")
	}
	if len(llmResp.Content.Parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(llmResp.Content.Parts))
	}

	// Text part.
	if llmResp.Content.Parts[0].Text != "Let me think..." {
		t.Errorf("text: got %q, want %q", llmResp.Content.Parts[0].Text, "Let me think...")
	}

	// Thinking part.
	if !llmResp.Content.Parts[1].Thought {
		t.Error("expected Thought=true on thinking part")
	}
	if llmResp.Content.Parts[1].Text != "I need to use a tool" {
		t.Errorf("thinking text: got %q, want %q", llmResp.Content.Parts[1].Text, "I need to use a tool")
	}
	if string(llmResp.Content.Parts[1].ThoughtSignature) != "sig123" {
		t.Errorf("signature: got %q, want %q", llmResp.Content.Parts[1].ThoughtSignature, "sig123")
	}

	// Tool use part.
	fc := llmResp.Content.Parts[2].FunctionCall
	if fc == nil {
		t.Fatal("expected FunctionCall on third part")
	}
	if fc.ID != "toolu_01D" {
		t.Errorf("ID: got %q, want %q", fc.ID, "toolu_01D")
	}
	if fc.Name != "get_weather" {
		t.Errorf("Name: got %q, want %q", fc.Name, "get_weather")
	}
	if fc.Args["location"] != "Paris" {
		t.Errorf("Args.location: got %v, want %q", fc.Args["location"], "Paris")
	}

	// Usage metadata.
	if llmResp.UsageMetadata == nil {
		t.Fatal("expected non-nil UsageMetadata")
	}
	if llmResp.UsageMetadata.PromptTokenCount != 20 {
		t.Errorf("prompt_tokens: got %d, want 20", llmResp.UsageMetadata.PromptTokenCount)
	}
	if llmResp.UsageMetadata.CandidatesTokenCount != 15 {
		t.Errorf("output_tokens: got %d, want 15", llmResp.UsageMetadata.CandidatesTokenCount)
	}
	if llmResp.UsageMetadata.TotalTokenCount != 35 {
		t.Errorf("total_tokens: got %d, want 35", llmResp.UsageMetadata.TotalTokenCount)
	}
}
