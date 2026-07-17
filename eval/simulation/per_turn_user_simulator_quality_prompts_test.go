package simulation

import (
	"strings"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
)

func TestGetPerTurnUserSimulatorQualityPrompt(t *testing.T) {
	prompt := GetPerTurnUserSimulatorQualityPrompt("plan", "history", "response", "[STOP]")
	if !strings.Contains(prompt, "plan") {
		t.Error("expected plan in prompt")
	}
	if !strings.Contains(prompt, "history") {
		t.Error("expected history in prompt")
	}
	if !strings.Contains(prompt, "response") {
		t.Error("expected response in prompt")
	}
	if !strings.Contains(prompt, "[STOP]") {
		t.Error("expected stop signal in prompt")
	}
}

func TestGetPerTurnUserSimulatorQualityPromptWithPersona(t *testing.T) {
	persona := &eval.UserPersona{
		Description: "Test persona",
		Behaviors: []eval.UserBehavior{
			{
				Name:                 "b1",
				Description:          "desc",
				BehaviorInstructions: []string{"instr1"},
				ViolationRubrics:     []string{"rubric1"},
			},
		},
	}
	prompt := GetPerTurnUserSimulatorQualityPromptWithPersona("plan", "history", "response", "[STOP]", persona)
	if !strings.Contains(prompt, "Test persona") {
		t.Error("expected persona description in prompt")
	}
	if !strings.Contains(prompt, "b1") {
		t.Error("expected behavior name in prompt")
	}
	if !strings.Contains(prompt, "instr1") {
		t.Error("expected behavior instructions in prompt")
	}
	if !strings.Contains(prompt, "rubric1") {
		t.Error("expected violation rubrics in prompt")
	}
}

func TestGetPerTurnUserSimulatorQualityPromptWithPersona_EmptyDescription(t *testing.T) {
	persona := &eval.UserPersona{Description: ""}
	prompt := GetPerTurnUserSimulatorQualityPromptWithPersona("plan", "history", "response", "[STOP]", persona)
	if !strings.Contains(prompt, "No specific persona.") {
		t.Error("expected default description for empty persona")
	}
}

func TestPerTurnUserSimulatorQualityPromptTemplate_HasPlaceholders(t *testing.T) {
	placeholders := []string{"{conversation_plan}", "{conversation_history}", "{generated_user_response}", "{stop_signal}"}
	for _, ph := range placeholders {
		if !strings.Contains(PerTurnUserSimulatorQualityPromptTemplate, ph) {
			t.Errorf("missing placeholder %s", ph)
		}
	}
}

func TestPerTurnUserSimulatorQualityWithPersonaPromptTemplate_HasPlaceholders(t *testing.T) {
	placeholders := []string{"{conversation_plan}", "{conversation_history}", "{generated_user_response}", "{stop_signal}", "{persona_description}", "{persona_behaviors}"}
	for _, ph := range placeholders {
		if !strings.Contains(PerTurnUserSimulatorQualityWithPersonaPromptTemplate, ph) {
			t.Errorf("missing placeholder %s", ph)
		}
	}
}
