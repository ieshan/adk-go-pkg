# Prompt Templating

Package `prompt` provides `text/template`-based prompt rendering for ADK-Go agents.

## Overview

The prompt package exposes a `TemplateEngine` that parses Go `text/template` strings
and executes them against a `TemplateData` object populated from agent context,
session state, artifacts, memory, and structured input.

### Key Types

| Type | Description |
|------|-------------|
| `TemplateEngine` | Parses and executes `text/template` prompts. Immutable after construction. |
| `Template` | A parsed prompt template. Call `Execute` to render. |
| `TemplateData` | Top-level data object passed to templates. Exposes `.State`, `.User`, `.Session`, `.Agent`, `.Memory`, `.App`, `.Input`, and `.Artifact`. |
| `TemplateRegistry` | Thread-safe registry of named templates. Parses once, executes many. |
| `TemplateLoader` | Loads templates from strings, files, or `embed.FS`. |
| `TemplateRef` | Tagged union for referencing a template by name, inline text, or file path. |

### Integration Points

- **`llmagent.InstructionProvider`** via `prompt.NewInstructionProvider` / `prompt.NewInstructionProviderFromTemplate`.
- **`config.Registry`** template registry for declarative agent configs (`InstructionTemplate` field).
- **`planner.PlanReActConfig`** / `planner.ThinkingConfig` custom system prompts.
- **`eval/simulation`** user simulator prompts.

## API Reference

### TemplateEngine

```go
type TemplateEngine struct { ... }

// Create with defaults
engine := prompt.New()

// Create with custom functions (merged over defaults)
engine := prompt.New(prompt.WithFuncs(template.FuncMap{
    "shout": strings.ToUpper,
}))
```

The engine is immutable after construction. The function map is fixed at creation
time, avoiding the common pitfall of calling `Funcs` after `Parse`.

### Template

```go
tmpl, err := engine.Parse("greeting", "Hello {{.Input.name}}!")
if err != nil { ... }

rendered, err := tmpl.Execute(data)
```

`MustParse` is like `Parse` but panics on error — useful for package-level
templates.

### TemplateData

`TemplateData` is the top-level data object passed to templates. Fields are
exposed as struct fields and nested data objects provide methods (rather than
function fields) because `text/template` can call methods directly.

```go
type TemplateData struct {
    State   *StateData
    User    *UserData
    Session *SessionData
    Agent   *AgentData
    Memory  *MemoryData
    App     *AppData
    Input   map[string]any
}
```

### Building TemplateData

Three builder functions cover different context types:

| Function | Use When | Populates |
|----------|----------|-----------|
| `BuildData(input)` | Standalone templates (no agent context) | `Input` only |
| `BuildDataFromReadonlyContext(ctx)` | `agent.ReadonlyContext` (e.g., `InstructionProvider`) | `State`, `User`, `Session`, `Agent`, `App`, and optionally `Memory`/`Artifacts` via type assertion |
| `BuildDataFromInvocationContext(ctx)` | `agent.InvocationContext` (e.g., inside agent `Run`) | `State`, `User`, `Session`, `Agent`, `App`, `Memory`, `Artifacts` |

`BuildDataFromReadonlyContext` uses a type assertion to access `Artifacts()`,
`Memory()`, `Session()`, and `Agent()` methods when the concrete context
implementation provides them beyond the public `ReadonlyContext` interface.

### Data Sub-Types

#### StateData

Wraps `session.ReadonlyState` for template access:

```go
{{.State.Get "user_name"}}       // returns value or nil
{{if .State.Has "country"}}...{{end}}
{{range $k, $v := .State.All}}{{$k}}={{$v}};{{end}}
```

#### UserData

Exposes the content that started the invocation:

```go
{{.User.Text}}        // concatenated text from user content
{{.User.Content}}     // *genai.Content
```

#### SessionData

Exposes session identifiers:

```go
{{.Session.ID}}
{{.Session.AppName}}
{{.Session.UserID}}
```

#### AgentData

Exposes agent metadata:

```go
{{.Agent.Name}}
{{.Agent.Description}}
```

#### MemoryData

Exposes memory search:

```go
{{range .Memory.Search "relevant query"}}
- {{.}}
{{end}}
```

`Search` returns `([]string, error)`. In templates, errors cause execution to fail.

