# Config Agent Loader

Package `config` provides types and utilities for declaring agent trees in
YAML or JSON and building them into live ADK-Go agents at runtime.

## Overview

Instead of constructing agents in code, you can describe them declaratively in
a configuration file. The `config` package:

1. Parses YAML/JSON into the sealed `AgentConfig` interface backed by type-specific structs (`LLMAgentConfig`, `SequentialAgentConfig`, `ParallelAgentConfig`, `LoopAgentConfig`).
2. Uses a `Registry` of model and tool factories to resolve references.
3. Recursively builds the full agent tree via `BuildWithPath` or `LoadAndBuild`.

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
    Model                    string              // Model ref: "prefix/model-id".
    ModelCode                *CodeConfig         // Registered model factory reference.
    Instruction              string              // Static system prompt.
    InstructionTemplate      *prompt.TemplateRef // Dynamic instruction via template (takes precedence over Instruction).
    StaticInstruction        string              // Global instruction for all agents in tree.
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
    ConfigPath string // Root-relative path to agent config file.
    Code       string // Registered agent name in Registry.
}
```

Exactly one of `config_path` or `code` must be set. `config_path` is resolved
relative to the parent config's directory inside the `*os.Root` passed to
`BuildWithPath`/`LoadAndBuild`; absolute paths and paths that escape the root
are rejected by the `os.Root` boundary.

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
| `LlmAgent` | `llm` | `llmagent.New` | LLM-backed agent with model, tools, and instruction. | All BaseAgentConfig fields, Model, ModelCode, Instruction, InstructionTemplate, StaticInstruction, InputSchema, OutputSchema, OutputKey, IncludeContents, Tools, Skillsets, GenerateConfig, DisallowTransferToParent, DisallowTransferToPeers, BeforeModelCallbacks, AfterModelCallbacks, OnModelErrorCallbacks, BeforeToolCallbacks, AfterToolCallbacks, OnToolErrorCallbacks |
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

// Register a prompt template registry for named instruction templates.
engine := prompt.New()
tmplReg := prompt.NewRegistry(engine)
tmplReg.Register("greeting", "Hello from {{.Agent.Name}}.")
reg.RegisterTemplateRegistry(tmplReg)

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

### StaticSchema

```go
func StaticSchema(s *genai.Schema) SchemaFactory
```

A convenience helper that wraps a pre-built `*genai.Schema` as a `SchemaFactory` for registration via `Registry.RegisterSchema`. Useful when you have a schema constructed in code rather than from config arguments.

### NewFilteredSource

```go
func NewFilteredSource(base skill.Source, names []string) skill.Source
```

Wraps an existing `skill.Source` and restricts the visible skills to those listed in `names`. Used internally by the builder when a `SkillsetRef` specifies `Names`; you can also use it directly when composing skill sources programmatically.

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
| `RegisterTemplateRegistry` | `TemplateRegistry` | `*prompt.TemplateRegistry` |

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
    Preload           string         // "", "complete", or "frontmatters"
    Names             []string       // Optional: specific skills to load (default: all)
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

## Instruction Templates

The `InstructionTemplate` field on `LLMAgentConfig` enables dynamic, template-based
instructions using the `prompt` package. It accepts a `prompt.TemplateRef` which
can reference a template by name, inline text, or file path.

When `InstructionTemplate` is set, it takes precedence over the static `Instruction`
field. The builder resolves the template using the registry's `TemplateRegistry`
(if registered) or a default `prompt.New()` engine, then wraps it as an
`llmagent.InstructionProvider` via `prompt.NewInstructionProviderFromTemplate`.

See [Prompt Templating](prompt.md) for template syntax, available data fields,
and built-in functions.

### Registering a Template Registry

```go
reg := config.NewRegistry()

engine := prompt.New()
tmplReg := prompt.NewRegistry(engine)
tmplReg.Register("greeting", "Hello from {{.Agent.Name}}.")
reg.RegisterTemplateRegistry(tmplReg)
```

### YAML Examples

**Inline template:**
```yaml
name: my-agent
agent_class: LlmAgent
model: openai/gpt-4o
instruction_template:
  inline: "You are {{.Agent.Name}}. User: {{.User.Text}}"
```

**Named template (requires registered TemplateRegistry):**
```yaml
name: my-agent
agent_class: LlmAgent
model: openai/gpt-4o
instruction_template:
  name: "greeting"
```

**File-based template (path is resolved relative to the `*os.Root` passed to `BuildWithPath`/`LoadAndBuild`):**
```yaml
name: my-agent
agent_class: LlmAgent
model: openai/gpt-4o
instruction_template:
  path: "prompts/system.tmpl"
