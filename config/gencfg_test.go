package config_test

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/ieshan/adk-go-pkg/config"
	"google.golang.org/genai"
)

func parseGenConfig(t *testing.T, gen string) *genai.GenerateContentConfig {
	t.Helper()
	data := "name: bot\nagent_class: LlmAgent\ngenerate_content_config:\n" + gen
	appCfg, err := config.Parse([]byte(data), "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return appCfg.AgentConfig.(*config.LLMAgentConfig).GenerateContentConfig
}

func TestDecodeGenerateConfig_ConventionsEquivalent(t *testing.T) {
	t.Parallel()
	snake := parseGenConfig(t, `  temperature: 0.7
  max_output_tokens: 512
  safety_settings:
    - category: HARM_CATEGORY_HARASSMENT
      threshold: BLOCK_LOW_AND_ABOVE
  thinking_config:
    include_thoughts: true
`)
	camel := parseGenConfig(t, `  temperature: 0.7
  maxOutputTokens: 512
  safetySettings:
    - category: HARM_CATEGORY_HARASSMENT
      threshold: BLOCK_LOW_AND_ABOVE
  thinkingConfig:
    includeThoughts: true
`)
	lower := parseGenConfig(t, `  temperature: 0.7
  maxoutputtokens: 512
  safetysettings:
    - category: HARM_CATEGORY_HARASSMENT
      threshold: BLOCK_LOW_AND_ABOVE
  thinkingconfig:
    includethoughts: true
`)

	if diff := cmp.Diff(snake, camel); diff != "" {
		t.Errorf("snake vs camel mismatch (-snake +camel):\n%s", diff)
	}
	if diff := cmp.Diff(snake, lower); diff != "" {
		t.Errorf("snake vs lower mismatch (-snake +lower):\n%s", diff)
	}
	if snake.Temperature == nil || *snake.Temperature != 0.7 {
		t.Errorf("Temperature: got %v, want 0.7", snake.Temperature)
	}
	if snake.MaxOutputTokens != 512 {
		t.Errorf("MaxOutputTokens: got %d, want 512", snake.MaxOutputTokens)
	}
}

func TestDecodeGenerateConfig_UnknownField(t *testing.T) {
	t.Parallel()
	data := "name: bot\nagent_class: LlmAgent\ngenerate_content_config:\n  fictitious_field: 1\n"
	_, err := config.Parse([]byte(data), "yaml")
	if !errors.Is(err, config.ErrUnknownField) {
		t.Fatalf("got %v, want ErrUnknownField", err)
	}
}

func TestDecodeGenerateConfig_AmbiguousField(t *testing.T) {
	t.Parallel()
	data := "name: bot\nagent_class: LlmAgent\ngenerate_content_config:\n  top_p: 0.9\n  topP: 0.9\n"
	_, err := config.Parse([]byte(data), "yaml")
	if !errors.Is(err, config.ErrAmbiguousField) {
		t.Fatalf("got %v, want ErrAmbiguousField", err)
	}
}

func TestDecodeGenerateConfig_MapKeysPreserved(t *testing.T) {
	t.Parallel()
	gc := parseGenConfig(t, `  labels:
    env_name: prod
  response_schema:
    type: object
    properties:
      user_name:
        type: string
        property_ordering: [a, b]
`)
	if gc.Labels["env_name"] != "prod" {
		t.Errorf("Labels: got %v, want key env_name preserved", gc.Labels)
	}
	prop, ok := gc.ResponseSchema.Properties["user_name"]
	if !ok {
		t.Fatalf("Properties keys not preserved: %v", gc.ResponseSchema.Properties)
	}
	if len(prop.PropertyOrdering) != 2 {
		t.Errorf("nested PropertyOrdering: got %v, want [a b]", prop.PropertyOrdering)
	}
}

func TestDecodeGenerateConfig_SystemInstructionTriShape(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value string
		check func(*genai.Content) bool
	}{
		{
			name:  "string",
			value: `  system_instruction: "Be concise"`,
			check: func(c *genai.Content) bool {
				return c.Role == "system" && len(c.Parts) == 1 && c.Parts[0].Text == "Be concise"
			},
		},
		{
			name:  "map",
			value: "  system_instruction:\n    role: system\n    parts:\n      - text: hi",
			check: func(c *genai.Content) bool {
				return c.Role == "system" && len(c.Parts) == 1 && c.Parts[0].Text == "hi"
			},
		},
		{
			name:  "list",
			value: "  system_instruction:\n    - text: hi",
			check: func(c *genai.Content) bool {
				return c.Role == "system" && len(c.Parts) == 1 && c.Parts[0].Text == "hi"
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gc := parseGenConfig(t, tc.value+"\n")
			if gc.SystemInstruction == nil {
				t.Fatal("SystemInstruction is nil")
			}
			if !tc.check(gc.SystemInstruction) {
				t.Errorf("SystemInstruction: got %+v", gc.SystemInstruction)
			}
		})
	}
}

func TestDecodeGenerateConfig_NestedStructs(t *testing.T) {
	t.Parallel()
	gc := parseGenConfig(t, `  safety_settings:
    - category: HARM_CATEGORY_HARASSMENT
      threshold: BLOCK_LOW_AND_ABOVE
  thinking_config:
    include_thoughts: true
  tool_config:
    function_calling_config:
      mode: ANY
  http_options:
    api_version: v1
`)
	if len(gc.SafetySettings) != 1 || gc.SafetySettings[0].Category != genai.HarmCategoryHarassment {
		t.Errorf("SafetySettings: got %+v", gc.SafetySettings)
	}
	if gc.ThinkingConfig == nil || !gc.ThinkingConfig.IncludeThoughts {
		t.Errorf("ThinkingConfig: got %+v", gc.ThinkingConfig)
	}
	if gc.ToolConfig == nil || gc.ToolConfig.FunctionCallingConfig == nil || gc.ToolConfig.FunctionCallingConfig.Mode != genai.FunctionCallingConfigModeAny {
		t.Errorf("ToolConfig: got %+v", gc.ToolConfig)
	}
	if gc.HTTPOptions == nil || gc.HTTPOptions.APIVersion != "v1" {
		t.Errorf("HTTPOptions: got %+v", gc.HTTPOptions)
	}
}

func TestDecodeGenerateConfig_NonMap(t *testing.T) {
	t.Parallel()
	data := "name: bot\nagent_class: LlmAgent\ngenerate_content_config: text\n"
	if _, err := config.Parse([]byte(data), "yaml"); err == nil {
		t.Fatal("got nil, want error for non-map generate_content_config")
	}
}

func TestDecodeGenerateConfig_NullValues(t *testing.T) {
	t.Parallel()
	gc := parseGenConfig(t, `  response_json_schema: null
  thinking_config: null
  system_instruction: null
  safety_settings: null
`)
	if gc.ResponseJsonSchema != nil || gc.ThinkingConfig != nil || gc.SystemInstruction != nil || gc.SafetySettings != nil {
		t.Errorf("null fields should stay unset: got %+v", gc)
	}
}

func TestDecodeGenerateConfig_NilWhenAbsent(t *testing.T) {
	t.Parallel()
	appCfg, err := config.Parse([]byte("name: bot\nagent_class: LlmAgent\n"), "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if gc := appCfg.AgentConfig.(*config.LLMAgentConfig).GenerateContentConfig; gc != nil {
		t.Errorf("GenerateContentConfig: got %+v, want nil", gc)
	}
}
