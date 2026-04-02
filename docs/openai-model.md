# OpenAI Model Provider

Package `model/openai` provides a drop-in `model.LLM` adapter for any
OpenAI-compatible Chat Completions API.

## Overview

The adapter translates between ADK-Go's `model.LLMRequest` / `model.LLMResponse`
types and the OpenAI `/v1/chat/completions` wire format. Because it targets the
OpenAI protocol (not a vendor SDK), it works with any server that speaks that
protocol.

### Supported Providers

| Provider | BaseURL |
|----------|---------|
| **OpenAI** | `https://api.openai.com/v1` (default) |
| **Ollama** | `http://localhost:11434/v1` |
| **LiteLLM** | `http://localhost:4000/v1` |
| **OpenRouter** | `https://openrouter.ai/api/v1` |
| **vLLM** | `http://localhost:8000/v1` |
| **Together AI** | `https://api.together.xyz/v1` |

Any other server that implements the OpenAI Chat Completions API will also work.

## API Reference

### Config

```go
type Config struct {
    // Model identifier (e.g. "gpt-4o", "llama3"). Required.
    Model string

    // API key sent as "Authorization: Bearer <APIKey>".
    // Leave empty for servers that do not require auth.
    APIKey string

    // Base URL without trailing slash. Defaults to "https://api.openai.com/v1".
    BaseURL string

    // HTTP client. Defaults to http.DefaultClient.
    HTTPClient *http.Client

    // Additional headers sent with every request.
    Headers map[string]string
}
```

### New

```go
func New(cfg Config) (model.LLM, error)
```

Creates a `model.LLM` that communicates with the configured endpoint. Returns
an error when `cfg.Model` is empty. The returned LLM is safe for concurrent use.

## Examples

### Basic Completion

```go
m, err := openai.New(openai.Config{
    Model:  "gpt-4o",
    APIKey: os.Getenv("OPENAI_API_KEY"),
})
if err != nil {
    log.Fatal(err)
}

req := &model.LLMRequest{
    Contents: []*genai.Content{
        genai.NewContentFromText("What is Go?", "user"),
    },
}

for resp, err := range m.GenerateContent(ctx, req, false) {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(resp.Content.Parts[0].Text)
}
```

### Streaming

Pass `stream=true` to receive incremental deltas via SSE:

```go
for resp, err := range m.GenerateContent(ctx, req, true) {
    if err != nil {
        break
    }
    if resp.TurnComplete {
        break
    }
    if resp.Partial {
        fmt.Print(resp.Content.Parts[0].Text)
    }
}
```

Each partial response has `Partial: true`. The final sentinel carries
`TurnComplete: true`.

### Tool Calling

Tools declared in `model.LLMRequest` are automatically translated to the OpenAI
`tools` array format. When the model calls a tool, the response carries
`FunctionCall` parts:

```go
req := &model.LLMRequest{
    Contents: []*genai.Content{
        genai.NewContentFromText("What is the weather in London?", "user"),
    },
    Config: &genai.GenerateContentConfig{
        Tools: []*genai.Tool{{
            FunctionDeclarations: []*genai.FunctionDeclaration{{
                Name:        "get_weather",
                Description: "Returns the current weather for a city.",
                Parameters: &genai.Schema{
                    Type: genai.TypeObject,
                    Properties: map[string]*genai.Schema{
                        "city": {Type: genai.TypeString},
                    },
                    Required: []string{"city"},
                },
            }},
        }},
    },
}

for resp, err := range m.GenerateContent(ctx, req, false) {
    if err != nil {
        log.Fatal(err)
    }
    for _, part := range resp.Content.Parts {
        if part.FunctionCall != nil {
            fmt.Printf("Tool call: %s(%v)\n", part.FunctionCall.Name, part.FunctionCall.Args)
        }
    }
}
```

### Structured Output

Request JSON Schema-constrained output with `ResponseSchema`:

```go
req := &model.LLMRequest{
    Contents: []*genai.Content{
        genai.NewContentFromText("List three colours.", "user"),
    },
    Config: &genai.GenerateContentConfig{
        ResponseSchema: &genai.Schema{
            Type: genai.TypeObject,
            Properties: map[string]*genai.Schema{
                "colours": {
                    Type:  genai.TypeArray,
                    Items: &genai.Schema{Type: genai.TypeString},
                },
            },
            Required: []string{"colours"},
        },
    },
}
```

This sends `response_format: { type: "json_schema", json_schema: { ... } }` to
the API. For unstructured JSON, set `ResponseMIMEType: "application/json"`
instead (sends `{ type: "json_object" }`).

### Custom Base URL (e.g. Ollama)

```go
m, err := openai.New(openai.Config{
    Model:   "llama3",
    BaseURL: "http://localhost:11434/v1",
    // No APIKey needed for local servers
})
```

## Error Handling

- `New` returns an error when `Config.Model` is empty.
- `GenerateContent` yields an error for HTTP failures, non-2xx status codes
  (including the response body), JSON marshal/unmarshal failures, and SSE parse
  errors.
- Context cancellation during streaming yields a response with `Interrupted: true`.

### Finish Reason Mapping

| OpenAI | genai |
|--------|-------|
| `stop` | `FinishReasonStop` |
| `length` | `FinishReasonMaxTokens` |
| `tool_calls` | `FinishReasonStop` |
| `content_filter` | `FinishReasonSafety` |
| `""` | `FinishReasonUnspecified` |
| other | `FinishReasonOther` |
