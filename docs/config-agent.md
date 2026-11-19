# Config Agent Loader

Package `config` provides types and utilities for declaring agent trees in
YAML or JSON and building them into live ADK-Go agents at runtime.

## Overview

Instead of constructing agents in code, you can describe them declaratively in
a configuration file. The `config` package:

1. Parses YAML/JSON into the sealed `AgentConfig` interface backed by
   type-specific structs (`LLMAgentConfig`, `SequentialAgentConfig`,
   `ParallelAgentConfig`, `LoopAgentConfig`, `WorkflowAgentConfig`).
2. Validates the document strictly: unknown keys fail with `ErrUnknownField`,
   and adk-python fields with no adk-go equivalent fail with
   `ErrUnsupportedField` instead of being silently dropped.
3. Uses a `Registry` of model, tool, skill, and callback factories to resolve
   references.
4. Recursively builds the full agent tree via `BuildWithPath` or
   `LoadAndBuild`.

`agent_class` is optional and defaults to `LlmAgent`.

## Schema Reference

### AgentConfig (Sealed Interface)

```go
type AgentConfig interface {
    Name() string
    Description() string
    SubAgentEntries() []SubAgentEntry
    isAgentConfig() // sealed — only package-defined types can implement it
}
```

### BaseAgentConfig

```go
type BaseAgentConfig struct {
    Name                 string          // Required unique agent identifier.
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
    Model                    string                       // Model ref: "prefix/model-id" or a bare name.
    ModelCode                *CodeConfig                  // Registered model factory reference.
    ModelArgs                map[string]any               // Arguments passed to the model factory.
    Instruction              string                       // Static system prompt.
    InstructionTemplate      *prompt.TemplateRef          // Dynamic instruction (takes precedence over Instruction).
    GlobalInstruction        string                       // Instruction prepended for the whole subtree.
    InputSchema              *SchemaRef                   // Inline or registered input schema.
    OutputSchema             *SchemaRef                   // Inline or registered output schema.
    OutputKey                string                       // Session state key for agent output.
    IncludeContents          string                       // "default" or "none".
    Tools                    []ToolRef                    // Tool references.
    Skillsets                []SkillsetRef                // Skill source references.
    GenerateContentConfig    *genai.GenerateContentConfig // Decoded generation parameters.
    DisallowTransferToParent bool                         // Prevent transfer to parent.
    DisallowTransferToPeers  bool                         // Prevent transfer to siblings.
    BeforeModelCallbacks     []CodeConfig
    AfterModelCallbacks      []CodeConfig
    OnModelErrorCallbacks    []CodeConfig
    BeforeToolCallbacks      []CodeConfig
    AfterToolCallbacks       []CodeConfig
    OnToolErrorCallbacks     []CodeConfig
}
```

### SequentialAgentConfig / ParallelAgentConfig

```go
type SequentialAgentConfig struct{ BaseAgentConfig }
type ParallelAgentConfig struct{ BaseAgentConfig }
```

### LoopAgentConfig

```go
type LoopAgentConfig struct {
    BaseAgentConfig
    MaxIterations int // Maximum iterations (0 = unlimited). Must be >= 0.
}
```

### WorkflowAgentConfig

```go
type WorkflowAgentConfig struct {
    BaseAgentConfig
    Edges          []any // Graph chains; see "Workflows".
    MaxConcurrency int   // Cap on concurrent nodes (<= 0 = unlimited).
}
```

Workflows use `edges` rather than `sub_agents`; `sub_agents` is rejected on a
Workflow config.

### AgentRefConfig

References another agent by file path or registered code name.

```go
type AgentRefConfig struct {
    ConfigPath string // Path to an agent config file, relative to the referencing file's dir.
    Code       string // Registered agent name in Registry.
}
```

Exactly one of `config_path` or `code` must be set. A `sub_agents` entry may be
a bare string (shorthand for `config_path`) or a map with only one of these
keys; any other key is an unknown-field error.

`config_path` is resolved relative to the **referencing file's own directory**
inside the `*os.Root` passed to `BuildWithPath`/`LoadAndBuild`; absolute paths
and paths that escape the root are rejected. A referenced file's own
`run_config`/`context_cache_config` sections are ignored.

**YAML Example:**

```yaml
sub_agents:
  - ./researcher.yaml
  - config_path: "./writer.yaml"
  - code: "myapp.agents.reviewer"
```

### CodeConfig

References a Go value (callback, model factory, schema, agent) by a registered name.

```go
type CodeConfig struct {
    Name string         // Registered identifier.
    Args map[string]any // Optional arguments passed to factory.
}
```

Callback references are **name-only**: `args` on a callback code config is
rejected with `ErrCallbackArgs`. A tools/callback entry may also be a bare
string (shorthand for `{name: s}`).

### SubAgentEntry

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

### Supported Agent Classes

