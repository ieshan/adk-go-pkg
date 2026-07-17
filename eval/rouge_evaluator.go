package eval

import (
	"context"
	"math"
	"strings"
	"unicode"
)

// RougeEvaluator evaluates agent final responses against expected final
// responses using the ROUGE-1 metric (unigram overlap F-measure).
type RougeEvaluator struct {
	evalMetric EvalMetric
}

// NewRougeEvaluator creates a new RougeEvaluator.
func NewRougeEvaluator(evalMetric EvalMetric) *RougeEvaluator {
	return &RougeEvaluator{evalMetric: evalMetric}
}

// EvaluateInvocations compares actual final responses against expected
// final responses using ROUGE-1 F-measure.
func (e *RougeEvaluator) EvaluateInvocations(
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

		if expected == nil || expected.FinalResponse == nil {
			result.EvalStatus = EvalStatusNotEvaluated
			perInvocationResults = append(perInvocationResults, result)
			continue
		}

		actualText := GetTextFromContent(actual.FinalResponse)
		expectedText := GetTextFromContent(expected.FinalResponse)

		score := Rouge1Score(actualText, expectedText)
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

// Rouge1Score computes the ROUGE-1 F-measure between candidate and reference
// texts. It uses unigram (single word) overlap with F-measure.
func Rouge1Score(candidate, reference string) float64 {
	candidateTokens := tokenize(candidate)
	referenceTokens := tokenize(reference)

	if len(referenceTokens) == 0 && len(candidateTokens) == 0 {
		return 1.0
	}
	if len(referenceTokens) == 0 || len(candidateTokens) == 0 {
		return 0.0
	}

	candidateCounts := countNgrams(candidateTokens, 1)
	referenceCounts := countNgrams(referenceTokens, 1)

	overlap := 0
	for gram, refCount := range referenceCounts {
		if candCount, ok := candidateCounts[gram]; ok {
			overlap += min(candCount, refCount)
		}
	}

	precision := float64(overlap) / float64(len(candidateTokens))
	recall := float64(overlap) / float64(len(referenceTokens))

	if precision+recall == 0 {
		return 0.0
	}

	fMeasure := 2 * precision * recall / (precision + recall)
	return math.Round(fMeasure*1000) / 1000
}

// tokenize splits text into lowercase alphanumeric tokens.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// countNgrams counts n-gram frequencies in a token list.
func countNgrams(tokens []string, n int) map[string]int {
	if n <= 0 || len(tokens) < n {
		return nil
	}

	counts := make(map[string]int)
	for i := 0; i <= len(tokens)-n; i++ {
		gram := strings.Join(tokens[i:i+n], " ")
		counts[gram]++
	}
	return counts
}
