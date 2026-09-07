package eval_test

import (
	"strings"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
)

func TestPrompts_HavePlaceholders(t *testing.T) {
	tests := []struct {
		name         string
		prompt       string
		placeholders []string
	}{
		{"FinalResponseMatchV2Prompt", eval.FinalResponseMatchV2Prompt, []string{"{prompt}", "{response}", "{golden_response}"}},
		{"RubricBasedFinalResponseQualityV1Prompt", eval.RubricBasedFinalResponseQualityV1Prompt, []string{"{user_prompt}", "{response_steps}", "{final_answer}", "{tool_declarations}", "{properties}"}},
		{"RubricBasedToolUseQualityV1Prompt", eval.RubricBasedToolUseQualityV1Prompt, []string{"{user_prompt}", "{tool_usage}", "{tool_declarations}", "{properties}"}},
		{"RubricBasedMultiTurnTrajectoryPrompt", eval.RubricBasedMultiTurnTrajectoryPrompt, []string{"{user_agent_dialogue}", "{agent_instructions}", "{agent_tool_definitions}", "{properties}"}},
		{"HallucinationSegmenterPrompt", eval.HallucinationSegmenterPrompt, []string{"{response}"}},
		{"HallucinationValidatorPrompt", eval.HallucinationValidatorPrompt, []string{"{sentences}", "{context}"}},
		{"PerTurnUserSimulatorQualityPrompt", eval.PerTurnUserSimulatorQualityPrompt, []string{"{conversation_plan}", "{conversation_history}", "{generated_user_response}", "{stop_signal}"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, ph := range tt.placeholders {
				if !strings.Contains(tt.prompt, ph) {
					t.Errorf("prompt %s missing placeholder %s", tt.name, ph)
				}
			}
		})
	}
}

func TestPrompts_NotEmpty(t *testing.T) {
	prompts := map[string]string{
		"FinalResponseMatchV2Prompt":              eval.FinalResponseMatchV2Prompt,
		"RubricBasedFinalResponseQualityV1Prompt": eval.RubricBasedFinalResponseQualityV1Prompt,
		"RubricBasedToolUseQualityV1Prompt":       eval.RubricBasedToolUseQualityV1Prompt,
		"RubricBasedMultiTurnTrajectoryPrompt":    eval.RubricBasedMultiTurnTrajectoryPrompt,
		"HallucinationSegmenterPrompt":            eval.HallucinationSegmenterPrompt,
		"HallucinationValidatorPrompt":            eval.HallucinationValidatorPrompt,
		"PerTurnUserSimulatorQualityPrompt":       eval.PerTurnUserSimulatorQualityPrompt,
	}
	for name, prompt := range prompts {
		if len(strings.TrimSpace(prompt)) == 0 {
			t.Errorf("prompt %s is empty", name)
		}
	}
}
