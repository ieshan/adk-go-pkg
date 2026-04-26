# Config Agent Loader

Package `config` provides types and utilities for declaring agent trees in
YAML or JSON and building them into live ADK-Go agents at runtime.

## Overview

Instead of constructing agents in code, you can describe them declaratively in
a configuration file. The `config` package:

1. Parses YAML/JSON into `AgentConfig` structs.
2. Uses a `Registry` of model and tool factories to resolve references.
3. Recursively builds the full agent tree via `Build`.

## Schema Reference

### AgentConfig

```go
type AgentConfig struct {
    Name           string            // Unique agent identifier.
    Type           string            // "llm", "sequential", "parallel", or "loop".
    Model          string            // Model ref: "prefix/model-id" (e.g. "openai/gpt-4o").
    Instruction    string            // System prompt.
    Description    string            // Human-readable description.
    Tools          []ToolRef         // Tool references.
    SubAgents      []AgentConfig     // Nested child agents.
    GenerateConfig map[string]any    // Generation parameters (temperature, topP, etc.).
    MaxIterations  int               // For loop-type agents.
}
```

### ToolRef

```go
type ToolRef struct {
    Name   string         // Registered tool name.
    Config map[string]any // Optional per-instance tool configuration.
}
```

### Supported Agent Types

| Type | ADK Agent | Description |
|------|-----------|-------------|
| `llm` | `llmagent.New` | LLM-backed agent with model, tools, and instruction. |
| `sequential` | `sequentialagent.New` | Runs sub-agents one after another. |
| `parallel` | `parallelagent.New` | Runs sub-agents concurrently. |
| `loop` | `loopagent.New` | Runs sub-agents in a loop up to `MaxIterations`. |

### Generation Config Keys

