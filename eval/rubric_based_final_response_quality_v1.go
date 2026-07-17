package eval

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/adk/v2/model"
)

// RubricBasedFinalResponseQualityV1Evaluator evaluates final response quality
// against rubrics using LLM as judge.
type RubricBasedFinalResponseQualityV1Evaluator struct {
	*RubricBasedEvaluator
}

const rubricTypeFinalResponseQuality = "FINAL_RESPONSE_QUALITY"

// NewRubricBasedFinalResponseQualityV1Evaluator creates a new evaluator.
func NewRubricBasedFinalResponseQualityV1Evaluator(evalMetric EvalMetric, llm model.LLM) (*RubricBasedFinalResponseQualityV1Evaluator, error) {
	criterion := getRubricsBasedCriterion(evalMetric)
	if len(criterion.Rubrics) == 0 {
		return nil, fmt.Errorf("rubrics are required for rubric_based_final_response_quality_v1")
	}

	base := NewRubricBasedEvaluator(
		evalMetric,
		criterion,
		llm,
		rubricTypeFinalResponseQuality,
		RubricBasedFinalResponseQualityV1Prompt,
	)

	e := &RubricBasedFinalResponseQualityV1Evaluator{
		RubricBasedEvaluator: base,
	}

	// Override prompt formatting.
	base.FormatAutoRaterPrompt = e.formatPrompt

	return e, nil
}

func (e *RubricBasedFinalResponseQualityV1Evaluator) formatPrompt(ctx context.Context, actual Invocation, expected *Invocation) (string, error) {
	includeIntermediate := e.criterion.IncludeIntermediateResponsesInFinal

	userPrompt := GetTextFromContent(actual.UserContent)
	finalAnswer := GetTextFromInvocation(actual, includeIntermediate)
	responseSteps := GetToolCallsAndResponsesAsJSONStr(actual)

	// Build rubrics section.
	var rubricLines []string
	for _, r := range e.effectiveRubricsList {
		rubricLines = append(rubricLines, fmt.Sprintf("Property: %s", r.RubricContent.TextProperty))
	}
	rubricsStr := strings.Join(rubricLines, "\n")

	// Get tool declarations from app details.
	toolDecls := "[]"
	if actual.AppDetails != nil {
		toolDecls = GetToolDeclarationsAsJSONStr(actual.AppDetails)
	}

	prompt := strings.ReplaceAll(e.autoRaterPromptTemplate, "{user_prompt}", userPrompt)
	prompt = strings.ReplaceAll(prompt, "{response_steps}", responseSteps)
	prompt = strings.ReplaceAll(prompt, "{final_answer}", finalAnswer)
	prompt = strings.ReplaceAll(prompt, "{properties}", rubricsStr)
	prompt = strings.ReplaceAll(prompt, "{tool_declarations}", toolDecls)
	return prompt, nil
}

// getRubricsBasedCriterion extracts RubricsBasedCriterion from EvalMetric.
func getRubricsBasedCriterion(evalMetric EvalMetric) RubricsBasedCriterion {
	if evalMetric.Criterion != nil {
		if c, ok := evalMetric.Criterion.(*RubricsBasedCriterion); ok {
			return *c
		}
	}
	return RubricsBasedCriterion{}
}
