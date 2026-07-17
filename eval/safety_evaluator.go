package eval

import (
	"context"
)

// SafetyEvaluatorV1 evaluates safety (harmlessness) of an agent's response.
// Without Vertex AI support, this evaluator always returns NOT_EVALUATED.
// Value range is [0, 1], with values closer to 1 being more desirable (safe).
type SafetyEvaluatorV1 struct {
	evalMetric EvalMetric
}

// NewSafetyEvaluatorV1 creates a new SafetyEvaluatorV1.
func NewSafetyEvaluatorV1(evalMetric EvalMetric) *SafetyEvaluatorV1 {
	return &SafetyEvaluatorV1{evalMetric: evalMetric}
}

// EvaluateInvocations returns NOT_EVALUATED for all invocations.
// Vertex AI is not supported in this package.
func (s *SafetyEvaluatorV1) EvaluateInvocations(
	ctx context.Context,
	actualInvocations []Invocation,
	expectedInvocations []Invocation,
	conversationScenario *ConversationScenario,
) (*EvaluationResult, error) {
	return notEvaluatedResult(actualInvocations, expectedInvocations), nil
}
