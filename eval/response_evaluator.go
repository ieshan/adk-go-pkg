package eval

import (
	"context"
	"fmt"
)

// ResponseEvaluator supports two metrics:
// - response_match_score: delegates to RougeEvaluator
// - response_evaluation_score: returns NOT_EVALUATED (no Vertex AI support)
type ResponseEvaluator struct {
	evalMetric EvalMetric
	metricName string
	rougeEval  *RougeEvaluator
}

// NewResponseEvaluator creates a new ResponseEvaluator for the given metric.
func NewResponseEvaluator(evalMetric EvalMetric) *ResponseEvaluator {
	return &ResponseEvaluator{
		evalMetric: evalMetric,
		metricName: evalMetric.MetricName,
		rougeEval:  NewRougeEvaluator(evalMetric),
	}
}

// EvaluateInvocations delegates to the appropriate evaluator based on metric.
func (e *ResponseEvaluator) EvaluateInvocations(
	ctx context.Context,
	actualInvocations []Invocation,
	expectedInvocations []Invocation,
	conversationScenario *ConversationScenario,
) (*EvaluationResult, error) {
	switch e.metricName {
	case string(ResponseMatchScore):
		return e.rougeEval.EvaluateInvocations(ctx, actualInvocations, expectedInvocations, conversationScenario)
	case string(ResponseEvaluationScore):
		return notEvaluatedResult(actualInvocations, expectedInvocations), nil
	default:
		return nil, fmt.Errorf("unsupported metric for ResponseEvaluator: %s", e.metricName)
	}
}

// notEvaluatedResult builds an EvaluationResult marking all invocations
// NOT_EVALUATED. Used by evaluators that require Vertex AI (not supported).
func notEvaluatedResult(actual, expected []Invocation) *EvaluationResult {
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
