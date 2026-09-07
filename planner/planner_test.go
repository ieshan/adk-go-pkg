package planner_test

import (
	"testing"

	"github.com/ieshan/adk-go-pkg/planner"
	"google.golang.org/genai"
)

// TestPlan_NoSteps verifies that a Plan with an empty Steps slice is valid
// and can be constructed without error.
func TestPlan_NoSteps(t *testing.T) {
	t.Parallel()
	plan := &planner.Plan{
		Steps:     []planner.PlanStep{},
		Reasoning: "Nothing to do.",
	}

	if len(plan.Steps) != 0 {
		t.Errorf("got %d steps, want 0", len(plan.Steps))
	}
	if plan.Reasoning != "Nothing to do." {
		t.Errorf("unexpected Reasoning: %q", plan.Reasoning)
	}
}

// TestToolDescription verifies that ToolDescription fields are accessible
// and that a nil Parameters field is valid.
func TestToolDescription(t *testing.T) {
	t.Parallel()
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
		t.Error("got non-nil Parameters, want nil")
	}
}

// TestPlan_NilSteps verifies that a Plan with nil Steps can be constructed
// and accessed without panicking. The zero-value Steps slice is valid.
func TestPlan_NilSteps(t *testing.T) {
	t.Parallel()
	plan := &planner.Plan{
		Steps:     nil,
		Reasoning: "No steps needed.",
	}
	// Reaching here without panicking is the primary assertion.
	// Accessing len on a nil slice is safe and returns 0.
	if len(plan.Steps) != 0 {
		t.Errorf("got %d steps, want 0 for nil Steps", len(plan.Steps))
	}
	if plan.Reasoning != "No steps needed." {
		t.Errorf("Reasoning: got %q, want %q", plan.Reasoning, "No steps needed.")
	}
}

// TestToolDescription_NilParameters verifies that a ToolDescription with a nil
// Parameters field can be constructed and accessed without panicking.
func TestToolDescription_NilParameters(t *testing.T) {
	t.Parallel()
	td := planner.ToolDescription{
		Name:        "parameterless_tool",
		Description: "A tool that takes no parameters.",
		Parameters:  nil,
	}
	// Reaching here without panicking is the primary assertion.
	if td.Parameters != nil {
		t.Errorf("Parameters: got %v, want nil", td.Parameters)
	}
	if td.Name != "parameterless_tool" {
		t.Errorf("Name: got %q, want %q", td.Name, "parameterless_tool")
	}
}

// TestToolDescription_EmptyName verifies that a ToolDescription with an empty
// Name can be constructed and accessed without panicking. An empty name is
// valid for reasoning-only steps that do not invoke a tool.
func TestToolDescription_EmptyName(t *testing.T) {
	t.Parallel()
	td := planner.ToolDescription{
		Name:        "",
		Description: "A tool with no name.",
	}
	// Reaching here without panicking is the primary assertion.
	if td.Name != "" {
		t.Errorf("Name: got %q, want empty string", td.Name)
	}
	if td.Description != "A tool with no name." {
		t.Errorf("Description: got %q, want %q", td.Description, "A tool with no name.")
	}
}
