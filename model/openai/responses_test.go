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

// cannedResponsesResponse is a reusable canned non-streaming Responses API JSON.
const cannedResponsesResponse = `{
  "id": "resp_123",
  "model": "gpt-4o-2024-08-06",
  "output": [
    {"type": "message", "role": "assistant", "content": [{"type": "output_text", "text": "Hello!"}]}
  ],
  "usage": {"input_tokens": 10, "output_tokens": 5, "total_tokens": 15}
}`

// --- buildResponsesRequest tests ---

func TestBuildResponsesRequest_Text(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "Hello"}}},
		},
	}
	rr, err := buildResponsesRequest(req, "gpt-4o", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rr.Model != "gpt-4o" {
		t.Errorf("model: got %q, want %q", rr.Model, "gpt-4o")
	}
	if len(rr.Input) != 1 {
		t.Fatalf("got %d input items, want 1", len(rr.Input))
	}
	item := rr.Input[0]
	if item.Type != "message" {
		t.Errorf("type: got %q, want %q", item.Type, "message")
	}
	if item.Role != "user" {
		t.Errorf("role: got %q, want %q", item.Role, "user")
	}
	if len(item.Content) != 1 {
		t.Fatalf("got %d content parts, want 1", len(item.Content))
	}
	if item.Content[0].Type != "input_text" {
		t.Errorf("content type: got %q, want %q", item.Content[0].Type, "input_text")
	}
	if item.Content[0].Text != "Hello" {
		t.Errorf("text: got %q, want %q", item.Content[0].Text, "Hello")
	}
}

func TestBuildResponsesRequest_AssistantText(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "model", Parts: []*genai.Part{{Text: "World!"}}},
		},
	}
	rr, err := buildResponsesRequest(req, "gpt-4o", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rr.Input) != 1 {
		t.Fatalf("got %d input items, want 1", len(rr.Input))
	}
	item := rr.Input[0]
	if item.Role != "assistant" {
		t.Errorf("role: got %q, want %q", item.Role, "assistant")
	}
	if len(item.Content) != 1 || item.Content[0].Type != "output_text" {
		t.Errorf("content type: got %v, want output_text", item.Content)
	}
}

func TestBuildResponsesRequest_SystemInstruction(t *testing.T) {
	t.Parallel()
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
	rr, err := buildResponsesRequest(req, "gpt-4o", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rr.Instructions != "You are a helpful assistant." {
		t.Errorf("instructions: got %q, want %q", rr.Instructions, "You are a helpful assistant.")
	}
}

func TestBuildResponsesRequest_FunctionCall(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "model", Parts: []*genai.Part{
				{FunctionCall: &genai.FunctionCall{
					ID:   "call_abc",
					Name: "get_weather",
					Args: map[string]any{"location": "NYC"},
				}},
			}},
		},
	}
	rr, err := buildResponsesRequest(req, "gpt-4o", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rr.Input) != 1 {
		t.Fatalf("got %d input items, want 1", len(rr.Input))
	}
	item := rr.Input[0]
	if item.Type != "function_call" {
		t.Errorf("type: got %q, want %q", item.Type, "function_call")
	}
	if item.CallID != "call_abc" {
		t.Errorf("call_id: got %q, want %q", item.CallID, "call_abc")
	}
	if item.Name != "get_weather" {
		t.Errorf("name: got %q, want %q", item.Name, "get_weather")
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(item.Arguments), &args); err != nil {
		t.Fatalf("arguments not valid JSON: %v", err)
	}
	if args["location"] != "NYC" {
		t.Errorf("arguments[location]: got %v, want %q", args["location"], "NYC")
	}
}

func TestBuildResponsesRequest_FunctionCallMissingID_GeneratesID(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "model", Parts: []*genai.Part{
				{FunctionCall: &genai.FunctionCall{Name: "search", Args: map[string]any{}}},
			}},
		},
	}
	rr, err := buildResponsesRequest(req, "gpt-4o", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rr.Input) != 1 {
		t.Fatalf("got %d input items, want 1", len(rr.Input))
	}
	callID := rr.Input[0].CallID
	if !strings.HasPrefix(callID, "adk-openai-call-") {
		t.Errorf("call_id: got %q, want prefix %q", callID, "adk-openai-call-")
	}
}

func TestBuildResponsesRequest_FunctionCallMissingName(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "model", Parts: []*genai.Part{
				{FunctionCall: &genai.FunctionCall{ID: "call_1", Name: ""}},
			}},
		},
	}
	_, err := buildResponsesRequest(req, "gpt-4o", false)
	if !errors.Is(err, ErrFunctionCallMissingName) {
		t.Errorf("got %v, want ErrFunctionCallMissingName", err)
	}
}

func TestBuildResponsesRequest_FunctionResponse(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "model", Parts: []*genai.Part{
				{FunctionCall: &genai.FunctionCall{ID: "call_1", Name: "get_weather", Args: map[string]any{}}},
			}},
			{Role: "user", Parts: []*genai.Part{
				{FunctionResponse: &genai.FunctionResponse{
					ID:       "call_1",
					Name:     "get_weather",
					Response: map[string]any{"temperature": 72},
				}},
			}},
		},
	}
	rr, err := buildResponsesRequest(req, "gpt-4o", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expect 2 items: function_call + function_call_output.
	if len(rr.Input) != 2 {
		t.Fatalf("got %d input items, want 2", len(rr.Input))
	}
	outItem := rr.Input[1]
	if outItem.Type != "function_call_output" {
		t.Errorf("type: got %q, want %q", outItem.Type, "function_call_output")
	}
	if outItem.CallID != "call_1" {
		t.Errorf("call_id: got %q, want %q", outItem.CallID, "call_1")
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(outItem.Output), &result); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if result["temperature"] != float64(72) {
		t.Errorf("output[temperature]: got %v, want 72", result["temperature"])
	}
}

