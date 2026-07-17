package eval

import (
	"testing"
)

func TestEvaluationResult_Aggregation(t *testing.T) {
	tests := []struct {
		name          string
		perInvocation []PerInvocationResult
		wantOverall   EvalStatus
	}{
		{
			name: "all passed",
			perInvocation: []PerInvocationResult{
				{EvalStatus: EvalStatusPassed, Score: Float64Ptr(1.0)},
				{EvalStatus: EvalStatusPassed, Score: Float64Ptr(1.0)},
			},
			wantOverall: EvalStatusPassed,
		},
		{
			name: "one failed",
			perInvocation: []PerInvocationResult{
				{EvalStatus: EvalStatusPassed, Score: Float64Ptr(1.0)},
				{EvalStatus: EvalStatusFailed, Score: Float64Ptr(0.0)},
			},
			wantOverall: EvalStatusFailed,
		},
		{
			name:          "empty results",
			perInvocation: nil,
			wantOverall:   EvalStatusNotEvaluated,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &EvaluationResult{
				PerInvocationResults: tt.perInvocation,
			}
			// Simple aggregation: if any failed, overall is failed.
			overall := EvalStatusPassed
			if len(tt.perInvocation) == 0 {
				overall = EvalStatusNotEvaluated
			}
			for _, pir := range tt.perInvocation {
				if pir.EvalStatus != EvalStatusPassed {
					overall = EvalStatusFailed
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
	pir := PerInvocationResult{
		EvalStatus: EvalStatusPassed,
		Score:      Float64Ptr(0.95),
	}
	if pir.EvalStatus != EvalStatusPassed {
		t.Errorf("EvalStatus = %v, want %v", pir.EvalStatus, EvalStatusPassed)
	}
	if pir.Score == nil || *pir.Score != 0.95 {
		t.Error("Score not set correctly")
	}
}
