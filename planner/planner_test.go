package planner_test

import (
	"testing"

	"github.com/ieshan/adk-go-pkg/planner"
	"google.golang.org/genai"
)

// TestPlan_NoSteps verifies that a Plan with an empty Steps slice is valid
// and can be constructed without error.
func TestPlan_NoSteps(t *testing.T) {
	plan := &planner.Plan{
		Steps:     []planner.PlanStep{},
		Reasoning: "Nothing to do.",
	}

	if plan == nil {
		t.Fatal("expected non-nil Plan")
	}
	if len(plan.Steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(plan.Steps))
	}
	if plan.Reasoning != "Nothing to do." {
		t.Errorf("unexpected Reasoning: %q", plan.Reasoning)
	}
}

// TestToolDescription verifies that ToolDescription fields are accessible
// and that a nil Parameters field is valid.
func TestToolDescription(t *testing.T) {
	td := planner.ToolDescription{
		Name:        "search_web",
		Description: "Searches the web and returns URLs.",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"query": {Type: genai.TypeString, Description: "The search query"},
			},
			Required: []string{"query"},
		},
	}

	if td.Name != "search_web" {
		t.Errorf("Name: got %q, want %q", td.Name, "search_web")
	}
	if td.Description != "Searches the web and returns URLs." {
		t.Errorf("Description: got %q", td.Description)
	}
	if td.Parameters == nil {
		t.Error("Parameters should not be nil")
	}
	if td.Parameters.Type != genai.TypeObject {
		t.Errorf("Parameters.Type: got %v, want TypeObject", td.Parameters.Type)
	}
	if _, ok := td.Parameters.Properties["query"]; !ok {
		t.Error("Parameters.Properties missing 'query' key")
	}

	// Verify that nil Parameters is also valid.
	tdNoParams := planner.ToolDescription{
		Name:        "no_op",
		Description: "Does nothing.",
		Parameters:  nil,
	}
	if tdNoParams.Parameters != nil {
		t.Error("expected nil Parameters")
	}
}