#### AppData

Exposes app metadata:

```go
{{.App.Name}}
```

#### Artifact (method on TemplateData)

Loads the text content of a named artifact:

```go
{{.Artifact "reference_doc"}}
```

The second argument, when `true`, treats a missing artifact as an empty string
rather than an error:

```go
{{.Artifact "optional_doc" true}}
```

### Template Functions

All functions are pipeline-friendly: the pipeline value is passed as the last
argument.

| Function | Signature | Example |
|----------|-----------|---------|
| `tojson` | `(v any) (string, error)` | `{{.Input.data \| tojson}}` |
| `fromjson` | `(s string) (any, error)` | `{{.Input.json \| fromjson}}` |
| `truncate` | `(n int, s string) string` | `{{.Input.text \| truncate 50}}` |
| `indent` | `(prefix, s string) string` | `{{.Input.text \| indent "  "}}` |
| `join` | `(sep string, elems []string) string` | `{{.Input.items \| join ", "}}` |
| `lower` | `(s string) string` | `{{.Input.text \| lower}}` |
| `upper` | `(s string) string` | `{{.Input.text \| upper}}` |
| `trim` | `(s string) string` | `{{.Input.text \| trim}}` |
| `default` | `(def, val any) any` | `{{.Input.lang \| default "en"}}` |
| `contains` | `(substr, s string) bool` | `{{.Input.text \| contains "lo"}}` |
| `hasprefix` | `(prefix, s string) bool` | `{{.Input.text \| hasprefix "he"}}` |
| `hassuffix` | `(suffix, s string) bool` | `{{.Input.text \| hassuffix "lo"}}` |

`default` returns `def` when `val` is considered empty (nil, false, 0, "", empty
map/slice); otherwise it returns `val`.

### TemplateRegistry

Thread-safe registry of named templates. Templates are parsed once and can be
executed multiple times.

```go
engine := prompt.New()
registry := prompt.NewRegistry(engine)

// Register from string
if err := registry.Register("greeting", "Hello {{.Input.name}}!"); err != nil {
    log.Fatal(err)
}

// Register from file beneath an os.Root
root, err := os.OpenRoot(".")
if err != nil {
    log.Fatal(err)
}
defer root.Close()
if err := registry.RegisterFile("system_prompt", root, "prompts/system.tmpl"); err != nil {
    log.Fatal(err)
}

// Get a template
tmpl, ok := registry.Get("greeting")

// Render by name with agent context (ctx is an agent.ReadonlyContext)
rendered, err := registry.Render("greeting", ctx)
if err != nil {
    log.Fatal(err)
}
_ = rendered

// List all registered template names (sorted)
names := registry.Names()
```

### TemplateLoader

Loads templates from strings, files beneath an `*os.Root`, or an `embed.FS`/custom `fs.FS`.

```go
engine := prompt.New()

// os.Root-backed (paths are scoped beneath root)
root, err := os.OpenRoot(".")
if err != nil {
    log.Fatal(err)
}
defer root.Close()
rootLoader := prompt.NewLoader(engine, root)

// embed.FS or any fs.FS
//go:embed templates/*.tmpl
var templateFS embed.FS
fsLoader := prompt.NewLoaderFromFS(engine, templateFS)

// Load from string (works on any loader regardless of filesystem)
tmpl, err := rootLoader.LoadFromString("inline", "value={{.Input.v}}")

// Load from file (name = filepath.Base(path)); path is resolved within the
// loader's filesystem. For rootLoader the path is root-relative; for
// fsLoader it is relative to the embed.FS root.
tmpl, err = fsLoader.LoadFromFile("templates/system.tmpl")
```

> **Note:** Passing a nil root to `NewLoader` or a nil `fs.FS` to
> `NewLoaderFromFS` creates a loader with no filesystem attached.
> `LoadFromFile` will return an error on such a loader; `LoadFromString`
> still works. Use `NewLoader(engine, root)` with an `*os.Root` or
> `NewLoaderFromFS(engine, fsys)` with an `embed.FS`/custom `fs.FS` to
> enable file loading.

### TemplateRef

A tagged union for referencing a template by name, inline text, or file path.
Exactly one of `Name`, `Inline`, or `Path` must be set.

