package planner_test

import (
	"context"
	"iter"
	"testing"

	"github.com/ieshan/adk-go-pkg/planner"
	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// mockLLM is a minimal in-process mock of model.LLM that returns a fixed
// response string. It is intentionally kept simple: no HTTP server, no
// external dependency.
type mockLLM struct {
	response string
}

func (m *mockLLM) Name() string { return "mock" }

func (m *mockLLM) GenerateContent(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{
			Content:      genai.NewContentFromText(m.response, "model"),
			TurnComplete: true,
		}, nil)
	}
}

// threeStepJSON is a well-formed JSON plan with 3 steps.
const threeStepJSON = `{
  "steps": [
    {
      "description": "Fetch latest news headlines",
      "toolName":    "fetch_news",
      "args":        {"category": "technology"},
      "dependsOn":   []
    },
    {
      "description": "Summarise the headlines",
      "toolName":    "summarise",
      "args":        {},
      "dependsOn":   [0]
    },
    {
      "description": "Send summary via email",
      "toolName":    "send_email",
      "args":        {"to": "user@example.com"},
      "dependsOn":   [1]
    }
  ],
  "reasoning": "Fetch data first, then summarise and deliver."
}`

// TestPlanReAct_GeneratePlan verifies that a well-formed JSON response from the
// LLM is correctly parsed into a Plan with the expected steps and reasoning.
func TestPlanReAct_GeneratePlan(t *testing.T) {
	p := planner.NewPlanReAct(planner.PlanReActConfig{
		Model:    &mockLLM{response: threeStepJSON},
		MaxSteps: 10,
	})

	plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "Fetch news, summarise, and email me.",
		ToolDescriptions: []planner.ToolDescription{
			{Name: "fetch_news", Description: "Fetches news headlines."},
			{Name: "summarise", Description: "Summarises text."},
			{Name: "send_email", Description: "Sends an email."},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned unexpected error: %v", err)
	}
	if plan == nil {
		t.Fatal("expected non-nil Plan")
	}
	if len(plan.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(plan.Steps))
	}

	// Step 0 checks.
	if plan.Steps[0].ToolName != "fetch_news" {
		t.Errorf("step 0 ToolName: got %q, want %q", plan.Steps[0].ToolName, "fetch_news")
	}
	if plan.Steps[0].Args["category"] != "technology" {
		t.Errorf("step 0 args[category]: got %v, want %q", plan.Steps[0].Args["category"], "technology")
	}
	if len(plan.Steps[0].DependsOn) != 0 {
		t.Errorf("step 0 DependsOn: got %v, want []", plan.Steps[0].DependsOn)
	}

	// Step 1 checks.
	if plan.Steps[1].ToolName != "summarise" {
		t.Errorf("step 1 ToolName: got %q, want %q", plan.Steps[1].ToolName, "summarise")
	}
	if len(plan.Steps[1].DependsOn) != 1 || plan.Steps[1].DependsOn[0] != 0 {
		t.Errorf("step 1 DependsOn: got %v, want [0]", plan.Steps[1].DependsOn)
	}

	// Step 2 checks.
	if plan.Steps[2].ToolName != "send_email" {
		t.Errorf("step 2 ToolName: got %q, want %q", plan.Steps[2].ToolName, "send_email")
	}
	if len(plan.Steps[2].DependsOn) != 1 || plan.Steps[2].DependsOn[0] != 1 {
		t.Errorf("step 2 DependsOn: got %v, want [1]", plan.Steps[2].DependsOn)
	}

	// Reasoning check.
	if plan.Reasoning == "" {
		t.Error("expected non-empty Reasoning")
	}
}

// TestPlanReAct_MaxSteps verifies that when the LLM returns more steps than
// MaxSteps, the plan is truncated to MaxSteps entries.
func TestPlanReAct_MaxSteps(t *testing.T) {
	// Build a JSON response with 15 steps.
	fifteenStepsJSON := buildNStepsJSON(15)

	p := planner.NewPlanReAct(planner.PlanReActConfig{
		Model:    &mockLLM{response: fifteenStepsJSON},
		MaxSteps: 5,
	})

	plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "Do 15 things.",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned unexpected error: %v", err)
	}
	if len(plan.Steps) != 5 {
		t.Errorf("expected 5 steps after truncation, got %d", len(plan.Steps))
	}
}

// TestPlanReAct_MalformedJSON verifies that a non-JSON model response causes
// GeneratePlan to return an error.
func TestPlanReAct_MalformedJSON(t *testing.T) {
	p := planner.NewPlanReAct(planner.PlanReActConfig{
		Model: &mockLLM{response: "This is not JSON at all!"},
	})

	_, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "Do something.",
	})
	if err == nil {
		t.Fatal("expected an error for malformed JSON, got nil")
	}
}

// TestPlanReAct_EmptyToolDescriptions verifies that GeneratePlan succeeds when
// no tool descriptions are provided — the planner should still parse the LLM
// response and return a valid plan.
func TestPlanReAct_EmptyToolDescriptions(t *testing.T) {
	// A simple single-step plan with no toolName (pure reasoning step).
	noToolJSON := `{
  "steps": [
    {
      "description": "Think about the answer",
      "toolName":    "",
      "args":        {},
      "dependsOn":   []
    }
  ],
  "reasoning": "No tools available; reasoning only."
}`

	p := planner.NewPlanReAct(planner.PlanReActConfig{
		Model:    &mockLLM{response: noToolJSON},
		MaxSteps: 10,
	})

	plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage:      "Tell me something interesting.",
		ToolDescriptions: nil, // explicitly empty
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned unexpected error: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(plan.Steps))
	}
	if plan.Steps[0].ToolName != "" {
		t.Errorf("expected empty ToolName for reasoning-only step, got %q", plan.Steps[0].ToolName)
	}
	if plan.Reasoning == "" {
		t.Error("expected non-empty Reasoning")
	}
}

// buildNStepsJSON constructs a JSON plan string with n identical steps for use
// in MaxSteps truncation tests.
func buildNStepsJSON(n int) string {
	steps := make([]string, n)
	for i := range n {
		steps[i] = `{"description":"step","toolName":"noop","args":{},"dependsOn":[]}`
	}
	joined := ""
	for i, s := range steps {
		if i > 0 {
			joined += ","
		}
		joined += s
	}
	return `{"steps":[` + joined + `],"reasoning":"many steps"}`
}