func TestBuildResponsesRequest_FunctionResponseMissingID_MatchedToPending(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "model", Parts: []*genai.Part{
				{FunctionCall: &genai.FunctionCall{ID: "call_1", Name: "fn", Args: map[string]any{}}},
			}},
			{Role: "user", Parts: []*genai.Part{
				{FunctionResponse: &genai.FunctionResponse{
					ID:       "",
					Name:     "fn",
					Response: map[string]any{"ok": true},
				}},
			}},
		},
	}
	rr, err := buildResponsesRequest(req, "gpt-4o", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rr.Input) != 2 {
		t.Fatalf("got %d input items, want 2", len(rr.Input))
	}
	if rr.Input[1].CallID != "call_1" {
		t.Errorf("call_id: got %q, want %q (matched to pending)", rr.Input[1].CallID, "call_1")
	}
}

func TestBuildResponsesRequest_FunctionResponseMissingCallID_NoPending(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{
				{FunctionResponse: &genai.FunctionResponse{
					ID:       "",
					Name:     "fn",
					Response: map[string]any{"ok": true},
				}},
			}},
		},
	}
	_, err := buildResponsesRequest(req, "gpt-4o", false)
	if !errors.Is(err, ErrFunctionResponseMissingCallID) {
		t.Errorf("got %v, want ErrFunctionResponseMissingCallID", err)
	}
}

func TestBuildResponsesRequest_FunctionResponseUnknownCallID(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "model", Parts: []*genai.Part{
				{FunctionCall: &genai.FunctionCall{ID: "call_1", Name: "fn", Args: map[string]any{}}},
			}},
			{Role: "user", Parts: []*genai.Part{
				{FunctionResponse: &genai.FunctionResponse{
					ID:       "call_unknown",
					Name:     "fn",
					Response: map[string]any{"ok": true},
				}},
			}},
		},
	}
	_, err := buildResponsesRequest(req, "gpt-4o", false)
	if !errors.Is(err, ErrFunctionResponseUnknownCallID) {
		t.Errorf("got %v, want ErrFunctionResponseUnknownCallID", err)
	}
}

func TestBuildResponsesRequest_ResponseSchema(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "give JSON"}}},
		},
		Config: &genai.GenerateContentConfig{
			ResponseSchema: &genai.Schema{
				Type:  genai.TypeObject,
				Title: "my_schema",
				Properties: map[string]*genai.Schema{
					"name": {Type: genai.TypeString},
				},
			},
		},
	}
	rr, err := buildResponsesRequest(req, "gpt-4o", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rr.Text == nil {
		t.Fatal("text: got nil, want non-nil")
	}
	fmtMap, ok := rr.Text.Format.(map[string]any)
	if !ok {
		t.Fatalf("format: got %T, want map[string]any", rr.Text.Format)
	}
	if fmtMap["type"] != "json_schema" {
		t.Errorf("format.type: got %v, want %q", fmtMap["type"], "json_schema")
	}
	if fmtMap["name"] != "my_schema" {
		t.Errorf("format.name: got %v, want %q", fmtMap["name"], "my_schema")
	}
	if fmtMap["strict"] != true {
		t.Errorf("format.strict: got %v, want true", fmtMap["strict"])
	}
	schema, ok := fmtMap["schema"].(map[string]any)
	if !ok {
		t.Fatalf("format.schema: got %T, want map[string]any", fmtMap["schema"])
	}
	if schema["additionalProperties"] != false {
		t.Errorf("schema.additionalProperties: got %v, want false", schema["additionalProperties"])
	}
}

func TestBuildResponsesRequest_ResponseJsonSchema(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "give JSON"}}},
		},
		Config: &genai.GenerateContentConfig{
			ResponseJsonSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"answer": map[string]any{"type": "string"},
				},
			},
		},
	}
	rr, err := buildResponsesRequest(req, "gpt-4o", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rr.Text == nil {
		t.Fatal("text: got nil, want non-nil")
	}
	fmtMap, ok := rr.Text.Format.(map[string]any)
	if !ok {
		t.Fatalf("format: got %T, want map[string]any", rr.Text.Format)
	}
	if fmtMap["type"] != "json_schema" {
		t.Errorf("format.type: got %v, want %q", fmtMap["type"], "json_schema")
	}
	schema, ok := fmtMap["schema"].(map[string]any)
	if !ok {
		t.Fatalf("format.schema: got %T, want map[string]any", fmtMap["schema"])
	}
	if schema["additionalProperties"] != false {
		t.Errorf("schema.additionalProperties: got %v, want false", schema["additionalProperties"])
	}
}

func TestBuildResponsesRequest_ResponseMIMETypeJSONObject(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "give JSON"}}},
		},
		Config: &genai.GenerateContentConfig{
			ResponseMIMEType: "application/json",
		},
	}
	rr, err := buildResponsesRequest(req, "gpt-4o", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rr.Text == nil {
		t.Fatal("text: got nil, want non-nil")
	}
	fmtMap, ok := rr.Text.Format.(map[string]any)
	if !ok {
		t.Fatalf("format: got %T, want map[string]any", rr.Text.Format)
	}
	if fmtMap["type"] != "json_object" {
		t.Errorf("format.type: got %v, want %q", fmtMap["type"], "json_object")
	}
}