See the full key table under [TranslateGenerateConfig](#translategenerateconfig).
Unknown keys are silently ignored.

## Registry Setup

The `Registry` maps model prefixes and tool names to factory functions.

```go
reg := config.NewRegistry()

// Register a model factory for the "openai" prefix.
// When the config says model: "openai/gpt-4o", this factory is called
// with cfg["model"] = "gpt-4o" plus any generateConfig keys.
reg.RegisterModel("openai", func(cfg map[string]any) (model.LLM, error) {
    modelName := cfg["model"].(string)
    return openai.New(openai.Config{
        Model:  modelName,
        APIKey: os.Getenv("OPENAI_API_KEY"),
    })
})

// Register a tool factory.
reg.RegisterTool("search", func(cfg map[string]any) (tool.Tool, error) {
    return mySearchTool(cfg)
})
```

### ModelFactory

```go
type ModelFactory func(cfg map[string]any) (model.LLM, error)
```

Receives a config map that always contains the key `"model"` set to the
portion of the model ref after the first `/`.

### ToolFactory

```go
type ToolFactory func(cfg map[string]any) (tool.Tool, error)
```

## Skillsets

Skillsets provide agents with access to specialized instruction sets stored in
SKILL.md files. Each skill is a directory containing:

- `SKILL.md` (required): YAML frontmatter + markdown instructions
- `references/` (optional): Additional documentation
- `assets/` (optional): Templates and resources
- `scripts/` (optional): Executable scripts (future support)

### SkillsetRef Schema

```go
type SkillsetRef struct {
    Name              string         // Factory name (e.g., "filesystem")
    Config            map[string]any // Factory-specific config
    Names             []string       // Optional: specific skills to load (default: all)
    Preload           string         // "", "complete", or "frontmatters"
    SystemInstruction string         // Optional custom instruction
}
```

### Preload Strategies

| Strategy | Description | Best For |
|----------|-------------|----------|
| `""` (default) | Load on-demand | Large skill sets, memory-constrained |
| `"complete"` | Load all data at init | Small skill sets, fast response needed |
| `"frontmatters"` | Load metadata only | Balanced, frequent skill listing |

### YAML Examples

**Basic filesystem skills:**
```yaml
name: my-agent
type: llm
skillsets:
  - name: filesystem
    config:
      path: "./skills"
```

**Preloaded skills with custom instruction:**
```yaml
skillsets:
  - name: filesystem
    config:
      path: "/app/skills"
    preload: complete
    systemInstruction: "Use these skills for domain-specific tasks."
```

**Multiple skill sources:**
```yaml
skillsets:
  - name: filesystem
    config:
      path: "./local-skills"
    preload: frontmatters
  - name: gcs
    config:
      bucket: "org-skills"
      prefix: "shared/"
```

**Specific skills only (filtering):**
```yaml
skillsets:
  - name: filesystem
    config:
      path: "./skills"  # Folder has 20+ skills
    names:              # But agent only sees these 2:
      - "weather"
      - "cooking"
    preload: frontmatters
```

### Registering Custom Skill Factories

```go
reg.RegisterSkill("s3", func(cfg map[string]any) (skill.Source, error) {
    bucket := cfg["bucket"].(string)
    // Create S3-based skill source...
    return s3Source, nil
})
```

## Loading and Building

### Load

```go
func Load(path string) (*AgentConfig, error)
```

Reads a config file. Format is inferred from extension: `.json`, `.yaml`, `.yml`.

### Parse

```go
func Parse(data []byte, format string) (*AgentConfig, error)
```

Parses raw bytes. `format` must be `"json"` or `"yaml"`.

### Build

```go
func Build(cfg *AgentConfig, reg *Registry) (agent.Agent, error)
```

Recursively builds a live agent tree from the config and registry.

### LoadAndBuild

```go
func LoadAndBuild(path string, reg *Registry) (agent.Agent, error)
```

Convenience function combining `Load` and `Build`.

## TranslateGenerateConfig

```go
func TranslateGenerateConfig(m map[string]any) (*genai.GenerateContentConfig, error)
```

Converts a generic `map[string]any` (typically the `GenerateConfig` field from
an `AgentConfig`) into a `*genai.GenerateContentConfig` suitable for passing to
an LLM.

Recognised keys and their target fields:

| Key | Target Field | Type |
|-----|-------------|------|
| `temperature` | `Temperature` | `*float32` |
| `topP` | `TopP` | `*float32` |
| `topK` | `TopK` | `*float32` |
| `maxOutputTokens` | `MaxOutputTokens` | `int32` |
| `candidateCount` | `CandidateCount` | `int32` |
| `stopSequences` | `StopSequences` | `[]string` |
| `responseMimeType` | `ResponseMIMEType` | `string` |

Unknown keys are silently ignored. A `nil` map returns an empty config without error.

### Example

```go
import (
    "log"

    "github.com/ieshan/adk-go-pkg/config"
)

gc, err := config.TranslateGenerateConfig(map[string]any{
    "temperature":     0.7,
    "maxOutputTokens": float64(1024),
})
if err != nil {
    log.Fatal(err)
}
// gc.Temperature is a *float32 pointing to 0.7
// gc.MaxOutputTokens is int32(1024)
```

`Build` calls `TranslateGenerateConfig` internally when constructing LLM agents,
but you can also call it directly if you are building agents programmatically.

## YAML Example

```yaml
name: root-agent
type: llm
model: openai/gpt-4o
instruction: "You are a helpful assistant."
tools:
  - name: search
    config:
      maxResults: 5
generateConfig:
  temperature: 0.7
  maxOutputTokens: 1024
```

## JSON Example

```json
{
  "name": "root-agent",
  "type": "llm",
  "model": "openai/gpt-4o",
  "instruction": "You are a helpful assistant.",
  "tools": [
    {"name": "search", "config": {"maxResults": 5}}
  ],
  "generateConfig": {
    "temperature": 0.7,
    "maxOutputTokens": 1024
  }
}
```

## Full Agent Tree Example

A multi-agent system with a sequential orchestrator:

```yaml
name: orchestrator
type: sequential
subAgents:
  - name: researcher
    type: llm
    model: openai/gpt-4o
    instruction: "Find information about the user's topic."
    tools:
      - name: search
      - name: scrape
    generateConfig:
      temperature: 0.3

  - name: writer
    type: llm
    model: openai/gpt-4o
    instruction: "Write a report based on the research."
    generateConfig:
      temperature: 0.7
      maxOutputTokens: 2048

  - name: reviewer
    type: loop
    maxIterations: 3
    subAgents:
      - name: critic
        type: llm
        model: openai/gpt-4o
        instruction: "Review the report. Output APPROVED if it meets quality standards."
```

Loading and running:

```go
reg := config.NewRegistry()
reg.RegisterModel("openai", openaiFactory)
reg.RegisterTool("search", searchFactory)
reg.RegisterTool("scrape", scrapeFactory)

agent, err := config.LoadAndBuild("agents/orchestrator.yaml", reg)
if err != nil {
    log.Fatal(err)
}
// Use agent with runner.New(...)
```
