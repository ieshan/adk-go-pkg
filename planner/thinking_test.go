package planner_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ieshan/adk-go-pkg/planner"
	"github.com/ieshan/adk-go-pkg/prompt"
	"github.com/ieshan/adk-go-pkg/testutil"
)

// thinkingResponseWithJSON is a response that embeds a JSON plan inside a
// markdown code fence, mimicking a model that "thinks aloud" before producing
// structured output.
const thinkingResponseWithJSON = "Let me think...\n```json\n{\"steps\":[{\"description\":\"search\",\"toolName\":\"search\",\"args\":{},\"dependsOn\":[]}],\"reasoning\":\"because\"}\n```"

// thinkingResponsePlain is a plain-text response with no JSON at all.
const thinkingResponsePlain = "I should first search the web, then summarise what I find."

// TestThinking_GeneratePlan_Structured verifies that when the model returns a
// response containing an embedded JSON plan (inside ```json fences), the
// ThinkingPlanner extracts the steps correctly and sets Plan.Reasoning to the
// full model response.
func TestThinking_GeneratePlan_Structured(t *testing.T) {
	mock := testutil.NewFakeLLM(testutil.NewTextResponse(thinkingResponseWithJSON))
	p := planner.NewThinking(planner.ThinkingConfig{Model: mock})

	plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "Search the web and summarise.",
		ToolDescriptions: []planner.ToolDescription{
			{Name: "search", Description: "Searches the web."},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned unexpected error: %v", err)
	}
	if plan == nil {
		t.Fatal("expected non-nil Plan")
	}

	// Steps must have been extracted from the embedded JSON.
	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 step from JSON extraction, got %d", len(plan.Steps))
	}
	if plan.Steps[0].ToolName != "search" {
		t.Errorf("step 0 ToolName: got %q, want %q", plan.Steps[0].ToolName, "search")
	}
	if plan.Steps[0].Description != "search" {
		t.Errorf("step 0 Description: got %q, want %q", plan.Steps[0].Description, "search")
	}

	// Reasoning must equal the full model response, not just the JSON portion.
	if plan.Reasoning != thinkingResponseWithJSON {
		t.Errorf("Reasoning mismatch:\n got:  %q\n want: %q", plan.Reasoning, thinkingResponseWithJSON)
	}
}

// TestThinking_GeneratePlan_Fallback verifies that when the model returns plain
// text with no embedded JSON, the ThinkingPlanner falls back to a single
// PlanStep whose Description equals the full response text, and that
// Plan.Reasoning is also set to the full response text.
func TestThinking_GeneratePlan_Fallback(t *testing.T) {
	mock := testutil.NewFakeLLM(testutil.NewTextResponse(thinkingResponsePlain))
	p := planner.NewThinking(planner.ThinkingConfig{Model: mock})

	plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "Do something.",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned unexpected error: %v", err)
	}
	if plan == nil {
		t.Fatal("expected non-nil Plan")
	}

	// Fallback: single step whose Description is the full response.
	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 fallback step, got %d", len(plan.Steps))
	}
	if plan.Steps[0].Description != thinkingResponsePlain {
		t.Errorf("fallback step Description mismatch:\n got:  %q\n want: %q",
			plan.Steps[0].Description, thinkingResponsePlain)
	}

	// Reasoning must equal the full model response.
	if plan.Reasoning != thinkingResponsePlain {
		t.Errorf("Reasoning mismatch:\n got:  %q\n want: %q", plan.Reasoning, thinkingResponsePlain)
	}
}

// TestThinking_ThinkingBudget verifies that when ThinkingBudget > 0 is set in
// ThinkingConfig, the budget value appears somewhere in the prompt sent to the
// model.
func TestThinking_ThinkingBudget(t *testing.T) {
	llmCapture := testutil.NewFakeLLM(testutil.NewTextResponse(thinkingResponsePlain))
	p := planner.NewThinking(planner.ThinkingConfig{
		Model:          llmCapture,
		ThinkingBudget: 100,
	})

	_, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "Do something.",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned unexpected error: %v", err)
	}

	lastCall := llmCapture.LastCall()
	if lastCall == nil {
		t.Fatal("expected FakeLLM to record the request, got nil")
	}

	// Extract the prompt text from the first content part.
	var promptText string
	for _, content := range lastCall.Contents {
		for _, part := range content.Parts {
			promptText += part.Text
		}
	}

	// Budget hint must be present in the prompt.
	if !strings.Contains(promptText, "100") {
		t.Errorf("expected budget value 100 to appear in prompt, prompt was:\n%s", promptText)
	}
}

// TestThinking_ThinkingInstructionTemplate verifies that when a
// ThinkingInstructionTemplate is set, the rendered system instruction includes
// the template data (tools, userMessage, instruction, budget).
func TestThinking_ThinkingInstructionTemplate(t *testing.T) {
	engine := prompt.New()
	tmpl := engine.MustParse("test-thinking-instruction", "Tools: {{.Input.tools}}\nUser: {{.Input.userMessage}}\nInstr: {{.Input.instruction}}\nBudget: {{.Input.budget}}")

	llm := testutil.NewFakeLLM(testutil.NewTextResponse(thinkingResponseWithJSON))
	p := planner.NewThinking(planner.ThinkingConfig{
		Model:                       llm,
		ThinkingInstructionTemplate: tmpl,
		ThinkingBudget:              50,
	})

	_, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "Search the web.",
		Instruction: "Be concise.",
		ToolDescriptions: []planner.ToolDescription{
			{Name: "search", Description: "Searches the web."},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned unexpected error: %v", err)
	}

	lastCall := llm.LastCall()
	if lastCall == nil {
		t.Fatal("expected FakeLLM to record the request, got nil")
	}
	if lastCall.Config == nil || lastCall.Config.SystemInstruction == nil {
		t.Fatal("expected system instruction in LLM request")
	}
	sysInst := ""
	for _, part := range lastCall.Config.SystemInstruction.Parts {
		sysInst += part.Text
	}
	if !strings.Contains(sysInst, "Tools:") {
		t.Errorf("expected 'Tools:' in system instruction, got:\n%s", sysInst)
	}
	if !strings.Contains(sysInst, "Search the web.") {
		t.Errorf("expected user message in system instruction, got:\n%s", sysInst)
	}
	if !strings.Contains(sysInst, "Be concise.") {
		t.Errorf("expected instruction in system instruction, got:\n%s", sysInst)
	}
	if !strings.Contains(sysInst, "Budget: 50") {
		t.Errorf("expected budget in system instruction, got:\n%s", sysInst)
	}
}

// TestThinking_ReasoningField verifies that Plan.Reasoning always contains the
// model's full raw response, regardless of whether JSON extraction succeeded
// or fell back.
func TestThinking_ReasoningField(t *testing.T) {
	tests := []struct {
		name     string
		response string
	}{
		{"structured JSON", thinkingResponseWithJSON},
		{"plain text fallback", thinkingResponsePlain},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := testutil.NewFakeLLM(testutil.NewTextResponse(tc.response))
			p := planner.NewThinking(planner.ThinkingConfig{Model: mock})

			plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
				UserMessage: "test",
			})
			if err != nil {
				t.Fatalf("GeneratePlan returned unexpected error: %v", err)
			}
			if plan.Reasoning != tc.response {
				t.Errorf("Reasoning mismatch for %q:\n got:  %q\n want: %q",
					tc.name, plan.Reasoning, tc.response)
			}
		})
	}
}
