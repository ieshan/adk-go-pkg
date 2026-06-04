# Config Agent Loader

Package `config` provides types and utilities for declaring agent trees in
YAML or JSON and building them into live ADK-Go agents at runtime.

## Overview

Instead of constructing agents in code, you can describe them declaratively in
a configuration file. The `config` package:

1. Parses YAML/JSON into the sealed `AgentConfig` interface backed by type-specific structs (`LLMAgentConfig`, `SequentialAgentConfig`, `ParallelAgentConfig`, `LoopAgentConfig`).
2. Uses a `Registry` of model and tool factories to resolve references.
3. Recursively builds the full agent tree via `Build`.

## Schema Reference

### AgentConfig (Sealed Interface)

```go
// AgentConfig is the sealed interface for all declarative agent configurations.
type AgentConfig interface {
    Type() string
    Name() string
    Description() string
    SubAgents() []AgentConfig
    SubAgentEntries() []SubAgentEntry
    isAgentConfig() // sealed — only package-defined types can implement it
}
```

### BaseAgentConfig

```go
type BaseAgentConfig struct {
    Name                 string          // Unique agent identifier.
    Description          string          // Human-readable description.
    SubAgentEntries      []SubAgentEntry // Nested child agents (inline or referenced).
    BeforeAgentCallbacks []CodeConfig    // Registered before-agent callbacks.
    AfterAgentCallbacks  []CodeConfig    // Registered after-agent callbacks.
}
```

### LLMAgentConfig

```go
type LLMAgentConfig struct {
    BaseAgentConfig
    Model                    string         // Model ref: "prefix/model-id".
    ModelCode                *CodeConfig    // Registered model factory reference.
    Instruction              string         // System prompt.
    StaticInstruction        string         // Global instruction for all agents in tree.
    InputSchema              *SchemaRef     // Registered input schema reference.
    OutputSchema             *SchemaRef     // Registered output schema reference.
    OutputKey                string         // Session state key for agent output.
    IncludeContents          string         // "none", "default".
    Tools                    []ToolRef      // Tool references.
    Skillsets                []SkillsetRef  // Skill source references.
    GenerateConfig           map[string]any // Generation parameters.
    DisallowTransferToParent bool           // Prevent transfer to parent.
    DisallowTransferToPeers  bool           // Prevent transfer to siblings.
    BeforeModelCallbacks     []CodeConfig   // Registered before-model callbacks.
    AfterModelCallbacks      []CodeConfig   // Registered after-model callbacks.
    OnModelErrorCallbacks    []CodeConfig   // Registered model-error callbacks.
    BeforeToolCallbacks      []CodeConfig   // Registered before-tool callbacks.
    AfterToolCallbacks       []CodeConfig   // Registered after-tool callbacks.
    OnToolErrorCallbacks     []CodeConfig   // Registered tool-error callbacks.
}
```

### SequentialAgentConfig

```go
type SequentialAgentConfig struct {
    BaseAgentConfig
}
```

### ParallelAgentConfig

```go
type ParallelAgentConfig struct {
    BaseAgentConfig
}
```

### LoopAgentConfig

```go
type LoopAgentConfig struct {
    BaseAgentConfig
    MaxIterations int // Maximum iterations (0 = unlimited).
}
```

### AgentRefConfig

References another agent by file path or registered code name.

```go
type AgentRefConfig struct {
    ConfigPath string // Path to agent config file (relative or absolute).
    Code       string // Registered agent name in Registry.
}
```

Exactly one of `config_path` or `code` must be set.

**YAML Example:**

```yaml
sub_agents:
  - config_path: "./researcher.yaml"
  - code: "myapp.agents.writer"
```

### CodeConfig

References a Go value (callback, model factory, schema, agent) by a registered name.

```go
type CodeConfig struct {
    Name string         // Registered identifier.
    Args map[string]any // Optional arguments passed to factory.
}
```

### SubAgentEntry

A tagged union representing either an inline `AgentConfig` or a reference to an external agent.

```go
type SubAgentEntry struct {
    Inline AgentConfig
    Ref    *AgentRefConfig
}
```

### ToolRef

```go
type ToolRef struct {
    Name string         // Registered tool name.
    Args map[string]any // Optional per-instance tool configuration.
}
```

### Supported Agent Types

