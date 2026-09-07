package simulation_test

import (
	"strings"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
	"github.com/ieshan/adk-go-pkg/eval/simulation"
)

func TestGetPerTurnUserSimulatorQualityPrompt(t *testing.T) {
	prompt := simulation.GetPerTurnUserSimulatorQualityPrompt("plan", "history", "response", "[STOP]")
	if !strings.Contains(prompt, "plan") {
		t.Errorf("got prompt without plan, want plan in prompt")
	}
	if !strings.Contains(prompt, "history") {
		t.Errorf("got prompt without history, want history in prompt")
	}
	if !strings.Contains(prompt, "response") {
		t.Errorf("got prompt without response, want response in prompt")
	}
	if !strings.Contains(prompt, "[STOP]") {
		t.Errorf("got prompt without [STOP], want [STOP] in prompt")
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
	prompt := simulation.GetPerTurnUserSimulatorQualityPromptWithPersona("plan", "history", "response", "[STOP]", persona)
	if !strings.Contains(prompt, "Test persona") {
		t.Errorf("got prompt without Test persona, want Test persona in prompt")
	}
	if !strings.Contains(prompt, "b1") {
		t.Errorf("got prompt without b1, want b1 in prompt")
	}
	if !strings.Contains(prompt, "instr1") {
		t.Errorf("got prompt without instr1, want instr1 in prompt")
	}
	if !strings.Contains(prompt, "rubric1") {
		t.Errorf("got prompt without rubric1, want rubric1 in prompt")
	}
}

func TestGetPerTurnUserSimulatorQualityPromptWithPersona_EmptyDescription(t *testing.T) {
	persona := &eval.UserPersona{Description: ""}
	prompt := simulation.GetPerTurnUserSimulatorQualityPromptWithPersona("plan", "history", "response", "[STOP]", persona)
	if !strings.Contains(prompt, "No specific persona.") {
		t.Errorf("got prompt without default description, want default description for empty persona")
	}
}

func TestPerTurnUserSimulatorQualityPromptTemplate_HasPlaceholders(t *testing.T) {
	placeholders := []string{"{conversation_plan}", "{conversation_history}", "{generated_user_response}", "{stop_signal}"}
	for _, ph := range placeholders {
		if !strings.Contains(simulation.PerTurnUserSimulatorQualityPromptTemplate, ph) {
			t.Errorf("missing placeholder %s", ph)
		}
	}
}

func TestPerTurnUserSimulatorQualityWithPersonaPromptTemplate_HasPlaceholders(t *testing.T) {
	placeholders := []string{"{conversation_plan}", "{conversation_history}", "{generated_user_response}", "{stop_signal}", "{persona_description}", "{persona_behaviors}"}
	for _, ph := range placeholders {
		if !strings.Contains(simulation.PerTurnUserSimulatorQualityWithPersonaPromptTemplate, ph) {
			t.Errorf("missing placeholder %s", ph)
		}
	}
}