```go
type TemplateRef struct {
    Name   string `json:"name,omitempty" yaml:"name,omitempty"`
    Inline string `json:"inline,omitempty" yaml:"inline,omitempty"`
    Path   string `json:"path,omitempty" yaml:"path,omitempty"`
}
```

`Resolve` resolves the reference using a `TemplateRegistry` (for `Name`) and a
`TemplateLoader` (for `Inline` and `Path`):

```go
ref := &prompt.TemplateRef{Inline: "You are {{.Agent.Name}}."}
tmpl, err := ref.Resolve(registry, loader)
```

## Instruction Providers

The prompt package integrates with ADK-Go's `llmagent.InstructionProvider` to
render dynamic instructions on each invocation.

### NewInstructionProvider

Parses template text and returns an `InstructionProvider`:

```go
provider, err := prompt.NewInstructionProvider(
    "You are {{.Agent.Name}}. User: {{.User.Text}}. Country: {{.State.Get \"country\"}}",
)
```

### NewInstructionProviderFromTemplate

Creates an `InstructionProvider` from an already-parsed template. Accepts an
optional `inputFn` that populates `TemplateData.Input` from the agent context:

```go
engine := prompt.New()
tmpl := engine.MustParse("instruction", "Hello {{.Input.greeting}} {{.State.Get \"user_name\"}}")

provider := prompt.NewInstructionProviderFromTemplate(tmpl, func(ctx agent.ReadonlyContext) map[string]any {
    return map[string]any{"greeting": "Hi"}
})
```

## Examples

### Basic Template Rendering

```go
engine := prompt.New()
tmpl, err := engine.Parse("greeting", "Hello {{.Input.name}}!")
if err != nil {
    log.Fatal(err)
}

data := prompt.BuildData(map[string]any{"name": "world"})
rendered, err := tmpl.Execute(data)
fmt.Println(rendered) // "Hello world!"
```

### Pipeline with Functions

```go
tmpl := engine.MustParse("pipe", "{{.Input.text | upper | truncate 5}}")
data := prompt.BuildData(map[string]any{"text": "hello world"})
rendered, _ := tmpl.Execute(data)
fmt.Println(rendered) // "HE..."
```

### Conditional

Use `if`/`else` with any boolean data:

```go
data := prompt.BuildData(map[string]any{"logged_in": true})
tmpl := engine.MustParse("cond", `{{if .Input.logged_in}}welcome back{{else}}please sign in{{end}}`)
rendered, _ := tmpl.Execute(data)
```

For state-backed conditionals, pass an `agent.ReadonlyContext` to `prompt.BuildDataFromReadonlyContext` and use `{{if .State.Has "key"}}`.

### Using Templates with Agents in Code

All examples below construct agents programmatically — no config files or
`config.Registry` required.

#### Static Instruction Provider

Use `NewInstructionProvider` when the template text is known at compile time.
The provider renders the template on each invocation, pulling data from the
agent's `ReadonlyContext`:

```go
package main

import (
	"log"

	"github.com/ieshan/adk-go-pkg/prompt"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
)

func main() {
	var llm model.LLM // your model

	provider, err := prompt.NewInstructionProvider(
		"You are {{.Agent.Name}}. The user said: {{.User.Text}}. " +
			"Country: {{.State.Get \"country\" | default \"unknown\"}}.",
	)
	if err != nil {
		log.Fatal(err)
	}

	ag, err := llmagent.New(llmagent.Config{
		Name:                "greeter",
		Model:               llm,
		InstructionProvider: provider,
	})
	if err != nil {
		log.Fatal(err)
	}
	_ = ag
}
```

#### Pre-Parsed Template with Input Function

Use `NewInstructionProviderFromTemplate` when you want to parse the template
once (e.g. from a file or `embed.FS`) and optionally inject structured input
via a callback:

