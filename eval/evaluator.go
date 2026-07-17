package eval

import (
	"context"
)

// Evaluator is the interface for evaluating agent invocations against
// expected results.
type Evaluator interface {
	// EvaluateInvocations compares actual invocations against expected
	// invocations and returns an evaluation result.
	EvaluateInvocations(
		ctx context.Context,
		actualInvocations []Invocation,
		expectedInvocations []Invocation,
		conversationScenario *ConversationScenario,
	) (*EvaluationResult, error)
}

// EvaluationResult holds the overall result of evaluating an eval case
// against a metric.
type EvaluationResult struct {
	// OverallScore is the aggregated score across all invocations.
	// nil if not evaluated.
	OverallScore *float64

	// OverallEvalStatus is the aggregated evaluation status.
	OverallEvalStatus EvalStatus

	// PerInvocationResults contains results for each invocation.
	PerInvocationResults []PerInvocationResult

	// OverallRubricScores contains aggregated rubric scores (if applicable).
	OverallRubricScores []RubricScore
}

// PerInvocationResult holds the evaluation result for a single invocation.
type PerInvocationResult struct {
	// ActualInvocation is the invocation that was evaluated.
	ActualInvocation Invocation

	// ExpectedInvocation is the expected invocation (if any).
	ExpectedInvocation *Invocation

	// Score is the score for this invocation.
	// nil if not evaluated.
	Score *float64

	// EvalStatus is the evaluation status for this invocation.
	EvalStatus EvalStatus

	// RubricScores contains scores for individual rubrics (if applicable).
	RubricScores []RubricScore
}

// AutoRaterScore holds the score produced by an LLM-based auto-rater.
type AutoRaterScore struct {
	// Score is the score from the auto-rater.
	Score *float64

	// RubricScores contains individual rubric scores (if applicable).
	RubricScores []RubricScore
}
