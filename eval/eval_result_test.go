package eval

import (
	"testing"
)

func TestEvalCaseResult_Basic(t *testing.T) {
	result := &EvalCaseResult{
		EvalID:          "test_case",
		FinalEvalStatus: EvalStatusPassed,
		OverallEvalMetricResults: []EvalMetricResult{
			{EvalMetric: EvalMetric{MetricName: "test_metric"}, EvalStatus: EvalStatusPassed},
		},
	}

	if result.EvalID != "test_case" {
		t.Errorf("got %s, want test_case", result.EvalID)
	}
	if result.FinalEvalStatus != EvalStatusPassed {
		t.Errorf("got %v, want PASSED", result.FinalEvalStatus)
	}
	if len(result.OverallEvalMetricResults) != 1 {
		t.Errorf("expected 1 metric result, got %d", len(result.OverallEvalMetricResults))
	}
}

func TestEvalSetResult_Basic(t *testing.T) {
	result := &EvalSetResult{
		EvalSetResultID:   "result_1",
		EvalSetResultName: "test_result",
		EvalSetID:         "test_set",
		EvalCaseResults: []EvalCaseResult{
			{EvalID: "case1", FinalEvalStatus: EvalStatusPassed},
			{EvalID: "case2", FinalEvalStatus: EvalStatusFailed},
		},
	}

	if result.EvalSetResultID != "result_1" {
		t.Errorf("got %s, want result_1", result.EvalSetResultID)
	}
	if len(result.EvalCaseResults) != 2 {
		t.Errorf("expected 2 case results, got %d", len(result.EvalCaseResults))
	}
}

func TestCreateEvalSetResult_Basic(t *testing.T) {
	results := []EvalCaseResult{
		{EvalID: "case1", FinalEvalStatus: EvalStatusPassed},
	}
	result := CreateEvalSetResult("test_app", "test_set", results)

	if result.EvalSetID != "test_set" {
		t.Errorf("got %s, want test_set", result.EvalSetID)
	}
	if len(result.EvalCaseResults) != 1 {
		t.Errorf("expected 1 case result, got %d", len(result.EvalCaseResults))
	}
	if result.CreationTimestamp <= 0 {
		t.Error("expected positive creation timestamp")
	}
}