func TestBuildResponsesRequest_ResponseMIMETypeUnsupported(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "hi"}}},
		},
		Config: &genai.GenerateContentConfig{
			ResponseMIMEType: "image/png",
		},
	}
	_, err := buildResponsesRequest(req, "gpt-4o", false)
	if !errors.Is(err, ErrUnsupportedMIMEType) {
		t.Errorf("got %v, want ErrUnsupportedMIMEType", err)
	}
}

func TestBuildResponsesRequest_ToolDeclaration(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "weather?"}}},
		},
		Config: &genai.GenerateContentConfig{
			Tools: []*genai.Tool{{
				FunctionDeclarations: []*genai.FunctionDeclaration{{
					Name:        "get_weather",
					Description: "Returns the weather.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"city": {Type: genai.TypeString},
						},
					},
				}},
			}},
		},
	}
	rr, err := buildResponsesRequest(req, "gpt-4o", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rr.Tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(rr.Tools))
	}
	tool := rr.Tools[0]
	if tool["type"] != "function" {
		t.Errorf("type: got %v, want %q", tool["type"], "function")
	}
	if tool["name"] != "get_weather" {
		t.Errorf("name: got %v, want %q", tool["name"], "get_weather")
	}
	if tool["description"] != "Returns the weather." {
		t.Errorf("description: got %v, want %q", tool["description"], "Returns the weather.")
	}
	params, ok := tool["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters: got %T, want map[string]any", tool["parameters"])
	}
	if params["type"] != "object" {
		t.Errorf("parameters.type: got %v, want %q", params["type"], "object")
	}
}

func TestBuildResponsesRequest_ToolDeclarationNoParameters(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "hi"}}},
		},
		Config: &genai.GenerateContentConfig{
			Tools: []*genai.Tool{{
				FunctionDeclarations: []*genai.FunctionDeclaration{{
					Name: "no_args",
				}},
			}},
		},
	}
	rr, err := buildResponsesRequest(req, "gpt-4o", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rr.Tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(rr.Tools))
	}
	params, ok := rr.Tools[0]["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters: got %T, want map[string]any", rr.Tools[0]["parameters"])
	}
	if params["type"] != "object" {
		t.Errorf("parameters.type: got %v, want %q", params["type"], "object")
	}
}

func TestBuildResponsesRequest_NonFunctionTool(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "hi"}}},
		},
		Config: &genai.GenerateContentConfig{
			Tools: []*genai.Tool{{
				Retrieval: &genai.Retrieval{},
			}},
		},
	}
	_, err := buildResponsesRequest(req, "gpt-4o", false)
	if err == nil {
		t.Fatal("got nil error, want error for non-function tool")
	}
}

func TestBuildResponsesRequest_ToolChoice(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		config  *genai.ToolConfig
		wantNil bool
		check   func(t *testing.T, v any)
	}{
		{
			name: "auto_no_allowed",
			config: &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode: genai.FunctionCallingConfigModeAuto,
				},
			},
			wantNil: true,
		},
		{
			name: "auto_with_allowed",
			config: &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode:                 genai.FunctionCallingConfigModeAuto,
					AllowedFunctionNames: []string{"fn1"},
				},
			},
			check: func(t *testing.T, v any) {
				t.Helper()
				m, ok := v.(map[string]any)
				if !ok {
					t.Fatalf("got %T, want map", v)
				}
				if m["type"] != "allowed_tools" {
					t.Errorf("type: got %v, want %q", m["type"], "allowed_tools")
				}
				if m["mode"] != "auto" {
					t.Errorf("mode: got %v, want %q", m["mode"], "auto")
				}
			},
		},
		{
			name: "none",
			config: &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode: genai.FunctionCallingConfigModeNone,
				},
			},
			check: func(t *testing.T, v any) {
				t.Helper()
				if v != "none" {
					t.Errorf("got %v, want %q", v, "none")
				}
			},
		},
		{
			name: "any_no_allowed",
			config: &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode: genai.FunctionCallingConfigModeAny,
				},
			},
			check: func(t *testing.T, v any) {
				t.Helper()
				if v != "required" {
					t.Errorf("got %v, want %q", v, "required")
				}
			},
		},
		{
			name: "any_with_allowed",
			config: &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode:                 genai.FunctionCallingConfigModeAny,
					AllowedFunctionNames: []string{"fn1", "fn2"},
				},
			},
			check: func(t *testing.T, v any) {
				t.Helper()
				m, ok := v.(map[string]any)
				if !ok {
					t.Fatalf("got %T, want map", v)
				}
				if m["mode"] != "required" {
					t.Errorf("mode: got %v, want %q", m["mode"], "required")
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := &model.LLMRequest{
				Contents: []*genai.Content{
					{Role: "user", Parts: []*genai.Part{{Text: "hi"}}},
				},
				Config: &genai.GenerateContentConfig{
					ToolConfig: tc.config,
				},
			}
			rr, err := buildResponsesRequest(req, "gpt-4o", false)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantNil {
				if rr.ToolChoice != nil {
					t.Errorf("tool_choice: got %v, want nil", rr.ToolChoice)
				}
				return
			}
			if tc.check != nil {
				tc.check(t, rr.ToolChoice)
			}
		})
	}
}