| Config `agent_class` | Go `Type()` | ADK Agent | Description | Valid Fields |
|----------------------|-------------|-----------|-------------|--------------|
| `LlmAgent` | `llm` | `llmagent.New` | LLM-backed agent with model, tools, and instruction. | All BaseAgentConfig fields, Model, ModelCode, Instruction, StaticInstruction, InputSchema, OutputSchema, OutputKey, IncludeContents, Tools, Skillsets, GenerateConfig, DisallowTransferToParent, DisallowTransferToPeers, BeforeModelCallbacks, AfterModelCallbacks, OnModelErrorCallbacks, BeforeToolCallbacks, AfterToolCallbacks, OnToolErrorCallbacks |
| `SequentialAgent` | `sequential` | `sequentialagent.New` | Runs sub-agents one after another. | BaseAgentConfig fields |
| `ParallelAgent` | `parallel` | `parallelagent.New` | Runs sub-agents concurrently. | BaseAgentConfig fields |
| `LoopAgent` | `loop` | `loopagent.New` | Runs sub-agents in a loop up to `MaxIterations`. | BaseAgentConfig fields, MaxIterations |

Note: `agent_class` is the config-file discriminator. `Type()` returns Go-internal short names consumed by builder logic. |

### Generation Config Keys

See the full key table under [TranslateGenerateConfig](#translategenerateconfig).
Unknown keys are silently ignored.

## Registry Setup

The `Registry` maps model prefixes, tool names, skill sources, callbacks, model codes, and agents to their respective factory functions or values.

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

// Register a modelCode factory.
reg.RegisterModelCode("myapp.models.custom", func(args map[string]any) (model.LLM, error) {
    return customModel(args)
})

// Register callbacks.
reg.RegisterBeforeModelCallback("myapp.cb.cache", beforeModelCache)
reg.RegisterAfterModelCallback("myapp.cb.log", afterModelLog)
reg.RegisterBeforeAgentCallback("myapp.cb.auth", beforeAgentAuth)

// Register a pre-built agent for code references.
reg.RegisterAgent("myapp.agents.sub", subAgent)
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

### ModelCodeFactory

```go
type ModelCodeFactory func(args map[string]any) (model.LLM, error)
```

Creates a model from configuration arguments. Used when `model_code` is specified instead of `model`.

### Callback Registration

The Registry provides typed registration and resolution for all callback types:

| Register | Resolve | Type |
|------------|---------|------|
| `RegisterBeforeModelCallback` | `ResolveBeforeModelCallback` | `llmagent.BeforeModelCallback` |
| `RegisterAfterModelCallback` | `ResolveAfterModelCallback` | `llmagent.AfterModelCallback` |
| `RegisterOnModelErrorCallback` | `ResolveOnModelErrorCallback` | `llmagent.OnModelErrorCallback` |
| `RegisterBeforeToolCallback` | `ResolveBeforeToolCallback` | `llmagent.BeforeToolCallback` |
| `RegisterAfterToolCallback` | `ResolveAfterToolCallback` | `llmagent.AfterToolCallback` |
| `RegisterOnToolErrorCallback` | `ResolveOnToolErrorCallback` | `llmagent.OnToolErrorCallback` |
| `RegisterBeforeAgentCallback` | `ResolveBeforeAgentCallback` | `agent.BeforeAgentCallback` |
| `RegisterAfterAgentCallback` | `ResolveAfterAgentCallback` | `agent.AfterAgentCallback` |
| `RegisterModelCode` | `ResolveModelCode` | `ModelCodeFactory` |
| `RegisterAgent` | `ResolveAgent` | `agent.Agent` |

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
agent_class: LlmAgent
skill_sets:
  - name: filesystem
    config:
      path: "./skills"
```

**Preloaded skills with custom instruction:**
```yaml
skill_sets:
  - name: filesystem
    config:
      path: "/app/skills"
    preload: complete
    system_instruction: "Use these skills for domain-specific tasks."
```

**Multiple skill sources:**
```yaml
skill_sets:
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
skill_sets:
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
func Load(path string) (*AppConfig, error)
```

Reads a config file. Format is inferred from extension: `.json`, `.yaml`, `.yml`.
Returns the sealed `AgentConfig` interface — type-assert to `*LLMAgentConfig`, `*SequentialAgentConfig`, etc. to access type-specific fields.

### Parse

```go
func Parse(data []byte, format string) (*AppConfig, error)
```

Parses raw bytes. `format` must be `"json"` or `"yaml"`.
YAML parsing validates type-specific field restrictions — setting an LLM-only field on a non-LLM agent type returns an error.

### Build

```go
func Build(ctx context.Context, cfg AgentConfig, reg *Registry) (agent.Agent, error)
```

Recursively builds a live agent tree from the config and registry. Uses a type switch internally to delegate to the correct agent constructor.

### BuildWithPath

```go
func BuildWithPath(ctx context.Context, cfg AgentConfig, reg *Registry, configPath string) (agent.Agent, error)
```

Like `Build`, but accepts the config file path so that relative `config_path` references in `AgentRefConfig` can be resolved correctly.

### LoadAndBuild

```go
func LoadAndBuild(ctx context.Context, path string, reg *Registry) (agent.Agent, *agent.RunConfig, *agent.LiveRunConfig, *ContextCacheConfig, error)
```

Convenience function combining `Load` and `BuildWithPath`.

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
agent_class: LlmAgent
model: openai/gpt-4o
instruction: "You are a helpful assistant."
static_instruction: "All agents in this tree are professional and concise."
output_key: result
include_contents: default
disallow_transfer_to_parent: true
disallow_transfer_to_peers: true
tools:
  - name: search
    args:
      maxResults: 5
before_model_callbacks:
  - name: myapp.cb.cache
after_model_callbacks:
  - name: myapp.cb.log
generate_content_config:
  temperature: 0.7
  maxOutputTokens: 1024
```

## JSON Example

```json
{
  "name": "root-agent",
  "agent_class": "LlmAgent",
  "model": "openai/gpt-4o",
  "instruction": "You are a helpful assistant.",
  "disallow_transfer_to_parent": true,
  "disallow_transfer_to_peers": true,
  "tools": [
    {"name": "search", "args": {"maxResults": 5}}
  ],
  "generate_content_config": {
    "temperature": 0.7,
    "maxOutputTokens": 1024
  }
}
```

## Full Agent Tree Example

A multi-agent system with a sequential orchestrator:

```yaml
name: orchestrator
agent_class: SequentialAgent
sub_agents:
  - name: researcher
    agent_class: LlmAgent
    model: openai/gpt-4o
    instruction: "Find information about the user's topic."
    tools:
      - name: search
      - name: scrape
    generate_content_config:
      temperature: 0.3

  - name: writer
    agent_class: LlmAgent
    model: openai/gpt-4o
    instruction: "Write a report based on the research."
    disallow_transfer_to_parent: true
    generate_content_config:
      temperature: 0.7
      maxOutputTokens: 2048

  - name: reviewer
    agent_class: LoopAgent
    max_iterations: 3
    sub_agents:
      - name: critic
        agent_class: LlmAgent
        model: openai/gpt-4o
        instruction: "Review the report. Output APPROVED if it meets quality standards."
```

Loading and running:

```go
reg := config.NewRegistry()
reg.RegisterModel("openai", openaiFactory)
reg.RegisterTool("search", searchFactory)
reg.RegisterTool("scrape", scrapeFactory)

agent, err := config.LoadAndBuild(ctx, "agents/orchestrator.yaml", reg)
if err != nil {
    log.Fatal(err)
}
// Use agent with runner.New(...)
```

## Parse-Only Types

The following types are parsed for schema parity but are not wired into agent execution in the current release:

- `RunConfig` — runtime behavior configuration (streaming mode, max LLM calls, etc.)
- `ContextCacheConfig` — context caching intervals and TTL

Programmatic construction with typed configs:

```go
root := &config.SequentialAgentConfig{
    BaseAgentConfig: config.BaseAgentConfig{Name: "orchestrator"},
}

researcher := &config.LLMAgentConfig{
    BaseAgentConfig: config.BaseAgentConfig{Name: "researcher"},
    Model:           "openai/gpt-4o",
    Instruction:     "Find information.",
    Tools:           []config.ToolRef{{Name: "search"}},
}

writer := &config.LLMAgentConfig{
    BaseAgentConfig:          config.BaseAgentConfig{Name: "writer"},
    Model:                    "openai/gpt-4o",
    Instruction:              "Write a report.",
    DisallowTransferToParent: true,
}

root.SubAgentEntries = []config.SubAgentEntry{
    {Inline: researcher},
    {Inline: writer},
}
agent, err := config.Build(ctx, root, reg)
```
