package simulation

import (
	"context"
	"errors"
	"strings"
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
		t.Errorf("got %v, want eval.UserSimulatorStatusSuccess", msg.Status)
	}
	if msg.UserMessage == nil {
		t.Fatal("got nil UserMessage, want non-nil")
	}
	text := ""
	for _, p := range msg.UserMessage.Parts {
		text += p.Text
	}
	if text != "Hello, let's begin" {
		t.Errorf("got %q, want starting prompt", text)
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
	// error intentionally ignored: testing turn-limit behavior on second call
	_, _ = sim.GetNextUserMessage(context.Background(), nil)
	// Second call should hit limit.
	msg, err := sim.GetNextUserMessage(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetNextUserMessage failed: %v", err)
	}
	if msg.Status != eval.UserSimulatorStatusTurnLimitReached {
		t.Errorf("got %v, want eval.UserSimulatorStatusTurnLimitReached", msg.Status)
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
		t.Errorf("got %v, want eval.UserSimulatorStatusStopSignalDetected", msg.Status)
	}
}

func TestLlmBackedUserSimulator_GetSimulationEvaluator(t *testing.T) {
	sim := NewLlmBackedUserSimulator(DefaultLlmBackedUserSimulatorConfig(), nil, testutil.NewFakeLLM())
	ev, err := sim.GetSimulationEvaluator()
	if !errors.Is(err, ErrSimulationEvaluatorNotImplemented) {
		t.Fatalf("got %v, want ErrSimulationEvaluatorNotImplemented", err)
	}
	if ev != nil {
		t.Error("got non-nil evaluator, want nil")
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
		t.Errorf("got %v, want eval.UserSimulatorStatusNoMessageGenerated", msg.Status)
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
		t.Error("got empty summary, want non-empty")
	}
}

// TestLlmBackedUserSimulator_SummarizeConversation_EdgeCases verifies that
// summarizeConversation handles edge-case inputs without panicking: nil
// events, nil events within the slice, empty Author, empty Parts,
// FunctionCall parts, FunctionResponse parts, and unicode content.
func TestLlmBackedUserSimulator_SummarizeConversation_EdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		config   LlmBackedUserSimulatorConfig
		events   []*session.Event
		want     string
		contains string
	}{
		{
			name:   "nil events slice",
			config: DefaultLlmBackedUserSimulatorConfig(),
			events: nil,
			want:   "",
		},
		{
			name:   "nil event within slice",
			config: DefaultLlmBackedUserSimulatorConfig(),
			events: []*session.Event{
				nil,
				{Author: "user", LLMResponse: model.LLMResponse{Content: &genai.Content{Parts: []*genai.Part{{Text: "hello"}}}}},
			},
			want: "user: hello",
		},
		{
			name:   "empty Author defaults to user",
			config: DefaultLlmBackedUserSimulatorConfig(),
			events: []*session.Event{
				{Author: "", LLMResponse: model.LLMResponse{Content: &genai.Content{Parts: []*genai.Part{{Text: "no author"}}}}},
			},
			want: "user: no author",
		},
		{
			name:   "empty Parts in content",
			config: DefaultLlmBackedUserSimulatorConfig(),
			events: []*session.Event{
				{Author: "user", LLMResponse: model.LLMResponse{Content: &genai.Content{Parts: []*genai.Part{}}}},
			},
			want: "",
		},
		{
			name:   "nil Content skipped",
			config: DefaultLlmBackedUserSimulatorConfig(),
			events: []*session.Event{
				{Author: "user", LLMResponse: model.LLMResponse{Content: nil}},
				{Author: "model", LLMResponse: model.LLMResponse{Content: &genai.Content{Parts: []*genai.Part{{Text: "ok"}}}}},
			},
			want: "model: ok",
		},
		{
			name: "FunctionCall part included when enabled",
			config: func() LlmBackedUserSimulatorConfig {
				c := DefaultLlmBackedUserSimulatorConfig()
				c.IncludeFunctionCalls = true
				return c
			}(),
			events: []*session.Event{
				{Author: "model", LLMResponse: model.LLMResponse{Content: &genai.Content{Parts: []*genai.Part{
					{FunctionCall: &genai.FunctionCall{Name: "search", Args: map[string]any{"q": "test"}}},
				}}}},
			},
			contains: "[Function call: search(map[q:test])]",
		},
		{
			name:   "FunctionCall part excluded when disabled",
			config: DefaultLlmBackedUserSimulatorConfig(),
			events: []*session.Event{
				{Author: "model", LLMResponse: model.LLMResponse{Content: &genai.Content{Parts: []*genai.Part{
					{FunctionCall: &genai.FunctionCall{Name: "search", Args: map[string]any{"q": "test"}}},
				}}}},
			},
			want: "",
		},
		{
			name: "FunctionResponse part included when enabled",
			config: func() LlmBackedUserSimulatorConfig {
				c := DefaultLlmBackedUserSimulatorConfig()
				c.IncludeFunctionCalls = true
				return c
			}(),
			events: []*session.Event{
				{Author: "model", LLMResponse: model.LLMResponse{Content: &genai.Content{Parts: []*genai.Part{
					{FunctionResponse: &genai.FunctionResponse{Name: "search"}},
				}}}},
			},
			contains: "[Function response: search]",
		},
		{
			name:   "FunctionResponse part excluded when disabled",
			config: DefaultLlmBackedUserSimulatorConfig(),
			events: []*session.Event{
				{Author: "model", LLMResponse: model.LLMResponse{Content: &genai.Content{Parts: []*genai.Part{
					{FunctionResponse: &genai.FunctionResponse{Name: "search"}},
				}}}},
			},
			want: "",
		},
		{
			name:   "unicode content preserved",
			config: DefaultLlmBackedUserSimulatorConfig(),
			events: []*session.Event{
				{Author: "ユーザー", LLMResponse: model.LLMResponse{Content: &genai.Content{Parts: []*genai.Part{{Text: "こんにちは世界"}}}}},
			},
			want: "ユーザー: こんにちは世界",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sim := NewLlmBackedUserSimulator(tt.config, nil, testutil.NewFakeLLM())
			// Reaching here without panicking is the primary assertion.
			got := sim.summarizeConversation(tt.events)
			if tt.contains != "" {
				if !strings.Contains(got, tt.contains) {
					t.Errorf("summarizeConversation: got %q, want it to contain %q", got, tt.contains)
				}
				return
			}
			if got != tt.want {
				t.Errorf("summarizeConversation: got %q, want %q", got, tt.want)
			}
		})
	}
}
