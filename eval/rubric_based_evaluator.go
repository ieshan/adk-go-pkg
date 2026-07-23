package eval

import (
	"regexp"
	"strings"

	"google.golang.org/adk/v2/model"
)

// RubricBasedEvaluator is a base class for rubric-based LLM evaluators.
// It extends LlmAsJudgeEvaluator with rubric parsing, aggregation, and
// summarization capabilities.
type RubricBasedEvaluator struct {
	*LlmAsJudgeEvaluator

	rubricType              string
	rubrics                 []Rubric
	effectiveRubricsList    []Rubric
	autoRaterPromptTemplate string
	responseParser          AutoRaterResponseParser
	resultsAggregator       PerInvocationResultsAggregator
	resultsSummarizer       InvocationResultsSummarizer
}

// AutoRaterResponseParser parses auto-rater responses into rubric scores.
type AutoRaterResponseParser interface {
	Parse(response string) ([]RubricResponse, error)
}

// RubricResponse represents a parsed rubric response from the auto-rater.
type RubricResponse struct {
	PropertyText string
	Rationale    string
	Score        *float64
}

// PerInvocationResultsAggregator aggregates multiple samples for a single
// invocation.
type PerInvocationResultsAggregator interface {
	Aggregate(samples []PerInvocationResult, threshold *float64) PerInvocationResult
}

// InvocationResultsSummarizer summarizes per-invocation results into an
// overall evaluation result.
type InvocationResultsSummarizer interface {
	Summarize(perInvocation []PerInvocationResult, threshold *float64) EvaluationResult
}

// DefaultAutoRaterResponseParser parses Property/Rationale/Verdict format.
type DefaultAutoRaterResponseParser struct{}

var (
	propertyPattern  = regexp.MustCompile(`(?i)Property:\s*(.*)`)
	rationalePattern = regexp.MustCompile(`(?i)Rationale:\s*(.*)`)
	verdictPattern   = regexp.MustCompile(`(?i)Verdict:\s*(.*)`)
)

func extractFirstGroup(re *regexp.Regexp, s string) []string {
	matches := re.FindAllStringSubmatch(s, -1)
	var results []string
	for _, m := range matches {
		if len(m) > 1 {
			results = append(results, m[1])
		}
	}
	return results
}

// Parse extracts rubric responses from the auto-rater's text output.
func (DefaultAutoRaterResponseParser) Parse(response string) ([]RubricResponse, error) {
	properties := extractFirstGroup(propertyPattern, response)
	rationales := extractFirstGroup(rationalePattern, response)
	verdicts := extractFirstGroup(verdictPattern, response)

	var results []RubricResponse
	for i, verdict := range verdicts {
		var score *float64
		lower := strings.ToLower(verdict)
		if strings.Contains(lower, "yes") {
			s := 1.0
			score = &s
		} else if strings.Contains(lower, "no") {
			s := 0.0
			score = &s
		}

		prop := ""
		if i < len(properties) {
			prop = strings.TrimSpace(properties[i])
		}
		rat := ""
		if i < len(rationales) {
			rat = strings.TrimSpace(rationales[i])
		}

		results = append(results, RubricResponse{
			PropertyText: prop,
			Rationale:    rat,
			Score:        score,
		})
	}
	return results, nil
}

// MajorityVotePerInvocationResultsAggregator aggregates using majority vote.
type MajorityVotePerInvocationResultsAggregator struct{}

// Aggregate combines multiple per-invocation samples using majority vote.
func (MajorityVotePerInvocationResultsAggregator) Aggregate(samples []PerInvocationResult, threshold *float64) PerInvocationResult {
	type buckets struct {
		noScores  []RubricScore
		positives []RubricScore
		negatives []RubricScore
	}

	scoreByRubricID := make(map[string]*buckets)

	for _, sample := range samples {
		for _, rs := range sample.RubricScores {
			b, ok := scoreByRubricID[rs.RubricID]
			if !ok {
				b = &buckets{}
				scoreByRubricID[rs.RubricID] = b
			}
			if rs.Score == nil {
				b.noScores = append(b.noScores, rs)
			} else if *rs.Score == 1.0 {
				b.positives = append(b.positives, rs)
			} else {
				b.negatives = append(b.negatives, rs)
			}
		}
	}

	var aggregated []RubricScore
	for _, b := range scoreByRubricID {
		if len(b.positives) == 0 && len(b.negatives) == 0 {
			if len(b.noScores) > 0 {
				aggregated = append(aggregated, b.noScores[0])
			}
		} else if len(b.positives) > len(b.negatives) {
			aggregated = append(aggregated, b.positives[0])
		} else {
			aggregated = append(aggregated, b.negatives[0])
		}
	}

	overallScore := GetAverageRubricScore(aggregated)

	result := PerInvocationResult{
		ActualInvocation:   samples[0].ActualInvocation,
		ExpectedInvocation: samples[0].ExpectedInvocation,
		Score:              overallScore,
		RubricScores:       aggregated,
		EvalStatus:         GetEvalStatus(overallScore, threshold),
	}
	return result
}

