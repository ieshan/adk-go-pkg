package anthropic

import (
	"encoding/json"
	"testing"

	"google.golang.org/adk/v2/model"
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
		t.Fatalf("got %d messages, want 2", len(msgs))
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
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	msg := msgs[0]
	if msg.Role != "user" {
		t.Errorf("role: got %q, want %q", msg.Role, "user")
	}

	blocks, ok := msg.Content.([]map[string]any)
	if !ok {
		t.Fatalf("content: got %T, want []map[string]any", msg.Content)
	}
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
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
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	msg := msgs[0]
	if msg.Role != "assistant" {
		t.Errorf("role: got %q, want %q", msg.Role, "assistant")
	}

	blocks, ok := msg.Content.([]map[string]any)
	if !ok {
		t.Fatalf("content: got %T, want []map[string]any", msg.Content)
	}
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
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
	b, err := json.Marshal(msgReq)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := raw["temperature"]; ok {
		t.Errorf("got temperature key, want it omitted")
	}
	if _, ok := raw["top_p"]; ok {
		t.Errorf("got top_p key, want it omitted")
	}
	if _, ok := raw["top_k"]; ok {
		t.Errorf("got top_k key, want it omitted")
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
		t.Fatalf("got wrong type for first message content, want []map[string]any")
	}
	if len(firstBlocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(firstBlocks))
	}
	if _, hasCC := firstBlocks[0]["cache_control"]; !hasCC {
		t.Errorf("got no cache_control on text block, want it present")
	}

	// Second message: tool_use block should NOT have cache_control.
	secondBlocks, ok := msgReq.Messages[1].Content.([]map[string]any)
	if !ok {
		t.Fatalf("got wrong type for second message content, want []map[string]any")
	}
	if len(secondBlocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(secondBlocks))
	}
	if _, hasCC := secondBlocks[0]["cache_control"]; hasCC {
		t.Errorf("got cache_control on tool_use block, want it stripped")
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
		t.Fatal("got nil error for non-HTTP FileData URI, want error")
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
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	blocks, ok := msgs[0].Content.([]map[string]any)
	if !ok || len(blocks) != 1 {
		t.Fatalf("got %v thinking blocks, want 1", msgs[0].Content)
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
		t.Fatal("got nil OutputConfig, want non-nil")
	}
	if msgReq.OutputConfig.Format != "json" {
		t.Errorf("format: got %q, want %q", msgReq.OutputConfig.Format, "json")
	}
	if msgReq.OutputConfig.JSONSchema == nil {
		t.Errorf("got nil JSONSchema, want non-nil")
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
		t.Fatal("got nil Thinking config, want non-nil")
	}
	if msgReq.Thinking.Type != "enabled" {
		t.Errorf("type: got %q, want %q", msgReq.Thinking.Type, "enabled")
	}
	if msgReq.Thinking.BudgetTokens != 1000 {
		t.Errorf("budget_tokens: got %d, want 1000", msgReq.Thinking.BudgetTokens)
	}
}

// TestBuildMessageRequest_AllowedFunctionNames_FiltersTools verifies that when
// multiple AllowedFunctionNames are specified, the tools list is filtered to
// only include those tools (since Anthropic's tool_choice cannot name multiple
// tools).
func TestBuildMessageRequest_AllowedFunctionNames_FiltersTools(t *testing.T) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "Hi"}}},
		},
		Config: &genai.GenerateContentConfig{
			Tools: []*genai.Tool{
				{FunctionDeclarations: []*genai.FunctionDeclaration{
					{Name: "get_weather"},
					{Name: "get_time"},
					{Name: "send_email"},
				}},
			},
			ToolConfig: &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode:                 genai.FunctionCallingConfigModeAuto,
					AllowedFunctionNames: []string{"get_weather", "get_time"},
				},
			},
		},
	}
	msgReq, err := buildMessageRequest(req, "claude-opus-4", false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msgReq.Tools) != 2 {
		t.Fatalf("len(Tools) = %d, want 2 (filtered)", len(msgReq.Tools))
	}
	names := map[string]bool{}
	for _, tool := range msgReq.Tools {
		if n, ok := tool["name"].(string); ok {
			names[n] = true
		}
	}
	if !names["get_weather"] || !names["get_time"] {
		t.Errorf("filtered tools = %v, want get_weather and get_time", names)
	}
	if names["send_email"] {
		t.Errorf("send_email should have been filtered out")
	}
}

// TestBuildMessageRequest_AllowedFunctionNames_SingleDoesNotFilter verifies
// that a single AllowedFunctionNames does not filter the tools list (it uses
// tool_choice with type=tool instead).
func TestBuildMessageRequest_AllowedFunctionNames_SingleDoesNotFilter(t *testing.T) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "Hi"}}},
		},
		Config: &genai.GenerateContentConfig{
			Tools: []*genai.Tool{
				{FunctionDeclarations: []*genai.FunctionDeclaration{
					{Name: "get_weather"},
					{Name: "get_time"},
				}},
			},
			ToolConfig: &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode:                 genai.FunctionCallingConfigModeAuto,
					AllowedFunctionNames: []string{"get_weather"},
				},
			},
		},
	}
	msgReq, err := buildMessageRequest(req, "claude-opus-4", false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msgReq.Tools) != 2 {
		t.Fatalf("len(Tools) = %d, want 2 (not filtered for single name)", len(msgReq.Tools))
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
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	blocks, ok := msgs[0].Content.([]map[string]any)
	if !ok || len(blocks) != 1 {
		t.Fatalf("got %v blocks, want 1", msgs[0].Content)
	}
	contentStr, ok := blocks[0]["content"].(string)
	if !ok {
		t.Fatalf("got %T, want string content", blocks[0]["content"])
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
		t.Fatal("got nil Content, want non-nil")
	}
	if len(llmResp.Content.Parts) != 3 {
		t.Fatalf("got %d parts, want 3", len(llmResp.Content.Parts))
	}

	// Text part.
	if llmResp.Content.Parts[0].Text != "Let me think..." {
		t.Errorf("text: got %q, want %q", llmResp.Content.Parts[0].Text, "Let me think...")
	}

	// Thinking part.
	if !llmResp.Content.Parts[1].Thought {
		t.Errorf("got Thought=false on thinking part, want true")
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
		t.Fatal("got no FunctionCall on third part, want one")
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
		t.Fatal("got nil UsageMetadata, want non-nil")
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

// FuzzContentsToMessages verifies that contentsToMessages never panics on
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
		msgs, err := contentsToMessages(contents)
		// The function must not panic — reaching here is the primary assertion.
		// Both error and non-error outcomes are acceptable as long as no panic
		// occurred.
		if err != nil {
			return
		}
		_ = msgs
	})
}
