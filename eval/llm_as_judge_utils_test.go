package eval

import (
	"testing"

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
			got := GetTextFromContent(tt.content)
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
		want      EvalStatus
	}{
		{"nil threshold", Float64Ptr(1.0), nil, EvalStatusNotEvaluated},
		{"nil score", nil, Float64Ptr(0.8), EvalStatusNotEvaluated},
		{"pass", Float64Ptr(0.9), Float64Ptr(0.8), EvalStatusPassed},
		{"fail", Float64Ptr(0.5), Float64Ptr(0.8), EvalStatusFailed},
		{"exact threshold", Float64Ptr(0.8), Float64Ptr(0.8), EvalStatusPassed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetEvalStatus(tt.score, tt.threshold)
			if got != tt.want {
				t.Errorf("GetEvalStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetAverageRubricScore(t *testing.T) {
	tests := []struct {
		name   string
		scores []RubricScore
		want   *float64
	}{
		{
			name:   "empty scores",
			scores: nil,
			want:   nil,
		},
		{
			name: "all nil scores",
			scores: []RubricScore{
				{RubricID: "r1", Score: nil},
				{RubricID: "r2", Score: nil},
			},
			want: nil,
		},
		{
			name: "mixed scores",
			scores: []RubricScore{
				{RubricID: "r1", Score: Float64Ptr(1.0)},
				{RubricID: "r2", Score: nil},
				{RubricID: "r3", Score: Float64Ptr(0.0)},
			},
			want: Float64Ptr(0.5),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetAverageRubricScore(tt.scores)
			if tt.want == nil {
				if got != nil {
					t.Errorf("GetAverageRubricScore() = %v, want nil", got)
				}
			} else {
				if got == nil {
					t.Fatalf("GetAverageRubricScore() = nil, want %v", *tt.want)
				}
				if *got != *tt.want {
					t.Errorf("GetAverageRubricScore() = %v, want %v", *got, *tt.want)
				}
			}
		})
	}
}

func TestFloat64Ptr(t *testing.T) {
	v := Float64Ptr(3.14)
	if v == nil || *v != 3.14 {
		t.Error("Float64Ptr(3.14) should return pointer to 3.14")
	}
}

func TestAggregateRubricScores(t *testing.T) {
	// AggregateRubricScores is an alias for GetAverageRubricScore; verify it
	// produces the same result.
	scores := []RubricScore{
		{RubricID: "r1", Score: Float64Ptr(1.0)},
		{RubricID: "r2", Score: Float64Ptr(0.0)},
	}
	got := AggregateRubricScores(scores)
	if got == nil || *got != 0.5 {
		t.Errorf("AggregateRubricScores() = %v, want 0.5", got)
	}

	// Empty scores should return nil.
	if got := AggregateRubricScores(nil); got != nil {
		t.Errorf("AggregateRubricScores(nil) = %v, want nil", got)
	}
}
