package eval_test

import (
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
)

func TestConversationScenario_StartingPrompt(t *testing.T) {
	scenario := &eval.ConversationScenario{
		StartingPrompt:   "Hello",
		ConversationPlan: "Plan A",
		UserPersona:      &eval.UserPersona{ID: "EXPERT"},
	}

	if scenario.StartingPrompt != "Hello" {
		t.Errorf("got %s, want Hello", scenario.StartingPrompt)
	}
	if scenario.ConversationPlan != "Plan A" {
		t.Errorf("got %s, want Plan A", scenario.ConversationPlan)
	}
	if scenario.UserPersona == nil || scenario.UserPersona.ID != "EXPERT" {
		t.Errorf("got persona %v, want EXPERT", scenario.UserPersona)
	}
}

func TestConversationGenerationConfig_Defaults(t *testing.T) {
	config := eval.ConversationGenerationConfig{
		Count: 5,
	}
	if config.Count != 5 {
		t.Errorf("got %d, want 5", config.Count)
	}
}
