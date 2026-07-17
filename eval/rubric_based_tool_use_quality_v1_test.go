package eval

import (
	"context"
	"testing"

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
	evalMetric := EvalMetric{
		MetricName: string(RubricBasedToolUseQualityV1),
		Threshold:  &threshold,
		Criterion: &RubricsBasedCriterion{
			Rubrics: []Rubric{
				{RubricID: "r1", RubricContent: RubricContent{TextProperty: "The agent used the correct tool."}},
			},
			LlmAsAJudgeCriterion: LlmAsAJudgeCriterion{JudgeModelOptions: JudgeModelOptions{NumSamples: 1}},
		},
	}
	e, err := NewRubricBasedToolUseQualityV1Evaluator(evalMetric, fakeLLM)
	if err != nil {
		t.Fatalf("NewRubricBasedToolUseQualityV1Evaluator failed: %v", err)
	}

	actual := []Invocation{{
		UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "get weather"}}},
	}}
	expected := []Invocation{{
		UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "get weather"}}},
	}}

	result, err := e.EvaluateInvocations(context.Background(), actual, expected, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusPassed {
		t.Errorf("OverallEvalStatus = %v, want PASSED", result.OverallEvalStatus)
	}
}

func TestRubricBasedToolUseQualityV1Evaluator_NoRubrics(t *testing.T) {
	fakeLLM := testutil.NewFakeLLM()
	evalMetric := EvalMetric{
		MetricName: string(RubricBasedToolUseQualityV1),
		Criterion:  &RubricsBasedCriterion{},
	}
	_, err := NewRubricBasedToolUseQualityV1Evaluator(evalMetric, fakeLLM)
	if err == nil {
		t.Error("expected error when no rubrics provided")
	}
}
