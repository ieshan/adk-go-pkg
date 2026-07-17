package eval

import (
	"context"
	"testing"

	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

func TestLlmAsJudgeEvaluator_NilFormatPrompt(t *testing.T) {
	base := NewLlmAsJudgeEvaluator(
		EvalMetric{MetricName: "test"},
		LlmAsAJudgeCriterion{JudgeModelOptions: DefaultJudgeModelOptions()},
		testutil.NewFakeLLM(),
		false,
	)
	// FormatAutoRaterPrompt is nil — should error.
	_, err := base.EvaluateInvocations(context.Background(), nil, nil, nil)
	if err == nil {
		t.Error("expected error when FormatAutoRaterPrompt is nil")
	}
}

func TestLlmAsJudgeEvaluator_NilConvertResponse(t *testing.T) {
	base := NewLlmAsJudgeEvaluator(
		EvalMetric{MetricName: "test"},
		LlmAsAJudgeCriterion{JudgeModelOptions: DefaultJudgeModelOptions()},
		testutil.NewFakeLLM(),
		false,
	)
	base.FormatAutoRaterPrompt = func(ctx context.Context, actual Invocation, expected *Invocation) (string, error) {
		return "test prompt", nil
	}
	// ConvertAutoRaterResponseToScore is nil — should error.
	_, err := base.EvaluateInvocations(context.Background(), nil, nil, nil)
	if err == nil {
		t.Error("expected error when ConvertAutoRaterResponseToScore is nil")
	}
}

func TestLlmAsJudgeEvaluator_NilLLM(t *testing.T) {
	base := NewLlmAsJudgeEvaluator(
		EvalMetric{MetricName: "test"},
		LlmAsAJudgeCriterion{JudgeModelOptions: DefaultJudgeModelOptions()},
		nil,
		false,
	)
	base.FormatAutoRaterPrompt = func(ctx context.Context, actual Invocation, expected *Invocation) (string, error) {
		return "test prompt", nil
	}
	base.ConvertAutoRaterResponseToScore = func(resp *model.LLMResponse) AutoRaterScore {
		return AutoRaterScore{Score: Float64Ptr(1.0)}
	}
	actual := []Invocation{{UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}}}
	_, err := base.EvaluateInvocations(context.Background(), actual, nil, nil)
	if err == nil {
		t.Error("expected error when LLM is nil")
	}
}

func TestLlmAsJudgeEvaluator_BasicEvaluation(t *testing.T) {
	threshold := 0.8
	fakeLLM := testutil.NewFakeLLM(testutil.NewTextResponse("valid"))
	base := NewLlmAsJudgeEvaluator(
		EvalMetric{MetricName: "test", Threshold: &threshold},
		LlmAsAJudgeCriterion{JudgeModelOptions: JudgeModelOptions{NumSamples: 1}},
		fakeLLM,
		false,
	)
	base.FormatAutoRaterPrompt = func(ctx context.Context, actual Invocation, expected *Invocation) (string, error) {
		return "test prompt", nil
	}
	base.ConvertAutoRaterResponseToScore = func(resp *model.LLMResponse) AutoRaterScore {
		return AutoRaterScore{Score: Float64Ptr(1.0)}
	}
	actual := []Invocation{{UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}}}
	result, err := base.EvaluateInvocations(context.Background(), actual, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if result.OverallEvalStatus != EvalStatusPassed {
		t.Errorf("OverallEvalStatus = %v, want PASSED", result.OverallEvalStatus)
	}
}

func TestLlmAsJudgeEvaluator_ExpectedRequiredButMissing(t *testing.T) {
	base := NewLlmAsJudgeEvaluator(
		EvalMetric{MetricName: "test"},
		LlmAsAJudgeCriterion{JudgeModelOptions: JudgeModelOptions{NumSamples: 1}},
		testutil.NewFakeLLM(),
		true, // expectedInvocationsRequired
	)
	base.FormatAutoRaterPrompt = func(ctx context.Context, actual Invocation, expected *Invocation) (string, error) {
		return "test prompt", nil
	}
	base.ConvertAutoRaterResponseToScore = func(resp *model.LLMResponse) AutoRaterScore {
		return AutoRaterScore{Score: Float64Ptr(1.0)}
	}
	actual := []Invocation{{UserContent: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}}}
	result, err := base.EvaluateInvocations(context.Background(), actual, nil, nil)
	if err != nil {
		t.Fatalf("EvaluateInvocations failed: %v", err)
	}
	if len(result.PerInvocationResults) != 1 {
		t.Fatalf("len(PerInvocationResults) = %d, want 1", len(result.PerInvocationResults))
	}
	if result.PerInvocationResults[0].EvalStatus != EvalStatusNotEvaluated {
		t.Errorf("EvalStatus = %v, want NOT_EVALUATED", result.PerInvocationResults[0].EvalStatus)
	}
}

func TestDefaultAggregateSamples(t *testing.T) {
	samples := []PerInvocationResult{
		{EvalStatus: EvalStatusPassed, Score: Float64Ptr(1.0)},
		{EvalStatus: EvalStatusFailed, Score: Float64Ptr(0.0)},
	}
	result := defaultAggregateSamples(samples)
	if result.Score == nil || *result.Score != 1.0 {
		t.Errorf("expected first sample score 1.0, got %v", result.Score)
	}
}

func TestDefaultAggregateSamples_Empty(t *testing.T) {
	result := defaultAggregateSamples(nil)
	if result.Score != nil {
		t.Errorf("expected nil score for empty samples, got %v", result.Score)
	}
}

func TestDefaultAggregateInvocations(t *testing.T) {
	threshold := 0.5
	perInvocation := []PerInvocationResult{
		{EvalStatus: EvalStatusPassed, Score: Float64Ptr(1.0)},
		{EvalStatus: EvalStatusPassed, Score: Float64Ptr(0.8)},
	}
	result := defaultAggregateInvocations(perInvocation, &threshold)
	if result.OverallScore == nil || *result.OverallScore != 0.9 {
		t.Errorf("OverallScore = %v, want 0.9", result.OverallScore)
	}
	if result.OverallEvalStatus != EvalStatusPassed {
		t.Errorf("OverallEvalStatus = %v, want PASSED", result.OverallEvalStatus)
	}
}

func TestDefaultAggregateInvocations_AllNotEvaluated(t *testing.T) {
	perInvocation := []PerInvocationResult{
		{EvalStatus: EvalStatusNotEvaluated},
		{EvalStatus: EvalStatusNotEvaluated},
	}
	result := defaultAggregateInvocations(perInvocation, nil)
	if result.OverallEvalStatus != EvalStatusNotEvaluated {
		t.Errorf("OverallEvalStatus = %v, want NOT_EVALUATED", result.OverallEvalStatus)
	}
}
