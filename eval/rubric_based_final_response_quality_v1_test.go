package eval_test

import (
	"context"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
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
	evalMetric := eval.EvalMetric{
		MetricName: string(eval.RubricBasedFinalResponseQualityV1),
		Threshold:  &threshold,
		Criterion: &eval.RubricsBasedCriterion{
			Rubrics: []eval.Rubric{
				{RubricID: "r1", RubricContent: eval.RubricContent{TextProperty: "The response is accurate."}},
			},
			LlmAsAJudgeCriterion: eval.LlmAsAJudgeCriterion{JudgeModelOptions: eval.JudgeModelOptions{NumSamples: 1}},
		},
	}
	e, err := eval.NewRubricBasedFinalResponseQualityV1Evaluator(evalMetric, fakeLLM)
	if err != nil {
		t.Fatalf("NewRubricBasedFinalResponseQualityV1Evaluator failed: %v", err)
	}

	actual := []eval.Invocation{{
		UserContent:   &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "what is 2+2?"}}},
		FinalResponse: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "4"}}},
	}}
	expected := []eval.Invocation{{
		UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "what is 2+2?"}}},
	}}

	result, err := e.EvaluateInvocations(context.Background(), actual, expected, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != eval.EvalStatusPassed {
		t.Errorf("OverallEvalStatus = %v, want PASSED", result.OverallEvalStatus)
	}
}

func TestRubricBasedFinalResponseQualityV1Evaluator_NoRubrics(t *testing.T) {
	fakeLLM := testutil.NewFakeLLM()
	evalMetric := eval.EvalMetric{
		MetricName: string(eval.RubricBasedFinalResponseQualityV1),
		Criterion:  &eval.RubricsBasedCriterion{},
	}
	_, err := eval.NewRubricBasedFinalResponseQualityV1Evaluator(evalMetric, fakeLLM)
	if err == nil {
		t.Error("got nil error, want error when no rubrics provided")
	}
}
