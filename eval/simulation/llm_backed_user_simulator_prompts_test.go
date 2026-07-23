package simulation

import (
	"strings"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
)

func TestGetLlmBackedUserSimulatorPrompt(t *testing.T) {
	prompt, err := GetLlmBackedUserSimulatorPrompt("test plan", "test history", "[STOP]")
	if err != nil {
		t.Fatalf("GetLlmBackedUserSimulatorPrompt failed: %v", err)
	}
	if !strings.Contains(prompt, "test plan") {
		t.Error("expected plan in prompt")
	}
	if !strings.Contains(prompt, "test history") {
		t.Error("expected history in prompt")
	}
	if !strings.Contains(prompt, "[STOP]") {
		t.Error("expected stop signal in prompt")
	}
}

func TestGetLlmBackedUserSimulatorPromptWithPersona(t *testing.T) {
	persona := &eval.UserPersona{
		Description: "A test persona",
		Behaviors: []eval.UserBehavior{
			{
				Name:                 "behavior1",
				Description:          "test behavior",
				BehaviorInstructions: []string{"do X", "do Y"},
				ViolationRubrics:     []string{"don't Z"},
			},
		},
	}
	prompt, err := GetLlmBackedUserSimulatorPromptWithPersona("plan", "history", "[STOP]", persona)
	if err != nil {
		t.Fatalf("GetLlmBackedUserSimulatorPromptWithPersona failed: %v", err)
	}
	if !strings.Contains(prompt, "A test persona") {
		t.Error("expected persona description in prompt")
	}
	if !strings.Contains(prompt, "behavior1") {
		t.Error("expected behavior name in prompt")
	}
	if !strings.Contains(prompt, "do X") {
		t.Error("expected behavior instructions in prompt")
	}
	if !strings.Contains(prompt, "don't Z") {
		t.Error("expected violation rubrics in prompt")
	}
}

func TestGetLlmBackedUserSimulatorPromptWithPersona_EmptyDescription(t *testing.T) {
	persona := &eval.UserPersona{
		Description: "",
	}
	prompt, err := GetLlmBackedUserSimulatorPromptWithPersona("plan", "history", "[STOP]", persona)
	if err != nil {
		t.Fatalf("GetLlmBackedUserSimulatorPromptWithPersona failed: %v", err)
	}
	if !strings.Contains(prompt, "No specific persona.") {
		t.Error("expected default description for empty persona")
	}
}

func TestDefaultUserSimulatorInstructionsTemplate_HasPlaceholders(t *testing.T) {
	placeholders := []string{"{{.Input.conversation_plan}}", "{{.Input.conversation_history}}", "{{.Input.stop_signal}}"}
	for _, ph := range placeholders {
		if !strings.Contains(DefaultUserSimulatorInstructionsTemplate, ph) {
			t.Errorf("missing placeholder %s in default template", ph)
		}
	}
}

func TestUserSimulatorInstructionsWithPersonaTemplate_HasPlaceholders(t *testing.T) {
	placeholders := []string{"{{.Input.conversation_plan}}", "{{.Input.conversation_history}}", "{{.Input.stop_signal}}", "{{.Input.persona_description}}", "{{.Input.persona_behaviors}}"}
	for _, ph := range placeholders {
		if !strings.Contains(UserSimulatorInstructionsWithPersonaTemplate, ph) {
			t.Errorf("missing placeholder %s in persona template", ph)
		}
	}
}

func TestIsValidUserSimulatorTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		required []string
		want     bool
	}{
		{
			"all_present",
			"Plan: {{.Input.conversation_plan}}, History: {{.Input.conversation_history}}, Stop: {{.Input.stop_signal}}",
			[]string{"conversation_plan", "conversation_history", "stop_signal"},
			true,
		},
		{
			"missing_one",
			"Plan: {{.Input.conversation_plan}}, History: {{.Input.conversation_history}}",
			[]string{"conversation_plan", "conversation_history", "stop_signal"},
			false,
		},
		{
			"none_present",
			"no placeholders here",
			[]string{"conversation_plan"},
			false,
		},
		{
			"empty_required",
			"no placeholders here",
			[]string{},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidUserSimulatorTemplate(tt.template, tt.required)
			if got != tt.want {
				t.Errorf("IsValidUserSimulatorTemplate() = %v, want %v", got, tt.want)
			}
		})
	}
}
