package eval_test

import (
	"context"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	registry := eval.NewMetricEvaluatorRegistry()

	registry.RegisterEvaluator(
		eval.MetricInfo{MetricName: "custom_test"},
		func(metric eval.EvalMetric) (eval.Evaluator, error) {
			return eval.NewCustomMetricEvaluator(metric, func(
				ctx context.Context,
				em eval.EvalMetric,
				actual []eval.Invocation,
				expected []eval.Invocation,
				cs *eval.ConversationScenario,
			) (*eval.EvaluationResult, error) {
				score := 1.0
				return &eval.EvaluationResult{
					OverallEvalStatus: eval.EvalStatusPassed,
					OverallScore:      &score,
				}, nil
			}), nil
		},
	)

	got, err := registry.GetEvaluator(eval.EvalMetric{MetricName: "custom_test"})
	if err != nil {
		t.Fatalf("GetEvaluator failed: %v", err)
	}
	if got == nil {
		t.Fatal("got nil evaluator, want non-nil")
	}
}

func TestRegistry_NotFound(t *testing.T) {
	registry := eval.NewMetricEvaluatorRegistry()
	_, err := registry.GetEvaluator(eval.EvalMetric{MetricName: "nonexistent"})
	if err == nil {
		t.Error("got nil error, want error for non-existent metric")
	}
}

func TestDefaultRegistry_HasAllMetrics(t *testing.T) {
	registry := eval.DefaultMetricEvaluatorRegistry()

	expectedMetrics := []string{
		"tool_trajectory_avg_score",
		"response_match_score",
		"response_evaluation_score",
		"final_response_match_v2",
		"rubric_based_final_response_quality_v1",
		"rubric_based_tool_use_quality_v1",
		"rubric_based_multi_turn_trajectory_quality_v1",
		"hallucinations_v1",
		"safety_v1",
		"multi_turn_task_success_v1",
		"multi_turn_trajectory_quality_v1",
		"multi_turn_tool_use_quality_v1",
		"per_turn_user_simulator_quality_v1",
	}

	for _, metric := range expectedMetrics {
		if !registry.HasMetric(metric) {
			t.Errorf("default registry missing metric %q", metric)
		}
	}
}
