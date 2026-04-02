// Package planner provides interfaces and types for generating structured
// execution plans before an agent acts.
//
// A Planner inspects the user's message, the available tools, and the
// conversation history, then produces an ordered sequence of [PlanStep] values
// that the agent should execute. This separates the "thinking" phase (planning)
// from the "doing" phase (execution), which can improve reliability and
// observability in complex agentic workflows.
//
// # Basic usage
//
//	cfg := planner.PlanReActConfig{
//	    Model:    myLLM,
//	    MaxSteps: 5,
//	}
//	p := planner.NewPlanReAct(cfg)
//
//	plan, err := p.GeneratePlan(ctx, &planner.PlanRequest{
//	    UserMessage: "Book a flight and send a confirmation email",
//	    ToolDescriptions: []planner.ToolDescription{
//	        {Name: "book_flight",  Description: "Books a flight"},
//	        {Name: "send_email",   Description: "Sends an email"},
//	    },
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for i, step := range plan.Steps {
//	    fmt.Printf("Step %d: %s\n", i+1, step.Description)
//	}
package planner

import (
	"context"

	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// Planner generates an execution plan before the agent acts.
//
// Implementations should be safe for concurrent use.
type Planner interface {
	// GeneratePlan constructs a [Plan] from the provided [PlanRequest].
	// It returns an error if the underlying model call fails or if the
	// response cannot be parsed into a valid plan.
	GeneratePlan(ctx context.Context, input *PlanRequest) (*Plan, error)
}

// PlanRequest is the input to plan generation.
//
// All fields are optional — a Planner must handle zero values gracefully
// (e.g. an empty ToolDescriptions slice means no tools are available).
//
// Example:
//
//	req := &planner.PlanRequest{
//	    UserMessage:  "Summarise today's news and send it to me",
//	    Instruction:  "Be concise and use at most 3 steps.",
//	    ToolDescriptions: []planner.ToolDescription{
//	        {Name: "fetch_news", Description: "Fetches top news headlines"},
//	        {Name: "send_email", Description: "Sends an email"},
//	    },
//	}
type PlanRequest struct {
	// UserMessage is the raw user input that the plan must address.
	UserMessage string

	// ToolDescriptions lists the tools available to the agent.
	// An empty slice means no tools are available.
	ToolDescriptions []ToolDescription

	// History is the prior conversation events, oldest first.
	// May be nil if there is no prior history.
	History []*session.Event

	// Instruction is an optional system-level directive injected into the
	// planning prompt (e.g. "prefer fewer steps").
	Instruction string
}

// ToolDescription describes a tool for the planner.
//
// The planner uses Name and Description to communicate what each tool does
// to the underlying LLM. Parameters provides optional schema detail that
// the LLM can use to reason about argument shapes.
//
// Example:
//
//	td := planner.ToolDescription{
//	    Name:        "search_web",
//	    Description: "Searches the web and returns a list of URLs.",
//	    Parameters: &genai.Schema{
//	        Type: genai.TypeObject,
//	        Properties: map[string]*genai.Schema{
//	            "query": {Type: genai.TypeString, Description: "Search query"},
//	        },
//	        Required: []string{"query"},
//	    },
//	}
type ToolDescription struct {
	// Name is the unique identifier of the tool (matches the tool's registered name).
	Name string

	// Description explains what the tool does in plain language.
	Description string

	// Parameters is the JSON Schema for the tool's arguments.
	// May be nil when the tool takes no parameters.
	Parameters *genai.Schema
}

// Plan is an ordered sequence of steps produced by a [Planner].
//
// Steps should be executed in order unless a step's DependsOn field
// indicates that it may run concurrently with (or after) another step.
//
// Example:
//
//	plan := &planner.Plan{
//	    Reasoning: "First fetch data, then summarise and email it.",
//	    Steps: []planner.PlanStep{
//	        {Description: "Fetch news",    ToolName: "fetch_news"},
//	        {Description: "Send summary",  ToolName: "send_email", DependsOn: []int{0}},
//	    },
//	}
type Plan struct {
	// Steps is the ordered list of actions to perform.
	Steps []PlanStep

	// Reasoning is the LLM's free-text explanation of why it chose these steps.
	Reasoning string
}

// PlanStep is a single action in a [Plan].
//
// Example — a step that calls the "search_web" tool:
//
//	step := planner.PlanStep{
//	    Description: "Search the web for recent Go releases",
//	    ToolName:    "search_web",
//	    Args:        map[string]any{"query": "Go language latest release"},
//	    DependsOn:   nil, // no dependencies; can run immediately
//	}
type PlanStep struct {
	// Description is a human-readable summary of what this step does.
	Description string

	// ToolName is the name of the tool to invoke (may be empty for
	// reasoning-only steps that do not call a tool).
	ToolName string

	// Args contains the arguments to pass to the tool.
	// Keys and value types must match the tool's parameter schema.
	Args map[string]any

	// DependsOn is a list of 0-based step indices that must complete
	// before this step can begin. An empty or nil slice means the step
	// has no dependencies.
	DependsOn []int
}
