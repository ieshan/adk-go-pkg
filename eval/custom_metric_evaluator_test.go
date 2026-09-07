package eval_test

import (
	"context"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
	"google.golang.org/genai"
)

func TestCustomMetricEvaluator_Pass(t *testing.T) {
	threshold := 0.8
	fn := func(
		ctx context.Context,
		em eval.EvalMetric,
		actual []eval.Invocation,
		expected []eval.Invocation,
		cs *eval.ConversationScenario,
	) (*eval.EvaluationResult, error) {
		score := 1.0
		return &eval.EvaluationResult{
			OverallEvalStatus: eval.GetEvalStatus(&score, em.Threshold),
			OverallScore:      &score,
		}, nil
	}
	evalMetric := eval.EvalMetric{MetricName: "custom_metric", Threshold: &threshold}
	evaluator := eval.NewCustomMetricEvaluator(evalMetric, fn)

	inv := []eval.Invocation{{FinalResponse: textToContent("response")}}
	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != eval.EvalStatusPassed {
		t.Errorf("got %v, want PASSED", result.OverallEvalStatus)
	}
}

func TestCustomMetricEvaluator_Fail(t *testing.T) {
	threshold := 0.8
	fn := func(
		ctx context.Context,
		em eval.EvalMetric,
		actual []eval.Invocation,
		expected []eval.Invocation,
		cs *eval.ConversationScenario,
	) (*eval.EvaluationResult, error) {
		score := 0.5
		return &eval.EvaluationResult{
			OverallEvalStatus: eval.GetEvalStatus(&score, em.Threshold),
			OverallScore:      &score,
		}, nil
	}
	evalMetric := eval.EvalMetric{MetricName: "custom_metric", Threshold: &threshold}
	evaluator := eval.NewCustomMetricEvaluator(evalMetric, fn)

	inv := []eval.Invocation{{FinalResponse: textToContent("response")}}
	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != eval.EvalStatusFailed {
		t.Errorf("got %v, want FAILED", result.OverallEvalStatus)
	}
}

func TestCustomMetricEvaluator_FunctionCallAccess(t *testing.T) {
	threshold := 1.0
	fn := func(
		ctx context.Context,
		em eval.EvalMetric,
		actual []eval.Invocation,
		expected []eval.Invocation,
		cs *eval.ConversationScenario,
	) (*eval.EvaluationResult, error) {
		calls := eval.GetAllToolCalls(actual[0])
		if len(calls) != 1 || calls[0].Name != "get_weather" {
			score := 0.0
			return &eval.EvaluationResult{
				OverallEvalStatus: eval.GetEvalStatus(&score, em.Threshold),
				OverallScore:      &score,
			}, nil
		}
		score := 1.0
		return &eval.EvaluationResult{
			OverallEvalStatus: eval.GetEvalStatus(&score, em.Threshold),
			OverallScore:      &score,
		}, nil
	}
	evalMetric := eval.EvalMetric{MetricName: "custom_tool_check", Threshold: &threshold}
	evaluator := eval.NewCustomMetricEvaluator(evalMetric, fn)

	inv := []eval.Invocation{{
		IntermediateData: &eval.LegacyIntermediateData{
			ToolUses: []genai.FunctionCall{{Name: "get_weather"}},
		},
	}}
	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != eval.EvalStatusPassed {
		t.Errorf("got %v, want PASSED for correct tool call", result.OverallEvalStatus)
	}
}

func TestCustomMetricEvaluator_NilFunc(t *testing.T) {
	evalMetric := eval.EvalMetric{MetricName: "custom_metric"}
	evaluator := eval.NewCustomMetricEvaluator(evalMetric, nil)

	_, err := evaluator.EvaluateInvocations(context.TODO(), nil, nil, nil)
	if err == nil {
		t.Errorf("got nil error, want non-nil error for nil metric function")
	}
}
