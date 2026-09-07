package planner_test

import (
	"context"
	"fmt"

	"github.com/ieshan/adk-go-pkg/planner"
	"github.com/ieshan/adk-go-pkg/testutil"
)

// ExamplePlanReActPlanner_GeneratePlan demonstrates creating a PlanReAct
// planner and generating a plan from a fake LLM with a canned JSON response.
func ExamplePlanReActPlanner_GeneratePlan() {
	cannedResponse := `{
  "steps": [
    {"description": "Search the web", "toolName": "search_web", "args": {"query": "Go 1.26"}, "dependsOn": []},
    {"description": "Summarise results", "toolName": "summarise", "args": {}, "dependsOn": [0]}
  ],
  "reasoning": "Search first, then summarise."
}`

	llm := testutil.NewFakeLLM(testutil.NewTextResponse(cannedResponse))
	p := planner.NewPlanReAct(planner.PlanReActConfig{
		Model:    llm,
		MaxSteps: 5,
	})

	plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "Find Go 1.26 release notes and summarise them.",
		ToolDescriptions: []planner.ToolDescription{
			{Name: "search_web", Description: "Searches the web."},
			{Name: "summarise", Description: "Summarises text."},
		},
	})
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Printf("Steps: %d\n", len(plan.Steps))
	fmt.Printf("Step 1: %s (%s)\n", plan.Steps[0].Description, plan.Steps[0].ToolName)
	fmt.Printf("Step 2: %s (%s)\n", plan.Steps[1].Description, plan.Steps[1].ToolName)
	fmt.Printf("Reasoning: %s\n", plan.Reasoning)
	// Output:
	// Steps: 2
	// Step 1: Search the web (search_web)
	// Step 2: Summarise results (summarise)
	// Reasoning: Search first, then summarise.
}
