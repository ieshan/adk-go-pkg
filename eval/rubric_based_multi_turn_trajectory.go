package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

// RubricBasedMultiTurnTrajectoryEvaluator evaluates multi-turn trajectory
// against rubrics using LLM as judge. It assembles the full dialogue history
// from all invocations and evaluates only the last turn with the complete
// conversation context. Prior turns are marked NOT_EVALUATED.
type RubricBasedMultiTurnTrajectoryEvaluator struct {
	*RubricBasedEvaluator

	// formattedDialogue is the assembled dialogue history, set during
	// EvaluateInvocations and consumed by formatPrompt.
	formattedDialogue     string
	formattedInstructions string
	formattedTools        string
}

const rubricTypeTrajectoryQuality = "TRAJECTORY_QUALITY"

// NewRubricBasedMultiTurnTrajectoryEvaluator creates a new evaluator.
func NewRubricBasedMultiTurnTrajectoryEvaluator(evalMetric EvalMetric, llm model.LLM) (*RubricBasedMultiTurnTrajectoryEvaluator, error) {
	criterion := getRubricsBasedCriterion(evalMetric)
	if len(criterion.Rubrics) == 0 {
		return nil, fmt.Errorf("rubrics are required for rubric_based_multi_turn_trajectory_quality_v1")
	}

	base := NewRubricBasedEvaluator(
		evalMetric,
		criterion,
		llm,
		rubricTypeTrajectoryQuality,
		RubricBasedMultiTurnTrajectoryPrompt,
	)

	e := &RubricBasedMultiTurnTrajectoryEvaluator{
		RubricBasedEvaluator: base,
	}

	base.FormatAutoRaterPrompt = e.formatPrompt

	return e, nil
}

// EvaluateInvocations overrides the base implementation to assemble the full
// dialogue history from all invocations, mark the first N-1 turns as
// NOT_EVALUATED, and evaluate only the last turn with the complete context.
func (e *RubricBasedMultiTurnTrajectoryEvaluator) EvaluateInvocations(
	ctx context.Context,
	actualInvocations []Invocation,
	expectedInvocations []Invocation,
	conversationScenario *ConversationScenario,
) (*EvaluationResult, error) {
	if len(actualInvocations) == 0 {
		return &EvaluationResult{OverallEvalStatus: EvalStatusNotEvaluated}, nil
	}

	// Assemble the full dialogue history, instructions, and tools.
	e.assembleDialogueHistory(actualInvocations)

	// Mark the first N-1 turns as NOT_EVALUATED.
	var perInvocationResults []PerInvocationResult
	for i := 0; i < len(actualInvocations)-1; i++ {
		var expected *Invocation
		if i < len(expectedInvocations) {
			expected = &expectedInvocations[i]
		}
		perInvocationResults = append(perInvocationResults, PerInvocationResult{
			ActualInvocation:   actualInvocations[i],
			ExpectedInvocation: expected,
			EvalStatus:         EvalStatusNotEvaluated,
		})
	}

	// Evaluate only the last turn using the base evaluator with full context.
	lastIdx := len(actualInvocations) - 1
	lastActual := []Invocation{actualInvocations[lastIdx]}
	var lastExpected []Invocation
	if lastIdx < len(expectedInvocations) {
		lastExpected = []Invocation{expectedInvocations[lastIdx]}
	}

	lastResult, err := e.RubricBasedEvaluator.EvaluateInvocations(ctx, lastActual, lastExpected, conversationScenario)
	if err != nil {
		return nil, fmt.Errorf("last turn evaluation failed: %w", err)
	}

	// Append the last-turn result.
	if len(lastResult.PerInvocationResults) > 0 {
		perInvocationResults = append(perInvocationResults, lastResult.PerInvocationResults...)
	} else {
		var expected *Invocation
		if lastIdx < len(expectedInvocations) {
			expected = &expectedInvocations[lastIdx]
		}
		perInvocationResults = append(perInvocationResults, PerInvocationResult{
			ActualInvocation:   actualInvocations[lastIdx],
			ExpectedInvocation: expected,
			Score:              lastResult.OverallScore,
			EvalStatus:         lastResult.OverallEvalStatus,
			RubricScores:       lastResult.OverallRubricScores,
		})
	}

	return &EvaluationResult{
		OverallScore:         lastResult.OverallScore,
		OverallEvalStatus:    lastResult.OverallEvalStatus,
		PerInvocationResults: perInvocationResults,
		OverallRubricScores:  lastResult.OverallRubricScores,
	}, nil
}

