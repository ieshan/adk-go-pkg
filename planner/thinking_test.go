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
	t.Parallel()
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
		t.Fatal("got nil Plan, want non-nil")
	}

	// Steps must have been extracted from the embedded JSON.
	if len(plan.Steps) != 1 {
		t.Fatalf("got %d steps from JSON extraction, want 1", len(plan.Steps))
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
	t.Parallel()
	mock := testutil.NewFakeLLM(testutil.NewTextResponse(thinkingResponsePlain))
	p := planner.NewThinking(planner.ThinkingConfig{Model: mock})

	plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "Do something.",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned unexpected error: %v", err)
	}
	if plan == nil {
		t.Fatal("got nil Plan, want non-nil")
	}

	// Fallback: single step whose Description is the full response.
	if len(plan.Steps) != 1 {
		t.Fatalf("got %d fallback steps, want 1", len(plan.Steps))
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
	t.Parallel()
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
		t.Fatal("got nil, want FakeLLM to record the request")
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
		t.Errorf("got prompt without budget value 100, want it present:\n%s", promptText)
	}
}

// TestThinking_ThinkingInstructionTemplate verifies that when a
// ThinkingInstructionTemplate is set, the rendered system instruction includes
// the template data (tools, userMessage, instruction, budget).
func TestThinking_ThinkingInstructionTemplate(t *testing.T) {
	t.Parallel()
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
		t.Fatal("got nil, want FakeLLM to record the request")
	}
	if lastCall.Config == nil || lastCall.Config.SystemInstruction == nil {
		t.Fatal("got nil system instruction in LLM request, want non-nil")
	}
	sysInst := ""
	for _, part := range lastCall.Config.SystemInstruction.Parts {
		sysInst += part.Text
	}
	if !strings.Contains(sysInst, "Tools:") {
		t.Errorf("got system instruction missing 'Tools:', want it present:\n%s", sysInst)
	}
	if !strings.Contains(sysInst, "Search the web.") {
		t.Errorf("got system instruction missing user message, want it present:\n%s", sysInst)
	}
	if !strings.Contains(sysInst, "Be concise.") {
		t.Errorf("got system instruction missing instruction, want it present:\n%s", sysInst)
	}
	if !strings.Contains(sysInst, "Budget: 50") {
		t.Errorf("got system instruction missing budget, want it present:\n%s", sysInst)
	}
}

// TestThinking_ReasoningField verifies that Plan.Reasoning always contains the
// model's full raw response, regardless of whether JSON extraction succeeded
// or fell back.
func TestThinking_ReasoningField(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		response string
	}{
		{"structured JSON", thinkingResponseWithJSON},
		{"plain text fallback", thinkingResponsePlain},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
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

// TestThinking_EmptyJSONBlock verifies that a response containing an empty
// ```json fence (no content between the fences) falls back to a single
// free-text step without panicking. The empty candidate fails to parse as
// JSON, so the fallback path is taken.
func TestThinking_EmptyJSONBlock(t *testing.T) {
	t.Parallel()
	emptyBlockResponse := "Let me think...\n```json\n```\nDone."
	mock := testutil.NewFakeLLM(testutil.NewTextResponse(emptyBlockResponse))
	p := planner.NewThinking(planner.ThinkingConfig{Model: mock})

	plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "test",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned unexpected error: %v", err)
	}
	if plan == nil {
		t.Fatal("got nil Plan, want non-nil")
	}
	// Fallback: single step whose Description is the full response.
	if len(plan.Steps) != 1 {
		t.Fatalf("got %d steps, want 1 (fallback for empty JSON block)", len(plan.Steps))
	}
	if plan.Steps[0].Description != emptyBlockResponse {
		t.Errorf("fallback step Description: got %q, want %q", plan.Steps[0].Description, emptyBlockResponse)
	}
	if plan.Reasoning != emptyBlockResponse {
		t.Errorf("Reasoning: got %q, want %q", plan.Reasoning, emptyBlockResponse)
	}
}

// TestThinking_MalformedFence verifies that a response with an unclosed ```
// fence does not panic and falls back to a single free-text step. The
// extractor cannot find a closing fence, so no JSON candidate is produced.
func TestThinking_MalformedFence(t *testing.T) {
	t.Parallel()
	malformedFenceResponse := "Thinking...\n```json\n{\"steps\":[{\"description\":\"x\",\"toolName\":\"\",\"args\":{},\"dependsOn\":[]}]}"
	mock := testutil.NewFakeLLM(testutil.NewTextResponse(malformedFenceResponse))
	p := planner.NewThinking(planner.ThinkingConfig{Model: mock})

	plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "test",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned unexpected error: %v", err)
	}
	if plan == nil {
		t.Fatal("got nil Plan, want non-nil")
	}
	// The unclosed fence means no ```json ... ``` match. However, the raw
	// marker {"steps": is present, so extraction may still succeed. Either
	// way, the plan must have at least one step and not panic.
	if len(plan.Steps) == 0 {
		t.Error("got 0 steps, want at least 1 (extracted or fallback)")
	}
	if plan.Reasoning != malformedFenceResponse {
		t.Errorf("Reasoning: got %q, want %q", plan.Reasoning, malformedFenceResponse)
	}
}

