package eval_test

import (
	"context"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/genai"
)

func TestRubricBasedToolUseQualityV1Evaluator_AllYes(t *testing.T) {
	threshold := 0.8
	fakeLLM := testutil.NewFakeLLM(testutil.NewTextResponse(
		`Property: The agent used the correct tool.
Rationale: The tool call matches the expected one.
Verdict: yes`,
	))
	evalMetric := eval.EvalMetric{
		MetricName: string(eval.RubricBasedToolUseQualityV1),
		Threshold:  &threshold,
		Criterion: &eval.RubricsBasedCriterion{
			Rubrics: []eval.Rubric{
				{RubricID: "r1", RubricContent: eval.RubricContent{TextProperty: "The agent used the correct tool."}},
			},
			LlmAsAJudgeCriterion: eval.LlmAsAJudgeCriterion{JudgeModelOptions: eval.JudgeModelOptions{NumSamples: 1}},
		},
	}
	e, err := eval.NewRubricBasedToolUseQualityV1Evaluator(evalMetric, fakeLLM)
	if err != nil {
		t.Fatalf("NewRubricBasedToolUseQualityV1Evaluator failed: %v", err)
	}

	actual := []eval.Invocation{{
		UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "get weather"}}},
	}}
	expected := []eval.Invocation{{
		UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "get weather"}}},
	}}

	result, err := e.EvaluateInvocations(context.Background(), actual, expected, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != eval.EvalStatusPassed {
		t.Errorf("OverallEvalStatus = %v, want PASSED", result.OverallEvalStatus)
	}
}

func TestRubricBasedToolUseQualityV1Evaluator_NoRubrics(t *testing.T) {
	fakeLLM := testutil.NewFakeLLM()
	evalMetric := eval.EvalMetric{
		MetricName: string(eval.RubricBasedToolUseQualityV1),
		Criterion:  &eval.RubricsBasedCriterion{},
	}
	_, err := eval.NewRubricBasedToolUseQualityV1Evaluator(evalMetric, fakeLLM)
	if err == nil {
		t.Error("got nil error, want error when no rubrics provided")
	}
}
