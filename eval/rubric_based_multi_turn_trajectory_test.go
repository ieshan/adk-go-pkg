package eval_test

import (
	"context"
	"testing"

	"github.com/ieshan/adk-go-pkg/eval"
	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/genai"
)

func TestRubricBasedMultiTurnTrajectoryEvaluator_LastTurnEvaluated(t *testing.T) {
	threshold := 0.8
	fakeLLM := testutil.NewFakeLLM(testutil.NewTextResponse(
		`Property: The trajectory is coherent.
Rationale: The agent followed the plan.
Verdict: yes`,
	))
	evalMetric := eval.EvalMetric{
		MetricName: string(eval.RubricBasedMultiTurnTrajectoryQualityV1),
		Threshold:  &threshold,
		Criterion: &eval.RubricsBasedCriterion{
			Rubrics: []eval.Rubric{
				{RubricID: "r1", RubricContent: eval.RubricContent{TextProperty: "The trajectory is coherent."}},
			},
			LlmAsAJudgeCriterion: eval.LlmAsAJudgeCriterion{JudgeModelOptions: eval.JudgeModelOptions{NumSamples: 1}},
		},
	}
	e, err := eval.NewRubricBasedMultiTurnTrajectoryEvaluator(evalMetric, fakeLLM)
	if err != nil {
		t.Fatalf("NewRubricBasedMultiTurnTrajectoryEvaluator failed: %v", err)
	}

	actual := []eval.Invocation{
		{UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "turn 1"}}}},
		{UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "turn 2"}}}},
	}
	expected := []eval.Invocation{
		{UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "turn 1"}}}},
		{UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "turn 2"}}}},
	}

	result, err := e.EvaluateInvocations(context.Background(), actual, expected, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if len(result.PerInvocationResults) != 2 {
		t.Errorf("len(PerInvocationResults) = %d, want 2", len(result.PerInvocationResults))
	}
	// First turn should be NOT_EVALUATED.
	if result.PerInvocationResults[0].EvalStatus != eval.EvalStatusNotEvaluated {
		t.Errorf("PerInvocationResults[0].EvalStatus = %v, want NOT_EVALUATED", result.PerInvocationResults[0].EvalStatus)
	}
	// Last turn should be evaluated (PASSED).
	if result.PerInvocationResults[1].EvalStatus != eval.EvalStatusPassed {
		t.Errorf("PerInvocationResults[1].EvalStatus = %v, want PASSED", result.PerInvocationResults[1].EvalStatus)
	}
}

func TestRubricBasedMultiTurnTrajectoryEvaluator_SingleTurn(t *testing.T) {
	threshold := 0.8
	fakeLLM := testutil.NewFakeLLM(testutil.NewTextResponse(
		`Property: The trajectory is coherent.
Rationale: Good.
Verdict: yes`,
	))
	evalMetric := eval.EvalMetric{
		MetricName: string(eval.RubricBasedMultiTurnTrajectoryQualityV1),
		Threshold:  &threshold,
		Criterion: &eval.RubricsBasedCriterion{
			Rubrics: []eval.Rubric{
				{RubricID: "r1", RubricContent: eval.RubricContent{TextProperty: "The trajectory is coherent."}},
			},
			LlmAsAJudgeCriterion: eval.LlmAsAJudgeCriterion{JudgeModelOptions: eval.JudgeModelOptions{NumSamples: 1}},
		},
	}
	e, err := eval.NewRubricBasedMultiTurnTrajectoryEvaluator(evalMetric, fakeLLM)
	if err != nil {
		t.Fatalf("NewRubricBasedMultiTurnTrajectoryEvaluator failed: %v", err)
	}

	actual := []eval.Invocation{
		{UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "only turn"}}}},
	}

	result, err := e.EvaluateInvocations(context.Background(), actual, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != eval.EvalStatusPassed {
		t.Errorf("OverallEvalStatus = %v, want PASSED", result.OverallEvalStatus)
	}
}

func TestRubricBasedMultiTurnTrajectoryEvaluator_NoRubrics(t *testing.T) {
	fakeLLM := testutil.NewFakeLLM()
	evalMetric := eval.EvalMetric{
		MetricName: string(eval.RubricBasedMultiTurnTrajectoryQualityV1),
		Criterion:  &eval.RubricsBasedCriterion{},
	}
	_, err := eval.NewRubricBasedMultiTurnTrajectoryEvaluator(evalMetric, fakeLLM)
	if err == nil {
		t.Errorf("got nil error, want non-nil error when no rubrics provided")
	}
}

func TestRubricBasedMultiTurnTrajectoryEvaluator_EmptyResults(t *testing.T) {
	fakeLLM := testutil.NewFakeLLM()
	evalMetric := eval.EvalMetric{
		MetricName: string(eval.RubricBasedMultiTurnTrajectoryQualityV1),
		Criterion: &eval.RubricsBasedCriterion{
			Rubrics:              []eval.Rubric{{RubricID: "r1", RubricContent: eval.RubricContent{TextProperty: "test"}}},
			LlmAsAJudgeCriterion: eval.LlmAsAJudgeCriterion{JudgeModelOptions: eval.JudgeModelOptions{NumSamples: 1}},
		},
	}
	e, err := eval.NewRubricBasedMultiTurnTrajectoryEvaluator(evalMetric, fakeLLM)
	if err != nil {
		t.Fatalf("NewRubricBasedMultiTurnTrajectoryEvaluator failed: %v", err)
	}

	result, err := e.EvaluateInvocations(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != eval.EvalStatusNotEvaluated {
		t.Errorf("OverallEvalStatus = %v, want NOT_EVALUATED", result.OverallEvalStatus)
	}
}
