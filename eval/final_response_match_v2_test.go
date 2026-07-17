package eval

import (
	"context"
	"testing"

	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/genai"
)

func TestFinalResponseMatchV2Evaluator_ValidResponse(t *testing.T) {
	threshold := 0.8
	fakeLLM := testutil.NewFakeLLM(testutil.NewTextResponse(
		`{"reasoning": "looks good", "is_the_agent_response_valid": "valid"}`,
	))
	evalMetric := EvalMetric{
		MetricName: string(FinalResponseMatchV2),
		Threshold:  &threshold,
		Criterion:  &LlmAsAJudgeCriterion{JudgeModelOptions: JudgeModelOptions{NumSamples: 1}},
	}
	e := NewFinalResponseMatchV2Evaluator(evalMetric, fakeLLM)

	actual := []Invocation{{
		UserContent:   &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "what is 2+2?"}}},
		FinalResponse: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "4"}}},
	}}
	expected := []Invocation{{
		UserContent:   &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "what is 2+2?"}}},
		FinalResponse: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "4"}}},
	}}

	result, err := e.EvaluateInvocations(context.Background(), actual, expected, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusPassed {
		t.Errorf("OverallEvalStatus = %v, want PASSED", result.OverallEvalStatus)
	}
}

func TestFinalResponseMatchV2Evaluator_InvalidResponse(t *testing.T) {
	threshold := 0.8
	fakeLLM := testutil.NewFakeLLM(testutil.NewTextResponse(
		`{"reasoning": "wrong answer", "is_the_agent_response_valid": "invalid"}`,
	))
	evalMetric := EvalMetric{
		MetricName: string(FinalResponseMatchV2),
		Threshold:  &threshold,
		Criterion:  &LlmAsAJudgeCriterion{JudgeModelOptions: JudgeModelOptions{NumSamples: 1}},
	}
	e := NewFinalResponseMatchV2Evaluator(evalMetric, fakeLLM)

	actual := []Invocation{{
		UserContent:   &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "what is 2+2?"}}},
		FinalResponse: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "5"}}},
	}}
	expected := []Invocation{{
		UserContent:   &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "what is 2+2?"}}},
		FinalResponse: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "4"}}},
	}}

	result, err := e.EvaluateInvocations(context.Background(), actual, expected, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusFailed {
		t.Errorf("OverallEvalStatus = %v, want FAILED", result.OverallEvalStatus)
	}
}

func TestFinalResponseMatchV2Evaluator_NilExpected(t *testing.T) {
	fakeLLM := testutil.NewFakeLLM()
	evalMetric := EvalMetric{
		MetricName: string(FinalResponseMatchV2),
		Criterion:  &LlmAsAJudgeCriterion{JudgeModelOptions: JudgeModelOptions{NumSamples: 1}},
	}
	e := NewFinalResponseMatchV2Evaluator(evalMetric, fakeLLM)

	actual := []Invocation{{UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}}}
	result, err := e.EvaluateInvocations(context.Background(), actual, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	// With expectedInvocationsRequired=true and no expected, should be NOT_EVALUATED.
	if result.OverallEvalStatus != EvalStatusNotEvaluated {
		t.Errorf("OverallEvalStatus = %v, want NOT_EVALUATED", result.OverallEvalStatus)
	}
}

func TestFinalResponseMatchV2Evaluator_EmptyResponse(t *testing.T) {
	fakeLLM := testutil.NewFakeLLM(testutil.NewTextResponse(""))
	evalMetric := EvalMetric{
		MetricName: string(FinalResponseMatchV2),
		Criterion:  &LlmAsAJudgeCriterion{JudgeModelOptions: JudgeModelOptions{NumSamples: 1}},
	}
	e := NewFinalResponseMatchV2Evaluator(evalMetric, fakeLLM)

	actual := []Invocation{{
		UserContent:   &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "hi"}}},
		FinalResponse: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "hello"}}},
	}}
	expected := []Invocation{{
		UserContent:   &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "hi"}}},
		FinalResponse: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "hello"}}},
	}}

	result, _ := e.EvaluateInvocations(context.Background(), actual, expected, nil)
	if result.OverallEvalStatus != EvalStatusNotEvaluated {
		t.Errorf("OverallEvalStatus = %v, want NOT_EVALUATED for empty response", result.OverallEvalStatus)
	}
}

func TestParseCritique(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Label
	}{
		{"valid", `{"is_the_agent_response_valid": "valid"}`, LabelValid},
		{"invalid", `{"is_the_agent_response_valid": "invalid"}`, LabelInvalid},
		{"almost", `{"is_the_agent_response_valid": "almost"}`, LabelInvalid},
		{"true", `{"is_the_agent_response_valid": "true"}`, LabelValid},
		{"false", `{"is_the_agent_response_valid": "false"}`, LabelInvalid},
		{"not_found", `{"something_else": "yes"}`, LabelNotFound},
		{"partially_valid", `{"is_the_agent_response_valid": "partially_valid"}`, LabelInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCritique(tt.input)
			if got != tt.want {
				t.Errorf("parseCritique() = %v, want %v", got, tt.want)
			}
		})
	}
}
