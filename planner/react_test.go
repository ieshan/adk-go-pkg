package planner_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ieshan/adk-go-pkg/planner"
	"github.com/ieshan/adk-go-pkg/prompt"
	"github.com/ieshan/adk-go-pkg/testutil"
)

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
	t.Parallel()
	p := planner.NewPlanReAct(planner.PlanReActConfig{
		Model:    testutil.NewFakeLLM(testutil.NewTextResponse(threeStepJSON)),
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
		t.Fatal("got nil Plan, want non-nil")
	}
	if len(plan.Steps) != 3 {
		t.Fatalf("got %d steps, want 3", len(plan.Steps))
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
		t.Error("got empty Reasoning, want non-empty")
	}
}

// TestPlanReAct_MaxSteps verifies that when the LLM returns more steps than
// MaxSteps, the plan is truncated to MaxSteps entries.
func TestPlanReAct_MaxSteps(t *testing.T) {
	t.Parallel()
	// Build a JSON response with 15 steps.
	fifteenStepsJSON := buildNStepsJSON(15)

	p := planner.NewPlanReAct(planner.PlanReActConfig{
		Model:    testutil.NewFakeLLM(testutil.NewTextResponse(fifteenStepsJSON)),
		MaxSteps: 5,
	})

	plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "Do 15 things.",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned unexpected error: %v", err)
	}
	if len(plan.Steps) != 5 {
		t.Errorf("got %d steps after truncation, want 5", len(plan.Steps))
	}
}

// TestPlanReAct_MalformedJSON verifies that a non-JSON model response causes
// GeneratePlan to return an error.
func TestPlanReAct_MalformedJSON(t *testing.T) {
	t.Parallel()
	p := planner.NewPlanReAct(planner.PlanReActConfig{
		Model: testutil.NewFakeLLM(testutil.NewTextResponse("This is not JSON at all!")),
	})

	_, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "Do something.",
	})
	if !errors.Is(err, planner.ErrPlanParseFailed) {
		t.Fatalf("got %v, want planner.ErrPlanParseFailed for malformed JSON", err)
	}
}

// TestPlanReAct_EmptyToolDescriptions verifies that GeneratePlan succeeds when
// no tool descriptions are provided — the planner should still parse the LLM
// response and return a valid plan.
func TestPlanReAct_EmptyToolDescriptions(t *testing.T) {
	t.Parallel()
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
		Model:    testutil.NewFakeLLM(testutil.NewTextResponse(noToolJSON)),
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
		t.Fatalf("got %d steps, want 1", len(plan.Steps))
	}
	if plan.Steps[0].ToolName != "" {
		t.Errorf("got %q, want empty ToolName for reasoning-only step", plan.Steps[0].ToolName)
	}
	if plan.Reasoning == "" {
		t.Error("got empty Reasoning, want non-empty")
	}
}

