package eval_test

import (
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
	"google.golang.org/genai"
)

func TestGetTextFromContent(t *testing.T) {
	tests := []struct {
		name    string
		content *genai.Content
		want    string
	}{
		{
			name:    "nil content",
			content: nil,
			want:    "",
		},
		{
			name:    "single text part",
			content: &genai.Content{Parts: []*genai.Part{{Text: "hello"}}},
			want:    "hello",
		},
		{
			name:    "multiple text parts",
			content: &genai.Content{Parts: []*genai.Part{{Text: "hello "}, {Text: "world"}}},
			want:    "hello world",
		},
		{
			name:    "no text parts",
			content: &genai.Content{Parts: []*genai.Part{{FunctionCall: &genai.FunctionCall{Name: "test"}}}},
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eval.GetTextFromContent(tt.content)
			if got != tt.want {
				t.Errorf("GetTextFromContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetEvalStatus(t *testing.T) {
	tests := []struct {
		name      string
		score     *float64
		threshold *float64
		want      eval.EvalStatus
	}{
		{"nil threshold", eval.Float64Ptr(1.0), nil, eval.EvalStatusNotEvaluated},
		{"nil score", nil, eval.Float64Ptr(0.8), eval.EvalStatusNotEvaluated},
		{"pass", eval.Float64Ptr(0.9), eval.Float64Ptr(0.8), eval.EvalStatusPassed},
		{"fail", eval.Float64Ptr(0.5), eval.Float64Ptr(0.8), eval.EvalStatusFailed},
		{"exact threshold", eval.Float64Ptr(0.8), eval.Float64Ptr(0.8), eval.EvalStatusPassed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eval.GetEvalStatus(tt.score, tt.threshold)
			if got != tt.want {
				t.Errorf("GetEvalStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetAverageRubricScore(t *testing.T) {
	tests := []struct {
		name   string
		scores []eval.RubricScore
		want   *float64
	}{
		{
			name:   "empty scores",
			scores: nil,
			want:   nil,
		},
		{
			name: "all nil scores",
			scores: []eval.RubricScore{
				{RubricID: "r1", Score: nil},
				{RubricID: "r2", Score: nil},
			},
			want: nil,
		},
		{
			name: "mixed scores",
			scores: []eval.RubricScore{
				{RubricID: "r1", Score: eval.Float64Ptr(1.0)},
				{RubricID: "r2", Score: nil},
				{RubricID: "r3", Score: eval.Float64Ptr(0.0)},
			},
			want: eval.Float64Ptr(0.5),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eval.GetAverageRubricScore(tt.scores)
			if tt.want == nil {
				if got != nil {
					t.Errorf("GetAverageRubricScore() = %v, want nil", got)
				}
			} else {
				if got == nil {
					t.Errorf("GetAverageRubricScore() = nil, want %v", *tt.want)
				}
				if *got != *tt.want {
					t.Errorf("GetAverageRubricScore() = %v, want %v", *got, *tt.want)
				}
			}
		})
	}
}

func TestFloat64Ptr(t *testing.T) {
	v := eval.Float64Ptr(3.14)
	if v == nil || *v != 3.14 {
		t.Error("Float64Ptr(3.14) should return pointer to 3.14")
	}
}

func TestAggregateRubricScores(t *testing.T) {
	// AggregateRubricScores is an alias for GetAverageRubricScore; verify it
	// produces the same result.
	scores := []eval.RubricScore{
		{RubricID: "r1", Score: eval.Float64Ptr(1.0)},
		{RubricID: "r2", Score: eval.Float64Ptr(0.0)},
	}
	got := eval.AggregateRubricScores(scores)
	if got == nil || *got != 0.5 {
		t.Errorf("AggregateRubricScores() = %v, want 0.5", got)
	}

	// Empty scores should return nil.
	if got := eval.AggregateRubricScores(nil); got != nil {
		t.Errorf("AggregateRubricScores(nil) = %v, want nil", got)
	}
}