func TestBuildResponsesRequest_BasicFields(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "Hi"}}},
		},
		Config: &genai.GenerateContentConfig{
			Temperature:     new(float32(0.7)),
			TopP:            new(float32(0.9)),
			MaxOutputTokens: 512,
		},
	}
	rr, err := buildResponsesRequest(req, "gpt-4o-mini", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rr.Temperature == nil || *rr.Temperature != 0.7 {
		t.Errorf("temperature: got %v, want 0.7", rr.Temperature)
	}
	if rr.TopP == nil || *rr.TopP != 0.9 {
		t.Errorf("top_p: got %v, want 0.9", rr.TopP)
	}
	if rr.MaxOutputTokens == nil || *rr.MaxOutputTokens != 512 {
		t.Errorf("max_output_tokens: got %v, want 512", rr.MaxOutputTokens)
	}
}

func TestBuildResponsesRequest_ReqModelOverrides(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Model: "gpt-4o-mini",
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "hi"}}},
		},
	}
	rr, err := buildResponsesRequest(req, "gpt-4o", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rr.Model != "gpt-4o-mini" {
		t.Errorf("model: got %q, want %q", rr.Model, "gpt-4o-mini")
	}
}

func TestBuildResponsesRequest_NilConfig(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "Hi"}}},
		},
		Config: nil,
	}
	rr, err := buildResponsesRequest(req, "gpt-4o", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rr.Model != "gpt-4o" {
		t.Errorf("model: got %q, want %q", rr.Model, "gpt-4o")
	}
	if rr.Temperature != nil {
		t.Errorf("temperature: got %v, want nil", rr.Temperature)
	}
}

func TestBuildResponsesRequest_EmptyContents(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: nil,
	}
	_, err := buildResponsesRequest(req, "gpt-4o", false)
	if !errors.Is(err, ErrNoContents) {
		t.Errorf("got %v, want ErrNoContents", err)
	}
}

func TestBuildResponsesRequest_NilRequest(t *testing.T) {
	t.Parallel()
	_, err := buildResponsesRequest(nil, "gpt-4o", false)
	if err == nil {
		t.Fatal("got nil error, want error for nil request")
	}
}

func TestBuildResponsesRequest_InlineDataUnsupported(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{
				{InlineData: &genai.Blob{MIMEType: "image/png", Data: []byte("img")}},
			}},
		},
	}
	_, err := buildResponsesRequest(req, "gpt-4o", false)
	if !errors.Is(err, ErrUnsupportedContentPart) {
		t.Errorf("got %v, want ErrUnsupportedContentPart", err)
	}
}

func TestBuildResponsesRequest_EmptyTextSkipped(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{
				{Text: "   "},
				{Text: "real"},
			}},
		},
	}
	rr, err := buildResponsesRequest(req, "gpt-4o", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rr.Input) != 1 {
		t.Fatalf("got %d input items, want 1", len(rr.Input))
	}
	if rr.Input[0].Content[0].Text != "real" {
		t.Errorf("text: got %q, want %q", rr.Input[0].Content[0].Text, "real")
	}
}

// --- Error path tests ---

func TestBuildResponsesRequest_ErrorStopSequences(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
		Config: &genai.GenerateContentConfig{
			StopSequences: []string{"###"},
		},
	}
	_, err := buildResponsesRequest(req, "gpt-4o", false)
	if !errors.Is(err, ErrStopSequencesNotSupported) {
		t.Errorf("got %v, want ErrStopSequencesNotSupported", err)
	}
}

func TestBuildResponsesRequest_ErrorTopK(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
		Config: &genai.GenerateContentConfig{
			TopK: new(float32(5)),
		},
	}
	_, err := buildResponsesRequest(req, "gpt-4o", false)
	if !errors.Is(err, ErrTopKNotSupported) {
		t.Errorf("got %v, want ErrTopKNotSupported", err)
	}
}

func TestBuildResponsesRequest_ErrorMultipleCandidates(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
		Config: &genai.GenerateContentConfig{
			CandidateCount: 2,
		},
	}
	_, err := buildResponsesRequest(req, "gpt-4o", false)
	if !errors.Is(err, ErrMultipleCandidatesNotSupported) {
		t.Errorf("got %v, want ErrMultipleCandidatesNotSupported", err)
	}
}

func TestBuildResponsesRequest_ErrorPenalties(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
		Config: &genai.GenerateContentConfig{
			FrequencyPenalty: new(float32(0.5)),
		},
	}
	_, err := buildResponsesRequest(req, "gpt-4o", false)
	if !errors.Is(err, ErrPenaltiesNotSupported) {
		t.Errorf("got %v, want ErrPenaltiesNotSupported", err)
	}
}

func TestBuildResponsesRequest_ErrorLabels(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
		Config: &genai.GenerateContentConfig{
			Labels: map[string]string{"key": "val"},
		},
	}
	_, err := buildResponsesRequest(req, "gpt-4o", false)
	if !errors.Is(err, ErrLabelsNotSupported) {
		t.Errorf("got %v, want ErrLabelsNotSupported", err)
	}
}

func TestBuildResponsesRequest_ErrorSafetySettings(t *testing.T) {
	t.Parallel()
	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
		Config: &genai.GenerateContentConfig{
			SafetySettings: []*genai.SafetySetting{{Category: "HARM_CATEGORY_HARASSMENT"}},
		},
	}
	_, err := buildResponsesRequest(req, "gpt-4o", false)
	if !errors.Is(err, ErrSafetySettingsNotSupported) {
		t.Errorf("got %v, want ErrSafetySettingsNotSupported", err)
	}
}

