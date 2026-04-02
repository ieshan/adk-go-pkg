# adk-go-pkg

Extension library for Google's ADK-Go.

`adk-go-pkg` provides production-ready building blocks that complement
[ADK-Go](https://google.golang.org/adk) with capabilities it does not ship
out of the box.

## Features

| Feature | Description |
|---------|-------------|
| **OpenAI Model Provider** | Drop-in `model.LLM` adapter for any OpenAI-compatible API (OpenAI, Ollama, LiteLLM, OpenRouter, vLLM, Together AI). |
| **Generic AG-UI Server** | Framework-agnostic AG-UI protocol server (`agui/`) with event emitter, state management, tool orchestration, middleware, and SSE handler. Zero ADK dependency. |
| **ADK-Go AG-UI Bridge** | Translates ADK-Go session events to AG-UI events (`aguiadk/`). Thread-to-session mapping, state/message snapshots, and client tool proxy. |
| **Event Compaction** | Pluggable strategies (truncation, LLM summarisation) for keeping session event histories within token budgets. |
| **Planners** | Structured plan generation (ReAct JSON and free-form Thinking) that separates reasoning from execution. |
| **File Artifact Service** | Filesystem-backed `artifact.Service` with automatic versioning and metadata sidecars. |
| **Session Rewind** | Roll a session back to any prior event, recalculating state from replayed deltas. |
| **Config Agent Loader** | Declare entire agent trees in YAML/JSON and build them at runtime via a factory registry. |

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
	"google.golang.org/adk/model"
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
			emitter.RunFinished(input.ThreadID, input.RunID)
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
	"log"
	"net/http"

	"github.com/ieshan/adk-go-pkg/agui"
	"github.com/ieshan/adk-go-pkg/aguiadk"
	"google.golang.org/adk/agent"
)

func main() {
	myAgent := agent.New("greeter", nil,
		agent.WithInstruction("You are a friendly greeter."),
	)

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

### Event Compaction

```go
package main

import (
	"context"
	"log"

	"github.com/ieshan/adk-go-pkg/compaction"
	"google.golang.org/adk/session"
)

func main() {
	tr := compaction.NewTruncation(20)
	cfg := compaction.Config{
		Strategy:   tr,
		MaxEvents:  100,
		KeepRecent: 20,
	}

	var events []*session.Event // from your session
	compacted, err := compaction.Apply(context.Background(), cfg, events)
	if err != nil {
		log.Fatal(err)
	}
	_ = compacted
}
```

[Detailed docs &rarr;](docs/compaction.md)

### Planners

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ieshan/adk-go-pkg/planner"
	"google.golang.org/adk/model"
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
	"google.golang.org/adk/artifact"
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
	"google.golang.org/adk/session"
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
	"log"

	"github.com/ieshan/adk-go-pkg/config"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
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

	agent, err := config.LoadAndBuild("agents/root.yaml", reg)
	if err != nil {
		log.Fatal(err)
	}
	_ = agent
}
```

[Detailed docs &rarr;](docs/config-agent.md)

## Compatibility

- **Go 1.25+** — Required because ADK-Go (`google.golang.org/adk`) specifies `go 1.25` in its `go.mod`. Uses `iter.Seq2` and range-over-func.
- **ADK-Go** (`google.golang.org/adk`)

## Dependencies

Beyond ADK-Go and `google.golang.org/genai`, the only additional direct dependencies are:

- [`github.com/ag-ui-protocol/ag-ui/sdks/community/go`](https://github.com/ag-ui-protocol/ag-ui) -- AG-UI event types and helpers
- [`sigs.k8s.io/yaml`](https://github.com/kubernetes-sigs/yaml) -- YAML parsing for the config loader

## License

TBD
