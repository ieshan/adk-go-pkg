package eval_test

import (
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
)

func TestEvalCaseResult_Basic(t *testing.T) {
	result := &eval.EvalCaseResult{
		EvalID:          "test_case",
		FinalEvalStatus: eval.EvalStatusPassed,
		OverallEvalMetricResults: []eval.EvalMetricResult{
			{EvalMetric: eval.EvalMetric{MetricName: "test_metric"}, EvalStatus: eval.EvalStatusPassed},
		},
	}

	if result.EvalID != "test_case" {
		t.Errorf("got %s, want test_case", result.EvalID)
	}
	if result.FinalEvalStatus != eval.EvalStatusPassed {
		t.Errorf("got %v, want PASSED", result.FinalEvalStatus)
	}
	if len(result.OverallEvalMetricResults) != 1 {
		t.Errorf("got %d metric results, want 1", len(result.OverallEvalMetricResults))
	}
}

func TestEvalSetResult_Basic(t *testing.T) {
	result := &eval.EvalSetResult{
		EvalSetResultID:   "result_1",
		EvalSetResultName: "test_result",
		EvalSetID:         "test_set",
		EvalCaseResults: []eval.EvalCaseResult{
			{EvalID: "case1", FinalEvalStatus: eval.EvalStatusPassed},
			{EvalID: "case2", FinalEvalStatus: eval.EvalStatusFailed},
		},
	}

	if result.EvalSetResultID != "result_1" {
		t.Errorf("got %s, want result_1", result.EvalSetResultID)
	}
	if len(result.EvalCaseResults) != 2 {
		t.Errorf("got %d case results, want 2", len(result.EvalCaseResults))
	}
}

func TestCreateEvalSetResult_Basic(t *testing.T) {
	results := []eval.EvalCaseResult{
		{EvalID: "case1", FinalEvalStatus: eval.EvalStatusPassed},
	}
	result := eval.CreateEvalSetResult("test_app", "test_set", results)

	if result.EvalSetID != "test_set" {
		t.Errorf("got %s, want test_set", result.EvalSetID)
	}
	if len(result.EvalCaseResults) != 1 {
		t.Errorf("got %d case result, want 1", len(result.EvalCaseResults))
	}
	if result.CreationTimestamp <= 0 {
		t.Errorf("got %v creation timestamp, want positive", result.CreationTimestamp)
	}
}