| Config `agent_class`   | ADK Agent           | Description |
|------------------------|---------------------|-------------|
| `LlmAgent` (default)   | `llmagent.New`      | LLM-backed agent with model, tools, and instruction. |
| `SequentialAgent`      | `sequentialagent.New` | Runs sub-agents one after another. |
| `ParallelAgent`        | `parallelagent.New` | Runs sub-agents concurrently. |
| `LoopAgent`            | `loopagent.New`     | Runs sub-agents in a loop up to `MaxIterations`. |
| `Workflow`             | `workflow.Workflow` | Runs an explicit graph described by `edges`. |

## Strict Field Validation

Parsing rejects:

- a key that is not part of the agent class's schema → `ErrUnknownField`;
- a real adk-python field with no adk-go target → `ErrUnsupportedField`
  (for example `static_instruction`, `custom_metadata`, `save_live_audio`);
- a missing required `name` (at any level);
- `model` together with `model_code`, or `model_args` together with
  `model_code`;
- `include_contents` other than `"default"`/`"none"` (and the key itself on
  non-LLM agents);
- `max_iterations < 0`;
- callback `args` → `ErrCallbackArgs`.

Duplicate object/mapping keys are rejected by the decoders
(`encoding/json/v2` for JSON, `yaml.v4` for YAML).

Parsing also rejects any real adk-python `run_config` field with no adk-go
equivalent (`speech_config`, `http_options`, `labels`, `service_tier`, and
others) with `ErrUnsupportedField`.

## Generation Config

`generate_content_config` is decoded directly into a
`*genai.GenerateContentConfig`. Keys are matched to fields by a normalized
comparison (strip `_`/`-`, case-fold), so `safety_settings`, `safetySettings`,
and `safetysettings` are equivalent. snake_case is recommended because it
matches adk-python. Unknown keys → `ErrUnknownField`; two keys that normalize
to the same field (e.g. `top_p` and `topP`) → `ErrAmbiguousField`.

Map keys such as `labels` names and `response_schema.properties` names are
preserved verbatim while nested struct fields are normalized.

`system_instruction` accepts the same tri-shape as adk-python's
`ContentUnion`: a string, a `{role, parts}` map, or a list of parts.

Inline `input_schema`/`output_schema` values use the same normalizing decoder,
so snake_case fields like `property_ordering` and `min_items` work inside them.

## Runtime Config

`run_config` is a spec-aligned extension mirroring adk-python's `RunConfig`
field names. Only fields with adk-go targets are supported:

```yaml
run_config:
  streaming_mode: sse                 # "", "none", "sse", or "bidi"
  save_input_blobs_as_artifacts: true # agent.RunConfig.SaveInputBlobsAsArtifacts
  save_live_blob: true                # agent.LiveRunConfig.SaveLiveBlob
  max_llm_calls: 100                  # see mapping below
```

`max_llm_calls` maps as: unset → adk-go `0` (default, 500/`ADK_MAX_LLM_CALLS`);
`<= 0` → `-1` (unbounded, matching adk-python); `> 0` → the value.

`BuildAppWithPath` returns both `*agent.RunConfig` and `*agent.LiveRunConfig`
when a `run_config` section is present. `LiveRunConfig` is non-nil even when
only `streaming_mode` is set; its zero values are harmless.

`streaming_mode: bidi` is accepted by `Parse` but rejected by
`BuildAppWithPath` because adk-go has no bidi streaming mode.

`context_cache_config` supports `cache_intervals`, `ttl_seconds`, and
`min_tokens`; `create_http_options` is rejected as unsupported.

## Registry Setup

```go
reg := config.NewRegistry()

// Register a model factory for the "openai" prefix.
reg.RegisterModel("openai", func(cfg map[string]any) (model.LLM, error) {
    modelName := cfg["model"].(string)
    return openai.New(openai.Config{Model: modelName, APIKey: os.Getenv("OPENAI_API_KEY")})
})

// Register a default model used for bare refs ("gemini-2.5-flash") and for
// configs that omit model entirely.
reg.RegisterDefaultModel(func(cfg map[string]any) (model.LLM, error) {
    name, _ := cfg["model"].(string)
    return gemini.New(name)
})

// Register a tool factory.
reg.RegisterTool("search", func(cfg map[string]any) (tool.Tool, error) {
    return mySearchTool(cfg)
})

// Register callbacks and a pre-built agent.
reg.RegisterBeforeAgentCallback("myapp.cb.auth", beforeAgentAuth)
reg.RegisterAgent("myapp.agents.sub", subAgent)
```

`model_args` are passed to the model factory. For a prefixed ref the factory
receives `model_args` plus `"model": <remainder>` (the ref wins over a `model`
key inside `model_args`). A bare ref passes the whole ref as `"model"`. An
omitted model passes `model_args` only; without a default factory this errors
with `ErrNoDefaultModel`.

