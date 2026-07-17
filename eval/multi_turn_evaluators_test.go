package eval

import (
	"context"
	"testing"
)

func TestMultiTurnTaskSuccessV1_NotEvaluated(t *testing.T) {
	evalMetric := EvalMetric{MetricName: string(MultiTurnTaskSuccessV1)}
	evaluator := NewMultiTurnTaskSuccessV1Evaluator(evalMetric)

	inv := []Invocation{{
		FinalResponse: textToContent("task completed"),
	}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusNotEvaluated {
		t.Errorf("should return NOT_EVALUATED, got %v", result.OverallEvalStatus)
	}
}

func TestMultiTurnTrajectoryQualityV1_NotEvaluated(t *testing.T) {
	evalMetric := EvalMetric{MetricName: string(MultiTurnTrajectoryQualityV1)}
	evaluator := NewMultiTurnTrajectoryQualityV1Evaluator(evalMetric)

	inv := []Invocation{{
		FinalResponse: textToContent("response"),
	}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusNotEvaluated {
		t.Errorf("should return NOT_EVALUATED, got %v", result.OverallEvalStatus)
	}
}

func TestMultiTurnToolUseQualityV1_NotEvaluated(t *testing.T) {
	evalMetric := EvalMetric{MetricName: string(MultiTurnToolUseQualityV1)}
	evaluator := NewMultiTurnToolUseQualityV1Evaluator(evalMetric)

	inv := []Invocation{{
		FinalResponse: textToContent("response"),
	}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusNotEvaluated {
		t.Errorf("should return NOT_EVALUATED, got %v", result.OverallEvalStatus)
	}
}
