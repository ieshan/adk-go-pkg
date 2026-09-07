package simulation_test

import (
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
	"github.com/ieshan/adk-go-pkg/eval/simulation"
	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/genai"
)

// newTestProvider returns a UserSimulatorProvider backed by a FakeLLM with
// the default LLM-backed config. Consolidates the duplicated construction
// across all provider tests.
func newTestProvider() *simulation.UserSimulatorProvider {
	return simulation.NewUserSimulatorProvider(testutil.NewFakeLLM(), simulation.DefaultLlmBackedUserSimulatorConfig())
}

func TestUserSimulatorProvider_StaticConversation(t *testing.T) {
	t.Parallel()
	provider := newTestProvider()
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
		t.Fatalf("got nil simulator, want non-nil")
	}
}

func TestUserSimulatorProvider_BothConversationAndScenario(t *testing.T) {
	t.Parallel()
	provider := newTestProvider()
	evalCase := eval.EvalCase{
		Conversation: []eval.Invocation{
			{UserContent: &genai.Content{Parts: []*genai.Part{{Text: "hello"}}, Role: "user"}},
		},
		ConversationScenario: &eval.ConversationScenario{StartingPrompt: "start"},
	}

	_, err := provider.Provide(evalCase)
	if err == nil {
		t.Errorf("got nil error, want non-nil error when both conversation and scenario are provided")
	}
}

func TestUserSimulatorProvider_NeitherProvided(t *testing.T) {
	t.Parallel()
	provider := newTestProvider()
	evalCase := eval.EvalCase{}

	_, err := provider.Provide(evalCase)
	if err == nil {
		t.Errorf("got nil error, want non-nil error when neither conversation nor scenario is provided")
	}
}

func TestUserSimulatorProvider_LlmBacked(t *testing.T) {
	t.Parallel()
	provider := newTestProvider()
	evalCase := eval.EvalCase{
		ConversationScenario: &eval.ConversationScenario{StartingPrompt: "start"},
	}

	sim, err := provider.Provide(evalCase)
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}
	if sim == nil {
		t.Fatalf("got nil simulator, want non-nil")
	}
}

func TestUserSimulatorProvider_UnknownType(t *testing.T) {
	t.Parallel()
	provider := simulation.NewUserSimulatorProvider(testutil.NewFakeLLM(), simulation.LlmBackedUserSimulatorConfig{Type: "unknown"})
	evalCase := eval.EvalCase{
		ConversationScenario: &eval.ConversationScenario{StartingPrompt: "start"},
	}

	_, err := provider.Provide(evalCase)
	if err == nil {
		t.Errorf("got nil error, want non-nil error for unknown config type")
	}
}