```go
package main

import (
	"embed"
	"log"

	"github.com/ieshan/adk-go-pkg/prompt"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
)

//go:embed prompts/*.tmpl
var promptFS embed.FS

func main() {
	var llm model.LLM // your model

	engine := prompt.New()
	loader := prompt.NewLoaderFromFS(engine, promptFS)

	tmpl, err := loader.LoadFromFile("prompts/system.tmpl")
	if err != nil {
		log.Fatal(err)
	}

	provider := prompt.NewInstructionProviderFromTemplate(tmpl, func(ctx agent.ReadonlyContext) map[string]any {
		// Build structured input from agent context or external sources.
		return map[string]any{
			"tone":      "professional",
			"language":  "en",
			"max_words": 200,
		}
	})

	ag, err := llmagent.New(llmagent.Config{
		Name:                "assistant",
		Model:               llm,
		InstructionProvider: provider,
	})
	if err != nil {
		log.Fatal(err)
	}
	_ = ag
}
```

The template in `prompts/system.tmpl` can reference both agent context data
(`{{.State.Get "key"}}`, `{{.User.Text}}`, `{{.Agent.Name}}`) and the injected
input (`{{.Input.tone}}`, `{{.Input.language}}`, `{{.Input.max_words}}`).

#### Template Registry with Multiple Named Templates

Register several templates and select one at runtime, useful when the agent's
instruction varies by mode or persona:

```go
package main

import (
	"log"

	"github.com/ieshan/adk-go-pkg/prompt"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
)

func main() {
	var llm model.LLM // your model

	engine := prompt.New()
	registry := prompt.NewRegistry(engine)

	if err := registry.Register("formal", "Greetings. You are {{.Agent.Name}}. How may I assist?"); err != nil {
		log.Fatal(err)
	}
	if err := registry.Register("casual", "Hey! I'm {{.Agent.Name}}. What's up?"); err != nil {
		log.Fatal(err)
	}

	// Select a template at runtime (e.g. based on config, user preference, etc.)
	tmpl, ok := registry.Get("casual")
	if !ok {
		log.Fatal("template not found")
	}

	provider := prompt.NewInstructionProviderFromTemplate(tmpl)

	ag, err := llmagent.New(llmagent.Config{
		Name:                "chatty",
		Model:               llm,
		InstructionProvider: provider,
	})
	if err != nil {
		log.Fatal(err)
	}
	_ = ag
}
```

#### Rendering Inside a Custom Agent's Run Function

For agents that implement `agent.Config.Run` directly (not `llmagent`), use
`BuildDataFromInvocationContext` to populate template data from the invocation
context:

```go
package main

import (
	"iter"
	"log"

	"github.com/ieshan/adk-go-pkg/prompt"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

func main() {
	engine := prompt.New()
	tmpl := engine.MustParse("custom", "Processing: {{.User.Text}} in {{.App.Name}}")

	myAgent, err := agent.New(agent.Config{
		Name: "custom-agent",
		Run: func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			return func(yield func(*session.Event, error) bool) {
				data := prompt.BuildDataFromInvocationContext(ctx)
				rendered, err := tmpl.Execute(data)
				if err != nil {
					yield(nil, err)
					return
				}

				content := genai.NewContentFromText(rendered, genai.RoleModel)
				event := &session.Event{Author: "custom-agent"}
				event.Content = content
				yield(event, nil)
			}
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	_ = myAgent
}
```

#### Rendering with Artifacts and Memory

Templates can access artifacts and memory when the context provides them.
`BuildDataFromReadonlyContext` uses a type assertion to access `Artifacts()`
and `Memory()` on concrete ADK context implementations:

```go
provider, err := prompt.NewInstructionProvider(
	`{{.Artifact "reference_doc"}}

User: {{.User.Text}}

Relevant memories:
{{range .Memory.Search "relevant context"}}
- {{.}}
{{end}}`,
)
if err != nil {
	log.Fatal(err)
}

ag, err := llmagent.New(llmagent.Config{
	Name:                "knowledge-agent",
	Model:               llm,
	InstructionProvider: provider,
})
```

Use the optional second argument to `Artifact` to suppress errors when an
artifact may not exist:

```go
provider, err := prompt.NewInstructionProvider(
	"{{.Artifact \"optional_context\" true}}\nYou are {{.Agent.Name}}.",
)
```

#### Sub-Agent Receiving Structured Input via State

In ADK-Go, parent and sub-agents share the same session state. When a parent
agent sets `OutputKey`, its response text is saved into state under that key.
A sub-agent's template can read that value via `{{.State.Get "key"}}`:

