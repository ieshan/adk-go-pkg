package simulation

import (
	"fmt"
	"strings"

	"github.com/ieshan/adk-go-pkg/eval"
	"github.com/ieshan/adk-go-pkg/prompt"
)

var simulatorEngine = prompt.New()

// GetLlmBackedUserSimulatorPrompt builds the default user simulator prompt.
func GetLlmBackedUserSimulatorPrompt(plan, history, stopSignal string) (string, error) {
	tmpl := simulatorEngine.MustParse("default-user-simulator", DefaultUserSimulatorInstructionsTemplate)
	return tmpl.Execute(prompt.BuildData(map[string]any{
		"conversation_plan":    plan,
		"conversation_history": history,
		"stop_signal":          stopSignal,
	}))
}

// GetLlmBackedUserSimulatorPromptWithPersona builds the user simulator
// prompt with a persona.
func GetLlmBackedUserSimulatorPromptWithPersona(plan, history, stopSignal string, persona *eval.UserPersona) (string, error) {
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

	tmpl := simulatorEngine.MustParse("persona-user-simulator", UserSimulatorInstructionsWithPersonaTemplate)
	return tmpl.Execute(prompt.BuildData(map[string]any{
		"conversation_plan":    plan,
		"conversation_history": history,
		"stop_signal":          stopSignal,
		"persona_description":  personaDesc,
		"persona_behaviors":    personaBehaviors,
	}))
}

// DefaultUserSimulatorInstructionsTemplate is the default prompt template
// for the LLM-backed user simulator.
const DefaultUserSimulatorInstructionsTemplate = `You are playing the role of a user interacting with an AI agent.

Your goal is to follow the conversation plan below and generate realistic user messages.

Conversation Plan:
{{.Input.conversation_plan}}

Conversation History:
{{.Input.conversation_history}}

Instructions:
1. Generate the next user message based on the conversation plan and history.
2. If the agent has completed all goals in the plan, respond with {{.Input.stop_signal}} to end the conversation.
3. If the agent asks a clarifying question, answer it using information from the conversation plan.
4. Do not make up information that is not in the conversation plan.
5. Keep your responses concise and natural.

Next user message:`

// UserSimulatorInstructionsWithPersonaTemplate is the prompt template
// for the LLM-backed user simulator with a persona.
const UserSimulatorInstructionsWithPersonaTemplate = `You are playing the role of a user interacting with an AI agent.

Persona:
{{.Input.persona_description}}

Behaviors:
{{.Input.persona_behaviors}}

Conversation Plan:
{{.Input.conversation_plan}}

Conversation History:
{{.Input.conversation_history}}

Instructions:
1. Generate the next user message based on the conversation plan, history, and your persona.
2. If the agent has completed all goals in the plan, respond with {{.Input.stop_signal}} to end the conversation.
3. If the agent asks a clarifying question, answer it using information from the conversation plan.
4. Do not make up information that is not in the conversation plan.
5. Follow your persona's behavior instructions and avoid violating the rubrics.
6. Keep your responses concise and natural.

Next user message:`

// IsValidUserSimulatorTemplate checks if the given template string contains
// all required parameters (as {{.Input.param}} placeholders). Returns true if all
// required params are present, false otherwise.
func IsValidUserSimulatorTemplate(templateStr string, requiredParams []string) bool {
	for _, param := range requiredParams {
		if !hasTemplatePlaceholder(templateStr, param) {
			return false
		}
	}
	return true
}

// hasTemplatePlaceholder reports whether templateStr contains a placeholder for
// the named Input parameter, accepting optional whitespace around the field.
func hasTemplatePlaceholder(templateStr, param string) bool {
	compact := fmt.Sprintf("{{.Input.%s}}", param)
	spaced := fmt.Sprintf("{{.Input.%s }}", param)
	return strings.Contains(templateStr, compact) || strings.Contains(templateStr, spaced)
}