// MeanInvocationResultsSummarizer summarizes using mean score.
type MeanInvocationResultsSummarizer struct{}

// Summarize computes the mean score across per-invocation results.
func (MeanInvocationResultsSummarizer) Summarize(perInvocation []PerInvocationResult, threshold *float64) EvaluationResult {
	rubricScoresByID := make(map[string][]RubricScore)
	var allScores []RubricScore

	for _, r := range perInvocation {
		for _, rs := range r.RubricScores {
			rubricScoresByID[rs.RubricID] = append(rubricScoresByID[rs.RubricID], rs)
			allScores = append(allScores, rs)
		}
	}

	var aggregated []RubricScore
	for rubricID, scores := range rubricScoresByID {
		avg := GetAverageRubricScore(scores)
		aggregated = append(aggregated, RubricScore{
			RubricID:  rubricID,
			Score:     avg,
			Rationale: "This is an aggregated score derived from individual entries. Please refer to individual entries in each invocation for actual rationale from the model.",
		})
	}

	overallScore := GetAverageRubricScore(allScores)
	return EvaluationResult{
		OverallScore:         overallScore,
		OverallEvalStatus:    GetEvalStatus(overallScore, threshold),
		PerInvocationResults: perInvocation,
		OverallRubricScores:  aggregated,
	}
}

// NewRubricBasedEvaluator creates a new RubricBasedEvaluator.
func NewRubricBasedEvaluator(
	evalMetric EvalMetric,
	criterionType RubricsBasedCriterion,
	llm model.LLM,
	rubricType string,
	promptTemplate string,
) *RubricBasedEvaluator {
	base := NewLlmAsJudgeEvaluator(evalMetric, criterionType.LlmAsAJudgeCriterion, llm, false)

	e := &RubricBasedEvaluator{
		LlmAsJudgeEvaluator:     base,
		rubricType:              rubricType,
		rubrics:                 criterionType.Rubrics,
		autoRaterPromptTemplate: promptTemplate,
		responseParser:          DefaultAutoRaterResponseParser{},
		resultsAggregator:       MajorityVotePerInvocationResultsAggregator{},
		resultsSummarizer:       MeanInvocationResultsSummarizer{},
	}

	base.ConvertAutoRaterResponseToScore = e.parseResponse
	base.AggregatePerInvocationSamplesFunc = e.aggregateSamples
	base.AggregateInvocationResultsFunc = e.aggregateInvocations

	// Initialize effective rubrics list from case-level rubrics.
	e.CreateEffectiveRubricsList(nil)

	return e
}

// CreateEffectiveRubricsList merges case-level and invocation-level rubrics.
func (e *RubricBasedEvaluator) CreateEffectiveRubricsList(invocationRubrics []Rubric) {
	byID := make(map[string]Rubric)

	for _, r := range e.rubrics {
		byID[r.RubricID] = r
	}

	if invocationRubrics != nil {
		filtered := invocationRubrics
		if e.rubricType != "" {
			filtered = nil
			for _, r := range invocationRubrics {
				if r.Type == e.rubricType {
					filtered = append(filtered, r)
				}
			}
		}
		for _, r := range filtered {
			if _, exists := byID[r.RubricID]; exists {
				continue // Don't overwrite
			}
			byID[r.RubricID] = r
		}
	}

	e.effectiveRubricsList = make([]Rubric, 0, len(byID))
	for _, r := range byID {
		e.effectiveRubricsList = append(e.effectiveRubricsList, r)
	}
}

func (e *RubricBasedEvaluator) parseResponse(resp *model.LLMResponse) AutoRaterScore {
	text := GetTextFromContent(resp.Content)
	if text == "" {
		return AutoRaterScore{}
	}

	rubricResponses, err := e.responseParser.Parse(text)
	if err != nil || len(rubricResponses) == 0 {
		return AutoRaterScore{}
	}

	// Build normalized rubric map.
	normalizedMap := make(map[string]Rubric)
	for _, r := range e.effectiveRubricsList {
		normalizedMap[normalizeText(r.RubricContent.TextProperty)] = r
	}

	var rubricScores []RubricScore
	for _, rr := range rubricResponses {
		normalized := normalizeText(rr.PropertyText)
		rubric, ok := normalizedMap[normalized]
		if !ok {
			continue
		}
		rubricScores = append(rubricScores, RubricScore{
			RubricID:  rubric.RubricID,
			Rationale: rr.Rationale,
			Score:     rr.Score,
		})
	}

	avgScore := GetAverageRubricScore(rubricScores)
	return AutoRaterScore{Score: avgScore, RubricScores: rubricScores}
}

func (e *RubricBasedEvaluator) aggregateSamples(samples []PerInvocationResult) PerInvocationResult {
	return e.resultsAggregator.Aggregate(samples, e.evalMetric.Threshold)
}

func (e *RubricBasedEvaluator) aggregateInvocations(perInvocation []PerInvocationResult) EvaluationResult {
	return e.resultsSummarizer.Summarize(perInvocation, e.evalMetric.Threshold)
}

func normalizeText(text string) string {
	return strings.ToLower(strings.TrimSpace(text))
}