// TestThinking_ZeroThinkingBudget verifies that a ThinkingBudget of 0 does not
// add a budget hint to the prompt and produces a valid plan without panicking.
func TestThinking_ZeroThinkingBudget(t *testing.T) {
	t.Parallel()
	llm := testutil.NewFakeLLM(testutil.NewTextResponse(thinkingResponsePlain))
	p := planner.NewThinking(planner.ThinkingConfig{
		Model:          llm,
		ThinkingBudget: 0,
	})

	plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "test",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned unexpected error: %v", err)
	}
	if plan == nil {
		t.Fatal("got nil Plan, want non-nil")
	}
	if len(plan.Steps) == 0 {
		t.Error("got 0 steps, want at least 1")
	}

	// Verify the prompt does not contain a budget hint.
	lastCall := llm.LastCall()
	if lastCall == nil {
		t.Fatal("got nil, want FakeLLM to record the request")
	}
	var promptText string
	for _, content := range lastCall.Contents {
		for _, part := range content.Parts {
			promptText += part.Text
		}
	}
	if strings.Contains(promptText, "budget:") {
		t.Errorf("got prompt with budget hint for ThinkingBudget=0, want no hint:\n%s", promptText)
	}
}

// TestThinking_NegativeThinkingBudget verifies that a negative ThinkingBudget
// is treated the same as zero (no budget hint) and produces a valid plan
// without panicking.
func TestThinking_NegativeThinkingBudget(t *testing.T) {
	t.Parallel()
	llm := testutil.NewFakeLLM(testutil.NewTextResponse(thinkingResponsePlain))
	p := planner.NewThinking(planner.ThinkingConfig{
		Model:          llm,
		ThinkingBudget: -1,
	})

	plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "test",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned unexpected error: %v", err)
	}
	if plan == nil {
		t.Fatal("got nil Plan, want non-nil")
	}
	if len(plan.Steps) == 0 {
		t.Error("got 0 steps, want at least 1")
	}

	// Verify the prompt does not contain a budget hint.
	lastCall := llm.LastCall()
	if lastCall == nil {
		t.Fatal("got nil, want FakeLLM to record the request")
	}
	var promptText string
	for _, content := range lastCall.Contents {
		for _, part := range content.Parts {
			promptText += part.Text
		}
	}
	if strings.Contains(promptText, "budget:") {
		t.Errorf("got prompt with budget hint for ThinkingBudget=-1, want no hint:\n%s", promptText)
	}
}

// FuzzExtractJSON verifies that the ThinkingPlanner's JSON extraction logic
// (exercised via GeneratePlan) never panics on arbitrary LLM response text.
// The planner should always return a non-nil plan with at least one step,
// using the fallback path when JSON extraction fails.
func FuzzExtractJSON(f *testing.F) {
	// Seed: valid markdown with JSON fence.
	f.Add(thinkingResponseWithJSON)
	// Seed: malformed fence (opening but no closing).
	f.Add("Let me think...\n```json\n{\"steps\":[")
	// Seed: empty string.
	f.Add("")

	f.Fuzz(func(t *testing.T, llmResponse string) {
		mock := testutil.NewFakeLLM(testutil.NewTextResponse(llmResponse))
		p := planner.NewThinking(planner.ThinkingConfig{Model: mock})

		plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
			UserMessage: "test",
		})

		// The function must not panic — reaching here is the primary assertion.
		if err != nil {
			t.Fatalf("GeneratePlan returned unexpected error: %v", err)
		}
		if plan == nil {
			t.Fatal("got nil Plan, want non-nil")
		}
		// The plan must always have at least one step (fallback or extracted).
		if len(plan.Steps) == 0 {
			t.Error("got 0 steps, want at least 1 (fallback or extracted)")
		}
		// Reasoning must always equal the full model response.
		if plan.Reasoning != llmResponse {
			t.Errorf("Reasoning mismatch:\n got:  %q\n want: %q", plan.Reasoning, llmResponse)
		}
	})
}
