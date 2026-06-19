package anthropic

import (
	"testing"

	"google.golang.org/genai"
)

// TestTranslateToolDeclarations verifies that genai.Tools with
// FunctionDeclarations produce Anthropic-style tool definitions with
// name, description, and input_schema.
func TestTranslateToolDeclarations(t *testing.T) {
	tools := []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{
					Name:        "get_weather",
					Description: "Get the current weather",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"location": {Type: genai.TypeString},
						},
						Required: []string{"location"},
					},
				},
				{
					Name:        "get_time",
					Description: "Get the current time",
				},
			},
		},
	}

	result := translateToolDeclarations(tools)
	if len(result) != 2 {
		t.Fatalf("expected 2 tool declarations, got %d", len(result))
	}

	first := result[0]
	if first["name"] != "get_weather" {
		t.Errorf("name: got %v, want %q", first["name"], "get_weather")
	}
	if first["description"] != "Get the current weather" {
		t.Errorf("description: got %v, want %q", first["description"], "Get the current weather")
	}
	schema, ok := first["input_schema"].(map[string]any)
	if !ok {
		t.Fatal("expected input_schema map")
	}
	if schema["type"] != "object" {
		t.Errorf("input_schema.type: got %v, want %q", schema["type"], "object")
	}

	second := result[1]
	if second["name"] != "get_time" {
		t.Errorf("name: got %v, want %q", second["name"], "get_time")
	}
	if _, hasSchema := second["input_schema"]; hasSchema {
		t.Error("expected no input_schema for declaration without parameters")
	}
}

// TestTranslateToolDeclarations_SkipsNonFunction verifies that tools with
// non-function types (e.g. Retrieval, GoogleSearch) are silently skipped.
func TestTranslateToolDeclarations_SkipsNonFunction(t *testing.T) {
	tools := []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{Name: "valid_func"},
			},
		},
		{}, // empty tool
		{
			FunctionDeclarations: nil,
		},
	}

	result := translateToolDeclarations(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool declaration, got %d", len(result))
	}
	if result[0]["name"] != "valid_func" {
		t.Errorf("name: got %v, want %q", result[0]["name"], "valid_func")
	}
}

// TestTranslateToolConfig verifies all tool_choice variants: auto, any, none,
// and specific tool name.
func TestTranslateToolConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  *genai.ToolConfig
		want any
	}{
		{
			name: "auto",
			cfg: &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeAuto},
			},
			want: map[string]any{"type": "auto"},
		},
		{
			name: "any",
			cfg: &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeAny},
			},
			want: map[string]any{"type": "any"},
		},
		{
			name: "none",
			cfg: &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeNone},
			},
			want: map[string]any{"type": "none"},
		},
		{
			name: "specific tool",
			cfg: &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode:                 genai.FunctionCallingConfigModeAuto,
					AllowedFunctionNames: []string{"get_weather"},
				},
			},
			want: map[string]any{"type": "tool", "name": "get_weather"},
		},
		{
			name: "multiple allowed names falls back to auto",
			cfg: &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode:                 genai.FunctionCallingConfigModeAuto,
					AllowedFunctionNames: []string{"a", "b"},
				},
			},
			want: map[string]any{"type": "auto"},
		},
		{
			name: "nil config",
			cfg:  nil,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := translateToolConfig(tt.cfg)
			if got == nil && tt.want == nil {
				return
			}
			gotMap, gotOk := got.(map[string]any)
			wantMap, wantOk := tt.want.(map[string]any)
			if gotOk != wantOk {
				t.Fatalf("type mismatch: got %T, want %T", got, tt.want)
			}
			if len(gotMap) != len(wantMap) {
				t.Fatalf("map length mismatch: got %v, want %v", gotMap, wantMap)
			}
			for k, v := range wantMap {
				if gotMap[k] != v {
					t.Errorf("%s: got %v, want %v", k, gotMap[k], v)
				}
			}
		})
	}
}

// TestFunctionCallToToolUse verifies that a genai.FunctionCall is converted to
// the correct Anthropic tool_use block.
func TestFunctionCallToToolUse(t *testing.T) {
	fc := &genai.FunctionCall{
		ID:   "toolu_01A",
		Name: "get_weather",
		Args: map[string]any{"location": "Paris"},
	}
	block := functionCallToToolUse(fc)
	if block["type"] != "tool_use" {
		t.Errorf("type: got %v, want %q", block["type"], "tool_use")
	}
	if block["id"] != "toolu_01A" {
		t.Errorf("id: got %v, want %q", block["id"], "toolu_01A")
	}
	if block["name"] != "get_weather" {
		t.Errorf("name: got %v, want %q", block["name"], "get_weather")
	}
	input, ok := block["input"].(map[string]any)
	if !ok {
		t.Fatal("expected input map")
	}
	if input["location"] != "Paris" {
		t.Errorf("input.location: got %v, want %q", input["location"], "Paris")
	}
}

// TestToolUseToFunctionCall verifies that an Anthropic tool_use block is
// converted back to a genai.FunctionCall.
func TestToolUseToFunctionCall(t *testing.T) {
	block := map[string]any{
		"type":  "tool_use",
		"id":    "toolu_01B",
		"name":  "get_time",
		"input": map[string]any{"timezone": "UTC"},
	}
	fc := toolUseToFunctionCall(block)
	if fc.ID != "toolu_01B" {
		t.Errorf("ID: got %q, want %q", fc.ID, "toolu_01B")
	}
	if fc.Name != "get_time" {
		t.Errorf("Name: got %q, want %q", fc.Name, "get_time")
	}
	if fc.Args["timezone"] != "UTC" {
		t.Errorf("Args.timezone: got %v, want %q", fc.Args["timezone"], "UTC")
	}
}

// TestToolUseToFunctionCall_InvalidJSON verifies graceful handling when the
// input field contains malformed data.
func TestToolUseToFunctionCall_InvalidJSON(t *testing.T) {
	// When input is not a valid JSON object, Args should be nil or handled
	// gracefully without panicking.
	block := map[string]any{
		"type":  "tool_use",
		"id":    "toolu_01C",
		"name":  "broken",
		"input": make(chan int), // non-serialisable type
	}
	fc := toolUseToFunctionCall(block)
	if fc.ID != "toolu_01C" {
		t.Errorf("ID: got %q, want %q", fc.ID, "toolu_01C")
	}
	if fc.Name != "broken" {
		t.Errorf("Name: got %q, want %q", fc.Name, "broken")
	}
	// Args may be nil for unparseable input — that's acceptable.
}
