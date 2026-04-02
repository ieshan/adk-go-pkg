# Planners

Package `planner` provides interfaces and implementations for generating
structured execution plans before an agent acts.

## Overview

A Planner separates the "thinking" phase from the "doing" phase. Given a user
message and available tools, it produces an ordered sequence of `PlanStep`
values that the agent should execute. This improves reliability and
observability in complex agentic workflows.

### Planner Types

| Type | Description |
|------|-------------|
| **PlanReAct** | Strict JSON planner -- prompts the LLM for a structured `{"steps":...}` JSON response, parses it, and enforces a step limit. |
| **Thinking** | Flexible planner -- asks the LLM to reason step-by-step, attempts JSON extraction, falls back to free-text when JSON is not found. |

## API Reference

### Planner Interface

```go
type Planner interface {
    GeneratePlan(ctx context.Context, input *PlanRequest) (*Plan, error)
}
```

### PlanRequest

```go
type PlanRequest struct {
    // Raw user input the plan must address.
    UserMessage string

    // Tools available to the agent.
    ToolDescriptions []ToolDescription

    // Prior conversation events (oldest first). May be nil.
    History []*session.Event

    // Optional system-level directive for the planning prompt.
    Instruction string
}
```

### ToolDescription

```go
type ToolDescription struct {
    Name        string        // Unique tool identifier.
    Description string        // What the tool does.
    Parameters  *genai.Schema // Optional JSON Schema for the tool's arguments.
}
```

### Plan

```go
type Plan struct {
    Steps     []PlanStep // Ordered actions to perform.
    Reasoning string     // LLM's explanation of the plan.
}
```

### PlanStep

```go
type PlanStep struct {
    Description string         // Human-readable summary.
    ToolName    string         // Tool to invoke (empty for reasoning-only steps).
    Args        map[string]any // Arguments to pass to the tool.
    DependsOn   []int          // 0-based indices of prerequisite steps.
}
```

## PlanReAct Planner

The `PlanReActPlanner` sends a structured planning prompt to an LLM and parses
the JSON response. Steps are truncated to `MaxSteps`.

### Config

```go
type PlanReActConfig struct {
    Model           model.LLM // Required.
    PlanInstruction string    // Custom system prompt (optional).
    MaxSteps        int       // Step cap. Default: 10.
}
```

### Constructor

```go
func NewPlanReAct(cfg PlanReActConfig) *PlanReActPlanner
```

### Example

```go
p := planner.NewPlanReAct(planner.PlanReActConfig{
    Model:    myLLM,
    MaxSteps: 5,
})

plan, err := p.GeneratePlan(ctx, &planner.PlanRequest{
    UserMessage: "Find the weather and email it to me.",
    ToolDescriptions: []planner.ToolDescription{
        {Name: "get_weather", Description: "Returns current weather for a city."},
        {Name: "send_email", Description: "Sends an email."},
    },
})
if err != nil {
    log.Fatal(err)
}

fmt.Println("Reasoning:", plan.Reasoning)
for i, step := range plan.Steps {
    fmt.Printf("Step %d: %s (tool: %s)\n", i+1, step.Description, step.ToolName)
}
```

### JSON Format

The LLM is expected to return:

```json
{
  "steps": [
    {
      "description": "Look up current weather",
      "toolName": "get_weather",
      "args": {"city": "London"},
      "dependsOn": []
    },
    {
      "description": "Email the weather report",
      "toolName": "send_email",
      "args": {"to": "user@example.com"},
      "dependsOn": [0]
    }
  ],
  "reasoning": "First fetch the data, then send it."
}
```

Markdown code fences around the JSON are automatically stripped.

## Thinking Planner

The `ThinkingPlanner` prompts the LLM to reason step-by-step. It attempts to
extract structured JSON from the response, but gracefully falls back to a
single free-text step when no parseable JSON is found. The full model response
is always available in `Plan.Reasoning`.

### Config

```go
type ThinkingConfig struct {
    Model          model.LLM // Required.
    ThinkingBudget int       // Optional token budget hint. 0 = no hint.
}
```

### Constructor

```go
func NewThinking(cfg ThinkingConfig) *ThinkingPlanner
```

### Example

```go
p := planner.NewThinking(planner.ThinkingConfig{
    Model:          myLLM,
    ThinkingBudget: 512,
})

plan, err := p.GeneratePlan(ctx, &planner.PlanRequest{
    UserMessage: "Summarise today's news and email it to me.",
    ToolDescriptions: []planner.ToolDescription{
        {Name: "fetch_news", Description: "Fetches top news headlines."},
        {Name: "send_email", Description: "Sends an email."},
    },
    Instruction: "Keep the plan under 3 steps.",
})
if err != nil {
    log.Fatal(err)
}

fmt.Println("Chain of thought:", plan.Reasoning)
for i, step := range plan.Steps {
    fmt.Printf("Step %d: %s\n", i+1, step.Description)
}
```

## Custom Planner

Implement the `Planner` interface to create domain-specific planning logic:

```go
type myPlanner struct{}

func (myPlanner) GeneratePlan(ctx context.Context, input *planner.PlanRequest) (*planner.Plan, error) {
    // Custom logic: always call all tools in sequence.
    steps := make([]planner.PlanStep, len(input.ToolDescriptions))
    for i, td := range input.ToolDescriptions {
        steps[i] = planner.PlanStep{
            Description: "Call " + td.Name,
            ToolName:    td.Name,
            Args:        map[string]any{},
            DependsOn:   []int{},
        }
        if i > 0 {
            steps[i].DependsOn = []int{i - 1}
        }
    }
    return &planner.Plan{
        Steps:     steps,
        Reasoning: "Sequentially invoke all available tools.",
    }, nil
}
```
