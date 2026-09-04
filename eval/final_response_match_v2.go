package eval

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"google.golang.org/adk/v2/model"
)

// FinalResponseMatchV2Evaluator uses an LLM to judge whether the agent's
// final response is valid or invalid compared to a reference response.
// Outputs a score of 0 or 1. Repeated invocation samples are aggregated
// by majority vote.
type FinalResponseMatchV2Evaluator struct {
	*LlmAsJudgeEvaluator
	promptTemplate string
}

// validLabelRegex and invalidLabelRegex parse the LLM judge response.
var (
	validLabelRegex = regexp.MustCompile(
		`"is_the_agent_response_valid":\s*\[*[\n\s]*"*([^"^\]^\s]*)"*[\n\s]*\]*\s*[,\n\}]`,
	)
	invalidLabelRegex = regexp.MustCompile(
		`"is_the_agent_response_invalid":\s*\[*[\n\s]*"*([^"^\]^\s]*)"*[\n\s]*\]*\s*[,\n\}]`,
	)
)

// NewFinalResponseMatchV2Evaluator creates a new FinalResponseMatchV2Evaluator.
func NewFinalResponseMatchV2Evaluator(evalMetric EvalMetric, llm model.LLM) *FinalResponseMatchV2Evaluator {
	criterion := getLlmAsAJudgeCriterion(evalMetric)

	base := NewLlmAsJudgeEvaluator(evalMetric, criterion, llm, true)

	e := &FinalResponseMatchV2Evaluator{
		LlmAsJudgeEvaluator: base,
		promptTemplate:      FinalResponseMatchV2Prompt,
	}

	base.FormatAutoRaterPrompt = e.formatPrompt
	base.ConvertAutoRaterResponseToScore = e.parseResponse
	base.AggregatePerInvocationSamplesFunc = e.aggregateSamples
	base.AggregateInvocationResultsFunc = e.aggregateInvocations

	return e
}

func (e *FinalResponseMatchV2Evaluator) formatPrompt(ctx context.Context, actual Invocation, expected *Invocation) (string, error) {
	if expected == nil {
		return "", fmt.Errorf("expected invocation is required for final_response_match_v2")
	}

	includeIntermediate := e.criterion.IncludeIntermediateResponsesInFinal
	reference := GetTextFromInvocation(*expected, includeIntermediate)
	response := GetTextFromInvocation(actual, includeIntermediate)
	userPrompt := GetTextFromContent(expected.UserContent)

	prompt := strings.ReplaceAll(e.promptTemplate, "{prompt}", userPrompt)
	prompt = strings.ReplaceAll(prompt, "{response}", response)
	prompt = strings.ReplaceAll(prompt, "{golden_response}", reference)
	return prompt, nil
}

func (e *FinalResponseMatchV2Evaluator) parseResponse(resp *model.LLMResponse) AutoRaterScore {
	text := GetTextFromContent(resp.Content)
	if text == "" {
		return AutoRaterScore{}
	}

	label := parseCritique(text)
	switch label {
	case LabelValid:
		return AutoRaterScore{Score: Float64Ptr(1.0)}
	case LabelInvalid:
		return AutoRaterScore{Score: Float64Ptr(0.0)}
	default:
		return AutoRaterScore{}
	}
}

func (e *FinalResponseMatchV2Evaluator) aggregateSamples(samples []PerInvocationResult) PerInvocationResult {
	var positive, negative []PerInvocationResult
	for _, s := range samples {
		if s.Score != nil {
			switch *s.Score {
			case 1.0:
				positive = append(positive, s)
			case 0.0:
				negative = append(negative, s)
			}
		}
	}

	if len(positive) > len(negative) && len(positive) > 0 {
		return positive[0]
	}
	if len(negative) > 0 {
		return negative[0]
	}
	if len(samples) > 0 {
		return samples[0]
	}
	return PerInvocationResult{}
}

func (e *FinalResponseMatchV2Evaluator) aggregateInvocations(perInvocation []PerInvocationResult) EvaluationResult {
	var numValid float64
	numEvaluated := 0
	for _, r := range perInvocation {
		if r.Score == nil || r.EvalStatus == EvalStatusNotEvaluated {
			continue
		}
		numEvaluated++
		numValid += *r.Score
	}

	if numEvaluated == 0 {
		return EvaluationResult{
			OverallEvalStatus:    EvalStatusNotEvaluated,
			PerInvocationResults: perInvocation,
		}
	}

	overallScore := numValid / float64(numEvaluated)
	return EvaluationResult{
		OverallScore:         &overallScore,
		OverallEvalStatus:    GetEvalStatus(&overallScore, e.evalMetric.Threshold),
		PerInvocationResults: perInvocation,
	}
}

// parseCritique parses the judge model response and extracts the label.
func parseCritique(response string) Label {
	validMatch := validLabelRegex.FindStringSubmatch(response)
	invalidMatch := invalidLabelRegex.FindStringSubmatch(response)

	if validMatch != nil {
		label := strings.Trim(strings.Trim(strings.TrimSpace(validMatch[1]), "\\s,}"), " ")
		switch label {
		case string(LabelInvalid), string(LabelAlmost), string(LabelFalse):
			return LabelInvalid
		case string(LabelPartiallyValid), "partially valid", "partially":
			return LabelInvalid
		case string(LabelValid), string(LabelTrue):
			return LabelValid
		default:
			return LabelNotFound
		}
	}

	if invalidMatch != nil {
		label := strings.Trim(strings.Trim(strings.TrimSpace(invalidMatch[1]), "\\s,}"), " ")
		if label == string(LabelTrue) || label == string(LabelInvalid) {
			return LabelInvalid
		}
		return LabelValid
	}

	return LabelNotFound
}

// getLlmAsAJudgeCriterion extracts the LlmAsAJudgeCriterion from an EvalMetric,
// or returns a default one if not present.
func getLlmAsAJudgeCriterion(evalMetric EvalMetric) LlmAsAJudgeCriterion {
	if evalMetric.Criterion != nil {
		if c, ok := evalMetric.Criterion.(*LlmAsAJudgeCriterion); ok {
			return *c
		}
		if c, ok := evalMetric.Criterion.(*RubricsBasedCriterion); ok {
			return c.LlmAsAJudgeCriterion
		}
		if c, ok := evalMetric.Criterion.(*HallucinationsCriterion); ok {
			return c.LlmAsAJudgeCriterion
		}
		if c, ok := evalMetric.Criterion.(*LlmBackedUserSimulatorCriterion); ok {
			return c.LlmAsAJudgeCriterion
		}
	}
	return LlmAsAJudgeCriterion{
		JudgeModelOptions: DefaultJudgeModelOptions(),
	}
}
