package eval

import (
	"context"
	"testing"

	"google.golang.org/genai"
)

func TestCustomMetricEvaluator_Pass(t *testing.T) {
	threshold := 0.8
	fn := func(
		ctx context.Context,
		em EvalMetric,
		actual []Invocation,
		expected []Invocation,
		cs *ConversationScenario,
	) (*EvaluationResult, error) {
		score := 1.0
		return &EvaluationResult{
			OverallEvalStatus: GetEvalStatus(&score, em.Threshold),
			OverallScore:      &score,
		}, nil
	}
	evalMetric := EvalMetric{MetricName: "custom_metric", Threshold: &threshold}
	evaluator := NewCustomMetricEvaluator(evalMetric, fn)

	inv := []Invocation{{FinalResponse: textToContent("response")}}
	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusPassed {
		t.Errorf("expected PASSED, got %v", result.OverallEvalStatus)
	}
}

func TestCustomMetricEvaluator_Fail(t *testing.T) {
	threshold := 0.8
	fn := func(
		ctx context.Context,
		em EvalMetric,
		actual []Invocation,
		expected []Invocation,
		cs *ConversationScenario,
	) (*EvaluationResult, error) {
		score := 0.5
		return &EvaluationResult{
			OverallEvalStatus: GetEvalStatus(&score, em.Threshold),
			OverallScore:      &score,
		}, nil
	}
	evalMetric := EvalMetric{MetricName: "custom_metric", Threshold: &threshold}
	evaluator := NewCustomMetricEvaluator(evalMetric, fn)

	inv := []Invocation{{FinalResponse: textToContent("response")}}
	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusFailed {
		t.Errorf("expected FAILED, got %v", result.OverallEvalStatus)
	}
}

func TestCustomMetricEvaluator_FunctionCallAccess(t *testing.T) {
	threshold := 1.0
	fn := func(
		ctx context.Context,
		em EvalMetric,
		actual []Invocation,
		expected []Invocation,
		cs *ConversationScenario,
	) (*EvaluationResult, error) {
		calls := GetAllToolCalls(actual[0])
		if len(calls) != 1 || calls[0].Name != "get_weather" {
			score := 0.0
			return &EvaluationResult{
				OverallEvalStatus: GetEvalStatus(&score, em.Threshold),
				OverallScore:      &score,
			}, nil
		}
		score := 1.0
		return &EvaluationResult{
			OverallEvalStatus: GetEvalStatus(&score, em.Threshold),
			OverallScore:      &score,
		}, nil
	}
	evalMetric := EvalMetric{MetricName: "custom_tool_check", Threshold: &threshold}
	evaluator := NewCustomMetricEvaluator(evalMetric, fn)

	inv := []Invocation{{
		IntermediateData: &LegacyIntermediateData{
			ToolUses: []genai.FunctionCall{{Name: "get_weather"}},
		},
	}}
	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusPassed {
		t.Errorf("expected PASSED for correct tool call, got %v", result.OverallEvalStatus)
	}
}

func TestCustomMetricEvaluator_NilFunc(t *testing.T) {
	evalMetric := EvalMetric{MetricName: "custom_metric"}
	evaluator := NewCustomMetricEvaluator(evalMetric, nil)

	_, err := evaluator.EvaluateInvocations(context.TODO(), nil, nil, nil)
	if err == nil {
		t.Error("expected error for nil metric function")
	}
}
