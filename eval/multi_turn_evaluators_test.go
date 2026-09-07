package eval_test

import (
	"context"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
)

func TestMultiTurnTaskSuccessV1_NotEvaluated(t *testing.T) {
	evalMetric := eval.EvalMetric{MetricName: string(eval.MultiTurnTaskSuccessV1)}
	evaluator := eval.NewMultiTurnTaskSuccessV1Evaluator(evalMetric)

	inv := []eval.Invocation{{
		FinalResponse: textToContent("task completed"),
	}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != eval.EvalStatusNotEvaluated {
		t.Errorf("got %v, want NOT_EVALUATED", result.OverallEvalStatus)
	}
}

func TestMultiTurnTrajectoryQualityV1_NotEvaluated(t *testing.T) {
	evalMetric := eval.EvalMetric{MetricName: string(eval.MultiTurnTrajectoryQualityV1)}
	evaluator := eval.NewMultiTurnTrajectoryQualityV1Evaluator(evalMetric)

	inv := []eval.Invocation{{
		FinalResponse: textToContent("response"),
	}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != eval.EvalStatusNotEvaluated {
		t.Errorf("got %v, want NOT_EVALUATED", result.OverallEvalStatus)
	}
}

func TestMultiTurnToolUseQualityV1_NotEvaluated(t *testing.T) {
	evalMetric := eval.EvalMetric{MetricName: string(eval.MultiTurnToolUseQualityV1)}
	evaluator := eval.NewMultiTurnToolUseQualityV1Evaluator(evalMetric)

	inv := []eval.Invocation{{
		FinalResponse: textToContent("response"),
	}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != eval.EvalStatusNotEvaluated {
		t.Errorf("got %v, want NOT_EVALUATED", result.OverallEvalStatus)
	}
}
