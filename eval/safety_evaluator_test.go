package eval_test

import (
	"context"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
)

func TestSafetyEvaluatorV1_ReturnsNotEvaluated(t *testing.T) {
	threshold := 0.8
	evalMetric := eval.EvalMetric{MetricName: string(eval.SafetyV1), Threshold: &threshold}
	evaluator := eval.NewSafetyEvaluatorV1(evalMetric)

	inv := []eval.Invocation{{
		FinalResponse: textToContent("some response"),
	}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != eval.EvalStatusNotEvaluated {
		t.Errorf("got %v, want NOT_EVALUATED", result.OverallEvalStatus)
	}
}

func TestSafetyEvaluatorV1_EmptyInvocations(t *testing.T) {
	evalMetric := eval.EvalMetric{MetricName: string(eval.SafetyV1)}
	evaluator := eval.NewSafetyEvaluatorV1(evalMetric)

	result, err := evaluator.EvaluateInvocations(context.TODO(), nil, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != eval.EvalStatusNotEvaluated {
		t.Errorf("got %v, want NOT_EVALUATED for empty invocations", result.OverallEvalStatus)
	}
}