## Skillsets

```go
type SkillsetRef struct {
    Name              string         // Factory name (e.g., "filesystem")
    Config            map[string]any // Factory-specific config
    Preload           string         // "", "complete", or "frontmatters"
    Names             []string       // Optional: specific skills to load (default: all)
    SystemInstruction string         // Optional custom instruction
}
```

See [Agent Skills Configuration](skills-config.md) for details.

## Instruction Templates

`instruction_template` accepts exactly one of `name`, `inline`, or `path`.
`path` is resolved relative to the config file's own directory inside the
`*os.Root`. When set, the template takes precedence over `instruction`.

## Loading and Building

```go
func Load(root *os.Root, path string) (*AppConfig, error)
func Parse(data []byte, format string) (*AppConfig, error)
func BuildWithPath(ctx context.Context, cfg AgentConfig, reg *Registry, root *os.Root, configPath string) (agent.Agent, error)
func BuildAppWithPath(ctx context.Context, appCfg *AppConfig, reg *Registry, root *os.Root, configPath string) (agent.Agent, *agent.RunConfig, *agent.LiveRunConfig, *ContextCacheConfig, error)
func LoadAndBuild(ctx context.Context, root *os.Root, path string, reg *Registry) (agent.Agent, *agent.RunConfig, *agent.LiveRunConfig, *ContextCacheConfig, error)
```

Each config's references resolve against its own directory; `root` scopes all
filesystem access and rejects escapes.

## Workflows

A Workflow describes a graph with `edges`. Each top-level `edges` entry is a
chain (a sequence). Inside a chain, an item is:

| Form | Meaning |
|---|---|
| `START` | the workflow entry sentinel |
| `name` | a registered node function (see below) |
| `file.yaml` / `file.yml` | a node file or agent config file |
| `{config_path: x}` / `{code: x}` | a single-key node reference |
| `{ROUTE: target, default: other}` | a route map; targets resolve like scalars |

A chain with no route maps needs at least two nodes and becomes a chain. A
chain with route maps needs at least one source node; each route-map entry
becomes an edge from the last scalar node to the resolved target. `default`
(case-insensitive) maps to the default route.

Example:

```yaml
name: pipeline
agent_class: Workflow
max_concurrency: 4
edges:
  - [START, fetch, {OK: transform, default: report}]
  - [transform, report]
```

### Node files

A `.yaml`/`.yml` reference may be a `FunctionNode`, `JoinNode`, `ToolNode`, or a
regular agent config:

```yaml
# FunctionNode
agent_class: FunctionNode
name: uppercase
func_code: myapp.nodes.uppercase
parallel_worker: false
rerun_on_resume: false
```

```yaml
# JoinNode
agent_class: JoinNode
name: join
```

```yaml
# ToolNode
agent_class: ToolNode
name: search_node
tool_code: search
args: {maxResults: 5}
```

Any other `agent_class` is parsed and built as a normal agent and wrapped in an
agent node; its own directory is the base for its references. Node-file
`description` is not accepted (no adk-go node constructor exposes it) and fails
with `ErrUnknownField`.

### Registering node functions

Go forbids type parameters on methods, so registration is a package function:

```go
config.RegisterNodeFunction(reg, "uppercase", func(ctx agent.Context, s string) (string, error) {
    return strings.ToUpper(s), nil
})
```

Any input/output types work: inputs are converted by type assertion, a string
fast path, or a JSON round-trip.

### Deduplication

Within a single build, nodes loaded from the same file share one instance
(required for join fan-in) and a repeated function reference shares one
FunctionNode. Agent files referenced more than once resolve to the same agent;
listing the same file twice under `sub_agents` therefore fails with ADK's
"appears multiple times" error. Caches are per-build, never global.

## YAML Example

```yaml
name: root-agent
agent_class: LlmAgent
model: openai/gpt-4o
instruction: "You are a helpful assistant."
instruction_template:
  inline: "You are {{.Agent.Name}}. User: {{.User.Text}}"
global_instruction: "All agents in this tree are professional and concise."
output_key: result
include_contents: default
disallow_transfer_to_parent: true
disallow_transfer_to_peers: true
tools:
  - search
  - name: scrape
    args:
      timeout: 30
skill_sets:
  - name: filesystem
    config:
      path: "./skills"
    preload: complete
generate_content_config:
  temperature: 0.7
  max_output_tokens: 1024
  safety_settings:
    - category: HARM_CATEGORY_HARASSMENT
      threshold: BLOCK_LOW_AND_ABOVE
before_model_callbacks:
  - myapp.cb.cache
run_config:
  streaming_mode: sse
  save_live_blob: true
  max_llm_calls: 100
context_cache_config:
  cache_intervals: 5
  ttl_seconds: 600
  min_tokens: 100
sub_agents:
  - ./researcher.yaml
```
