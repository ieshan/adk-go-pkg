package eval

import (
	"testing"
)

func TestDefaultAutoRaterResponseParser_Parse(t *testing.T) {
	response := `Property: The response is grammatically correct.
Rationale: The response has no grammar errors.
Verdict: yes

Property: The response is concise.
Rationale: The response is too long.
Verdict: no`

	parser := DefaultAutoRaterResponseParser{}
	results, err := parser.Parse(response)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].PropertyText != "The response is grammatically correct." {
		t.Errorf("PropertyText[0] = %q", results[0].PropertyText)
	}
	if results[0].Score == nil || *results[0].Score != 1.0 {
		t.Errorf("Score[0] = %v, want 1.0", results[0].Score)
	}
	if results[1].Score == nil || *results[1].Score != 0.0 {
		t.Errorf("Score[1] = %v, want 0.0", results[1].Score)
	}
}

func TestDefaultAutoRaterResponseParser_EmptyResponse(t *testing.T) {
	parser := DefaultAutoRaterResponseParser{}
	results, err := parser.Parse("")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}

func TestMajorityVotePerInvocationResultsAggregator(t *testing.T) {
	threshold := 0.8
	samples := []PerInvocationResult{
		{RubricScores: []RubricScore{{RubricID: "r1", Score: Float64Ptr(1.0)}}},
		{RubricScores: []RubricScore{{RubricID: "r1", Score: Float64Ptr(1.0)}}},
		{RubricScores: []RubricScore{{RubricID: "r1", Score: Float64Ptr(0.0)}}},
	}
	agg := MajorityVotePerInvocationResultsAggregator{}
	result := agg.Aggregate(samples, &threshold)
	if result.Score == nil || *result.Score != 1.0 {
		t.Errorf("Score = %v, want 1.0 (majority vote)", result.Score)
	}
	if result.EvalStatus != EvalStatusPassed {
		t.Errorf("EvalStatus = %v, want PASSED", result.EvalStatus)
	}
}

func TestMajorityVotePerInvocationResultsAggregator_NegativeMajority(t *testing.T) {
	threshold := 0.8
	samples := []PerInvocationResult{
		{RubricScores: []RubricScore{{RubricID: "r1", Score: Float64Ptr(0.0)}}},
		{RubricScores: []RubricScore{{RubricID: "r1", Score: Float64Ptr(0.0)}}},
		{RubricScores: []RubricScore{{RubricID: "r1", Score: Float64Ptr(1.0)}}},
	}
	agg := MajorityVotePerInvocationResultsAggregator{}
	result := agg.Aggregate(samples, &threshold)
	if result.Score == nil || *result.Score != 0.0 {
		t.Errorf("Score = %v, want 0.0 (majority vote)", result.Score)
	}
	if result.EvalStatus != EvalStatusFailed {
		t.Errorf("EvalStatus = %v, want FAILED", result.EvalStatus)
	}
}

func TestMeanInvocationResultsSummarizer(t *testing.T) {
	threshold := 0.5
	perInvocation := []PerInvocationResult{
		{RubricScores: []RubricScore{{RubricID: "r1", Score: Float64Ptr(1.0)}, {RubricID: "r2", Score: Float64Ptr(0.0)}}},
		{RubricScores: []RubricScore{{RubricID: "r1", Score: Float64Ptr(1.0)}, {RubricID: "r2", Score: Float64Ptr(1.0)}}},
	}
	summarizer := MeanInvocationResultsSummarizer{}
	result := summarizer.Summarize(perInvocation, &threshold)
	if result.OverallScore == nil {
		t.Fatal("OverallScore is nil")
	}
	// Average of [1.0, 0.0, 1.0, 1.0] = 0.75
	if *result.OverallScore != 0.75 {
		t.Errorf("OverallScore = %v, want 0.75", *result.OverallScore)
	}
	if result.OverallEvalStatus != EvalStatusPassed {
		t.Errorf("OverallEvalStatus = %v, want PASSED", result.OverallEvalStatus)
	}
	if len(result.OverallRubricScores) != 2 {
		t.Errorf("len(OverallRubricScores) = %d, want 2", len(result.OverallRubricScores))
	}
}

func TestMeanInvocationResultsSummarizer_Empty(t *testing.T) {
	summarizer := MeanInvocationResultsSummarizer{}
	result := summarizer.Summarize(nil, nil)
	if result.OverallEvalStatus != EvalStatusNotEvaluated {
		t.Errorf("OverallEvalStatus = %v, want NOT_EVALUATED", result.OverallEvalStatus)
	}
}

func TestCreateEffectiveRubricsList(t *testing.T) {
	evalMetric := EvalMetric{
		MetricName: "test",
		Criterion: &RubricsBasedCriterion{
			Rubrics: []Rubric{
				{RubricID: "r1", RubricContent: RubricContent{TextProperty: "prop1"}},
			},
		},
	}
	base := NewRubricBasedEvaluator(
		evalMetric,
		*evalMetric.Criterion.(*RubricsBasedCriterion),
		nil,
		"TOOL_USE_QUALITY",
		"template",
	)
	// Add invocation-level rubrics with matching type.
	invocationRubrics := []Rubric{
		{RubricID: "r2", RubricContent: RubricContent{TextProperty: "prop2"}, Type: "TOOL_USE_QUALITY"},
		{RubricID: "r1", RubricContent: RubricContent{TextProperty: "override"}, Type: "TOOL_USE_QUALITY"},
	}
	base.CreateEffectiveRubricsList(invocationRubrics)
	if len(base.effectiveRubricsList) != 2 {
		t.Errorf("len(effectiveRubricsList) = %d, want 2", len(base.effectiveRubricsList))
	}
	// Ensure case-level rubric r1 is not overwritten.
	for _, r := range base.effectiveRubricsList {
		if r.RubricID == "r1" && r.RubricContent.TextProperty != "prop1" {
			t.Errorf("case-level rubric r1 was overwritten")
		}
	}
}

func TestCreateEffectiveRubricsList_NoInvocationRubrics(t *testing.T) {
	evalMetric := EvalMetric{
		MetricName: "test",
		Criterion: &RubricsBasedCriterion{
			Rubrics: []Rubric{
				{RubricID: "r1", RubricContent: RubricContent{TextProperty: "prop1"}},
			},
		},
	}
	base := NewRubricBasedEvaluator(
		evalMetric,
		*evalMetric.Criterion.(*RubricsBasedCriterion),
		nil,
		"",
		"template",
	)
	base.CreateEffectiveRubricsList(nil)
	if len(base.effectiveRubricsList) != 1 {
		t.Errorf("len(effectiveRubricsList) = %d, want 1", len(base.effectiveRubricsList))
	}
}

func TestNormalizeText(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"  Hello World  ", "hello world"},
		{"Test", "test"},
		{"  ", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizeText(tt.input)
		if got != tt.want {
			t.Errorf("normalizeText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
