package eval

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/adk/v2/model"
)

// RubricBasedToolUseQualityV1Evaluator evaluates tool use quality against
// rubrics using LLM as judge.
type RubricBasedToolUseQualityV1Evaluator struct {
	*RubricBasedEvaluator
}

const rubricTypeToolUseQuality = "TOOL_USE_QUALITY"

// NewRubricBasedToolUseQualityV1Evaluator creates a new evaluator.
func NewRubricBasedToolUseQualityV1Evaluator(evalMetric EvalMetric, llm model.LLM) (*RubricBasedToolUseQualityV1Evaluator, error) {
	criterion := getRubricsBasedCriterion(evalMetric)
	if len(criterion.Rubrics) == 0 {
		return nil, fmt.Errorf("rubrics are required for rubric_based_tool_use_quality_v1")
	}

	base := NewRubricBasedEvaluator(
		evalMetric,
		criterion,
		llm,
		rubricTypeToolUseQuality,
		RubricBasedToolUseQualityV1Prompt,
	)

	e := &RubricBasedToolUseQualityV1Evaluator{
		RubricBasedEvaluator: base,
	}

	base.FormatAutoRaterPrompt = e.formatPrompt
	return e, nil
}

func (e *RubricBasedToolUseQualityV1Evaluator) formatPrompt(ctx context.Context, actual Invocation, expected *Invocation) (string, error) {
	userPrompt := GetTextFromContent(actual.UserContent)
	toolUsage := GetToolCallsAndResponsesAsJSONStr(actual)

	var rubricLines []string
	for _, r := range e.effectiveRubricsList {
		rubricLines = append(rubricLines, fmt.Sprintf("Property: %s", r.RubricContent.TextProperty))
	}
	rubricsStr := strings.Join(rubricLines, "\n")

	toolDecls := "[]"
	if actual.AppDetails != nil {
		toolDecls = GetToolDeclarationsAsJSONStr(actual.AppDetails)
	}

	prompt := strings.ReplaceAll(e.autoRaterPromptTemplate, "{user_prompt}", userPrompt)
	prompt = strings.ReplaceAll(prompt, "{tool_usage}", toolUsage)
	prompt = strings.ReplaceAll(prompt, "{properties}", rubricsStr)
	prompt = strings.ReplaceAll(prompt, "{tool_declarations}", toolDecls)
	return prompt, nil
}