// --- translateResponsesResponse tests ---

func TestTranslateResponsesResponse_Text(t *testing.T) {
	t.Parallel()
	resp := &responsesResponse{
		ID:    "resp_1",
		Model: "gpt-4o",
		Output: []responsesOutputItem{
			{Type: "message", Role: "assistant", Content: []responsesOutputContent{
				{Type: "output_text", Text: "Hello!"},
			}},
		},
	}
	llmResp, err := translateResponsesResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if llmResp.Content == nil || len(llmResp.Content.Parts) != 1 {
		t.Fatalf("unexpected content: %v", llmResp.Content)
	}
	if llmResp.Content.Parts[0].Text != "Hello!" {
		t.Errorf("text: got %q, want %q", llmResp.Content.Parts[0].Text, "Hello!")
	}
	if llmResp.FinishReason != genai.FinishReasonStop {
		t.Errorf("finish reason: got %v, want %v", llmResp.FinishReason, genai.FinishReasonStop)
	}
}

func TestTranslateResponsesResponse_FunctionCall(t *testing.T) {
	t.Parallel()
	resp := &responsesResponse{
		ID:    "resp_1",
		Model: "gpt-4o",
		Output: []responsesOutputItem{
			{Type: "function_call", CallID: "call_abc", Name: "get_weather", Arguments: `{"location":"NYC"}`},
		},
	}
	llmResp, err := translateResponsesResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(llmResp.Content.Parts) != 1 {
		t.Fatalf("got %d parts, want 1", len(llmResp.Content.Parts))
	}
	fc := llmResp.Content.Parts[0].FunctionCall
	if fc == nil {
		t.Fatal("got nil FunctionCall, want non-nil")
	}
	if fc.ID != "call_abc" {
		t.Errorf("ID: got %q, want %q", fc.ID, "call_abc")
	}
	if fc.Name != "get_weather" {
		t.Errorf("Name: got %q, want %q", fc.Name, "get_weather")
	}
	if fc.Args["location"] != "NYC" {
		t.Errorf("Args[location]: got %v, want %q", fc.Args["location"], "NYC")
	}
}

func TestTranslateResponsesResponse_FunctionCallEmptyArgs(t *testing.T) {
	t.Parallel()
	resp := &responsesResponse{
		ID:    "resp_1",
		Model: "gpt-4o",
		Output: []responsesOutputItem{
			{Type: "function_call", CallID: "call_1", Name: "no_args", Arguments: ""},
		},
	}
	llmResp, err := translateResponsesResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fc := llmResp.Content.Parts[0].FunctionCall
	if fc.Args == nil {
		t.Fatal("got nil Args, want non-nil")
	}
	if len(fc.Args) != 0 {
		t.Errorf("Args: got %v, want empty map", fc.Args)
	}
}

func TestTranslateResponsesResponse_FunctionCallInvalidJSON(t *testing.T) {
	t.Parallel()
	resp := &responsesResponse{
		ID:    "resp_1",
		Model: "gpt-4o",
		Output: []responsesOutputItem{
			{Type: "function_call", CallID: "call_1", Name: "fn", Arguments: "{invalid}"},
		},
	}
	_, err := translateResponsesResponse(resp)
	if err == nil {
		t.Fatal("got nil error, want error for invalid JSON arguments")
	}
}

func TestTranslateResponsesResponse_Reasoning(t *testing.T) {
	t.Parallel()
	resp := &responsesResponse{
		ID:    "resp_1",
		Model: "gpt-4o",
		Output: []responsesOutputItem{
			{Type: "reasoning", Content: []responsesOutputContent{
				{Type: "output_text", Text: "Thinking..."},
			}},
		},
	}
	llmResp, err := translateResponsesResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(llmResp.Content.Parts) != 1 {
		t.Fatalf("got %d parts, want 1", len(llmResp.Content.Parts))
	}
	if llmResp.Content.Parts[0].Text != "Thinking..." {
		t.Errorf("text: got %q, want %q", llmResp.Content.Parts[0].Text, "Thinking...")
	}
	if !llmResp.Content.Parts[0].Thought {
		t.Errorf("thought: got false, want true")
	}
}

func TestTranslateResponsesResponse_Refusal(t *testing.T) {
	t.Parallel()
	resp := &responsesResponse{
		ID:    "resp_1",
		Model: "gpt-4o",
		Output: []responsesOutputItem{
			{Type: "message", Role: "assistant", Content: []responsesOutputContent{
				{Type: "refusal", Refusal: "I cannot help with that."},
			}},
		},
	}
	llmResp, err := translateResponsesResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if llmResp.Content.Parts[0].Text != "I cannot help with that." {
		t.Errorf("text: got %q, want %q", llmResp.Content.Parts[0].Text, "I cannot help with that.")
	}
}

func TestTranslateResponsesResponse_EmptyOutput(t *testing.T) {
	t.Parallel()
	resp := &responsesResponse{
		ID:     "resp_1",
		Model:  "gpt-4o",
		Output: []responsesOutputItem{{Type: "message", Role: "assistant"}},
	}
	_, err := translateResponsesResponse(resp)
	if !errors.Is(err, ErrNoTextOrToolContent) {
		t.Errorf("got %v, want ErrNoTextOrToolContent", err)
	}
}

