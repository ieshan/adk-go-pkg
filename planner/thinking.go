package planner

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// ThinkingConfig holds the configuration for a [ThinkingPlanner].
//
// Example:
//
//	cfg := planner.ThinkingConfig{
//	    Model:          myLLM,
//	    ThinkingBudget: 512,
//	}
//	p := planner.NewThinking(cfg)
type ThinkingConfig struct {
	// Model is the LLM used to generate the plan. Required.
	Model model.LLM

	// ThinkingBudget is an optional hint for the model indicating the
	// approximate token budget it should use for its reasoning process.
	// A value of 0 means no budget hint is added to the prompt.
	ThinkingBudget int
}

// ThinkingPlanner generates execution plans by prompting an LLM to reason
// step-by-step about the user's request.  Unlike [PlanReActPlanner], it does
// not require the model to emit strictly-formatted JSON: it first tries to
// extract a JSON plan embedded in the response (inside ```json fences or as a
// raw {"steps":…} object), and falls back to a single free-text step when no
// parseable JSON is found.
//
// The full model response is always stored in [Plan.Reasoning], giving
// callers access to the model's chain-of-thought even when structured
// extraction succeeds.
//
// It implements the [Planner] interface.
//
// Example:
//
//	p := planner.NewThinking(planner.ThinkingConfig{
//	    Model:          myLLM,
//	    ThinkingBudget: 256,
//	})
//
//	plan, err := p.GeneratePlan(ctx, &planner.PlanRequest{
//	    UserMessage: "Find the weather and email it to me.",
//	    ToolDescriptions: []planner.ToolDescription{
//	        {Name: "get_weather", Description: "Returns current weather for a city."},
//	        {Name: "send_email",  Description: "Sends an email."},
//	    },
//	})
type ThinkingPlanner struct {
	cfg ThinkingConfig
}

// NewThinking constructs a [ThinkingPlanner] from the given configuration.
//
// Example:
//
//	p := planner.NewThinking(planner.ThinkingConfig{Model: myLLM})
func NewThinking(cfg ThinkingConfig) *ThinkingPlanner {
	return &ThinkingPlanner{cfg: cfg}
}

// GeneratePlan sends a step-by-step reasoning prompt to the configured LLM and
// returns a [Plan].
//
// The algorithm is:
//  1. Build a prompt that asks the model to think step-by-step, lists the
//     available tools, and includes the user message.  If [ThinkingConfig.ThinkingBudget]
//     is positive, a budget hint is appended to the prompt.
//  2. Call the model with streaming disabled.
//  3. Collect the full response text and store it in [Plan.Reasoning].
//  4. Attempt to extract a JSON plan from the response.  The extractor looks
//     for ```json … ``` fences first, then for a raw object starting with
//     `{"steps":`.
//  5. If JSON is found and parses correctly, populate [Plan.Steps] from it.
//  6. Otherwise fall back to a single [PlanStep] whose Description contains
//     the full response text.
//
// GeneratePlan returns an error only when the model call itself fails.  JSON
// parse failures are handled via the fallback path (step 6) rather than
// propagated as errors.
//
// Example:
//
//	plan, err := p.GeneratePlan(ctx, &planner.PlanRequest{
//	    UserMessage: "Search the web for Go 1.24 release notes and email a summary.",
//	    ToolDescriptions: []planner.ToolDescription{
//	        {Name: "search_web", Description: "Searches the web."},
//	        {Name: "send_email", Description: "Sends an email."},
//	    },
//	})
func (p *ThinkingPlanner) GeneratePlan(ctx context.Context, input *PlanRequest) (*Plan, error) {
	prompt := p.buildPrompt(input)

	req := &model.LLMRequest{
		Contents: []*genai.Content{
			genai.NewContentFromText(prompt, "user"),
		},
		Config: &genai.GenerateContentConfig{
			SystemInstruction: genai.NewContentFromText(p.thinkingInstruction(), "system"),
		},
	}

	var responseText string
	var callErr error

	for resp, err := range p.cfg.Model.GenerateContent(ctx, req, false) {
		if err != nil {
			callErr = err
			break
		}
		if resp == nil || resp.Content == nil {
			continue
		}
		for _, part := range resp.Content.Parts {
			if part.Text != "" {
				responseText += part.Text
			}
		}
	}
	if callErr != nil {
		return nil, fmt.Errorf("planner: thinking model call failed: %w", callErr)
	}

	plan := &Plan{
		Reasoning: responseText,
	}

	// Try to extract and parse a JSON plan from the response.
	if extracted, ok := extractJSONFromResponse(responseText); ok {
		if parsed, err := parsePlanJSON(extracted); err == nil {
			plan.Steps = parsed.Steps
			return plan, nil
		}
	}

	// Fallback: create a single free-text step from the full response.
	plan.Steps = []PlanStep{
		{Description: responseText},
	}
	return plan, nil
}

