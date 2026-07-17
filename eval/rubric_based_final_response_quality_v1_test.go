package eval

import (
	"context"
	"testing"

	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/genai"
)

func TestRubricBasedFinalResponseQualityV1Evaluator_AllYes(t *testing.T) {
	threshold := 0.8
	fakeLLM := testutil.NewFakeLLM(testutil.NewTextResponse(
		`Property: The response is accurate.
Rationale: The response is correct.
Verdict: yes`,
	))
	evalMetric := EvalMetric{
		MetricName: string(RubricBasedFinalResponseQualityV1),
		Threshold:  &threshold,
		Criterion: &RubricsBasedCriterion{
			Rubrics: []Rubric{
				{RubricID: "r1", RubricContent: RubricContent{TextProperty: "The response is accurate."}},
			},
			LlmAsAJudgeCriterion: LlmAsAJudgeCriterion{JudgeModelOptions: JudgeModelOptions{NumSamples: 1}},
		},
	}
	e, err := NewRubricBasedFinalResponseQualityV1Evaluator(evalMetric, fakeLLM)
	if err != nil {
		t.Fatalf("NewRubricBasedFinalResponseQualityV1Evaluator failed: %v", err)
	}

	actual := []Invocation{{
		UserContent:   &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "what is 2+2?"}}},
		FinalResponse: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "4"}}},
	}}
	expected := []Invocation{{
		UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "what is 2+2?"}}},
	}}

	result, err := e.EvaluateInvocations(context.Background(), actual, expected, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusPassed {
		t.Errorf("OverallEvalStatus = %v, want PASSED", result.OverallEvalStatus)
	}
}

func TestRubricBasedFinalResponseQualityV1Evaluator_NoRubrics(t *testing.T) {
	fakeLLM := testutil.NewFakeLLM()
	evalMetric := EvalMetric{
		MetricName: string(RubricBasedFinalResponseQualityV1),
		Criterion:  &RubricsBasedCriterion{},
	}
	_, err := NewRubricBasedFinalResponseQualityV1Evaluator(evalMetric, fakeLLM)
	if err == nil {
		t.Error("expected error when no rubrics provided")
	}
}