```

## Loading and Building

### Load

```go
func Load(root *os.Root, path string) (*AppConfig, error)
```

Reads a config file beneath `root`. Format is inferred from extension: `.json`, `.yaml`, `.yml`.
`path` is interpreted relative to `root`; the `*os.Root` enforces kernel-level path traversal protection so callers cannot escape the configured boundary. Returns an `*AppConfig` whose `.AgentConfig` field is the sealed `AgentConfig` interface — type-assert to `*LLMAgentConfig`, `*SequentialAgentConfig`, etc. to access type-specific fields.

### Parse

```go
func Parse(data []byte, format string) (*AppConfig, error)
```

Parses raw bytes. `format` must be `"json"` or `"yaml"`.
Both formats validate type-specific field restrictions — setting an LLM-only field (`before_model_callbacks`, `after_model_callbacks`, `before_tool_callbacks`, `after_tool_callbacks`, `on_model_error_callbacks`, `on_tool_error_callbacks`) on a non-LLM agent type returns an error. Agent-level callbacks (`before_agent_callbacks`, `after_agent_callbacks`) are accepted for all agent types.

### BuildWithPath

```go
func BuildWithPath(ctx context.Context, cfg AgentConfig, reg *Registry, root *os.Root, configPath string) (agent.Agent, error)
```

Recursively builds a live agent tree from the config and registry. Uses a type switch internally to delegate to the correct agent constructor. `root` scopes all filesystem access for sub-agent `config_path` references and file-based instruction templates; pass `nil` when the config tree contains no file references. The `configPath` parameter is the root-relative path of the parent config, used to resolve relative `config_path` references in `AgentRefConfig`; pass an empty string when not loading from a file.

### BuildAppWithPath

```go
func BuildAppWithPath(ctx context.Context, appCfg *AppConfig, reg *Registry, root *os.Root, configPath string) (agent.Agent, *agent.RunConfig, *agent.LiveRunConfig, *ContextCacheConfig, error)
```

Like `BuildWithPath`, but accepts a full `*AppConfig` (which wraps an `AgentConfig` alongside optional `RunConfig`, `LiveRunConfig`, and `ContextCacheConfig`) and returns the resolved runtime configs alongside the built agent. The returned `RunConfig`, `LiveRunConfig`, and `ContextCacheConfig` may be `nil` if absent in the config file.

### LoadAndBuild

```go
func LoadAndBuild(ctx context.Context, root *os.Root, path string, reg *Registry) (agent.Agent, *agent.RunConfig, *agent.LiveRunConfig, *ContextCacheConfig, error)
```

Convenience function combining `Load` and `BuildAppWithPath`. Reads the config file at `path` beneath `root`, builds the agent tree, and returns the runtime configs. The returned `RunConfig`, `LiveRunConfig`, and `ContextCacheConfig` may be `nil` if absent in the config file.

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
| `responseLogprobs` | `ResponseLogprobs` | `bool` |
| `logprobs` | `Logprobs` | `*int32` |
| `presencePenalty` | `PresencePenalty` | `*float32` |
| `frequencyPenalty` | `FrequencyPenalty` | `*float32` |
| `seed` | `Seed` | `*int32` |
| `audioTimestamp` | `AudioTimestamp` | `bool` |
| `cachedContent` | `CachedContent` | `string` |
| `enableEnhancedCivicAnswers` | `EnableEnhancedCivicAnswers` | `*bool` |
| `serviceTier` | `ServiceTier` | `genai.ServiceTier` |
| `mediaResolution` | `MediaResolution` | `genai.MediaResolution` |
| `responseModalities` | `ResponseModalities` | `[]string` |
| `labels` | `Labels` | `map[string]string` |
| `responseSchema` | `ResponseSchema` | `*genai.Schema` |
| `responseJsonSchema` | `ResponseJsonSchema` | `any` |
| `safetySettings` | `SafetySettings` | `[]genai.SafetySetting` |
| `tools` | `Tools` | `[]genai.Tool` |
| `toolConfig` | `ToolConfig` | `*genai.ToolConfig` |
| `thinkingConfig` | `ThinkingConfig` | `*genai.ThinkingConfig` |
| `speechConfig` | `SpeechConfig` | `*genai.SpeechConfig` |
| `imageConfig` | `ImageConfig` | `*genai.ImageConfig` |
| `routingConfig` | `RoutingConfig` | `*genai.RoutingConfig` |
| `modelSelectionConfig` | `ModelSelectionConfig` | `*genai.ModelSelectionConfig` |
| `modelArmorConfig` | `ModelArmorConfig` | `*genai.ModelArmorConfig` |
| `httpOptions` | `HTTPOptions` | `*genai.HTTPOptions` |
| `systemInstruction` | `SystemInstruction` | `*genai.Content` |

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
# Optional: use a template instead of a static instruction
# instruction_template:
#   inline: "You are {{.Agent.Name}}. User: {{.User.Text}}"
#   # or by name: name: "greeting"
#   # or from file: path: "prompts/system.tmpl"
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
  "instruction_template": {"inline": "You are {{.Agent.Name}}."},
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

root, err := os.OpenRoot(".")
if err != nil {
    log.Fatal(err)
}
defer root.Close()

agent, runCfg, liveRunCfg, ctxCacheCfg, err := config.LoadAndBuild(ctx, root, "agents/orchestrator.yaml", reg)
if err != nil {
    log.Fatal(err)
}
// Use agent with runner.New(...).
// runCfg, liveRunCfg, and ctxCacheCfg may be nil if not specified in the YAML.
_ = runCfg
_ = liveRunCfg
_ = ctxCacheCfg
```

## Parse-Only Types

The following types are parsed from config and returned by `BuildAppWithPath`/`LoadAndBuild` but are not wired into agent execution in the current release:

- `RunConfig` — runtime behavior configuration (streaming mode, save live blob, custom metadata)
- `LiveRunConfig` — live run configuration (max LLM calls, etc.)
- `ContextCacheConfig` — context caching intervals, TTL, and minimum token threshold
- `CustomMetadata` — parsed from config but not propagated to `agent.RunConfig`

### StreamingMode

The `streaming_mode` field controls how the ADK runner streams responses:

| Value | Description |
|-------|-------------|
| `none` | No streaming (default) |
| `sse` | Server-sent events streaming |
| `bidi` | Accepted by Parse but rejected by Build — ADK-Go does not support bidirectional streaming |

`bidi` is kept as a parseable value for forward compatibility. `BuildAppWithPath` returns an error if `streaming_mode: bidi` is used.

Programmatic construction with typed configs:

```go
orchestrator := &config.SequentialAgentConfig{
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

orchestrator.SubAgentEntries = []config.SubAgentEntry{
    {Inline: researcher},
    {Inline: writer},
}
agent, err := config.BuildWithPath(ctx, orchestrator, reg, nil, "")
```