// thinkingInstruction returns the system-level instruction for the thinking
// planner. This is sent via SystemInstruction on the LLM request Config.
func (p *ThinkingPlanner) thinkingInstruction() string {
	var sb strings.Builder

	sb.WriteString("You are a planning assistant. Think step by step about how to accomplish the user's request.\n")
	sb.WriteString("If possible, produce a structured JSON plan matching the schema below.\n\n")
	sb.WriteString("JSON schema (optional but preferred):\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"steps\": [\n")
	sb.WriteString("    {\n")
	sb.WriteString("      \"description\": \"<human-readable description>\",\n")
	sb.WriteString("      \"toolName\":    \"<tool name or empty string>\",\n")
	sb.WriteString("      \"args\":        {},\n")
	sb.WriteString("      \"dependsOn\":   []\n")
	sb.WriteString("    }\n")
	sb.WriteString("  ],\n")
	sb.WriteString("  \"reasoning\": \"<brief explanation>\"\n")
	sb.WriteString("}\n")

	return sb.String()
}

// buildPrompt assembles the user-facing portion of the thinking prompt.
//
// The system instruction is now sent via SystemInstruction on the LLM request
// Config, so this method only includes tool descriptions, optional budget hint,
// caller instruction, and the user message.
func (p *ThinkingPlanner) buildPrompt(input *PlanRequest) string {
	var sb strings.Builder

	// Optional thinking budget hint.
	if p.cfg.ThinkingBudget > 0 {
		sb.WriteString(fmt.Sprintf("Think carefully, budget: %d tokens\n\n", p.cfg.ThinkingBudget))
	}

	// Tool descriptions section.
	if len(input.ToolDescriptions) > 0 {
		sb.WriteString("## Available Tools\n\n")
		for _, td := range input.ToolDescriptions {
			sb.WriteString("- **")
			sb.WriteString(td.Name)
			sb.WriteString("**: ")
			sb.WriteString(td.Description)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("## Available Tools\n\n(none)\n\n")
	}

	// Optional caller instruction.
	if input.Instruction != "" {
		sb.WriteString("## Additional Instruction\n\n")
		sb.WriteString(input.Instruction)
		sb.WriteString("\n\n")
	}

	// User message.
	sb.WriteString("## User Request\n\n")
	sb.WriteString(input.UserMessage)

	return sb.String()
}

// extractJSONFromResponse tries to find a JSON object in raw.
//
// It checks two locations in order:
//  1. Inside a ```json … ``` markdown code fence.
//  2. A raw JSON object starting with `{"steps":` anywhere in the string.
//
// It returns the candidate JSON string and true when a candidate is found,
// or ("", false) when none is detected.
func extractJSONFromResponse(raw string) (string, bool) {
	// 1. Look for ```json … ``` fences.
	const fenceOpen = "```json"
	const fenceClose = "```"

	if start := strings.Index(raw, fenceOpen); start != -1 {
		after := raw[start+len(fenceOpen):]
		if end := strings.Index(after, fenceClose); end != -1 {
			candidate := strings.TrimSpace(after[:end])
			return candidate, true
		}
	}

	// 2. Look for a raw JSON object that starts with {"steps":
	// NOTE: This is a heuristic check. Whitespace variations (e.g. `{ "steps":`)
	// or different field ordering (e.g. `{"reasoning":...,"steps":...}`) will not
	// match and will cause a fallback to the single-step plan. This is by design:
	// the fallback path is always safe, and requiring an exact prefix keeps the
	// extraction logic simple and predictable.
	const rawMarker = `{"steps":`
	if idx := strings.Index(raw, rawMarker); idx != -1 {
		return raw[idx:], true
	}

	return "", false
}
