package openai

import (
	"encoding/json"
	"testing"

	"google.golang.org/genai"
)

// TestTranslateToolDeclarations verifies that a single FunctionDeclaration is
// converted to the correct OpenAI tool definition format.
func TestTranslateToolDeclarations(t *testing.T) {
	tools := []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{
					Name:        "get_weather",
					Description: "Get the current weather for a location",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"location": {
								Type:        genai.TypeString,
								Description: "The city and state",
							},
						},
						Required: []string{"location"},
					},
				},
			},
		},
	}

	got := translateToolDeclarations(tools)

	if len(got) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(got))
	}

	tool := got[0]
	if tool["type"] != "function" {
		t.Errorf("type: got %v, want %q", tool["type"], "function")
	}

	fn, ok := tool["function"].(map[string]any)
	if !ok {
		t.Fatalf("function field is not map[string]any: %T", tool["function"])
	}
	if fn["name"] != "get_weather" {
		t.Errorf("function.name: got %v, want %q", fn["name"], "get_weather")
	}
	if fn["description"] != "Get the current weather for a location" {
		t.Errorf("function.description: got %v, want %q", fn["description"], "Get the current weather for a location")
	}

	params, ok := fn["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("function.parameters is not map[string]any: %T", fn["parameters"])
	}
	if params["type"] != "object" {
		t.Errorf("parameters.type: got %v, want %q", params["type"], "object")
	}

	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("parameters.properties is not map[string]any: %T", params["properties"])
	}
	locProp, ok := props["location"].(map[string]any)
	if !ok {
		t.Fatalf("properties.location is not map[string]any: %T", props["location"])
	}
	if locProp["type"] != "string" {
		t.Errorf("location.type: got %v, want %q", locProp["type"], "string")
	}
}

// TestTranslateToolDeclarations_Multiple verifies that 2 FunctionDeclarations
// within a single Tool are expanded into 2 separate OpenAI tool entries.
func TestTranslateToolDeclarations_Multiple(t *testing.T) {
	tools := []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{Name: "func_a", Description: "First function"},
				{Name: "func_b", Description: "Second function"},
			},
		},
	}

	got := translateToolDeclarations(tools)

	if len(got) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(got))
	}

	names := make(map[string]bool)
	for _, tool := range got {
		fn, ok := tool["function"].(map[string]any)
		if !ok {
			t.Fatalf("function field is not map[string]any")
		}
		name, _ := fn["name"].(string)
		names[name] = true
	}

	if !names["func_a"] {
		t.Error("expected func_a in results")
	}
	if !names["func_b"] {
		t.Error("expected func_b in results")
	}
}

// TestTranslateToolDeclarations_Empty verifies that nil or empty tool slices
// return nil.
func TestTranslateToolDeclarations_Empty(t *testing.T) {
	if got := translateToolDeclarations(nil); got != nil {
		t.Errorf("nil input: expected nil, got %v", got)
	}
	if got := translateToolDeclarations([]*genai.Tool{}); got != nil {
		t.Errorf("empty input: expected nil, got %v", got)
	}
	// Tool with no declarations also yields nil.
	if got := translateToolDeclarations([]*genai.Tool{{}}); got != nil {
		t.Errorf("empty declarations: expected nil, got %v", got)
	}
}

// TestTranslateToolConfig_Auto verifies that AUTO mode maps to "auto".
func TestTranslateToolConfig_Auto(t *testing.T) {
	cfg := &genai.ToolConfig{
		FunctionCallingConfig: &genai.FunctionCallingConfig{
			Mode: genai.FunctionCallingConfigModeAuto,
		},
	}
	got := translateToolConfig(cfg)
	if got != "auto" {
		t.Errorf("got %v, want %q", got, "auto")
	}
}

// TestTranslateToolConfig_Any verifies that ANY mode maps to "required".
func TestTranslateToolConfig_Any(t *testing.T) {
	cfg := &genai.ToolConfig{
		FunctionCallingConfig: &genai.FunctionCallingConfig{
			Mode: genai.FunctionCallingConfigModeAny,
		},
	}
	got := translateToolConfig(cfg)
	if got != "required" {
		t.Errorf("got %v, want %q", got, "required")
	}
}

// TestTranslateToolConfig_None verifies that NONE mode maps to "none".
func TestTranslateToolConfig_None(t *testing.T) {
	cfg := &genai.ToolConfig{
		FunctionCallingConfig: &genai.FunctionCallingConfig{
			Mode: genai.FunctionCallingConfigModeNone,
		},
	}
	got := translateToolConfig(cfg)
	if got != "none" {
		t.Errorf("got %v, want %q", got, "none")
	}
}

