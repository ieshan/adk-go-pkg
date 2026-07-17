package eval

import (
	"testing"

	"google.golang.org/genai"
)

func TestAppDetails_Basic(t *testing.T) {
	details := &AppDetails{
		AgentDetails: map[string]AgentDetails{
			"agent1": {
				Name: "agent1",
				ToolDeclarations: []*genai.Tool{
					{FunctionDeclarations: []*genai.FunctionDeclaration{{Name: "get_weather"}}},
				},
			},
		},
	}

	agent, ok := details.AgentDetails["agent1"]
	if !ok {
		t.Fatal("expected agent1 in agents map")
	}
	if agent.Name != "agent1" {
		t.Errorf("got %s, want agent1", agent.Name)
	}
	if len(agent.ToolDeclarations) != 1 {
		t.Errorf("expected 1 tool, got %d", len(agent.ToolDeclarations))
	}
}

func TestAgentDetails_Basic(t *testing.T) {
	agent := AgentDetails{
		Name: "test_agent",
		ToolDeclarations: []*genai.Tool{
			{FunctionDeclarations: []*genai.FunctionDeclaration{{Name: "tool1"}, {Name: "tool2"}}},
		},
	}

	if agent.Name != "test_agent" {
		t.Errorf("got %s, want test_agent", agent.Name)
	}
	if len(agent.ToolDeclarations) != 1 || len(agent.ToolDeclarations[0].FunctionDeclarations) != 2 {
		t.Errorf("expected 2 function declarations")
	}
}

func TestAppDetails_GetDeveloperInstructions(t *testing.T) {
	details := &AppDetails{
		AgentDetails: map[string]AgentDetails{
			"agent1": {Name: "agent1", Instructions: "You are a helpful assistant."},
		},
	}

	t.Run("found", func(t *testing.T) {
		got := details.GetDeveloperInstructions("agent1")
		if got != "You are a helpful assistant." {
			t.Errorf("GetDeveloperInstructions = %q, want %q", got, "You are a helpful assistant.")
		}
	})

	t.Run("not_found", func(t *testing.T) {
		got := details.GetDeveloperInstructions("nonexistent")
		if got != "" {
			t.Errorf("GetDeveloperInstructions = %q, want empty", got)
		}
	})

	t.Run("nil_receiver", func(t *testing.T) {
		var nilDetails *AppDetails
		got := nilDetails.GetDeveloperInstructions("agent1")
		if got != "" {
			t.Errorf("GetDeveloperInstructions on nil = %q, want empty", got)
		}
	})
}

func TestAppDetails_GetToolsByAgentName(t *testing.T) {
	tool := &genai.Tool{FunctionDeclarations: []*genai.FunctionDeclaration{{Name: "get_weather"}}}
	details := &AppDetails{
		AgentDetails: map[string]AgentDetails{
			"agent1": {Name: "agent1", ToolDeclarations: []*genai.Tool{tool}},
			"agent2": {Name: "agent2"},
		},
	}

	result := details.GetToolsByAgentName()
	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}
	if len(result["agent1"]) != 1 {
		t.Errorf("agent1 tools = %d, want 1", len(result["agent1"]))
	}
	if len(result["agent2"]) != 0 {
		t.Errorf("agent2 tools = %d, want 0", len(result["agent2"]))
	}

	t.Run("nil_receiver", func(t *testing.T) {
		var nilDetails *AppDetails
		if nilDetails.GetToolsByAgentName() != nil {
			t.Error("expected nil for nil receiver")
		}
	})
}
