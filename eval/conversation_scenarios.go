package eval

// ConversationScenario describes a scenario for a conversation between a
// simulated user and the agent under test.
type ConversationScenario struct {
	// StartingPrompt is the fixed first user message given to the agent.
	// Subsequent user messages are obtained by the user simulation system.
	StartingPrompt string `json:"startingPrompt"`

	// ConversationPlan is a plan that the user simulation system follows
	// as it plays out the conversation.
	ConversationPlan string `json:"conversationPlan"`

	// UserPersona is the persona the user simulator should adopt.
	// If a persona ID (string) is specified, the system will try to use
	// one of the default personas.
	UserPersona *UserPersona `json:"userPersona,omitempty"`
}

// ConversationGenerationConfig is the configuration for generating
// conversation scenarios.
type ConversationGenerationConfig struct {
	// Count is the number of conversation scenarios to generate.
	Count int `json:"count"`

	// GenerationInstruction is an optional natural language goal to guide
	// the eval set generation.
	GenerationInstruction string `json:"generationInstruction,omitempty"`

	// EnvironmentContext describes the backend data or state accessible
	// to the agent's tools. This acts as the "ground truth" for the
	// simulation.
	EnvironmentContext string `json:"environmentContext,omitempty"`

	// ModelName is the name of the Gemini model to use for generating
	// scenarios (e.g., "gemini-2.5-flash").
	ModelName string `json:"modelName"`
}
