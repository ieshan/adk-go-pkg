package simulation

import (
	"fmt"
	"strings"

	"github.com/ieshan/adk-go-pkg/eval"
)

// GetLlmBackedUserSimulatorPrompt builds the default user simulator prompt.
func GetLlmBackedUserSimulatorPrompt(plan, history, stopSignal string) string {
	prompt := DefaultUserSimulatorInstructionsTemplate
	prompt = strings.ReplaceAll(prompt, "{{ conversation_plan }}", plan)
	prompt = strings.ReplaceAll(prompt, "{{ conversation_history }}", history)
	prompt = strings.ReplaceAll(prompt, "{{ stop_signal }}", stopSignal)
	return prompt
}

// GetLlmBackedUserSimulatorPromptWithPersona builds the user simulator
// prompt with a persona.
func GetLlmBackedUserSimulatorPromptWithPersona(plan, history, stopSignal string, persona *eval.UserPersona) string {
	prompt := UserSimulatorInstructionsWithPersonaTemplate

	// Build persona description.
	personaDesc := persona.Description
	if personaDesc == "" {
		personaDesc = "No specific persona."
	}

	// Build behavior instructions.
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

	prompt = strings.ReplaceAll(prompt, "{{ conversation_plan }}", plan)
	prompt = strings.ReplaceAll(prompt, "{{ conversation_history }}", history)
	prompt = strings.ReplaceAll(prompt, "{{ stop_signal }}", stopSignal)
	prompt = strings.ReplaceAll(prompt, "{{ persona_description }}", personaDesc)
	prompt = strings.ReplaceAll(prompt, "{{ persona_behaviors }}", personaBehaviors)
	return prompt
}

// DefaultUserSimulatorInstructionsTemplate is the default prompt template
// for the LLM-backed user simulator.
const DefaultUserSimulatorInstructionsTemplate = `You are playing the role of a user interacting with an AI agent.

Your goal is to follow the conversation plan below and generate realistic user messages.

Conversation Plan:
{{ conversation_plan }}

Conversation History:
{{ conversation_history }}

Instructions:
1. Generate the next user message based on the conversation plan and history.
2. If the agent has completed all goals in the plan, respond with {{ stop_signal }} to end the conversation.
3. If the agent asks a clarifying question, answer it using information from the conversation plan.
4. Do not make up information that is not in the conversation plan.
5. Keep your responses concise and natural.

Next user message:`

// UserSimulatorInstructionsWithPersonaTemplate is the prompt template
// for the LLM-backed user simulator with a persona.
const UserSimulatorInstructionsWithPersonaTemplate = `You are playing the role of a user interacting with an AI agent.

Persona:
{{ persona_description }}

Behaviors:
{{ persona_behaviors }}

Conversation Plan:
{{ conversation_plan }}

Conversation History:
{{ conversation_history }}

Instructions:
1. Generate the next user message based on the conversation plan, history, and your persona.
2. If the agent has completed all goals in the plan, respond with {{ stop_signal }} to end the conversation.
3. If the agent asks a clarifying question, answer it using information from the conversation plan.
4. Do not make up information that is not in the conversation plan.
5. Follow your persona's behavior instructions and avoid violating the rubrics.
6. Keep your responses concise and natural.

Next user message:`

// IsValidUserSimulatorTemplate checks if the given template string contains
// all required parameters (as {{ param }} placeholders). Returns true if all
// required params are present, false otherwise.
func IsValidUserSimulatorTemplate(templateStr string, requiredParams []string) bool {
	for _, param := range requiredParams {
		placeholder := fmt.Sprintf("{{ %s }}", param)
		if !strings.Contains(templateStr, placeholder) {
			return false
		}
	}
	return true
}