func TestTranslateResponsesResponse_EmptyOutputItems(t *testing.T) {
	t.Parallel()
	resp := &responsesResponse{
		ID:     "resp_1",
		Model:  "gpt-4o",
		Output: nil,
	}
	_, err := translateResponsesResponse(resp)
	if !errors.Is(err, ErrNoOutputItems) {
		t.Errorf("got %v, want ErrNoOutputItems", err)
	}
}

func TestTranslateResponsesResponse_UnknownOutputItemType(t *testing.T) {
	t.Parallel()
	resp := &responsesResponse{
		ID:    "resp_1",
		Model: "gpt-4o",
		Output: []responsesOutputItem{
			{Type: "unknown_type"},
		},
	}
	_, err := translateResponsesResponse(resp)
	if !errors.Is(err, ErrUnsupportedOutputItemType) {
		t.Errorf("got %v, want ErrUnsupportedOutputItemType", err)
	}
}

func TestTranslateResponsesResponse_UnknownMessageContentType(t *testing.T) {
	t.Parallel()
	resp := &responsesResponse{
		ID:    "resp_1",
		Model: "gpt-4o",
		Output: []responsesOutputItem{
			{Type: "message", Role: "assistant", Content: []responsesOutputContent{
				{Type: "unknown"},
			}},
		},
	}
	_, err := translateResponsesResponse(resp)
	if !errors.Is(err, ErrUnsupportedMessageContentType) {
		t.Errorf("got %v, want ErrUnsupportedMessageContentType", err)
	}
}

func TestTranslateResponsesResponse_Usage(t *testing.T) {
	t.Parallel()
	resp := &responsesResponse{
		ID:    "resp_1",
		Model: "gpt-4o",
		Output: []responsesOutputItem{
			{Type: "message", Role: "assistant", Content: []responsesOutputContent{
				{Type: "output_text", Text: "ok"},
			}},
		},
		Usage: &responsesUsage{
			InputTokens:         10,
			OutputTokens:        5,
			TotalTokens:         15,
			InputTokensDetails:  &responsesTokenDetails{CachedTokens: 3},
			OutputTokensDetails: &responsesTokenDetails{ReasoningTokens: 2},
		},
	}
	llmResp, err := translateResponsesResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if llmResp.UsageMetadata == nil {
		t.Fatal("got nil UsageMetadata, want non-nil")
	}
	if llmResp.UsageMetadata.PromptTokenCount != 10 {
		t.Errorf("PromptTokenCount: got %d, want 10", llmResp.UsageMetadata.PromptTokenCount)
	}
	if llmResp.UsageMetadata.CandidatesTokenCount != 5 {
		t.Errorf("CandidatesTokenCount: got %d, want 5", llmResp.UsageMetadata.CandidatesTokenCount)
	}
	if llmResp.UsageMetadata.TotalTokenCount != 15 {
		t.Errorf("TotalTokenCount: got %d, want 15", llmResp.UsageMetadata.TotalTokenCount)
	}
	if llmResp.UsageMetadata.CachedContentTokenCount != 3 {
		t.Errorf("CachedContentTokenCount: got %d, want 3", llmResp.UsageMetadata.CachedContentTokenCount)
	}
	if llmResp.UsageMetadata.ThoughtsTokenCount != 2 {
		t.Errorf("ThoughtsTokenCount: got %d, want 2", llmResp.UsageMetadata.ThoughtsTokenCount)
	}
}

func TestTranslateResponsesResponse_FinishReason(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		incomplete *responsesIncompleteDet
		want       genai.FinishReason
	}{
		{"nil", nil, genai.FinishReasonStop},
		{"empty", &responsesIncompleteDet{Reason: ""}, genai.FinishReasonStop},
		{"max_output_tokens", &responsesIncompleteDet{Reason: "max_output_tokens"}, genai.FinishReasonMaxTokens},
		{"content_filter", &responsesIncompleteDet{Reason: "content_filter"}, genai.FinishReasonSafety},
		{"other", &responsesIncompleteDet{Reason: "something_else"}, genai.FinishReasonOther},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp := &responsesResponse{
				ID:    "resp_1",
				Model: "gpt-4o",
				Output: []responsesOutputItem{
					{Type: "message", Role: "assistant", Content: []responsesOutputContent{
						{Type: "output_text", Text: "ok"},
					}},
				},
				IncompleteDetails: tc.incomplete,
			}
			llmResp, err := translateResponsesResponse(resp)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if llmResp.FinishReason != tc.want {
				t.Errorf("finish reason: got %v, want %v", llmResp.FinishReason, tc.want)
			}
		})
	}
}

func TestTranslateResponsesResponse_Metadata(t *testing.T) {
	t.Parallel()
	resp := &responsesResponse{
		ID:    "resp_meta_123",
		Model: "gpt-4o-2024",
		Output: []responsesOutputItem{
			{Type: "message", Role: "assistant", Content: []responsesOutputContent{
				{Type: "output_text", Text: "ok"},
			}},
		},
	}
	llmResp, err := translateResponsesResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if llmResp.CustomMetadata["openai_response_id"] != "resp_meta_123" {
		t.Errorf("openai_response_id: got %v, want %q", llmResp.CustomMetadata["openai_response_id"], "resp_meta_123")
	}
	if llmResp.CustomMetadata["openai_model"] != "gpt-4o-2024" {
		t.Errorf("openai_model: got %v, want %q", llmResp.CustomMetadata["openai_model"], "gpt-4o-2024")
	}
}

