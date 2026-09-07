package eval_test

import (
	"context"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
)

func TestResponseEvaluator_ROUGEDelegation(t *testing.T) {
	threshold := 0.8
	evalMetric := eval.EvalMetric{MetricName: string(eval.ResponseMatchScore), Threshold: &threshold}
	evaluator := eval.NewResponseEvaluator(evalMetric)

	inv := []eval.Invocation{{
		FinalResponse: textToContent("the quick brown fox"),
	}}
	expected := []eval.Invocation{{
		FinalResponse: textToContent("the quick brown fox"),
	}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, expected, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallScore == nil || *result.OverallScore != 1.0 {
		t.Errorf("got %v, want 1.0 for identical strings", result.OverallScore)
	}
}

func TestResponseEvaluator_CoherenceNotEvaluated(t *testing.T) {
	evalMetric := eval.EvalMetric{MetricName: string(eval.ResponseEvaluationScore)}
	evaluator := eval.NewResponseEvaluator(evalMetric)

	inv := []eval.Invocation{{
		FinalResponse: textToContent("some response"),
	}}
	expected := []eval.Invocation{{
		FinalResponse: textToContent("some response"),
	}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, expected, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != eval.EvalStatusNotEvaluated {
		t.Errorf("got %v, want NOT_EVALUATED for coherence without Vertex AI", result.OverallEvalStatus)
	}
}

func TestResponseEvaluator_UnsupportedMetric(t *testing.T) {
	evalMetric := eval.EvalMetric{MetricName: "unsupported_metric"}
	evaluator := eval.NewResponseEvaluator(evalMetric)

	_, err := evaluator.EvaluateInvocations(context.TODO(), nil, nil, nil)
	if err == nil {
		t.Errorf("got nil error, want non-nil error for unsupported metric")
	}
}
