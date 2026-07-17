package eval

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

// TrajectoryEvaluator evaluates tool use trajectories against expected
// sequences. It supports three match types: EXACT, IN_ORDER, and ANY_ORDER.
type TrajectoryEvaluator struct {
	evalMetric EvalMetric
	matchType  MatchType
}

// NewTrajectoryEvaluator creates a new TrajectoryEvaluator.
func NewTrajectoryEvaluator(evalMetric EvalMetric) *TrajectoryEvaluator {
	matchType := MatchExact // default
	if evalMetric.Criterion != nil {
		if tc, ok := evalMetric.Criterion.(*ToolTrajectoryCriterion); ok {
			matchType = tc.MatchType
		}
	}
	return &TrajectoryEvaluator{
		evalMetric: evalMetric,
		matchType:  matchType,
	}
}

// EvaluateInvocations compares actual tool calls against expected tool calls.
func (e *TrajectoryEvaluator) EvaluateInvocations(
	ctx context.Context,
	actualInvocations []Invocation,
	expectedInvocations []Invocation,
	conversationScenario *ConversationScenario,
) (*EvaluationResult, error) {
	var perInvocationResults []PerInvocationResult
	var totalScore float64
	evaluatedCount := 0

	for i, actual := range actualInvocations {
		var expected *Invocation
		if i < len(expectedInvocations) {
			expected = &expectedInvocations[i]
		}

		result := PerInvocationResult{
			ActualInvocation:   actual,
			ExpectedInvocation: expected,
		}

		if expected == nil {
			result.EvalStatus = EvalStatusNotEvaluated
			perInvocationResults = append(perInvocationResults, result)
			continue
		}

		actualTools := GetAllToolCalls(actual)
		expectedTools := GetAllToolCalls(*expected)

		score := e.compareToolCalls(actualTools, expectedTools)
		result.Score = &score
		result.EvalStatus = GetEvalStatus(&score, e.evalMetric.Threshold)
		totalScore += score
		evaluatedCount++
		perInvocationResults = append(perInvocationResults, result)
	}

	overallResult := EvaluationResult{
		PerInvocationResults: perInvocationResults,
	}

	if evaluatedCount == 0 {
		overallResult.OverallEvalStatus = EvalStatusNotEvaluated
		return &overallResult, nil
	}

	avgScore := totalScore / float64(evaluatedCount)
	overallResult.OverallScore = &avgScore
	overallResult.OverallEvalStatus = GetEvalStatus(&avgScore, e.evalMetric.Threshold)
	return &overallResult, nil
}

// compareToolCalls compares actual tool calls against expected tool calls
// using the evaluator's match type.
func (e *TrajectoryEvaluator) compareToolCalls(actual, expected []genai.FunctionCall) float64 {
	if len(expected) == 0 && len(actual) == 0 {
		return 1.0
	}
	if len(expected) == 0 {
		return 0.0
	}

	switch e.matchType {
	case MatchExact:
		return compareExact(actual, expected)
	case MatchInOrder:
		return compareInOrder(actual, expected)
	case MatchAnyOrder:
		return compareAnyOrder(actual, expected)
	default:
		return compareExact(actual, expected)
	}
}

// compareExact returns 1.0 if actual and expected match exactly (same order,
// same names, same args), 0.0 otherwise.
func compareExact(actual, expected []genai.FunctionCall) float64 {
	if len(actual) != len(expected) {
		return 0.0
	}
	for i, exp := range expected {
		if !functionCallsEqual(actual[i], exp) {
			return 0.0
		}
	}
	return 1.0
}

// compareInOrder returns 1.0 if all expected tool calls appear in the actual
// list in the same relative order (extras allowed), 0.0 otherwise.
func compareInOrder(actual, expected []genai.FunctionCall) float64 {
	expIdx := 0
	for _, act := range actual {
		if expIdx >= len(expected) {
			break
		}
		if functionCallsEqual(act, expected[expIdx]) {
			expIdx++
		}
	}
	if expIdx == len(expected) {
		return 1.0
	}
	return 0.0
}

// compareAnyOrder returns 1.0 if all expected tool calls appear in the actual
// list (any order, extras allowed), 0.0 otherwise.
func compareAnyOrder(actual, expected []genai.FunctionCall) float64 {
	used := make([]bool, len(actual))
	for _, exp := range expected {
		found := false
		for i, act := range actual {
			if used[i] {
				continue
			}
			if functionCallsEqual(act, exp) {
				used[i] = true
				found = true
				break
			}
		}
		if !found {
			return 0.0
		}
	}
	return 1.0
}

// functionCallsEqual compares two FunctionCall objects for equality
// (same name and same args).
func functionCallsEqual(a, b genai.FunctionCall) bool {
	if a.Name != b.Name {
		return false
	}
	return mapsEqual(a.Args, b.Args)
}

// mapsEqual compares two map[string]any for equality.
func mapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok {
			return false
		}
		if !valuesEqual(va, vb) {
			return false
		}
	}
	return true
}

// valuesEqual compares two any values for equality.
func valuesEqual(a, b any) bool {
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case float64:
		bv, ok := b.(float64)
		return ok && av == bv
	case int:
		bv, ok := b.(int)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case map[string]any:
		bv, ok := b.(map[string]any)
		return ok && mapsEqual(av, bv)
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !valuesEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	}
}
