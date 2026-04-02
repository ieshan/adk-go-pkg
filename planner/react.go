package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

const defaultMaxSteps = 10

// defaultPlanInstruction is the system prompt injected into every planning
// request when the caller has not supplied a custom PlanInstruction.
const defaultPlanInstruction = `You are a planning assistant. Given a user request and a list of available tools, produce a structured JSON execution plan.

Your response MUST be valid JSON matching this exact schema:
{
  "steps": [
    {
      "description": "<human-readable description of this step>",
      "toolName":    "<name of the tool to call, or empty string if no tool>",
      "args":        { "<argName>": <argValue>, ... },
      "dependsOn":   [<0-based index of prerequisite steps>, ...]
    }
  ],
  "reasoning": "<brief explanation of your plan>"
}

Rules:
- Return ONLY the JSON object — no markdown fences, no extra text.
- Steps are 0-indexed; dependsOn references must be valid indices.
- args may be an empty object {} when the tool requires no arguments.
- dependsOn may be an empty array [] when the step has no prerequisites.`

// PlanReActConfig holds the configuration for a [PlanReActPlanner].
//
// Example:
//
//	cfg := planner.PlanReActConfig{
//	    Model:           myLLM,
//	    PlanInstruction: "Limit the plan to three steps or fewer.",
//	    MaxSteps:        3,
//	}
//	p := planner.NewPlanReAct(cfg)
type PlanReActConfig struct {
	// Model is the LLM used to generate the plan. Required.
	Model model.LLM

	// PlanInstruction overrides the default planning system prompt.
	// When empty the built-in prompt is used.
	PlanInstruction string

	// MaxSteps caps the number of steps in the returned plan.
	// Steps beyond this limit are silently truncated.
	// Defaults to 10 when zero or negative.
	MaxSteps int
}

// PlanReActPlanner generates structured execution plans by sending a planning
// prompt to an LLM and parsing the JSON response.
//
// It implements the [Planner] interface.
//
// Example:
//
//	p := planner.NewPlanReAct(planner.PlanReActConfig{
//	    Model:    myLLM,
//	    MaxSteps: 5,
//	})
//
//	plan, err := p.GeneratePlan(ctx, &planner.PlanRequest{
//	    UserMessage: "Find the weather and email it to me.",
//	    ToolDescriptions: []planner.ToolDescription{
//	        {Name: "get_weather", Description: "Returns current weather for a city."},
//	        {Name: "send_email",  Description: "Sends an email."},
//	    },
//	})
type PlanReActPlanner struct {
	cfg PlanReActConfig
}

// NewPlanReAct constructs a [PlanReActPlanner] from the given configuration.
//
// If cfg.MaxSteps is zero or negative it is set to the default value (10).
// If cfg.PlanInstruction is empty, the built-in planning prompt is used.
//
// Example:
//
//	p := planner.NewPlanReAct(planner.PlanReActConfig{Model: myLLM})
func NewPlanReAct(cfg PlanReActConfig) *PlanReActPlanner {
	if cfg.MaxSteps <= 0 {
		cfg.MaxSteps = defaultMaxSteps
	}
	if cfg.PlanInstruction == "" {
		cfg.PlanInstruction = defaultPlanInstruction
	}
	return &PlanReActPlanner{cfg: cfg}
}

// GeneratePlan sends a planning prompt to the configured LLM and returns the
// parsed [Plan].
//
// The prompt includes:
//   - The system-level planning instruction (from [PlanReActConfig.PlanInstruction]).
//   - A formatted list of available tool descriptions.
//   - An optional caller-supplied instruction from [PlanRequest.Instruction].
//   - The user message from [PlanRequest.UserMessage].
//
// Steps are truncated to [PlanReActConfig.MaxSteps] after parsing.
//
// Returns an error if the model call fails or the response is not valid JSON.
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
//	if err != nil {
//	    log.Fatal(err)
//	}
func (p *PlanReActPlanner) GeneratePlan(ctx context.Context, input *PlanRequest) (*Plan, error) {
	prompt := p.buildPrompt(input)

	req := &model.LLMRequest{
		Contents: []*genai.Content{
			genai.NewContentFromText(prompt, "user"),
		},
		Config: &genai.GenerateContentConfig{
			SystemInstruction: genai.NewContentFromText(p.cfg.PlanInstruction, "system"),
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
		return nil, fmt.Errorf("planner: model call failed: %w", callErr)
	}

	plan, err := parsePlanJSON(responseText)
	if err != nil {
		return nil, fmt.Errorf("planner: failed to parse plan JSON: %w", err)
	}

	// Enforce MaxSteps limit.
	if len(plan.Steps) > p.cfg.MaxSteps {
		plan.Steps = plan.Steps[:p.cfg.MaxSteps]
	}

	return plan, nil
}

// buildPrompt assembles the full planning prompt from the request.
func (p *PlanReActPlanner) buildPrompt(input *PlanRequest) string {
	var sb strings.Builder

	// The plan instruction is now sent via SystemInstruction on the LLM
	// request Config, so it is not included in the user prompt.

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

// planJSON is the JSON shape expected from the LLM.
type planJSON struct {
	Steps     []planStepJSON `json:"steps"`
	Reasoning string         `json:"reasoning"`
}

type planStepJSON struct {
	Description string         `json:"description"`
	ToolName    string         `json:"toolName"`
	Args        map[string]any `json:"args"`
	DependsOn   []int          `json:"dependsOn"`
}

// parsePlanJSON decodes the raw LLM text into a [Plan].
func parsePlanJSON(raw string) (*Plan, error) {
	raw = strings.TrimSpace(raw)

	// Strip common markdown code fences that some models add.
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var pj planJSON
	if err := json.Unmarshal([]byte(raw), &pj); err != nil {
		return nil, err
	}

	plan := &Plan{
		Reasoning: pj.Reasoning,
		Steps:     make([]PlanStep, 0, len(pj.Steps)),
	}
	for _, sj := range pj.Steps {
		step := PlanStep{
			Description: sj.Description,
			ToolName:    sj.ToolName,
			Args:        sj.Args,
			DependsOn:   sj.DependsOn,
		}
		if step.Args == nil {
			step.Args = make(map[string]any)
		}
		if step.DependsOn == nil {
			step.DependsOn = []int{}
		}
		plan.Steps = append(plan.Steps, step)
	}

	return plan, nil
}