// TestTranslateToolConfig_Nil verifies that a nil ToolConfig returns nil.
func TestTranslateToolConfig_Nil(t *testing.T) {
	got := translateToolConfig(nil)
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

// TestFunctionCallToToolCall verifies that a genai FunctionCall is converted
// to the correct OpenAI tool_call map format.
func TestFunctionCallToToolCall(t *testing.T) {
	fc := &genai.FunctionCall{
		ID:   "call_abc123",
		Name: "get_weather",
		Args: map[string]any{
			"location": "San Francisco, CA",
			"unit":     "celsius",
		},
	}

	got := functionCallToToolCall(fc)

	if got["id"] != "call_abc123" {
		t.Errorf("id: got %v, want %q", got["id"], "call_abc123")
	}
	if got["type"] != "function" {
		t.Errorf("type: got %v, want %q", got["type"], "function")
	}

	fn, ok := got["function"].(map[string]any)
	if !ok {
		t.Fatalf("function field is not map[string]any: %T", got["function"])
	}
	if fn["name"] != "get_weather" {
		t.Errorf("function.name: got %v, want %q", fn["name"], "get_weather")
	}

	argsStr, ok := fn["arguments"].(string)
	if !ok {
		t.Fatalf("function.arguments is not string: %T", fn["arguments"])
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
		t.Fatalf("arguments is not valid JSON: %v", err)
	}
	if args["location"] != "San Francisco, CA" {
		t.Errorf("args.location: got %v, want %q", args["location"], "San Francisco, CA")
	}
	if args["unit"] != "celsius" {
		t.Errorf("args.unit: got %v, want %q", args["unit"], "celsius")
	}
}

// TestToolCallsToFunctionCalls verifies that a single OpenAI tool_call entry
// is converted back to a genai FunctionCall Part.
func TestToolCallsToFunctionCalls(t *testing.T) {
	toolCalls := []map[string]any{
		{
			"id":   "call_xyz",
			"type": "function",
			"function": map[string]any{
				"name":      "search",
				"arguments": `{"query":"golang testing"}`,
			},
		},
	}

	parts := toolCallsToFunctionCalls(toolCalls)

	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}

	fc := parts[0].FunctionCall
	if fc == nil {
		t.Fatal("expected FunctionCall to be set")
	}
	if fc.ID != "call_xyz" {
		t.Errorf("ID: got %q, want %q", fc.ID, "call_xyz")
	}
	if fc.Name != "search" {
		t.Errorf("Name: got %q, want %q", fc.Name, "search")
	}
	if fc.Args["query"] != "golang testing" {
		t.Errorf("Args.query: got %v, want %q", fc.Args["query"], "golang testing")
	}
}

// TestToolCallsToFunctionCalls_Parallel verifies that 2 tool_call entries
// produce 2 corresponding genai FunctionCall Parts.
func TestToolCallsToFunctionCalls_Parallel(t *testing.T) {
	toolCalls := []map[string]any{
		{
			"id":   "call_1",
			"type": "function",
			"function": map[string]any{
				"name":      "func_a",
				"arguments": `{"x":1}`,
			},
		},
		{
			"id":   "call_2",
			"type": "function",
			"function": map[string]any{
				"name":      "func_b",
				"arguments": `{"y":2}`,
			},
		},
	}

	parts := toolCallsToFunctionCalls(toolCalls)

	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}

	ids := map[string]bool{}
	for _, p := range parts {
		if p.FunctionCall == nil {
			t.Fatal("expected FunctionCall to be set on all parts")
		}
		ids[p.FunctionCall.ID] = true
	}
	if !ids["call_1"] {
		t.Error("expected call_1 in results")
	}
	if !ids["call_2"] {
		t.Error("expected call_2 in results")
	}
}

// TestToolCallsToFunctionCalls_InvalidJSON verifies that tool calls with
// invalid JSON arguments are silently skipped while valid ones are still
// processed. This exercises the graceful-degradation path in
// toolCallsToFunctionCalls.
func TestToolCallsToFunctionCalls_InvalidJSON(t *testing.T) {
	toolCalls := []map[string]any{
		{
			"id":   "call_good",
			"type": "function",
			"function": map[string]any{
				"name":      "valid_fn",
				"arguments": `{"key":"value"}`,
			},
		},
		{
			"id":   "call_bad",
			"type": "function",
			"function": map[string]any{
				"name":      "broken_fn",
				"arguments": `{not valid json!!!`,
			},
		},
		{
			"id":   "call_also_good",
			"type": "function",
			"function": map[string]any{
				"name":      "another_fn",
				"arguments": `{"x":42}`,
			},
		},
	}

	parts := toolCallsToFunctionCalls(toolCalls)

	// The invalid entry should be skipped; only the two valid ones remain.
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts (invalid skipped), got %d", len(parts))
	}

	if parts[0].FunctionCall.ID != "call_good" {
		t.Errorf("parts[0] ID: got %q, want %q", parts[0].FunctionCall.ID, "call_good")
	}
	if parts[0].FunctionCall.Name != "valid_fn" {
		t.Errorf("parts[0] Name: got %q, want %q", parts[0].FunctionCall.Name, "valid_fn")
	}
	if parts[0].FunctionCall.Args["key"] != "value" {
		t.Errorf("parts[0] Args[key]: got %v, want %q", parts[0].FunctionCall.Args["key"], "value")
	}

	if parts[1].FunctionCall.ID != "call_also_good" {
		t.Errorf("parts[1] ID: got %q, want %q", parts[1].FunctionCall.ID, "call_also_good")
	}
}
