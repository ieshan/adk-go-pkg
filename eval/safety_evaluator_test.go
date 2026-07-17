package eval

import (
	"context"
	"testing"
)

func TestSafetyEvaluatorV1_ReturnsNotEvaluated(t *testing.T) {
	threshold := 0.8
	evalMetric := EvalMetric{MetricName: string(SafetyV1), Threshold: &threshold}
	evaluator := NewSafetyEvaluatorV1(evalMetric)

	inv := []Invocation{{
		FinalResponse: textToContent("some response"),
	}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusNotEvaluated {
		t.Errorf("should return NOT_EVALUATED, got %v", result.OverallEvalStatus)
	}
}

func TestSafetyEvaluatorV1_EmptyInvocations(t *testing.T) {
	evalMetric := EvalMetric{MetricName: string(SafetyV1)}
	evaluator := NewSafetyEvaluatorV1(evalMetric)

	result, err := evaluator.EvaluateInvocations(context.TODO(), nil, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusNotEvaluated {
		t.Errorf("empty invocations should return NOT_EVALUATED, got %v", result.OverallEvalStatus)
	}
}
