package eval

import (
	"context"
	"testing"

	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/genai"
)

func TestPerTurnUserSimulatorQualityV1Evaluator_FirstTurnExactMatch(t *testing.T) {
	threshold := 0.8
	fakeLLM := testutil.NewFakeLLM(testutil.NewTextResponse(`{"is_valid": true}`))
	evalMetric := EvalMetric{
		MetricName: string(PerTurnUserSimulatorQualityV1),
		Threshold:  &threshold,
		Criterion: &LlmBackedUserSimulatorCriterion{
			LlmAsAJudgeCriterion: LlmAsAJudgeCriterion{JudgeModelOptions: JudgeModelOptions{NumSamples: 1}},
		},
	}
	e, err := NewPerTurnUserSimulatorQualityV1Evaluator(evalMetric, fakeLLM)
	if err != nil {
		t.Fatalf("NewPerTurnUserSimulatorQualityV1Evaluator failed: %v", err)
	}

	actual := []Invocation{{
		UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "Hello, what can you do?"}}},
	}}

	result, err := e.EvaluateInvocations(context.Background(), actual, nil, &ConversationScenario{
		StartingPrompt:   "Hello, what can you do?",
		ConversationPlan: "Ask about weather, then ask about news.",
	})
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusPassed {
		t.Errorf("first turn exact match should PASSED, got %v", result.OverallEvalStatus)
	}
	if result.PerInvocationResults[0].Score == nil || *result.PerInvocationResults[0].Score != 1.0 {
		t.Errorf("first turn exact match should score 1.0")
	}
}

func TestPerTurnUserSimulatorQualityV1Evaluator_FirstTurnMismatch(t *testing.T) {
	threshold := 0.8
	fakeLLM := testutil.NewFakeLLM(testutil.NewTextResponse(`{"is_valid": true}`))
	evalMetric := EvalMetric{
		MetricName: string(PerTurnUserSimulatorQualityV1),
		Threshold:  &threshold,
		Criterion: &LlmBackedUserSimulatorCriterion{
			LlmAsAJudgeCriterion: LlmAsAJudgeCriterion{JudgeModelOptions: JudgeModelOptions{NumSamples: 1}},
		},
	}
	e, err := NewPerTurnUserSimulatorQualityV1Evaluator(evalMetric, fakeLLM)
	if err != nil {
		t.Fatalf("NewPerTurnUserSimulatorQualityV1Evaluator failed: %v", err)
	}

	actual := []Invocation{{
		UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "random text"}}},
	}}

	result, err := e.EvaluateInvocations(context.Background(), actual, nil, &ConversationScenario{
		StartingPrompt:   "Hello, what can you do?",
		ConversationPlan: "Ask about weather.",
	})
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusFailed {
		t.Errorf("first turn mismatch should FAILED, got %v", result.OverallEvalStatus)
	}
}

func TestPerTurnUserSimulatorQualityV1Evaluator_ValidResponse(t *testing.T) {
	threshold := 0.8
	fakeLLM := testutil.NewFakeLLM(testutil.NewTextResponse(`{"is_valid": true}`))
	evalMetric := EvalMetric{
		MetricName: string(PerTurnUserSimulatorQualityV1),
		Threshold:  &threshold,
		Criterion: &LlmBackedUserSimulatorCriterion{
			LlmAsAJudgeCriterion: LlmAsAJudgeCriterion{JudgeModelOptions: JudgeModelOptions{NumSamples: 1}},
		},
	}
	e, err := NewPerTurnUserSimulatorQualityV1Evaluator(evalMetric, fakeLLM)
	if err != nil {
		t.Fatalf("NewPerTurnUserSimulatorQualityV1Evaluator failed: %v", err)
	}

	actual := []Invocation{
		{UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "starting prompt"}}}},
		{UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "what is the weather?"}}}},
	}

	result, err := e.EvaluateInvocations(context.Background(), actual, nil, &ConversationScenario{
		StartingPrompt:   "starting prompt",
		ConversationPlan: "Ask about weather, then ask about news.",
	})
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusPassed {
		t.Errorf("OverallEvalStatus = %v, want PASSED", result.OverallEvalStatus)
	}
}

func TestPerTurnUserSimulatorQualityV1Evaluator_InvalidResponse(t *testing.T) {
	threshold := 0.8
	fakeLLM := testutil.NewFakeLLM(testutil.NewTextResponse(`{"is_valid": false}`))
	evalMetric := EvalMetric{
		MetricName: string(PerTurnUserSimulatorQualityV1),
		Threshold:  &threshold,
		Criterion: &LlmBackedUserSimulatorCriterion{
			LlmAsAJudgeCriterion: LlmAsAJudgeCriterion{JudgeModelOptions: JudgeModelOptions{NumSamples: 1}},
		},
	}
	e, err := NewPerTurnUserSimulatorQualityV1Evaluator(evalMetric, fakeLLM)
	if err != nil {
		t.Fatalf("NewPerTurnUserSimulatorQualityV1Evaluator failed: %v", err)
	}

	actual := []Invocation{
		{UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "starting prompt"}}}},
		{UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "random text"}}}},
	}

	result, err := e.EvaluateInvocations(context.Background(), actual, nil, &ConversationScenario{
		StartingPrompt:   "starting prompt",
		ConversationPlan: "Ask about weather.",
	})
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusFailed {
		t.Errorf("OverallEvalStatus = %v, want FAILED", result.OverallEvalStatus)
	}
}

func TestPerTurnUserSimulatorQualityV1Evaluator_NilScenario(t *testing.T) {
	threshold := 0.8
	fakeLLM := testutil.NewFakeLLM(testutil.NewTextResponse(`{"is_valid": true}`))
	evalMetric := EvalMetric{
		MetricName: string(PerTurnUserSimulatorQualityV1),
		Threshold:  &threshold,
		Criterion: &LlmBackedUserSimulatorCriterion{
			LlmAsAJudgeCriterion: LlmAsAJudgeCriterion{JudgeModelOptions: JudgeModelOptions{NumSamples: 1}},
		},
	}
	e, err := NewPerTurnUserSimulatorQualityV1Evaluator(evalMetric, fakeLLM)
	if err != nil {
		t.Fatalf("NewPerTurnUserSimulatorQualityV1Evaluator failed: %v", err)
	}

	actual := []Invocation{{
		UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "hi"}}},
	}}

	_, err = e.EvaluateInvocations(context.Background(), actual, nil, nil)
	if err == nil {
		t.Error("expected error when conversation_scenario is nil")
	}
}

func TestParseIsValidLabel(t *testing.T) {
	tests := []struct {
		name string
		resp string
		want Label
	}{
		{"valid_true", `{"is_valid": true}`, LabelValid},
		{"valid_string", `{"is_valid": "valid"}`, LabelValid},
		{"invalid_false", `{"is_valid": false}`, LabelInvalid},
		{"invalid_string", `{"is_valid": "invalid"}`, LabelInvalid},
		{"invalid_almost", `{"is_valid": "almost"}`, LabelInvalid},
		{"invalid_partially", `{"is_valid": "partially"}`, LabelInvalid},
		{"not_found_missing", `{"other": "value"}`, LabelNotFound},
		{"not_found_garbage", `the quick brown fox`, LabelNotFound},
		{"valid_in_array", `{"is_valid": [true]}`, LabelValid},
		{"invalid_in_array", `{"is_valid": [false]}`, LabelInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseIsValidLabel(tt.resp)
			if got != tt.want {
				t.Errorf("parseIsValidLabel(%q) = %v, want %v", tt.resp, got, tt.want)
			}
		})
	}
}
