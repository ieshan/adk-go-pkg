package simulation

import (
	"context"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

func TestLlmBackedUserSimulator_StartingPrompt(t *testing.T) {
	scenario := &eval.ConversationScenario{
		StartingPrompt: "Hello, let's begin",
	}
	config := DefaultLlmBackedUserSimulatorConfig()
	fakeLLM := testutil.NewFakeLLM()
	sim := NewLlmBackedUserSimulator(config, scenario, fakeLLM)

	msg, err := sim.GetNextUserMessage(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetNextUserMessage failed: %v", err)
	}
	if msg.Status != eval.UserSimulatorStatusSuccess {
		t.Errorf("expected eval.UserSimulatorStatusSuccess, got %v", msg.Status)
	}
	if msg.UserMessage == nil {
		t.Fatal("expected non-nil UserMessage")
	}
	text := ""
	for _, p := range msg.UserMessage.Parts {
		text += p.Text
	}
	if text != "Hello, let's begin" {
		t.Errorf("expected starting prompt, got %q", text)
	}
}

func TestLlmBackedUserSimulator_TurnLimitReached(t *testing.T) {
	config := LlmBackedUserSimulatorConfig{
		Type:                  "llm_backed",
		MaxAllowedInvocations: 1,
	}
	fakeLLM := testutil.NewFakeLLM()
	sim := NewLlmBackedUserSimulator(config, &eval.ConversationScenario{StartingPrompt: "start"}, fakeLLM)

	// First call returns starting prompt (count becomes 1).
	_, _ = sim.GetNextUserMessage(context.Background(), nil)
	// Second call should hit limit.
	msg, err := sim.GetNextUserMessage(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetNextUserMessage failed: %v", err)
	}
	if msg.Status != eval.UserSimulatorStatusTurnLimitReached {
		t.Errorf("expected eval.UserSimulatorStatusTurnLimitReached, got %v", msg.Status)
	}
}

func TestLlmBackedUserSimulator_StopSignal(t *testing.T) {
	config := DefaultLlmBackedUserSimulatorConfig()
	fakeLLM := testutil.NewFakeLLM(
		testutil.NewTextResponse("</finished>"),
	)
	sim := NewLlmBackedUserSimulator(config, nil, fakeLLM)

	msg, err := sim.GetNextUserMessage(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetNextUserMessage failed: %v", err)
	}
	if msg.Status != eval.UserSimulatorStatusStopSignalDetected {
		t.Errorf("expected eval.UserSimulatorStatusStopSignalDetected, got %v", msg.Status)
	}
}

func TestLlmBackedUserSimulator_GetSimulationEvaluator(t *testing.T) {
	sim := NewLlmBackedUserSimulator(DefaultLlmBackedUserSimulatorConfig(), nil, testutil.NewFakeLLM())
	ev, err := sim.GetSimulationEvaluator()
	if err == nil {
		t.Fatal("expected error from GetSimulationEvaluator")
	}
	if ev != nil {
		t.Error("expected nil evaluator")
	}
}

func TestLlmBackedUserSimulatorConfig_ValidateCustomInstructions(t *testing.T) {
	tests := []struct {
		name    string
		config  LlmBackedUserSimulatorConfig
		wantErr bool
	}{
		{
			name: "valid instructions",
			config: LlmBackedUserSimulatorConfig{
				CustomInstructions: "{{.Input.stop_signal}} {{.Input.conversation_plan}} {{.Input.conversation_history}}",
			},
			wantErr: false,
		},
		{
			name: "missing stop_signal",
			config: LlmBackedUserSimulatorConfig{
				CustomInstructions: "{{.Input.conversation_plan}} {{.Input.conversation_history}}",
			},
			wantErr: true,
		},
		{
			name:    "empty instructions",
			config:  LlmBackedUserSimulatorConfig{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.ValidateCustomInstructions()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCustomInstructions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLlmBackedUserSimulator_NoMessageGenerated(t *testing.T) {
	config := DefaultLlmBackedUserSimulatorConfig()
	fakeLLM := testutil.NewFakeLLM(
		testutil.NewTextResponse(""),
	)
	sim := NewLlmBackedUserSimulator(config, nil, fakeLLM)

	msg, err := sim.GetNextUserMessage(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetNextUserMessage failed: %v", err)
	}
	if msg.Status != eval.UserSimulatorStatusNoMessageGenerated {
		t.Errorf("expected eval.UserSimulatorStatusNoMessageGenerated, got %v", msg.Status)
	}
}

func TestLlmBackedUserSimulator_SummarizeConversation(t *testing.T) {
	sim := NewLlmBackedUserSimulator(DefaultLlmBackedUserSimulatorConfig(), nil, testutil.NewFakeLLM())
	events := []*session.Event{
		{Author: "user", LLMResponse: model.LLMResponse{Content: &genai.Content{Parts: []*genai.Part{{Text: "hello"}}}}},
		{Author: "model", LLMResponse: model.LLMResponse{Content: &genai.Content{Parts: []*genai.Part{{Text: "hi there"}}}}},
	}
	summary := sim.summarizeConversation(events)
	if summary == "" {
		t.Error("expected non-empty summary")
	}
}
