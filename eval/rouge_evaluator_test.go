package eval

import (
	"context"
	"testing"
)

func TestRougeEvaluator_IdenticalStrings(t *testing.T) {
	threshold := 0.8
	evaluator := &RougeEvaluator{
		evalMetric: EvalMetric{MetricName: "response_match_score", Threshold: &threshold},
	}
	inv := []Invocation{{
		FinalResponse: textToContent("the quick brown fox jumps over the lazy dog"),
	}}
	expected := []Invocation{{
		FinalResponse: textToContent("the quick brown fox jumps over the lazy dog"),
	}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, expected, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallScore == nil || *result.OverallScore != 1.0 {
		t.Errorf("got %v, want 1.0 for identical strings", result.OverallScore)
	}
}

func TestRougeEvaluator_NoOverlap(t *testing.T) {
	threshold := 0.8
	evaluator := &RougeEvaluator{
		evalMetric: EvalMetric{MetricName: "response_match_score", Threshold: &threshold},
	}
	inv := []Invocation{{
		FinalResponse: textToContent("apple banana cherry"),
	}}
	expected := []Invocation{{
		FinalResponse: textToContent("dog elephant frog"),
	}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, expected, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallScore == nil || *result.OverallScore != 0.0 {
		t.Errorf("got %v, want 0.0 for no overlap", result.OverallScore)
	}
}

func TestRougeEvaluator_PartialOverlap(t *testing.T) {
	threshold := 0.8
	evaluator := &RougeEvaluator{
		evalMetric: EvalMetric{MetricName: "response_match_score", Threshold: &threshold},
	}
	inv := []Invocation{{
		FinalResponse: textToContent("the quick brown fox"),
	}}
	expected := []Invocation{{
		FinalResponse: textToContent("the quick brown dog"),
	}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, expected, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallScore == nil {
		t.Fatalf("got nil score, want non-nil")
	}
	score := *result.OverallScore
	if score <= 0.0 || score >= 1.0 {
		t.Errorf("got %f, want between 0 and 1 for partial overlap", score)
	}
}

func TestRougeEvaluator_EmptyStrings(t *testing.T) {
	threshold := 0.8
	evaluator := &RougeEvaluator{
		evalMetric: EvalMetric{MetricName: "response_match_score", Threshold: &threshold},
	}
	inv := []Invocation{{
		FinalResponse: textToContent(""),
	}}
	expected := []Invocation{{
		FinalResponse: textToContent(""),
	}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, expected, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	// Both empty — should not error, score should be 0 or 1 depending on convention.
	if result.OverallEvalStatus == EvalStatusNotEvaluated && len(inv) > 0 {
		t.Error("empty strings should not result in NOT_EVALUATED when invocations exist")
	}
}

func TestRougeEvaluator_CaseInsensitivity(t *testing.T) {
	threshold := 0.8
	evaluator := &RougeEvaluator{
		evalMetric: EvalMetric{MetricName: "response_match_score", Threshold: &threshold},
	}
	inv := []Invocation{{
		FinalResponse: textToContent("The Quick Brown Fox"),
	}}
	expected := []Invocation{{
		FinalResponse: textToContent("the quick brown fox"),
	}}

	result, err := evaluator.EvaluateInvocations(context.TODO(), inv, expected, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallScore == nil || *result.OverallScore != 1.0 {
		t.Errorf("got %v, want 1.0 for case-insensitive match", result.OverallScore)
	}
}

func TestCountNgrams(t *testing.T) {
	tokens := []string{"the", "quick", "brown", "fox"}
	grams := countNgrams(tokens, 1)
	if len(grams) != 4 {
		t.Errorf("unigram count = %d, want 4", len(grams))
	}

	grams = countNgrams(tokens, 2)
	if len(grams) != 3 {
		t.Errorf("bigram count = %d, want 3", len(grams))
	}
	if grams["the quick"] != 1 {
		t.Error("missing bigram 'the quick'")
	}
}
