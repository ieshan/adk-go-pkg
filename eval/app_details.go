package eval

import (
	"google.golang.org/genai"
)

// AgentDetails contains details about an individual agent in the App.
// This could be a root agent or a sub-agent in the agent tree.
type AgentDetails struct {
	// Name uniquely identifies the agent in the App.
	Name string `json:"name"`

	// Instructions is the system instruction set on the Agent.
	Instructions string `json:"instructions,omitempty"`

	// ToolDeclarations is a list of tools available to the Agent.
	ToolDeclarations []*genai.Tool `json:"toolDeclarations,omitempty"`
}

// AppDetails contains details about the App (the agentic system).
// Only details relevant to the eval system are captured.
type AppDetails struct {
	// AgentDetails maps agent name to details of that agent.
	AgentDetails map[string]AgentDetails `json:"agentDetails,omitempty"`
}

// GetDeveloperInstructions returns the system instructions for the given agent.
// Returns empty string if the agent is not found.
func (a *AppDetails) GetDeveloperInstructions(agentName string) string {
	if a == nil {
		return ""
	}
	details, ok := a.AgentDetails[agentName]
	if !ok {
		return ""
	}
	return details.Instructions
}

// GetToolsByAgentName returns a map of agent name to tool declarations.
func (a *AppDetails) GetToolsByAgentName() map[string][]*genai.Tool {
	if a == nil {
		return nil
	}
	result := make(map[string][]*genai.Tool, len(a.AgentDetails))
	for name, details := range a.AgentDetails {
		result[name] = details.ToolDeclarations
	}
	return result
}
