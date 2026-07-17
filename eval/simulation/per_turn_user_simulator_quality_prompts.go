package simulation

import (
	"fmt"
	"strings"

	"github.com/ieshan/adk-go-pkg/eval"
)

// GetPerTurnUserSimulatorQualityPrompt builds the prompt for evaluating
// user simulator quality per turn without a persona.
func GetPerTurnUserSimulatorQualityPrompt(plan, history, generatedResponse, stopSignal string) string {
	prompt := PerTurnUserSimulatorQualityPromptTemplate
	prompt = strings.ReplaceAll(prompt, "{conversation_plan}", plan)
	prompt = strings.ReplaceAll(prompt, "{conversation_history}", history)
	prompt = strings.ReplaceAll(prompt, "{generated_user_response}", generatedResponse)
	prompt = strings.ReplaceAll(prompt, "{stop_signal}", stopSignal)
	return prompt
}

// GetPerTurnUserSimulatorQualityPromptWithPersona builds the prompt for
// evaluating user simulator quality per turn with a persona.
func GetPerTurnUserSimulatorQualityPromptWithPersona(plan, history, generatedResponse, stopSignal string, persona *eval.UserPersona) string {
	prompt := PerTurnUserSimulatorQualityWithPersonaPromptTemplate

	personaDesc := persona.Description
	if personaDesc == "" {
		personaDesc = "No specific persona."
	}

	var behaviorLines []string
	for _, b := range persona.Behaviors {
		behaviorLines = append(behaviorLines, fmt.Sprintf("- %s: %s", b.Name, b.Description))
		if instr := b.GetBehaviorInstructionsStr(); instr != "" {
			behaviorLines = append(behaviorLines, fmt.Sprintf("  Instructions:\n  %s", strings.ReplaceAll(instr, "\n", "\n  ")))
		}
		if rubrics := b.GetViolationRubricsStr(); rubrics != "" {
			behaviorLines = append(behaviorLines, fmt.Sprintf("  Violation rubrics:\n  %s", strings.ReplaceAll(rubrics, "\n", "\n  ")))
		}
	}
	personaBehaviors := strings.Join(behaviorLines, "\n")

	prompt = strings.ReplaceAll(prompt, "{conversation_plan}", plan)
	prompt = strings.ReplaceAll(prompt, "{conversation_history}", history)
	prompt = strings.ReplaceAll(prompt, "{generated_user_response}", generatedResponse)
	prompt = strings.ReplaceAll(prompt, "{stop_signal}", stopSignal)
	prompt = strings.ReplaceAll(prompt, "{persona_description}", personaDesc)
	prompt = strings.ReplaceAll(prompt, "{persona_behaviors}", personaBehaviors)
	return prompt
}

// PerTurnUserSimulatorQualityPromptTemplate is the prompt template for
// evaluating user simulator quality without a persona.
const PerTurnUserSimulatorQualityPromptTemplate = `You are an expert evaluator for a user simulator in a multi-turn conversation with an AI agent. Your task is to evaluate whether the generated user response follows the conversation plan and conversation history.

Conversation Plan:
{conversation_plan}

Conversation History:
{conversation_history}

Generated User Response:
{generated_user_response}

Stop Signal: {stop_signal}

Evaluate whether the generated user response:
1. Follows the conversation plan.
2. Is consistent with the conversation history.
3. Does not introduce information not present in the plan or history.
4. Uses the stop signal appropriately when the conversation is complete.

Answer "yes" if the response is valid, "no" otherwise.`

// PerTurnUserSimulatorQualityWithPersonaPromptTemplate is the prompt template
// for evaluating user simulator quality with a persona.
const PerTurnUserSimulatorQualityWithPersonaPromptTemplate = `You are an expert evaluator for a user simulator in a multi-turn conversation with an AI agent. Your task is to evaluate whether the generated user response follows the conversation plan, conversation history, and persona behaviors.

Persona:
{persona_description}

Behaviors:
{persona_behaviors}

Conversation Plan:
{conversation_plan}

Conversation History:
{conversation_history}

Generated User Response:
{generated_user_response}

Stop Signal: {stop_signal}

Evaluate whether the generated user response:
1. Follows the conversation plan.
2. Is consistent with the conversation history.
3. Does not introduce information not present in the plan or history.
4. Uses the stop signal appropriately when the conversation is complete.
5. Follows the persona's behavior instructions and avoids violating the rubrics.

Answer "yes" if the response is valid, "no" otherwise.`