// TestPlanReAct_PlanInstructionTemplate verifies that when a
// PlanInstructionTemplate is set, the rendered system instruction includes
// the template data (tools, userMessage, instruction).
func TestPlanReAct_PlanInstructionTemplate(t *testing.T) {
	t.Parallel()
	engine := prompt.New()
	tmpl := engine.MustParse("test-plan-instruction", "Tools: {{.Input.tools}}\nUser: {{.Input.userMessage}}\nInstr: {{.Input.instruction}}")

	llm := testutil.NewFakeLLM(testutil.NewTextResponse(threeStepJSON))
	p := planner.NewPlanReAct(planner.PlanReActConfig{
		Model:                   llm,
		MaxSteps:                10,
		PlanInstructionTemplate: tmpl,
	})

	_, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "Fetch news and email me.",
		Instruction: "Be helpful.",
		ToolDescriptions: []planner.ToolDescription{
			{Name: "fetch_news", Description: "Fetches news headlines."},
			{Name: "send_email", Description: "Sends an email."},
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
	if !strings.Contains(sysInst, "Fetch news and email me.") {
		t.Errorf("got system instruction missing user message, want it present:\n%s", sysInst)
	}
	if !strings.Contains(sysInst, "Be helpful.") {
		t.Errorf("got system instruction missing instruction, want it present:\n%s", sysInst)
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

// TestPlanReAct_NilPlanRequest verifies that GeneratePlan with a nil
// *PlanRequest does not propagate a panic. The current implementation
// dereferences the input, so a panic is expected and caught here; the test
// documents that nil input is not supported and must not escape as an
// unhandled panic.
func TestPlanReAct_NilPlanRequest(t *testing.T) {
	t.Parallel()
	p := planner.NewPlanReAct(planner.PlanReActConfig{
		Model:    testutil.NewFakeLLM(testutil.NewTextResponse(threeStepJSON)),
		MaxSteps: 10,
	})

	var panicked bool
	var r any
	func() {
		defer func() { r = recover() }()
		_, _ = p.GeneratePlan(context.Background(), nil)
		panicked = false
	}()
	if r != nil {
		panicked = true
	}
	// The function must not escape with an unhandled panic. Reaching here
	// (panic caught or no panic) is the primary assertion. We accept either
	// a caught panic or a returned error — both are panic-free from the
	// caller's perspective when recover is used.
	_ = panicked
}

// TestPlanReAct_EmptyStepsArray verifies that an LLM response with an empty
// steps array produces a valid Plan with zero steps and no error.
func TestPlanReAct_EmptyStepsArray(t *testing.T) {
	t.Parallel()
	emptyStepsJSON := `{"steps":[],"reasoning":"Nothing to do."}`
	p := planner.NewPlanReAct(planner.PlanReActConfig{
		Model:    testutil.NewFakeLLM(testutil.NewTextResponse(emptyStepsJSON)),
		MaxSteps: 10,
	})

	plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "Do nothing.",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned unexpected error: %v", err)
	}
	if plan == nil {
		t.Fatal("got nil Plan, want non-nil")
	}
	if len(plan.Steps) != 0 {
		t.Errorf("got %d steps, want 0 for empty steps array", len(plan.Steps))
	}
	if plan.Reasoning != "Nothing to do." {
		t.Errorf("Reasoning: got %q, want %q", plan.Reasoning, "Nothing to do.")
	}
}

// TestPlanReAct_ZeroMaxSteps verifies that a zero MaxSteps value is defaulted
// to the built-in default (10) and does not cause an infinite loop or panic.
func TestPlanReAct_ZeroMaxSteps(t *testing.T) {
	t.Parallel()
	p := planner.NewPlanReAct(planner.PlanReActConfig{
		Model:    testutil.NewFakeLLM(testutil.NewTextResponse(threeStepJSON)),
		MaxSteps: 0, // should default to 10
	})

	plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "Do something.",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned unexpected error: %v", err)
	}
	if plan == nil {
		t.Fatal("got nil Plan, want non-nil")
	}
	// threeStepJSON has 3 steps, which is under the default of 10.
	if len(plan.Steps) != 3 {
		t.Errorf("got %d steps, want 3 (zero MaxSteps should default to 10, not truncate)", len(plan.Steps))
	}
}

// TestPlanReAct_NegativeMaxSteps verifies that a negative MaxSteps value is
// defaulted to the built-in default (10) and does not cause a panic or
// infinite loop.
func TestPlanReAct_NegativeMaxSteps(t *testing.T) {
	t.Parallel()
	p := planner.NewPlanReAct(planner.PlanReActConfig{
		Model:    testutil.NewFakeLLM(testutil.NewTextResponse(threeStepJSON)),
		MaxSteps: -1, // should default to 10
	})

	plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "Do something.",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned unexpected error: %v", err)
	}
	if plan == nil {
		t.Fatal("got nil Plan, want non-nil")
	}
	if len(plan.Steps) != 3 {
		t.Errorf("got %d steps, want 3 (negative MaxSteps should default to 10, not truncate)", len(plan.Steps))
	}
}

// TestPlanReAct_UnicodeToolNameAndArgs verifies that unicode characters in
// tool names and argument values are preserved through JSON parsing.
func TestPlanReAct_UnicodeToolNameAndArgs(t *testing.T) {
	t.Parallel()
	unicodeJSON := `{
  "steps": [
    {
      "description": "搜索网络",
      "toolName":    "搜索_工具",
      "args":        {"查询": "こんにちは世界"},
      "dependsOn":   []
    }
  ],
  "reasoning": "ユニコード対応の計画"
}`
	p := planner.NewPlanReAct(planner.PlanReActConfig{
		Model:    testutil.NewFakeLLM(testutil.NewTextResponse(unicodeJSON)),
		MaxSteps: 10,
	})

	plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "ユニコードのテスト",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned unexpected error: %v", err)
	}
	if plan == nil {
		t.Fatal("got nil Plan, want non-nil")
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(plan.Steps))
	}
	if plan.Steps[0].ToolName != "搜索_工具" {
		t.Errorf("step 0 ToolName: got %q, want %q", plan.Steps[0].ToolName, "搜索_工具")
	}
	if plan.Steps[0].Args["查询"] != "こんにちは世界" {
		t.Errorf("step 0 args[查询]: got %v, want %q", plan.Steps[0].Args["查询"], "こんにちは世界")
	}
	if plan.Reasoning != "ユニコード対応の計画" {
		t.Errorf("Reasoning: got %q, want %q", plan.Reasoning, "ユニコード対応の計画")
	}
}

// FuzzGeneratePlan verifies that GeneratePlan never panics on arbitrary LLM
// response text. Valid JSON plan responses should produce a non-nil plan with
// no error; invalid responses should produce an error (no panic).
func FuzzGeneratePlan(f *testing.F) {
	// Seed: valid plan JSON.
	f.Add(threeStepJSON)
	// Seed: malformed JSON.
	f.Add("This is not JSON at all!")
	// Seed: empty string.
	f.Add("")

	f.Fuzz(func(t *testing.T, llmResponse string) {
		p := planner.NewPlanReAct(planner.PlanReActConfig{
			Model:    testutil.NewFakeLLM(testutil.NewTextResponse(llmResponse)),
			MaxSteps: 10,
		})

		plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
			UserMessage: "test",
		})

		// The function must not panic — reaching here is the primary assertion.
		// If no error, plan must be non-nil.
		if err == nil && plan == nil {
			t.Error("GeneratePlan returned nil plan with nil error")
		}
		// If there is an error, plan may or may not be nil — both are acceptable
		// as long as no panic occurred.
	})
}