// --- safeInt32 tests ---

func TestSafeInt32(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input int64
		want  int32
	}{
		{0, 0},
		{100, 100},
		{-100, -100},
		{int64(2147483647), 2147483647},
		{int64(2147483648), 2147483647}, // overflow → max
		{int64(-2147483648), -2147483648},
		{int64(-2147483649), -2147483648}, // underflow → min
	}
	for _, tc := range cases {
		got := safeInt32(tc.input)
		if got != tc.want {
			t.Errorf("safeInt32(%d) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

// --- enforceStrictOpenAISchema tests ---

func TestEnforceStrictOpenAISchema_Object(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"b": map[string]any{"type": "string"},
			"a": map[string]any{"type": "integer"},
		},
	}
	enforceStrictOpenAISchema(schema)
	if schema["additionalProperties"] != false {
		t.Errorf("additionalProperties: got %v, want false", schema["additionalProperties"])
	}
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("required: got %T, want []string", schema["required"])
	}
	if len(required) != 2 || required[0] != "a" || required[1] != "b" {
		t.Errorf("required: got %v, want [a b] (sorted)", required)
	}
}

func TestEnforceStrictOpenAISchema_Ref(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"$ref":       "#/$defs/foo",
		"type":       "object",
		"properties": map[string]any{"x": map[string]any{"type": "string"}},
	}
	enforceStrictOpenAISchema(schema)
	if schema["$ref"] != "#/$defs/foo" {
		t.Errorf("$ref: got %v, want %q", schema["$ref"], "#/$defs/foo")
	}
	if _, exists := schema["type"]; exists {
		t.Errorf("type should be stripped from $ref node, got %v", schema["type"])
	}
}

func TestEnforceStrictOpenAISchema_Defs(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"item": map[string]any{"$ref": "#/$defs/sub"},
		},
		"$defs": map[string]any{
			"sub": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
			},
		},
	}
	enforceStrictOpenAISchema(schema)
	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatal("$defs not found")
	}
	sub, ok := defs["sub"].(map[string]any)
	if !ok {
		t.Fatal("sub def not found")
	}
	if sub["additionalProperties"] != false {
		t.Errorf("sub additionalProperties: got %v, want false", sub["additionalProperties"])
	}
}

func TestEnforceStrictOpenAISchema_AnyOf(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"anyOf": []any{
			map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "string"}}},
			map[string]any{"type": "null"},
		},
	}
	enforceStrictOpenAISchema(schema)
	arr, ok := schema["anyOf"].([]any)
	if !ok {
		t.Fatal("anyOf not found")
	}
	first, ok := arr[0].(map[string]any)
	if !ok {
		t.Fatal("first anyOf element not a map")
	}
	if first["additionalProperties"] != false {
		t.Errorf("anyOf[0] additionalProperties: got %v, want false", first["additionalProperties"])
	}
}

func TestEnforceStrictOpenAISchema_Items(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type":       "object",
			"properties": map[string]any{"val": map[string]any{"type": "string"}},
		},
	}
	enforceStrictOpenAISchema(schema)
	items, ok := schema["items"].(map[string]any)
	if !ok {
		t.Fatal("items not found")
	}
	if items["additionalProperties"] != false {
		t.Errorf("items additionalProperties: got %v, want false", items["additionalProperties"])
	}
}

func TestEnforceStrictOpenAISchema_Nested(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"outer": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"inner": map[string]any{"type": "string"},
				},
			},
		},
	}
	enforceStrictOpenAISchema(schema)
	outer, ok := schema["properties"].(map[string]any)["outer"].(map[string]any)
	if !ok {
		t.Fatal("outer not found")
	}
	if outer["additionalProperties"] != false {
		t.Errorf("outer additionalProperties: got %v, want false", outer["additionalProperties"])
	}
}

// --- normalizeSchema tests ---

