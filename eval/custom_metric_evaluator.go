package eval

import (
	"context"
	"fmt"
)

// CustomMetricFunc is the function signature for custom metric evaluators.
type CustomMetricFunc func(
	ctx context.Context,
	evalMetric EvalMetric,
	actualInvocations []Invocation,
	expectedInvocations []Invocation,
	conversationScenario *ConversationScenario,
) (*EvaluationResult, error)

// CustomMetricEvaluator evaluates using a user-provided function.
type CustomMetricEvaluator struct {
	evalMetric EvalMetric
	metricFunc CustomMetricFunc
}

// NewCustomMetricEvaluator creates a new CustomMetricEvaluator with the
// given function.
func NewCustomMetricEvaluator(evalMetric EvalMetric, fn CustomMetricFunc) *CustomMetricEvaluator {
	return &CustomMetricEvaluator{
		evalMetric: evalMetric,
		metricFunc: fn,
	}
}

// EvaluateInvocations delegates to the custom metric function.
func (e *CustomMetricEvaluator) EvaluateInvocations(
	ctx context.Context,
	actualInvocations []Invocation,
	expectedInvocations []Invocation,
	conversationScenario *ConversationScenario,
) (*EvaluationResult, error) {
	if e.metricFunc == nil {
		return nil, fmt.Errorf("custom metric function is not set")
	}
	return e.metricFunc(ctx, e.evalMetric, actualInvocations, expectedInvocations, conversationScenario)
}
