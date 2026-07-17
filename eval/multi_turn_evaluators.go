package eval

import (
	"context"
)

// MultiTurnTaskSuccessV1Evaluator evaluates if the agent achieved the goals
// of the conversation. Without Vertex AI support, this evaluator always
// returns NOT_EVALUATED.
type MultiTurnTaskSuccessV1Evaluator struct {
	evalMetric EvalMetric
}

// NewMultiTurnTaskSuccessV1Evaluator creates a new MultiTurnTaskSuccessV1Evaluator.
func NewMultiTurnTaskSuccessV1Evaluator(evalMetric EvalMetric) *MultiTurnTaskSuccessV1Evaluator {
	return &MultiTurnTaskSuccessV1Evaluator{evalMetric: evalMetric}
}

// EvaluateInvocations returns NOT_EVALUATED for all invocations.
// Only the last turn is considered; prior turns are marked NOT_EVALUATED.
// Vertex AI is not supported in this package.
func (e *MultiTurnTaskSuccessV1Evaluator) EvaluateInvocations(
	ctx context.Context,
	actualInvocations []Invocation,
	expectedInvocations []Invocation,
	conversationScenario *ConversationScenario,
) (*EvaluationResult, error) {
	return multiTurnNotEvaluatedResult(actualInvocations, expectedInvocations), nil
}

// MultiTurnTrajectoryQualityV1Evaluator evaluates the overall trajectory
// quality of the conversation. Without Vertex AI support, this evaluator
// always returns NOT_EVALUATED.
type MultiTurnTrajectoryQualityV1Evaluator struct {
	evalMetric EvalMetric
}

// NewMultiTurnTrajectoryQualityV1Evaluator creates a new evaluator.
func NewMultiTurnTrajectoryQualityV1Evaluator(evalMetric EvalMetric) *MultiTurnTrajectoryQualityV1Evaluator {
	return &MultiTurnTrajectoryQualityV1Evaluator{evalMetric: evalMetric}
}

// EvaluateInvocations returns NOT_EVALUATED for all invocations.
// Only the last turn is considered; prior turns are marked NOT_EVALUATED.
// Vertex AI is not supported in this package.
func (e *MultiTurnTrajectoryQualityV1Evaluator) EvaluateInvocations(
	ctx context.Context,
	actualInvocations []Invocation,
	expectedInvocations []Invocation,
	conversationScenario *ConversationScenario,
) (*EvaluationResult, error) {
	return multiTurnNotEvaluatedResult(actualInvocations, expectedInvocations), nil
}

// MultiTurnToolUseQualityV1Evaluator evaluates the function calls made
// during a multi-turn conversation. Without Vertex AI support, this
// evaluator always returns NOT_EVALUATED.
type MultiTurnToolUseQualityV1Evaluator struct {
	evalMetric EvalMetric
}

// NewMultiTurnToolUseQualityV1Evaluator creates a new evaluator.
func NewMultiTurnToolUseQualityV1Evaluator(evalMetric EvalMetric) *MultiTurnToolUseQualityV1Evaluator {
	return &MultiTurnToolUseQualityV1Evaluator{evalMetric: evalMetric}
}

// EvaluateInvocations returns NOT_EVALUATED for all invocations.
// Only the last turn is considered; prior turns are marked NOT_EVALUATED.
// Vertex AI is not supported in this package.
func (e *MultiTurnToolUseQualityV1Evaluator) EvaluateInvocations(
	ctx context.Context,
	actualInvocations []Invocation,
	expectedInvocations []Invocation,
	conversationScenario *ConversationScenario,
) (*EvaluationResult, error) {
	return multiTurnNotEvaluatedResult(actualInvocations, expectedInvocations), nil
}

// multiTurnNotEvaluatedResult builds an EvaluationResult where all but the
// last invocation are marked NOT_EVALUATED (and the last is also NOT_EVALUATED
// since Vertex AI is not supported).
func multiTurnNotEvaluatedResult(actual, expected []Invocation) *EvaluationResult {
	perInvocation := make([]PerInvocationResult, len(actual))
	for i := range actual {
		var exp *Invocation
		if i < len(expected) {
			exp = &expected[i]
		}
		perInvocation[i] = PerInvocationResult{
			ActualInvocation:   actual[i],
			ExpectedInvocation: exp,
			EvalStatus:         EvalStatusNotEvaluated,
		}
	}
	return &EvaluationResult{
		OverallEvalStatus:    EvalStatusNotEvaluated,
		PerInvocationResults: perInvocation,
	}
}
