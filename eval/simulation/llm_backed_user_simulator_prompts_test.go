package simulation_test

import (
	"strings"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
	"github.com/ieshan/adk-go-pkg/eval/simulation"
)

func TestGetLlmBackedUserSimulatorPrompt(t *testing.T) {
	prompt, err := simulation.GetLlmBackedUserSimulatorPrompt("test plan", "test history", "[STOP]")
	if err != nil {
		t.Fatalf("GetLlmBackedUserSimulatorPrompt failed: %v", err)
	}
	if !strings.Contains(prompt, "test plan") {
		t.Errorf("got prompt without test plan, want test plan in prompt")
	}
	if !strings.Contains(prompt, "test history") {
		t.Errorf("got prompt without test history, want test history in prompt")
	}
	if !strings.Contains(prompt, "[STOP]") {
		t.Errorf("got prompt without [STOP], want [STOP] in prompt")
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
	prompt, err := simulation.GetLlmBackedUserSimulatorPromptWithPersona("plan", "history", "[STOP]", persona)
	if err != nil {
		t.Fatalf("GetLlmBackedUserSimulatorPromptWithPersona failed: %v", err)
	}
	if !strings.Contains(prompt, "A test persona") {
		t.Errorf("got prompt without persona description, want persona description in prompt")
	}
	if !strings.Contains(prompt, "behavior1") {
		t.Errorf("got prompt without behavior1, want behavior1 in prompt")
	}
	if !strings.Contains(prompt, "do X") {
		t.Errorf("got prompt without behavior instructions, want behavior instructions in prompt")
	}
	if !strings.Contains(prompt, "don't Z") {
		t.Errorf("got prompt without violation rubrics, want violation rubrics in prompt")
	}
}

func TestGetLlmBackedUserSimulatorPromptWithPersona_EmptyDescription(t *testing.T) {
	persona := &eval.UserPersona{
		Description: "",
	}
	prompt, err := simulation.GetLlmBackedUserSimulatorPromptWithPersona("plan", "history", "[STOP]", persona)
	if err != nil {
		t.Fatalf("GetLlmBackedUserSimulatorPromptWithPersona failed: %v", err)
	}
	if !strings.Contains(prompt, "No specific persona.") {
		t.Errorf("got prompt without default description, want default description for empty persona")
	}
}

func TestDefaultUserSimulatorInstructionsTemplate_HasPlaceholders(t *testing.T) {
	placeholders := []string{"{{.Input.conversation_plan}}", "{{.Input.conversation_history}}", "{{.Input.stop_signal}}"}
	for _, ph := range placeholders {
		if !strings.Contains(simulation.DefaultUserSimulatorInstructionsTemplate, ph) {
			t.Errorf("missing placeholder %s in default template", ph)
		}
	}
}

func TestUserSimulatorInstructionsWithPersonaTemplate_HasPlaceholders(t *testing.T) {
	placeholders := []string{"{{.Input.conversation_plan}}", "{{.Input.conversation_history}}", "{{.Input.stop_signal}}", "{{.Input.persona_description}}", "{{.Input.persona_behaviors}}"}
	for _, ph := range placeholders {
		if !strings.Contains(simulation.UserSimulatorInstructionsWithPersonaTemplate, ph) {
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
			got := simulation.IsValidUserSimulatorTemplate(tt.template, tt.required)
			if got != tt.want {
				t.Errorf("IsValidUserSimulatorTemplate() = %v, want %v", got, tt.want)
			}
		})
	}
}

// FuzzIsValidUserSimulatorTemplate verifies that
// simulation.IsValidUserSimulatorTemplate never panics on arbitrary string
// input. Both true and false outcomes are acceptable as long as no panic
// occurred.
func FuzzIsValidUserSimulatorTemplate(f *testing.F) {
	// Seed: valid template with all required placeholders.
	f.Add("Plan: {{.Input.conversation_plan}}, History: {{.Input.conversation_history}}, Stop: {{.Input.stop_signal}}")
	// Seed: invalid template (no placeholders).
	f.Add("no placeholders here")
	// Seed: empty string.
	f.Add("")

	f.Fuzz(func(t *testing.T, input string) {
		_ = simulation.IsValidUserSimulatorTemplate(input, []string{"conversation_plan", "conversation_history", "stop_signal"})
		// The function must not panic — reaching here is the primary assertion.
		// Both true and false outcomes are acceptable.
	})
}
