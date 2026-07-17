package simulation

import (
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/genai"
)

func TestUserSimulatorProvider_StaticConversation(t *testing.T) {
	provider := NewUserSimulatorProvider(testutil.NewFakeLLM(), DefaultLlmBackedUserSimulatorConfig())
	evalCase := eval.EvalCase{
		Conversation: []eval.Invocation{
			{UserContent: &genai.Content{Parts: []*genai.Part{{Text: "hello"}}, Role: "user"}},
		},
	}

	sim, err := provider.Provide(evalCase)
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}
	if sim == nil {
		t.Fatal("expected non-nil simulator")
	}
}

func TestUserSimulatorProvider_BothConversationAndScenario(t *testing.T) {
	provider := NewUserSimulatorProvider(testutil.NewFakeLLM(), DefaultLlmBackedUserSimulatorConfig())
	evalCase := eval.EvalCase{
		Conversation: []eval.Invocation{
			{UserContent: &genai.Content{Parts: []*genai.Part{{Text: "hello"}}, Role: "user"}},
		},
		ConversationScenario: &eval.ConversationScenario{StartingPrompt: "start"},
	}

	_, err := provider.Provide(evalCase)
	if err == nil {
		t.Error("expected error when both conversation and scenario are provided")
	}
}

func TestUserSimulatorProvider_NeitherProvided(t *testing.T) {
	provider := NewUserSimulatorProvider(testutil.NewFakeLLM(), DefaultLlmBackedUserSimulatorConfig())
	evalCase := eval.EvalCase{}

	_, err := provider.Provide(evalCase)
	if err == nil {
		t.Error("expected error when neither conversation nor scenario is provided")
	}
}

func TestUserSimulatorProvider_LlmBacked(t *testing.T) {
	provider := NewUserSimulatorProvider(testutil.NewFakeLLM(), DefaultLlmBackedUserSimulatorConfig())
	evalCase := eval.EvalCase{
		ConversationScenario: &eval.ConversationScenario{StartingPrompt: "start"},
	}

	sim, err := provider.Provide(evalCase)
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}
	if sim == nil {
		t.Fatal("expected non-nil simulator")
	}
}

func TestUserSimulatorProvider_UnknownType(t *testing.T) {
	provider := NewUserSimulatorProvider(testutil.NewFakeLLM(), LlmBackedUserSimulatorConfig{Type: "unknown"})
	evalCase := eval.EvalCase{
		ConversationScenario: &eval.ConversationScenario{StartingPrompt: "start"},
	}

	_, err := provider.Provide(evalCase)
	if err == nil {
		t.Error("expected error for unknown config type")
	}
}
