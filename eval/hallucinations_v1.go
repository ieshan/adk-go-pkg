package eval

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

// positiveLabels are labels that count as non-hallucinated.
var positiveLabels = map[string]bool{
	"supported":      true,
	"not_applicable": true,
}

// sentenceRegex extracts sentences wrapped in <sentence>...</sentence> tags.// sentenceRegex extracts sentences wrapped in <sentence>...</sentence> tags.
var sentenceRegex = regexp.MustCompile(`(?s)<sentence>(.*?)</sentence>`)

// labelLineRegex extracts the label value from a "label: ..." line.
var labelLineRegex = regexp.MustCompile(`(?i)^label:\s*(.*)$`)

// HallucinationsV1Evaluator detects hallucinations in agent responses using
// a two-step LLM process: a segmenter that breaks the response into
// individual sentences, and a validator that checks each sentence for
// factual consistency against tool call responses.
type HallucinationsV1Evaluator struct {
	*LlmAsJudgeEvaluator
	segmenterPrompt string
	validatorPrompt string
}

// NewHallucinationsV1Evaluator creates a new HallucinationsV1Evaluator.
func NewHallucinationsV1Evaluator(evalMetric EvalMetric, llm model.LLM) (*HallucinationsV1Evaluator, error) {
	criterion := getLlmAsAJudgeCriterion(evalMetric)

	base := NewLlmAsJudgeEvaluator(evalMetric, criterion, llm, false)

	e := &HallucinationsV1Evaluator{
		LlmAsJudgeEvaluator: base,
		segmenterPrompt:     HallucinationSegmenterPrompt,
		validatorPrompt:     HallucinationValidatorPrompt,
	}

	base.FormatAutoRaterPrompt = e.formatPrompt
	base.ConvertAutoRaterResponseToScore = e.parseResponse
	base.AggregateInvocationResultsFunc = e.aggregateInvocations

	return e, nil
}

func (e *HallucinationsV1Evaluator) formatPrompt(ctx context.Context, actual Invocation, expected *Invocation) (string, error) {
	includeIntermediate := e.criterion.IncludeIntermediateResponsesInFinal
	responseText := GetTextFromInvocation(actual, includeIntermediate)

	// Step 1: Segment the response into sentences using the LLM.
	segmenterPrompt := strings.ReplaceAll(e.segmenterPrompt, "{response}", responseText)

	segmentedSentences, err := e.segmentResponse(ctx, segmenterPrompt)
	if err != nil {
		return "", fmt.Errorf("segmentation failed: %w", err)
	}

	if len(segmentedSentences) == 0 {
		return "", fmt.Errorf("no sentences segmented from response")
	}

	// Step 2: Build validator prompt with segmented sentences and context.
	contextJSON := GetToolCallsAndResponsesAsJSONStr(actual)

	// Re-wrap sentences in <sentence> tags for the validator input.
	var wrapped []string
	for _, s := range segmentedSentences {
		wrapped = append(wrapped, fmt.Sprintf("<sentence>%s</sentence>", s))
	}

	validatorPrompt := strings.ReplaceAll(e.validatorPrompt, "{context}", contextJSON)
	validatorPrompt = strings.ReplaceAll(validatorPrompt, "{sentences}", strings.Join(wrapped, "\n"))

	return validatorPrompt, nil
}

func (e *HallucinationsV1Evaluator) segmentResponse(ctx context.Context, prompt string) ([]string, error) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{
				Parts: []*genai.Part{{Text: prompt}},
				Role:  "user",
			},
		},
	}

	var lastResp *model.LLMResponse
	for resp, err := range e.llm.GenerateContent(ctx, req, false) {
		if err != nil {
			return nil, fmt.Errorf("segmenter LLM call failed: %w", err)
		}
		lastResp = resp
	}

	if lastResp == nil {
		return nil, fmt.Errorf("segmenter returned no response")
	}

	text := GetTextFromContent(lastResp.Content)
	// Parse sentences from <sentence>...</sentence> tags.
	matches := sentenceRegex.FindAllStringSubmatch(text, -1)
	var sentences []string
	for _, m := range matches {
		s := strings.TrimSpace(m[1])
		if s != "" {
			sentences = append(sentences, s)
		}
	}
	return sentences, nil
}

func (e *HallucinationsV1Evaluator) parseResponse(resp *model.LLMResponse) AutoRaterScore {
	text := GetTextFromContent(resp.Content)
	if text == "" {
		return AutoRaterScore{}
	}

	// Parse validation blocks from the validator response.
	labels := parseValidationLabels(text)
	if len(labels) == 0 {
		return AutoRaterScore{}
	}

	// Accuracy = (supported + not_applicable) / total labeled sentences.
	var positive float64
	for _, label := range labels {
		label = strings.ToLower(strings.TrimSpace(label))
		if positiveLabels[label] {
			positive++
		}
	}

	score := positive / float64(len(labels))
	return AutoRaterScore{Score: &score}
}

// parseValidationLabels extracts the label values from each validation block
// in the validator LLM response. Blocks are separated by "sentence:" prefixes.
func parseValidationLabels(responseText string) []string {
	var labels []string
	for _, block := range strings.Split(responseText, "\nsentence:") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if match := labelLineRegex.FindStringSubmatch(line); match != nil {
				labels = append(labels, strings.TrimSpace(match[1]))
				break
			}
		}
	}
	return labels
}

func (e *HallucinationsV1Evaluator) aggregateInvocations(perInvocation []PerInvocationResult) EvaluationResult {
	return defaultAggregateInvocations(perInvocation, e.evalMetric.Threshold)
}