```go
package main

import (
	"log"

	"github.com/ieshan/adk-go-pkg/prompt"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
)

func main() {
	var llm model.LLM // your model

	// Parent agent saves its summary to state under "research_summary".
	parent, err := llmagent.New(llmagent.Config{
		Name:        "researcher",
		Model:       llm,
		Instruction: "Research the user's topic and provide a concise summary.",
		OutputKey:   "research_summary",
	})
	if err != nil {
		log.Fatal(err)
	}

	// Sub-agent reads the parent's output from state and uses it in its
	// templated instruction.
	subProvider, err := prompt.NewInstructionProvider(
		`You are a report writer.
Use the following research summary as your primary source:

{{.State.Get "research_summary"}}

Write a polished report based on this summary.`,
	)
	if err != nil {
		log.Fatal(err)
	}

	writer, err := llmagent.New(llmagent.Config{
		Name:                "writer",
		Model:               llm,
		InstructionProvider: subProvider,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Wire the sub-agent into the parent.
	// In practice, use a sequential/parallel agent or transfer to connect them.
	_ = parent
	_ = writer
}
```

#### Sub-Agent with Input Function Transforming State

When the sub-agent needs to transform or restructure the parent's output before
template rendering, use `NewInstructionProviderFromTemplate` with an `inputFn`.
The `inputFn` receives `agent.ReadonlyContext`, which exposes `ReadonlyState()`
so you can read parent state and build structured input:

```go
package main

import (
	"encoding/json"
	"log"

	"github.com/ieshan/adk-go-pkg/prompt"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

func main() {
	var llm model.LLM // your model

	// Parent saves JSON-structured data to state via OutputKey.
	parent, err := llmagent.New(llmagent.Config{
		Name:        "data-collector",
		Model:       llm,
		Instruction: "Collect user requirements and output as JSON.",
		OutputKey:   "collected_data",
		OutputSchema: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"topic":     {Type: genai.TypeString},
				"audience":  {Type: genai.TypeString},
				"tone":      {Type: genai.TypeString},
				"key_points": {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	engine := prompt.New()
	tmpl := engine.MustParse("writer_instruction", `You are a content writer.
Topic: {{.Input.topic}}
Audience: {{.Input.audience}}
Tone: {{.Input.tone}}

Key points to cover:
{{range .Input.key_points}}
- {{.}}
{{end}}

Write a compelling piece based on these requirements.`)

	// The inputFn reads the parent's structured output from state,
	// unmarshals it, and passes it as template Input.
	subProvider := prompt.NewInstructionProviderFromTemplate(tmpl, func(ctx agent.ReadonlyContext) map[string]any {
		raw, err := ctx.ReadonlyState().Get("collected_data")
		if err != nil {
			return map[string]any{"topic": "unknown", "audience": "general", "tone": "neutral", "key_points": []any{}}
		}

		var data map[string]any
		switch v := raw.(type) {
		case string:
			if err := json.Unmarshal([]byte(v), &data); err != nil {
				return map[string]any{"topic": raw, "audience": "general", "tone": "neutral", "key_points": []any{}}
			}
		case map[string]any:
			data = v
		default:
			return map[string]any{"topic": "unknown", "audience": "general", "tone": "neutral", "key_points": []any{}}
		}
		return data
	})

	writer, err := llmagent.New(llmagent.Config{
		Name:                "content-writer",
		Model:               llm,
		InstructionProvider: subProvider,
	})
	if err != nil {
		log.Fatal(err)
	}

	_ = parent
	_ = writer
}
```

#### Sequential Pipeline: Parent Output Feeds Sub-Agent Template

Use a `sequentialagent` to run the parent first, then the sub-agent. The
sub-agent's template reads the parent's `OutputKey` from shared state:

```go
package main

import (
	"log"

	"github.com/ieshan/adk-go-pkg/prompt"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/agent/workflowagents/sequentialagent"
	"google.golang.org/adk/v2/model"
)

func main() {
	var llm model.LLM // your model

	// Step 1: Analyzer saves its analysis to state.
	analyzer, err := llmagent.New(llmagent.Config{
		Name:        "analyzer",
		Model:       llm,
		Instruction: "Analyze the user's input and provide a structured analysis.",
		OutputKey:   "analysis_result",
	})
	if err != nil {
		log.Fatal(err)
	}

	// Step 2: Synthesizer reads the analysis from state via template.
	synthProvider, err := prompt.NewInstructionProvider(
		`You are a synthesizer.

