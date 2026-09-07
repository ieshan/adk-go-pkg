package eval_test

import (
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
)

func TestEvaluationResult_Aggregation(t *testing.T) {
	tests := []struct {
		name          string
		perInvocation []eval.PerInvocationResult
		wantOverall   eval.EvalStatus
	}{
		{
			name: "all passed",
			perInvocation: []eval.PerInvocationResult{
				{EvalStatus: eval.EvalStatusPassed, Score: eval.Float64Ptr(1.0)},
				{EvalStatus: eval.EvalStatusPassed, Score: eval.Float64Ptr(1.0)},
			},
			wantOverall: eval.EvalStatusPassed,
		},
		{
			name: "one failed",
			perInvocation: []eval.PerInvocationResult{
				{EvalStatus: eval.EvalStatusPassed, Score: eval.Float64Ptr(1.0)},
				{EvalStatus: eval.EvalStatusFailed, Score: eval.Float64Ptr(0.0)},
			},
			wantOverall: eval.EvalStatusFailed,
		},
		{
			name:          "empty results",
			perInvocation: nil,
			wantOverall:   eval.EvalStatusNotEvaluated,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &eval.EvaluationResult{
				PerInvocationResults: tt.perInvocation,
			}
			// Simple aggregation: if any failed, overall is failed.
			overall := eval.EvalStatusPassed
			if len(tt.perInvocation) == 0 {
				overall = eval.EvalStatusNotEvaluated
			}
			for _, pir := range tt.perInvocation {
				if pir.EvalStatus != eval.EvalStatusPassed {
					overall = eval.EvalStatusFailed
				}
			}
			result.OverallEvalStatus = overall
			if result.OverallEvalStatus != tt.wantOverall {
				t.Errorf("OverallEvalStatus = %v, want %v", result.OverallEvalStatus, tt.wantOverall)
			}
		})
	}
}

func TestPerInvocationResult(t *testing.T) {
	pir := eval.PerInvocationResult{
		EvalStatus: eval.EvalStatusPassed,
		Score:      eval.Float64Ptr(0.95),
	}
	if pir.EvalStatus != eval.EvalStatusPassed {
		t.Errorf("EvalStatus = %v, want %v", pir.EvalStatus, eval.EvalStatusPassed)
	}
	if pir.Score == nil || *pir.Score != 0.95 {
		t.Error("Score not set correctly")
	}
}