func TestNormalizeSchema_Map(t *testing.T) {
	t.Parallel()
	input := map[string]any{"type": "object"}
	result, err := normalizeSchema(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["type"] != "object" {
		t.Errorf("type: got %v, want %q", result["type"], "object")
	}
}

func TestNormalizeSchema_Nil(t *testing.T) {
	t.Parallel()
	_, err := normalizeSchema(nil)
	if !errors.Is(err, ErrEmptyJSONSchema) {
		t.Errorf("got %v, want ErrEmptyJSONSchema", err)
	}
}

func TestNormalizeSchema_OtherType(t *testing.T) {
	t.Parallel()
	// A struct marshals to a JSON object.
	type mySchema struct {
		Type       string         `json:"type"`
		Properties map[string]any `json:"properties,omitempty"`
	}
	input := mySchema{Type: "object"}
	result, err := normalizeSchema(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["type"] != "object" {
		t.Errorf("type: got %v, want %q", result["type"], "object")
	}
}

// --- callTracker tests ---

func TestCallTracker_GeneratesID(t *testing.T) {
	t.Parallel()
	ct := &callTracker{}
	id1 := ct.newCallID()
	id2 := ct.newCallID()
	if id1 != "adk-openai-call-1" {
		t.Errorf("first ID: got %q, want %q", id1, "adk-openai-call-1")
	}
	if id2 != "adk-openai-call-2" {
		t.Errorf("second ID: got %q, want %q", id2, "adk-openai-call-2")
	}
}

func TestCallTracker_MatchByID(t *testing.T) {
	t.Parallel()
	ct := &callTracker{}
	ct.registerCall("call_1")
	id, err := ct.resolveResponseCallID("call_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "call_1" {
		t.Errorf("got %q, want %q", id, "call_1")
	}
}

func TestCallTracker_MatchOldestWhenIDMissing(t *testing.T) {
	t.Parallel()
	ct := &callTracker{}
	ct.registerCall("call_1")
	ct.registerCall("call_2")
	id, err := ct.resolveResponseCallID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "call_1" {
		t.Errorf("got %q, want %q (oldest)", id, "call_1")
	}
}

func TestCallTracker_NoPendingAndIDMissing(t *testing.T) {
	t.Parallel()
	ct := &callTracker{}
	_, err := ct.resolveResponseCallID("")
	if !errors.Is(err, ErrFunctionResponseMissingCallID) {
		t.Errorf("got %v, want ErrFunctionResponseMissingCallID", err)
	}
}

func TestCallTracker_UnknownID(t *testing.T) {
	t.Parallel()
	ct := &callTracker{}
	ct.registerCall("call_1")
	_, err := ct.resolveResponseCallID("call_unknown")
	if !errors.Is(err, ErrFunctionResponseUnknownCallID) {
		t.Errorf("got %v, want ErrFunctionResponseUnknownCallID", err)
	}
}

// --- HTTP integration tests ---

func TestGenerateResponses_NonStreaming(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("got %s, want POST", r.Method)
		}
		if r.URL.Path != "/responses" {
			t.Errorf("got %s, want path /responses", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, cannedResponsesResponse)
	}))
	t.Cleanup(srv.Close)

	m, err := New(Config{
		Model:   "gpt-4o",
		APIKey:  "sk-test",
		BaseURL: srv.URL,
		API:     APIResponses,
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
		t.Errorf("got TurnComplete=false, want true")
	}
	if resp.Content == nil || len(resp.Content.Parts) == 0 {
		t.Fatal("got no content")
	}
	if resp.Content.Parts[0].Text != "Hello!" {
		t.Errorf("text: got %q, want %q", resp.Content.Parts[0].Text, "Hello!")
	}
}

func TestGenerateResponses_Non2xxError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"bad request"}}`, http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	m, err := New(Config{
		Model:   "gpt-4o",
		APIKey:  "sk-test",
		BaseURL: srv.URL,
		API:     APIResponses,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
	}
	_, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) == 0 {
		t.Fatal("got no errors, want at least one")
	}
	var httpErr *HTTPError
	if !errors.As(errs[0], &httpErr) {
		t.Fatalf("got %T, want *HTTPError", errs[0])
	}
	if httpErr.StatusCode != http.StatusBadRequest {
		t.Errorf("status code: got %d, want %d", httpErr.StatusCode, http.StatusBadRequest)
	}
}

func TestGenerateResponses_Non2xxErrorBodyClose(t *testing.T) {
	t.Parallel()

	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount > 1 {
			http.Error(w, `{"error":"bad"}`, http.StatusInternalServerError)
			return
		}
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
		_, _ = bufrw.WriteString("partial")
		_ = bufrw.Flush()
		_ = conn.Close()
	}))
	t.Cleanup(srv.Close)

	transport := &http.Transport{MaxConnsPerHost: 1}
	t.Cleanup(transport.CloseIdleConnections)

	m, err := New(Config{
		Model:      "gpt-4o",
		APIKey:     "sk-test",
		BaseURL:    srv.URL,
		HTTPClient: &http.Client{Transport: transport},
		API:        APIResponses,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
	}

	_, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) == 0 {
		t.Fatal("got no errors from truncated 500, want at least one")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	_, errs2 := collectResponses(m, ctx, req, false)
	if len(errs2) == 0 {
		t.Fatal("got no errors from second 500, want at least one")
	}
	found500 := false
	for _, e := range errs2 {
		var httpErr *HTTPError
		if errors.As(e, &httpErr) && httpErr.StatusCode == http.StatusInternalServerError {
			found500 = true
			break
		}
	}
	if !found500 {
		t.Errorf("second errors: got %v, want HTTPError 500", errs2)
	}
}

func TestGenerateResponses_UnmarshalError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, "{invalid json}")
	}))
	t.Cleanup(srv.Close)

	m, err := New(Config{
		Model:   "gpt-4o",
		APIKey:  "sk-test",
		BaseURL: srv.URL,
		API:     APIResponses,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
	}
	_, errs := collectResponses(m, context.Background(), req, false)
	if len(errs) == 0 {
		t.Fatal("got no errors, want at least one for invalid JSON")
	}
}

func TestGenerateResponses_RequestBody(t *testing.T) {
	t.Parallel()
	var receivedBody responsesRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, cannedResponsesResponse)
	}))
	t.Cleanup(srv.Close)

	m, err := New(Config{
		Model:   "gpt-4o",
		APIKey:  "sk-test",
		BaseURL: srv.URL,
		API:     APIResponses,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}},
	}
	_, _ = collectResponses(m, context.Background(), req, false)
	if receivedBody.Model != "gpt-4o" {
		t.Errorf("model: got %q, want %q", receivedBody.Model, "gpt-4o")
	}
	if len(receivedBody.Input) != 1 {
		t.Errorf("input: got %d items, want 1", len(receivedBody.Input))
	}
}
