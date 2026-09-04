# adk-go-pkg

[![Go Reference](https://pkg.go.dev/badge/github.com/ieshan/adk-go-pkg.svg)](https://pkg.go.dev/github.com/ieshan/adk-go-pkg)
[![Go Report Card](https://goreportcard.com/badge/github.com/ieshan/adk-go-pkg)](https://goreportcard.com/report/github.com/ieshan/adk-go-pkg)

Extension library for Google's ADK-Go.

`adk-go-pkg` provides production-ready building blocks that complement
[ADK-Go](https://google.golang.org/adk/v2) with capabilities it does not ship
out of the box.

## Features

| Feature | Description |
|---------|-------------|
| **OpenAI Model Provider** | Drop-in `model.LLM` adapter for any OpenAI-compatible API (OpenAI, Ollama, LiteLLM, OpenRouter, vLLM, Together AI). |
| **Anthropic Model Provider** | Drop-in `model.LLM` adapter for Anthropic's Messages API and compatible providers (Claude, Amazon Bedrock, Google Vertex AI). Supports streaming, tool calling, images, structured output, thinking blocks, and prompt caching. |
| **Generic AG-UI Server** | Framework-agnostic AG-UI protocol server (`agui/`) with event emitter, state management (RFC 6902 JSON Patch via `evanphx/json-patch`), predictive state tracker, tool orchestration, middleware, encrypted-value scrubbing, and SSE handler. Zero ADK dependency. |
| **ADK-Go AG-UI Bridge** | Translates ADK-Go session events to AG-UI events (`aguiadk/`). Thread-to-session mapping, state/message snapshots, streaming tool calls, client tool hand-back (NextRun, Inline, and HandBack modes), HITL runstore & resume, tool call validation, activity snapshots, suppressed tool mode, and preset configurations. |
| **Prompt Templating** | `text/template`-based prompt rendering engine with agent context data (state, user, session, artifacts, memory), 13 built-in functions, template registry, loader (files/embed.FS), and `llmagent.InstructionProvider` integration. |
| **Planners** | Structured plan generation (ReAct JSON and free-form Thinking) that separates reasoning from execution. |
| **File Artifact Service** | Filesystem-backed `artifact.Service` with automatic versioning and metadata sidecars. |
| **Session Rewind** | Roll a session back to any prior event, recalculating state from replayed deltas. |
| **Config Agent Loader** | Declare entire agent trees in YAML/JSON and build them at runtime via a factory registry. Now includes Agent Skills support. |
| **Agent Skills Config** | Declarative skill integration via YAML/JSON. Supports filesystem sources with preload optimization and specific skill loading (wildcard or filtered by name). |
| **Test Utilities** | Complete fake implementations of all ADK-Go interfaces for deterministic testing without external LLM providers. Includes FakeLLM, FakeAgent, FakeSession, and RunnerBuilder. |
| **Evaluation Framework** | Evaluate agent performance with eval sets, built-in metrics (trajectory, response match, rubrics, safety, hallucinations), LLM-as-judge auto-raters, user simulation, and a local eval service. Mirrors ADK Python's eval package. |
| **AG-UI MCP Support** | Inject MCP (Model Context Protocol) server tools into AG-UI agents. Two integration paths: `MCPMiddleware` for generic tool injection + server-side execution, and `MCPAppsMiddleware` for UI-enabled tools + proxied MCP requests. Bridge wiring via `aguiadk.BuildMCPServerToolsets` using ADK-Go's `mcptoolset`. |

## Installation

```bash
go get github.com/ieshan/adk-go-pkg
```

## Quick Start

### OpenAI Model Provider

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/ieshan/adk-go-pkg/model/openai"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

func main() {
	m, err := openai.New(openai.Config{
		Model:  "gpt-4o",
		APIKey: os.Getenv("OPENAI_API_KEY"),
	})
	if err != nil {
		log.Fatal(err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{
			genai.NewContentFromText("Hello!", "user"),
		},
	}

	for resp, err := range m.GenerateContent(context.Background(), req, false) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(resp.Content.Parts[0].Text)
	}
}
```

[Detailed docs &rarr;](docs/openai-model.md)

### Anthropic Model Provider

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/ieshan/adk-go-pkg/model/anthropic"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

func main() {
	m, err := anthropic.New(anthropic.Config{
		Model:  "claude-sonnet-4-20250514",
		APIKey: os.Getenv("ANTHROPIC_API_KEY"),
	})
	if err != nil {
		log.Fatal(err)
	}

	req := &model.LLMRequest{
		Contents: []*genai.Content{
			genai.NewContentFromText("Hello!", "user"),
		},
	}

	for resp, err := range m.GenerateContent(context.Background(), req, false) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(resp.Content.Parts[0].Text)
	}
}
```

[Detailed docs &rarr;](docs/anthropic-model.md)

### Generic AG-UI Server (`agui/`)

```go
package main

import (
	"context"
	"iter"
	"log"
	"net/http"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func main() {
	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		ch := make(chan events.Event, 64)
		emitter := agui.NewEventEmitter(ch)
		go func() {
			defer close(ch)
			emitter.RunStarted(input.ThreadID, input.RunID)
			msgID := emitter.GenerateMessageID()
			role := "assistant"
			emitter.TextMessageStart(msgID, &role)
			emitter.TextMessageContent(msgID, "Hello from AG-UI!")
			emitter.TextMessageEnd(msgID)
			emitter.RunFinishedWithOptions(input.ThreadID, input.RunID)
		}()
		return agui.ChanToIter(ctx, ch)
	})

	handler, err := agui.Handler(agui.Config{Agent: agent})
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(http.ListenAndServe(":8080", handler))
}
```

[Detailed docs &rarr;](docs/agui-server.md)

### ADK-Go AG-UI Bridge (`aguiadk/`)

```go
package main

import (
	"iter"
	"log"
	"net/http"

	"github.com/ieshan/adk-go-pkg/agui"
	"github.com/ieshan/adk-go-pkg/aguiadk"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

func main() {
	myAgent, err := agent.New(agent.Config{
		Name: "greeter",
		Run: func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			return func(yield func(*session.Event, error) bool) {
				content := genai.NewContentFromText("Hello from ADK!", genai.RoleModel)
				yield(&session.Event{Author: "greeter", Content: content}, nil)
			}
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	handler, err := aguiadk.Handler(
		aguiadk.Config{
			Agent:   myAgent,
			AppName: "my-chatbot",
			UserID:  "default-user",
		},
		agui.Config{},
	)
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/api/agent", handler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

[Detailed docs &rarr;](docs/aguiadk-bridge.md)

### Prompt Templating

```go
package main

import (
	"fmt"
	"log"

	"github.com/ieshan/adk-go-pkg/prompt"
)

func main() {
	engine := prompt.New()
	tmpl, err := engine.Parse("greeting", "Hello {{.Input.name}}!")
	if err != nil {
		log.Fatal(err)
	}

	data := prompt.BuildData(map[string]any{"name": "world"})
	rendered, err := tmpl.Execute(data)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(rendered)
}
```

For `llmagent.InstructionProvider` integration:

```go
provider, err := prompt.NewInstructionProvider(
	"You are {{.Agent.Name}}. User: {{.User.Text}}. Country: {{.State.Get \"country\"}}",
)
// Pass to llmagent.Config{InstructionProvider: provider}
```

[Detailed docs &rarr;](docs/prompt.md)

### Planners

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ieshan/adk-go-pkg/planner"
	"google.golang.org/adk/v2/model"
)

func main() {
	var myLLM model.LLM // your model

	p := planner.NewPlanReAct(planner.PlanReActConfig{
		Model:    myLLM,
		MaxSteps: 5,
	})

	plan, err := p.GeneratePlan(context.Background(), &planner.PlanRequest{
		UserMessage: "Book a flight and send a confirmation email",
		ToolDescriptions: []planner.ToolDescription{
			{Name: "book_flight", Description: "Books a flight"},
			{Name: "send_email", Description: "Sends an email"},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	for i, step := range plan.Steps {
		fmt.Printf("Step %d: %s\n", i+1, step.Description)
	}
}
```

[Detailed docs &rarr;](docs/planners.md)

### File Artifact Service

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ieshan/adk-go-pkg/artifact/file"
	"google.golang.org/adk/v2/artifact"
	"google.golang.org/genai"
)

func main() {
	svc, err := file.New(file.Config{RootDir: "/tmp/artifacts"})
	if err != nil {
		log.Fatal(err)
	}

	resp, err := svc.Save(context.Background(), &artifact.SaveRequest{
		AppName:   "myapp",
		UserID:    "alice",
		SessionID: "session-1",
		FileName:  "report.txt",
		Part:      &genai.Part{Text: "quarterly report"},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("saved version:", resp.Version)

	// Get artifact version metadata without loading content
	versionResp, err := svc.GetArtifactVersion(context.Background(), &artifact.GetArtifactVersionRequest{
		AppName:   "myapp",
		UserID:    "alice",
		SessionID: "session-1",
		FileName:  "report.txt",
		Version:   0, // 0 means latest
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("MIME type:", versionResp.ArtifactVersion.MimeType)
}
```

[Detailed docs &rarr;](docs/file-artifact.md)

### Session Rewind

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ieshan/adk-go-pkg/session/rewind"
	"google.golang.org/adk/v2/session"
)

func main() {
	ctx := context.Background()
	svc := session.InMemoryService()

	// ... create session, append events ...

	rewound, err := rewind.RewindToIndex(ctx, svc, "my-app", "user-1", "session-abc", 2)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("events remaining:", rewound.Events().Len())
}
```

[Detailed docs &rarr;](docs/session-rewind.md)

### Config Agent Loader

```go
package main

import (
	"context"
	"log"

	"github.com/ieshan/adk-go-pkg/config"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/tool"
)

func main() {
	reg := config.NewRegistry()

	reg.RegisterModel("openai", func(cfg map[string]any) (model.LLM, error) {
		// build your model from cfg["model"] and other keys
		return nil, nil
	})
	reg.RegisterTool("search", func(cfg map[string]any) (tool.Tool, error) {
		// build your tool
		return nil, nil
	})

	agent, runCfg, liveRunCfg, ctxCacheCfg, err := config.LoadAndBuild(context.Background(), "agents/root.yaml", reg)
	if err != nil {
		log.Fatal(err)
	}
	_ = agent
	_ = runCfg       // *agent.RunConfig (may be nil)
	_ = liveRunCfg   // *agent.LiveRunConfig (may be nil)
	_ = ctxCacheCfg  // *config.ContextCacheConfig (may be nil)
}
```

[Detailed docs &rarr;](docs/config-agent.md)

### Agent Skills Config

```go
package main

import (
	"context"
	"log"

	"github.com/ieshan/adk-go-pkg/config"
)

func main() {
	reg := config.NewRegistry()
	// Filesystem skill factory is built-in, no registration needed

	// Load agent with skills from YAML
	agent, _, _, _, err := config.LoadAndBuild(context.Background(), "agents/skills-agent.yaml", reg)
	if err != nil {
		log.Fatal(err)
	}
	// Agent now has access to skills defined in ./skills/
	_ = agent
}
```

**Example YAML configuration:**

```yaml
name: skills-agent
type: llm
model: gemini/gemini-2.5-flash
instruction: "You are a helpful assistant with access to specialized skills."
skillsets:
  - name: filesystem
    config:
      path: "./skills"
    preload: complete
    # Optional: load only specific skills instead of all
    # names: ["weather", "cooking"]
```

[Detailed docs &rarr;](docs/skills-config.md)

### Test Utilities

```go
package main

import (
    "testing"

    "github.com/ieshan/adk-go-pkg/testutil"
    "google.golang.org/adk/v2/agent"
    "google.golang.org/adk/v2/agent/llmagent"
    "google.golang.org/adk/v2/runner"
    "google.golang.org/genai"
)

func TestMyAgent(t *testing.T) {
    // Create fake LLM with preconfigured responses
    llm := testutil.NewFakeLLM(
        testutil.NewTextResponse("I'll help you!"),
    )

    // Build agent with fake LLM
    ag, _ := llmagent.New(llmagent.Config{
        Name:  "test-agent",
        Model: llm,
        Instruction: "You are helpful.",
    })

    // Use RunnerBuilder for end-to-end testing
    r, fakes, _ := testutil.NewRunnerBuilder().
        WithAgent(ag).
        BuildWithFakes()

    // Run and collect events
    events, _ := testutil.CollectEvents(r.Run(ctx, "user-1", "session-1",
        genai.NewContentFromText("Hello", "user"), agent.RunConfig{}))

    // Assert on results and calls
    if len(events) == 0 {
        t.Error("expected events")
    }
    if fakes.SessionService.AppendEventCount() == 0 {
        t.Error("expected events to be appended")
    }
}
```

[Detailed docs &rarr;](docs/testutil.md)

### Evaluation Framework

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/ieshan/adk-go-pkg/eval"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()

	// Create an in-memory eval sets manager and add a case.
	setsMgr := eval.NewInMemoryEvalSetsManager()
	setsMgr.CreateEvalSet(ctx, "my-app", "basic-eval")
	setsMgr.AddEvalCase(ctx, "my-app", "basic-eval", eval.EvalCase{
		EvalID: "case-1",
		Conversation: []eval.Invocation{
			{UserContent: genai.NewContentFromText("Hello", "user")},
		},
	})

	// Create an agent evaluator with your agent runner and LLM.
	var agentRunner eval.AgentRunner // your agent runner
	var llm model.LLM               // your judge LLM (optional)

	evaluator := eval.NewAgentEvaluator(
		agentRunner, setsMgr, nil, eval.DefaultMetricEvaluatorRegistry(),
		llm,
	)

	// Configure metrics with thresholds.
	config := eval.EvalConfig{
		Criteria: map[string]json.RawMessage{
			"tool_trajectory_avg_score": json.RawMessage(`{"threshold": 0.8}`),
		},
	}

	result, err := evaluator.Evaluate(ctx, "my-app", "basic-eval", config)
	if err != nil {
		log.Fatal(err)
	}
	for _, cr := range result.EvalCaseResults {
		fmt.Printf("Case %s: %s\n", cr.EvalID, cr.FinalEvalStatus)
	}
}
```

[Detailed docs &rarr;](docs/eval.md)

### AG-UI MCP Support

Inject MCP server tools into any AG-UI agent and execute them server-side:

```go
package main

import (
	"context"
	"iter"
	"log"
	"net/http"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func main() {
	agent := agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
		// your agent logic
		return nil
	})

	mcpMW := agui.NewMCPMiddleware([]agui.MCPClientConfig{
		{Type: "http", URL: "https://example.com/mcp", ServerID: "srv1"},
	}, agui.MCPMiddlewareOptions{MaxIterations: 32})

	handler, err := agui.Handler(agui.Config{Agent: mcpMW(agent)})
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(http.ListenAndServe(":8080", handler))
}
```

For ADK-Go native integration, use `aguiadk.BuildMCPServerToolsets` to create `mcptoolset.Toolset` instances from MCP configs. See [docs/agui-mcp.md](docs/agui-mcp.md) for both integration paths, `MCPAppsMiddleware` for UI-enabled tools, and proxied MCP request handling.

## Compatibility

- **Go 1.26+** — Uses `iter.Seq2` and range-over-func.
- **ADK-Go v2.0.0+** (`google.golang.org/adk/v2`) — Required for Agent Skills support
- **GenAI v1.65.0** (`google.golang.org/genai`)

## Recent Changes

- **Prompt Templating**: New `prompt` package with `text/template`-based rendering engine, agent context data (state, user, session, artifacts, memory), 13 built-in functions, thread-safe `TemplateRegistry`, `TemplateLoader` (files/embed.FS), `TemplateRef` tagged union, and `llmagent.InstructionProvider` integration. Config loader supports `InstructionTemplate` field for declarative templated instructions. See [docs/prompt.md](docs/prompt.md).
- **AG-UI ADK Bridge Gap Fix**: Closed all 10 AG-UI protocol feature gaps between the `agui`/`aguiadk` packages and the AG-UI example server. New features: disconnect cancellation, client tool hand-back (NextRun, Inline, and HandBack modes via `ClientToolset`), streaming tool calls (progressive `TOOL_CALL_*` from partial `FunctionCall` parts), HITL runstore & resume (`RunStore` with TTL, atomic claim, approval interrupts), tool call validation (synthetic IDs, error `TOOL_CALL_RESULT` for malformed calls), suppressed tool mode (`Config.SuppressToolEvents` + `Config.ToolToStateMapper` emits `STATE_DELTA` instead of `TOOL_CALL_*`), predictive state tracker (`agui.PredictiveStateTracker` for ghosted `/_predictive` deltas), activity snapshots (`tool_use` and `approval_request`), encrypted value scrubbing in `MessagesSnapshot`, and preset configurations (`AgenticChatPreset`, `HumanInTheLoopPreset`, `GenerativeUIPreset`, `SharedStatePreset`, `InlineToolsPreset`, `HandBackPreset`, `PredictiveStatePreset`, `AgenticGenerativeUIPreset`). `StateManager.Apply` now uses `evanphx/json-patch/v5` for RFC 6902 compliance. See [docs/aguiadk-bridge.md](docs/aguiadk-bridge.md).
- **Evaluation Framework**: New `eval` package with eval sets, 13 built-in metrics, LLM-as-judge evaluators, user simulation, and local eval service. Mirrors ADK Python eval package. See [docs/eval.md](docs/eval.md).
- **Anthropic Model Provider**: Drop-in `model.LLM` adapter for Anthropic's Messages API. Supports streaming, tool calling, images, structured output, thinking blocks, and prompt caching. See [docs/anthropic-model.md](docs/anthropic-model.md).
- **Test Utilities**: New `testutil` package with fake implementations of all ADK-Go interfaces (FakeLLM, FakeAgent, FakeSession, FakeArtifactService, FakeMemoryService, FakeSessionService, RunnerBuilder). Enables fast, deterministic testing without external LLM providers. See [docs/testutil.md](docs/testutil.md).
- **Agent Skills Config**: Skillset support in config loader. Define skills in YAML/JSON with filesystem sources, preload optimization, and specific skill loading (wildcard or filtered by name).
- **OpenAI Model Provider**: Supports genai `FunctionResponse.Parts` structure for function calling.
- **AG-UI MCP Support**: MCP (Model Context Protocol) integration for AG-UI agents. `MCPMiddleware` injects MCP server tools and executes them server-side in an agentic loop. `MCPAppsMiddleware` handles UI-enabled tools (SEP-1865) and proxied MCP requests from frontends. `aguiadk.BuildMCPServerToolsets` bridges MCP servers to ADK-Go's native `mcptoolset`. See [docs/agui-mcp.md](docs/agui-mcp.md).
- **File Artifact Service**: `GetArtifactVersion` method for metadata retrieval without loading full content.

## Dependencies

Beyond ADK-Go and `google.golang.org/genai`, the only additional direct dependencies are:

- [`github.com/ag-ui-protocol/ag-ui/sdks/community/go`](https://github.com/ag-ui-protocol/ag-ui) -- AG-UI event types and helpers
- [`github.com/evanphx/json-patch/v5`](https://github.com/evanphx/json-patch) -- RFC 6902 JSON Patch for `StateManager.Apply`
- [`github.com/google/jsonschema-go`](https://github.com/google/jsonschema-go) -- JSON Schema for `ClientToolset` parameter validation
- [`github.com/google/uuid`](https://github.com/google/uuid) -- UUID generation for eval session IDs
- [`github.com/modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk) -- MCP client for transport, tool listing, and execution
- [`go.yaml.in/yaml/v4`](https://github.com/go-yaml/yaml) -- YAML parsing for the config loader
- [`golang.org/x/sync`](https://pkg.go.dev/golang.org/x/sync) -- `errgroup` for parallel MCP server queries

## License

TBD
