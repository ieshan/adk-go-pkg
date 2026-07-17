package eval

import (
	"context"
	"testing"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	registry := NewMetricEvaluatorRegistry()

	registry.RegisterEvaluator(
		MetricInfo{MetricName: "custom_test"},
		func(metric EvalMetric) (Evaluator, error) {
			return NewCustomMetricEvaluator(metric, func(
				ctx context.Context,
				em EvalMetric,
				actual []Invocation,
				expected []Invocation,
				cs *ConversationScenario,
			) (*EvaluationResult, error) {
				score := 1.0
				return &EvaluationResult{
					OverallEvalStatus: EvalStatusPassed,
					OverallScore:      &score,
				}, nil
			}), nil
		},
	)

	got, err := registry.GetEvaluator(EvalMetric{MetricName: "custom_test"})
	if err != nil {
		t.Fatalf("GetEvaluator failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil evaluator")
	}
}

func TestRegistry_NotFound(t *testing.T) {
	registry := NewMetricEvaluatorRegistry()
	_, err := registry.GetEvaluator(EvalMetric{MetricName: "nonexistent"})
	if err == nil {
		t.Error("expected error for non-existent metric")
	}
}

func TestDefaultRegistry_HasAllMetrics(t *testing.T) {
	registry := DefaultMetricEvaluatorRegistry()

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