// assembleDialogueHistory builds the full dialogue history, agent instructions,
// and tool definitions from all invocations, matching the Python
// _assemble_dialogue_history method.
func (e *RubricBasedMultiTurnTrajectoryEvaluator) assembleDialogueHistory(invocations []Invocation) {
	var dialogueLines []string
	var instructionsParts []string
	var toolsParts []string

	for turnIdx, inv := range invocations {
		turnNum := turnIdx + 1

		// USER TURN
		if inv.UserContent != nil {
			textParts := extractTextParts(inv.UserContent.Parts)
			if textParts != "" {
				dialogueLines = append(dialogueLines, fmt.Sprintf("USER TURN %d: %s", turnNum, textParts))
			}
		}

		// INTERMEDIATE TOOL EVENTS
		if inv.IntermediateData != nil {
			if events := inv.IntermediateData.GetInvocationEvents(); events != nil {
				for _, event := range events {
					role := fmt.Sprintf("AGENT (%s)", event.Author)
					if strings.EqualFold(event.Author, "user") {
						role = "USER"
					}
					if event.Content != nil {
						textParts := extractTextParts(event.Content.Parts)
						if textParts != "" {
							dialogueLines = append(dialogueLines, fmt.Sprintf("%s TURN %d: %s", role, turnNum, textParts))
						}
						for _, p := range event.Content.Parts {
							if p.FunctionCall != nil {
								args := "{}"
								if p.FunctionCall.Args != nil {
									if b, err := json.Marshal(p.FunctionCall.Args); err == nil {
										args = string(b)
									}
								}
								dialogueLines = append(dialogueLines, fmt.Sprintf("%s TURN %d (tool call): %s(%s)", role, turnNum, p.FunctionCall.Name, args))
							}
							if p.FunctionResponse != nil {
								resp := "{}"
								if p.FunctionResponse.Response != nil {
									if b, err := json.Marshal(p.FunctionResponse.Response); err == nil {
										resp = string(b)
									}
								}
								dialogueLines = append(dialogueLines, fmt.Sprintf("%s TURN %d (tool output): %s -> %s", role, turnNum, p.FunctionResponse.Name, resp))
							}
						}
					}
				}
			}
		}

		// FINAL AGENT TURN
		if inv.FinalResponse != nil {
			agentName := "agent"
			if inv.IntermediateData != nil {
				if events := inv.IntermediateData.GetInvocationEvents(); len(events) > 0 {
					agentName = events[0].Author
				}
			}
			role := fmt.Sprintf("AGENT (%s)", agentName)
			textParts := extractTextParts(inv.FinalResponse.Parts)
			if textParts != "" {
				dialogueLines = append(dialogueLines, fmt.Sprintf("%s TURN %d: %s", role, turnNum, textParts))
			}
		}

		// Collect instructions and tools from AppDetails.
		if inv.AppDetails != nil && inv.AppDetails.AgentDetails != nil {
			for agentID, details := range inv.AppDetails.AgentDetails {
				instructionsParts = append(instructionsParts, fmt.Sprintf("Agent %s Instructions:\n%s", agentID, details.Instructions))
				toolsParts = append(toolsParts, fmt.Sprintf("Agent: %s", agentID))
				for _, tool := range details.ToolDeclarations {
					for _, fn := range tool.FunctionDeclarations {
						toolsParts = append(toolsParts, fmt.Sprintf("- %s: %s", fn.Name, fn.Description))
					}
				}
			}
		}
	}

	e.formattedDialogue = strings.Join(dialogueLines, "\n")
	e.formattedInstructions = strings.Join(uniqueStrings(instructionsParts), "\n\n")
	e.formattedTools = strings.Join(uniqueStrings(toolsParts), "\n")
}

// formatPrompt renders the prompt for the last turn using the pre-assembled
// dialogue history, instructions, and tools.
func (e *RubricBasedMultiTurnTrajectoryEvaluator) formatPrompt(ctx context.Context, actual Invocation, expected *Invocation) (string, error) {
	e.CreateEffectiveRubricsList(actual.Rubrics)

	var rubricList []map[string]string
	for _, r := range e.effectiveRubricsList {
		entry := map[string]string{"property": r.RubricContent.TextProperty}
		if r.Type != "" {
			entry["type"] = r.Type
		}
		rubricList = append(rubricList, entry)
	}
	formattedRubrics, err := json.MarshalIndent(rubricList, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal rubrics: %w", err)
	}

	prompt := strings.ReplaceAll(e.autoRaterPromptTemplate, "{user_agent_dialogue}", e.formattedDialogue)
	prompt = strings.ReplaceAll(prompt, "{properties}", string(formattedRubrics))
	prompt = strings.ReplaceAll(prompt, "{agent_instructions}", e.formattedInstructions)
	prompt = strings.ReplaceAll(prompt, "{agent_tool_definitions}", e.formattedTools)
	return prompt, nil
}

// extractTextParts joins all non-empty text parts from a slice of genai.Part.
func extractTextParts(parts []*genai.Part) string {
	var texts []string
	for _, p := range parts {
		if p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, " ")
}

// uniqueStrings returns the input slice with duplicates removed, preserving order.
func uniqueStrings(in []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
