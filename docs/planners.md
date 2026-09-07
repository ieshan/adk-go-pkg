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
    // NOTE: Currently ignored by both PlanReActPlanner and ThinkingPlanner.
    History []*session.Event

    // Optional directive injected into the user prompt (under
    // "## Additional Instruction"), not the system prompt.
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
the JSON response. Steps are truncated to `MaxSteps` via a simple slice
operation (`plan.Steps[:MaxSteps]`); no `DependsOn` indices are adjusted, so
truncation may leave steps referencing indices that no longer exist.

### Config

```go
type PlanReActConfig struct {
    Model                   model.LLM      // Required.
    PlanInstruction         string         // Custom system prompt (optional).
    PlanInstructionTemplate *prompt.Template // Overrides default instruction; takes precedence over PlanInstruction.
    MaxSteps                int            // Step cap. Default: 10.
}
```

#### PlanInstructionTemplate

When set, `PlanInstructionTemplate` overrides the default planning system
prompt (and takes precedence over the plain `PlanInstruction` string). The
template is rendered with the following data, accessible via `{{.Input.*}}`:

| Field          | Description                                              |
|----------------|----------------------------------------------------------|
| `tools`        | Formatted list of available tool descriptions.           |
| `userMessage`  | The raw user message from `PlanRequest.UserMessage`.     |
| `instruction`  | The caller-supplied instruction from `PlanRequest.Instruction`. |

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

Markdown code fences around the JSON are automatically stripped. This stripping
applies only to the whole response (via `TrimPrefix`/`TrimSuffix` on the entire
string); it does not remove fences embedded around individual JSON objects
within a larger response.

## Thinking Planner

The `ThinkingPlanner` prompts the LLM to reason step-by-step. It attempts to
extract structured JSON from the response, but gracefully falls back to a
single free-text step when no parseable JSON is found. The full model response
is always available in `Plan.Reasoning`.

### JSON Extraction Heuristics

The planner searches the response text for a JSON candidate in two stages,
returning the first match:

1. **```json code fence** — looks for a ```` ```json ```` opening fence and
   extracts the text up to the next ```` ``` ```` closing fence. The extracted
   content is trimmed and treated as the candidate.
2. **Raw `{"steps":` prefix** — if no fence is found, it scans for the exact
   substring `{"steps":` and returns everything from that index to the end of
   the response.

Both checks are heuristic and intentionally simple:

- The raw-marker match requires the exact prefix `{"steps":` with no
  whitespace variation (e.g. `{ "steps":` will not match) and no alternative
  field ordering (e.g. `{"reasoning":...,"steps":...}` will not match).
- The candidate is then passed to `parsePlanJSON`, which strips leading/
  trailing code fences from the whole candidate and unmarshals it. If parsing
  fails, the planner falls back to a single free-text step rather than
  returning an error.

This keeps extraction predictable; the fallback path is always safe.

### Config

```go
type ThinkingConfig struct {
    Model                      model.LLM       // Required.
    ThinkingBudget             int             // Optional token budget hint. 0 = no hint.
    ThinkingInstructionTemplate *prompt.Template // Overrides default thinking system prompt.
}
```

#### ThinkingInstructionTemplate

When set, `ThinkingInstructionTemplate` overrides the default thinking system
prompt. The template is rendered with the following data, accessible via
`{{.Input.*}}`:

| Field          | Description                                              |
|----------------|----------------------------------------------------------|
| `tools`        | Formatted list of available tool descriptions.           |
| `userMessage`  | The raw user message from `PlanRequest.UserMessage`.     |
| `instruction`  | The caller-supplied instruction from `PlanRequest.Instruction`. |
| `budget`       | The configured `ThinkingBudget` (0 when unset).          |

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
