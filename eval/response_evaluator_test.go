package eval

import (
	"context"
	"testing"
)

func TestResponseEvaluator_ROUGEDelegation(t *testing.T) {
	threshold := 0.8
	evalMetric := EvalMetric{MetricName: string(ResponseMatchScore), Threshold: &threshold}
	evaluator := NewResponseEvaluator(evalMetric)

	inv := []Invocation{{
		FinalResponse: textToContent("the quick brown fox"),
	}}
	expected := []Invocation{{
		FinalResponse: textToContent("the quick brown fox"),
	}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, expected, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallScore == nil || *result.OverallScore != 1.0 {
		t.Errorf("identical strings should score 1.0, got %v", result.OverallScore)
	}
}

func TestResponseEvaluator_CoherenceNotEvaluated(t *testing.T) {
	evalMetric := EvalMetric{MetricName: string(ResponseEvaluationScore)}
	evaluator := NewResponseEvaluator(evalMetric)

	inv := []Invocation{{
		FinalResponse: textToContent("some response"),
	}}
	expected := []Invocation{{
		FinalResponse: textToContent("some response"),
	}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, expected, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusNotEvaluated {
		t.Errorf("coherence without Vertex AI should return NOT_EVALUATED, got %v", result.OverallEvalStatus)
	}
}

func TestResponseEvaluator_UnsupportedMetric(t *testing.T) {
	evalMetric := EvalMetric{MetricName: "unsupported_metric"}
	evaluator := NewResponseEvaluator(evalMetric)

	_, err := evaluator.EvaluateInvocations(context.TODO(), nil, nil, nil)
	if err == nil {
		t.Error("expected error for unsupported metric")
	}
}
