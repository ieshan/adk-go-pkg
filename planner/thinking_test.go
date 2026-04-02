package planner_test

import (
	"context"
	"iter"
	"strings"
	"testing"

	"github.com/ieshan/adk-go-pkg/planner"
	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// capturingLLM is a mock model.LLM that records the last LLMRequest it received
// and returns a fixed response string.  It is used in tests that need to
// inspect the prompt sent to the model (e.g. ThinkingBudget tests).
type capturingLLM struct {
	response    string
	lastRequest *model.LLMRequest
}

func (c *capturingLLM) Name() string { return "capturing-mock" }

func (c *capturingLLM) GenerateContent(_ context.Context, req *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	c.lastRequest = req
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{
			Content:      genai.NewContentFromText(c.response, "model"),
			TurnComplete: true,
		}, nil)
	}
}

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
	mock := &mockLLM{response: thinkingResponseWithJSON}
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
	mock := &mockLLM{response: thinkingResponsePlain}
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
	llmCapture := &capturingLLM{response: thinkingResponsePlain}
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

	if llmCapture.lastRequest == nil {
		t.Fatal("expected capturingLLM to record the request, got nil")
	}

	// Extract the prompt text from the first content part.
	var promptText string
	for _, content := range llmCapture.lastRequest.Contents {
		for _, part := range content.Parts {
			promptText += part.Text
		}
	}

	// Budget hint must be present in the prompt.
	if !strings.Contains(promptText, "100") {
		t.Errorf("expected budget value 100 to appear in prompt, prompt was:\n%s", promptText)
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
			mock := &mockLLM{response: tc.response}
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
