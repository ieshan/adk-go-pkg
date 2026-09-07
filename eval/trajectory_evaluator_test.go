package eval

import (
	"context"
	"testing"

	"google.golang.org/genai"
)

func TestTrajectoryEvaluator_ExactMatch(t *testing.T) {
	tests := []struct {
		name     string
		actual   []genai.FunctionCall
		expected []genai.FunctionCall
		wantPass bool
	}{
		{
			name: "exact match",
			actual: []genai.FunctionCall{
				{Name: "get_weather", Args: map[string]any{"city": "SF"}},
			},
			expected: []genai.FunctionCall{
				{Name: "get_weather", Args: map[string]any{"city": "SF"}},
			},
			wantPass: true,
		},
		{
			name: "mismatch name",
			actual: []genai.FunctionCall{
				{Name: "get_weather"},
			},
			expected: []genai.FunctionCall{
				{Name: "get_time"},
			},
			wantPass: false,
		},
		{
			name: "mismatch args",
			actual: []genai.FunctionCall{
				{Name: "get_weather", Args: map[string]any{"city": "SF"}},
			},
			expected: []genai.FunctionCall{
				{Name: "get_weather", Args: map[string]any{"city": "NYC"}},
			},
			wantPass: false,
		},
		{
			name:     "both empty",
			actual:   nil,
			expected: nil,
			wantPass: true,
		},
		{
			name: "extra actual call",
			actual: []genai.FunctionCall{
				{Name: "get_weather"},
				{Name: "get_time"},
			},
			expected: []genai.FunctionCall{
				{Name: "get_weather"},
			},
			wantPass: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			threshold := 1.0
			evaluator := &TrajectoryEvaluator{
				evalMetric: EvalMetric{MetricName: "tool_trajectory_exact_match", Threshold: &threshold},
				matchType:  MatchExact,
			}
			actual := []Invocation{{
				IntermediateData: &LegacyIntermediateData{ToolUses: tt.actual},
			}}
			expected := []Invocation{{
				IntermediateData: &LegacyIntermediateData{ToolUses: tt.expected},
			}}

			result, err := evaluator.EvaluateInvocations(context.TODO(), actual, expected, nil)
			if err != nil {
				t.Fatalf("EvaluateInvocations failed: %v", err)
			}
			if tt.wantPass && result.OverallEvalStatus != EvalStatusPassed {
				t.Errorf("OverallEvalStatus = %v, want %v", result.OverallEvalStatus, EvalStatusPassed)
			}
			if !tt.wantPass && result.OverallEvalStatus == EvalStatusPassed {
				t.Errorf("OverallEvalStatus = %v, want non-passed", result.OverallEvalStatus)
			}
		})
	}
}

func TestTrajectoryEvaluator_InOrder(t *testing.T) {
	threshold := 1.0
	evaluator := &TrajectoryEvaluator{
		evalMetric: EvalMetric{MetricName: "tool_trajectory_in_order_match", Threshold: &threshold},
		matchType:  MatchInOrder,
	}
	actual := []Invocation{{
		IntermediateData: &LegacyIntermediateData{
			ToolUses: []genai.FunctionCall{
				{Name: "get_weather"},
				{Name: "get_time"},
				{Name: "extra_call"},
			},
		},
	}}
	expected := []Invocation{{
		IntermediateData: &LegacyIntermediateData{
			ToolUses: []genai.FunctionCall{
				{Name: "get_weather"},
				{Name: "get_time"},
			},
		},
	}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), actual, expected, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusPassed {
		t.Errorf("got %v, want PASSED for InOrder with extras", result.OverallEvalStatus)
	}
}

func TestTrajectoryEvaluator_AnyOrder(t *testing.T) {
	threshold := 1.0
	evaluator := &TrajectoryEvaluator{
		evalMetric: EvalMetric{MetricName: "tool_trajectory_any_order_match", Threshold: &threshold},
		matchType:  MatchAnyOrder,
	}
	actual := []Invocation{{
		IntermediateData: &LegacyIntermediateData{
			ToolUses: []genai.FunctionCall{
				{Name: "get_time"},
				{Name: "get_weather"},
			},
		},
	}}
	expected := []Invocation{{
		IntermediateData: &LegacyIntermediateData{
			ToolUses: []genai.FunctionCall{
				{Name: "get_weather"},
				{Name: "get_time"},
			},
		},
	}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), actual, expected, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusPassed {
		t.Errorf("got %v, want PASSED for AnyOrder with reordered calls", result.OverallEvalStatus)
	}
}

func TestTrajectoryEvaluator_EmptyToolCalls(t *testing.T) {
	threshold := 1.0
	evaluator := &TrajectoryEvaluator{
		evalMetric: EvalMetric{MetricName: "tool_trajectory_exact_match", Threshold: &threshold},
		matchType:  MatchExact,
	}
	actual := []Invocation{{}}
	expected := []Invocation{{}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), actual, expected, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusPassed {
		t.Errorf("got %v, want PASSED for empty tool calls", result.OverallEvalStatus)
	}
}