Previous analysis:
{{.State.Get "analysis_result"}}

Synthesize this into a final recommendation.`,
	)
	if err != nil {
		log.Fatal(err)
	}

	synthesizer, err := llmagent.New(llmagent.Config{
		Name:                "synthesizer",
		Model:               llm,
		InstructionProvider: synthProvider,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Run analyzer first, then synthesizer. State is shared.
	pipeline, err := sequentialagent.New(sequentialagent.Config{
		AgentConfig: agent.Config{
			Name:      "analysis-pipeline",
			SubAgents: []agent.Agent{analyzer, synthesizer},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	_ = pipeline
}
```

#### Sub-Agent Reading Multiple State Keys

When a parent agent (or earlier pipeline steps) write multiple values to state,
the sub-agent template can read all of them:

```go
package main

import (
	"log"

	"github.com/ieshan/adk-go-pkg/prompt"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
)

func main() {
	var llm model.LLM // your model

	provider, err := prompt.NewInstructionProvider(
		`You are a decision agent.

Context from previous agents:
- Research: {{.State.Get "research_summary"}}
- Analysis: {{.State.Get "analysis_result"}}
- Risk assessment: {{.State.Get "risk_assessment"}}

Based on all the above, make a final decision.`,
	)
	if err != nil {
		log.Fatal(err)
	}

	decisionAgent, err := llmagent.New(llmagent.Config{
		Name:                "decision-maker",
		Model:               llm,
		InstructionProvider: provider,
	})
	if err != nil {
		log.Fatal(err)
	}
	_ = decisionAgent
}
```

You can also iterate over all state keys if the set of keys is dynamic:

```go
tmpl := engine.MustParse("dynamic", `State snapshot:
{{range $k, $v := .State.All}}- {{$k}}: {{$v}}
{{end}}`)
```

### With Config Agent Loader

Use `InstructionTemplate` in YAML to declare a templated instruction:

```yaml
name: my-agent
agent_class: LlmAgent
model: openai/gpt-4o
instruction_template:
  inline: "You are {{.Agent.Name}}. User: {{.User.Text}}"
```

Or reference a named template from a registry:

```yaml
name: my-agent
agent_class: LlmAgent
model: openai/gpt-4o
instruction_template:
  name: "greeting"
```

Or load from a file:

```yaml
name: my-agent
agent_class: LlmAgent
model: openai/gpt-4o
instruction_template:
  path: "prompts/system.tmpl"
```

Register a `TemplateRegistry` on the config `Registry` to resolve named templates:

```go
reg := config.NewRegistry()
engine := prompt.New()
tmplReg := prompt.NewRegistry(engine)
tmplReg.Register("greeting", "Hello from {{.Agent.Name}}.")
reg.RegisterTemplateRegistry(tmplReg)
```

`InstructionTemplate` takes precedence over `Instruction` when both are set.

### With embed.FS

```go
package main

import (
	"embed"
	"log"

	"github.com/ieshan/adk-go-pkg/prompt"
)

//go:embed prompts/*.tmpl
var promptFS embed.FS

func main() {
	engine := prompt.New()
	loader := prompt.NewLoaderFromFS(engine, promptFS)

	tmpl, err := loader.LoadFromFile("prompts/system.tmpl")
	if err != nil {
		log.Fatal(err)
	}
	_ = tmpl
}
```

### Registry with Concurrent Access

```go
package main

import (
	"fmt"
	"sync"

	"github.com/ieshan/adk-go-pkg/prompt"
)

func main() {
	engine := prompt.New()
	registry := prompt.NewRegistry(engine)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("tmpl%d", i)
			if err := registry.Register(name, "{{.Input.v}}"); err != nil {
				return
			}
			if tmpl, ok := registry.Get(name); ok {
				_, _ = tmpl.Execute(prompt.BuildData(map[string]any{"v": i}))
			}
		}(i)
	}
	wg.Wait()
}
```

## See Also

- [Config Agent Loader](config-agent.md) — `InstructionTemplate` field for declarative agent configs
- [Planners](planners.md) — custom system prompts for plan generation
- [Test Utilities](testutil.md) — `FakeReadonlyContext`, `FakeInvocationContext` for testing templates
