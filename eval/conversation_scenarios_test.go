package eval

import (
	"testing"
)

func TestConversationScenario_StartingPrompt(t *testing.T) {
	scenario := &ConversationScenario{
		StartingPrompt:   "Hello",
		ConversationPlan: "Plan A",
		UserPersona:      &UserPersona{ID: "EXPERT"},
	}

	if scenario.StartingPrompt != "Hello" {
		t.Errorf("got %s, want Hello", scenario.StartingPrompt)
	}
	if scenario.ConversationPlan != "Plan A" {
		t.Errorf("got %s, want Plan A", scenario.ConversationPlan)
	}
	if scenario.UserPersona == nil || scenario.UserPersona.ID != "EXPERT" {
		t.Error("expected EXPERT persona")
	}
}

func TestConversationGenerationConfig_Defaults(t *testing.T) {
	config := ConversationGenerationConfig{
		Count: 5,
	}
	if config.Count != 5 {
		t.Errorf("expected 5, got %d", config.Count)
	}
}
