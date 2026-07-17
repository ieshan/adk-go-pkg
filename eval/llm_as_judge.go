package eval

import (
	"context"
	"fmt"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

// LlmAsJudgeEvaluator is the base struct for LLM-based auto-rater evaluators.
// Concrete evaluators embed this struct and override the function fields
// for prompt formatting, response parsing, and aggregation.
type LlmAsJudgeEvaluator struct {
	evalMetric                  EvalMetric
	llm                         model.LLM
	criterion                   LlmAsAJudgeCriterion
	expectedInvocationsRequired bool

	// FormatAutoRaterPrompt formats the prompt for the auto-rater LLM.
	// Must be set by the embedding evaluator.
	FormatAutoRaterPrompt func(ctx context.Context, actual Invocation, expected *Invocation) (string, error)

	// ConvertAutoRaterResponseToScore converts the LLM response to an
	// AutoRaterScore. Must be set by the embedding evaluator.
	ConvertAutoRaterResponseToScore func(response *model.LLMResponse) AutoRaterScore

	// AggregatePerInvocationSamplesFunc aggregates multiple samples for a
	// single invocation into one result. Optional — defaults to taking
	// the first successful result.
	AggregatePerInvocationSamplesFunc func(samples []PerInvocationResult) PerInvocationResult

	// AggregateInvocationResultsFunc aggregates per-invocation results
	// into an overall evaluation result. Optional — defaults to averaging
	// scores.
	AggregateInvocationResultsFunc func(perInvocation []PerInvocationResult) EvaluationResult
}

// NewLlmAsJudgeEvaluator creates a new base LlmAsJudgeEvaluator.
func NewLlmAsJudgeEvaluator(
	evalMetric EvalMetric,
	criterionType LlmAsAJudgeCriterion,
	llm model.LLM,
	expectedInvocationsRequired bool,
) *LlmAsJudgeEvaluator {
	return &LlmAsJudgeEvaluator{
		evalMetric:                  evalMetric,
		llm:                         llm,
		criterion:                   criterionType,
		expectedInvocationsRequired: expectedInvocationsRequired,
	}
}

// EvaluateInvocations runs the auto-rater LLM for each invocation, collects
// scores, and aggregates results.
func (e *LlmAsJudgeEvaluator) EvaluateInvocations(
	ctx context.Context,
	actualInvocations []Invocation,
	expectedInvocations []Invocation,
	conversationScenario *ConversationScenario,
) (*EvaluationResult, error) {
	if e.FormatAutoRaterPrompt == nil {
		return nil, fmt.Errorf("FormatAutoRaterPrompt is not set")
	}
	if e.ConvertAutoRaterResponseToScore == nil {
		return nil, fmt.Errorf("ConvertAutoRaterResponseToScore is not set")
	}

	numSamples := e.criterion.JudgeModelOptions.NumSamples
	if numSamples <= 0 {
		numSamples = 5
	}

	var perInvocationResults []PerInvocationResult

	for i, actual := range actualInvocations {
		var expected *Invocation
		if i < len(expectedInvocations) {
			expected = &expectedInvocations[i]
		}

		if e.expectedInvocationsRequired && expected == nil {
			perInvocationResults = append(perInvocationResults, PerInvocationResult{
				ActualInvocation:   actual,
				ExpectedInvocation: expected,
				EvalStatus:         EvalStatusNotEvaluated,
			})
			continue
		}

		prompt, err := e.FormatAutoRaterPrompt(ctx, actual, expected)
		if err != nil {
			return nil, fmt.Errorf("failed to format auto-rater prompt: %w", err)
		}

		var samples []PerInvocationResult
		for s := 0; s < numSamples; s++ {
			score, err := e.callAutoRater(ctx, prompt)
			if err != nil {
				return nil, fmt.Errorf("auto-rater call %d failed for invocation %d: %w", s, i, err)
			}

			result := PerInvocationResult{
				ActualInvocation:   actual,
				ExpectedInvocation: expected,
				Score:              score.Score,
				RubricScores:       score.RubricScores,
			}
			if score.Score != nil {
				result.EvalStatus = GetEvalStatus(score.Score, e.evalMetric.Threshold)
			} else {
				result.EvalStatus = EvalStatusNotEvaluated
			}
			samples = append(samples, result)
		}

		// Aggregate samples for this invocation.
		var aggregated PerInvocationResult
		if e.AggregatePerInvocationSamplesFunc != nil {
			aggregated = e.AggregatePerInvocationSamplesFunc(samples)
		} else {
			aggregated = defaultAggregateSamples(samples)
		}
		perInvocationResults = append(perInvocationResults, aggregated)
	}

	// Aggregate across invocations.
	var result EvaluationResult
	if e.AggregateInvocationResultsFunc != nil {
		result = e.AggregateInvocationResultsFunc(perInvocationResults)
	} else {
		result = defaultAggregateInvocations(perInvocationResults, e.evalMetric.Threshold)
	}

	return &result, nil
}

// callAutoRater calls the LLM with the given prompt and converts the response.
func (e *LlmAsJudgeEvaluator) callAutoRater(ctx context.Context, prompt string) (AutoRaterScore, error) {
	if e.llm == nil {
		return AutoRaterScore{}, fmt.Errorf("LLM is not set for auto-rater")
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{
				Parts: []*genai.Part{{Text: prompt}},
				Role:  "user",
			},
		},
	}

	AddDefaultRetryOptionsIfNotPresent(req)

	var lastResp *model.LLMResponse
	for resp, err := range e.llm.GenerateContent(ctx, req, false) {
		if err != nil {
			return AutoRaterScore{}, fmt.Errorf("LLM generate content failed: %w", err)
		}
		lastResp = resp
	}

	if lastResp == nil {
		return AutoRaterScore{}, fmt.Errorf("LLM returned no response")
	}

	return e.ConvertAutoRaterResponseToScore(lastResp), nil
}

// defaultAggregateSamples takes the first successful sample.
func defaultAggregateSamples(samples []PerInvocationResult) PerInvocationResult {
	if len(samples) == 0 {
		return PerInvocationResult{}
	}
	for _, s := range samples {
		if s.Score != nil {
			return s
		}
	}
	return samples[0]
}

// defaultAggregateInvocations averages scores across invocations.
func defaultAggregateInvocations(perInvocation []PerInvocationResult, threshold *float64) EvaluationResult {
	var total float64
	count := 0
	for _, r := range perInvocation {
		if r.Score != nil && r.EvalStatus != EvalStatusNotEvaluated {
			total += *r.Score
			count++
		}
	}

	result := EvaluationResult{
		PerInvocationResults: perInvocation,
	}

	if count == 0 {
		result.OverallEvalStatus = EvalStatusNotEvaluated
		return result
	}

	avg := total / float64(count)
	result.OverallScore = &avg
	result.OverallEvalStatus = GetEvalStatus(&avg, threshold)
	return result
}
