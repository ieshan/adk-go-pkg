# Anthropic Model Provider

Package `model/anthropic` provides a drop-in `model.LLM` adapter for Anthropic's
Messages API and any compatible third-party provider.

## Overview

The adapter translates between ADK-Go's `model.LLMRequest` / `model.LLMResponse`
types and the Anthropic `/v1/messages` wire format. Because it targets the
protocol directly (not a vendor SDK), it works with any server that exposes an
Anthropic-compatible endpoint.

### Supported Providers

| Provider | BaseURL | Auth | API Version |
|----------|---------|------|-------------|
| **Anthropic Direct** | `https://api.anthropic.com` (default) | `x-api-key` | `2023-06-01` |
| **LiteLLM Gateway** | Custom | `x-api-key` or `Bearer` | `2023-06-01` |
| **AWS Bedrock** | Custom | `Bearer` or SigV4 | `bedrock-2023-05-31` |
| **Google Vertex AI** | Custom | GCP `Bearer` | Provider-specific |
| **Corporate Gateway** | Custom | `x-api-key` or `Bearer` | Provider-specific |

Any other server that implements the Anthropic Messages API will also work.

## API Reference

### Config

```go
type Config struct {
    // Model identifier (e.g. "claude-opus-4", "claude-sonnet-4"). Required.
    Model string

    // API key. Sent as "x-api-key" by default, or "Authorization: Bearer"
    // when AuthScheme is "bearer".
    APIKey string

    // Base URL without trailing slash. Defaults to "https://api.anthropic.com".
    BaseURL string

    // API version sent as the "anthropic-version" header.
    // Defaults to "2023-06-01".
    APIVersion string

    // Auth scheme: "x-api-key" (default) or "bearer".
    AuthScheme string

    // HTTP client. Defaults to http.DefaultClient.
    HTTPClient *http.Client

    // Additional headers sent with every request.
    Headers map[string]string

    // Top-level automatic prompt caching config.
    // Example: map[string]any{"type": "ephemeral"}
    CacheControl map[string]any
}
```

### New

```go
func New(cfg Config) (model.LLM, error)
```

Creates a `model.LLM` that communicates with the configured endpoint. Returns
an error when `cfg.Model` is empty. Config fields are copied defensively at
construction time; the returned LLM is safe for concurrent use.

## Examples

### Basic Completion

```go
m, err := anthropic.New(anthropic.Config{
    Model:  "claude-opus-4",
    APIKey: os.Getenv("ANTHROPIC_API_KEY"),
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

Each partial response has `Partial: true`. The final `message_stop` event carries
`TurnComplete: true`. Unlike OpenAI's SSE protocol, Anthropic has no `[DONE]`
sentinel; the stream ends naturally at `message_stop`.

### Tool Calling

Tools declared in `model.LLMRequest` are automatically translated to Anthropic's
`tools` array with `input_schema`. When the model calls a tool, the response
carries `FunctionCall` parts:

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

### Images

Inline images (`InlineData`) are base64-encoded and sent as `source.type:
"base64"`. HTTP/HTTPS URLs in `FileData` are sent as `source.type: "url"`:

```go
req := &model.LLMRequest{
    Contents: []*genai.Content{
        {
            Role: "user",
            Parts: []*genai.Part{
                genai.NewPartFromText("What is in this image?"),
                {InlineData: &genai.Blob{
                    MIMEType: "image/png",
                    Data:     imageBytes,
                }},
                {FileData: &genai.FileData{
                    FileURI:  "https://example.com/image.jpg",
                    MIMEType: "image/jpeg",
                }},
            },
        },
    },
}
```

Non-HTTP `FileData` URIs (e.g. `gs://`) return an error.

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

This sends `output_config: { format: "json", json_schema: { ... } }` to the
API. For unstructured JSON, set `ResponseMIMEType: "application/json"` instead
(sends `output_config: { format: "json" }`).

You can also pass a pre-built schema map via `ResponseJsonSchema`.

### Thinking Blocks

Enable thinking with `ThinkingConfig`:

```go
budget := int32(2048)
req := &model.LLMRequest{
    Contents: []*genai.Content{
        genai.NewContentFromText("Solve this step by step.", "user"),
    },
    Config: &genai.GenerateContentConfig{
        ThinkingConfig: &genai.ThinkingConfig{
            IncludeThoughts: true,
            ThinkingBudget:  &budget,
        },
    },
}
```

Thinking blocks in the response have `Part.Thought = true` and
`Part.ThoughtSignature` set. The signature must be preserved and echoed back in
subsequent requests for multi-turn continuity.

### Prompt Caching

**Automatic caching** (Anthropic manages the breakpoint):

```go
m, err := anthropic.New(anthropic.Config{
    Model:        "claude-opus-4",
    APIKey:       os.Getenv("ANTHROPIC_API_KEY"),
    CacheControl: map[string]any{"type": "ephemeral"},
})
```

**Explicit block-level caching** via `PartMetadata`:

```go
part := genai.NewPartFromText("Large document context...")
part.PartMetadata = map[string]any{
    "cache_control": map[string]any{"type": "ephemeral"},
}
```

Cache usage in responses is available in `LLMResponse.CustomMetadata`:

```go
if resp.CustomMetadata != nil {
    creation := resp.CustomMetadata["cache_creation_input_tokens"]
    read := resp.CustomMetadata["cache_read_input_tokens"]
}
```

### Third-Party Providers

**LiteLLM gateway with Bearer auth:**

```go
m, err := anthropic.New(anthropic.Config{
    Model:      "claude-opus-4",
    APIKey:     "gateway-token",
    BaseURL:    "https://gateway.example.com",
    AuthScheme: "bearer",
})
```

**AWS Bedrock:**

```go
m, err := anthropic.New(anthropic.Config{
    Model:      "claude-opus-4",
    APIKey:     "bedrock-api-key",
    BaseURL:    "https://bedrock-runtime.us-east-1.amazonaws.com",
    APIVersion: "bedrock-2023-05-31",
})
```

**Custom headers (tenant routing, beta features):**

```go
m, err := anthropic.New(anthropic.Config{
    Model:  "claude-opus-4",
    APIKey: os.Getenv("ANTHROPIC_API_KEY"),
    Headers: map[string]string{
        "X-Tenant-ID":    "tenant-42",
        "anthropic-beta": "computer-use-2024-10-22",
    },
})
```

## Error Handling

- `New` returns an error when `Config.Model` is empty.
- `GenerateContent` yields an error for HTTP failures, non-2xx status codes
  (including the response body), JSON marshal/unmarshal failures, and SSE parse
  errors.
- Context cancellation during streaming yields a response with `Interrupted: true`.

## Important Design Notes

### Temperature, TopP, and TopK are Omitted

Anthropic Claude Opus 4.7+ and newer models reject `temperature`, `top_p`, and
`top_k` with HTTP 400 errors. The adapter **never forwards them**, regardless of
what the user sets in `GenerateContentConfig`. Use prompting style guidance
instead.

### Max Tokens Default

Anthropic requires `max_tokens` on every request. If `MaxOutputTokens` is zero,
the adapter defaults to `4096`.

### System Instruction Format

Unlike OpenAI (which uses a `system` role message), Anthropic uses a top-level
`system` field. The adapter extracts text from `SystemInstruction` parts and
places it at the request root automatically.

### Tool Result Content

`FunctionResponse.Response` (a `map[string]any`) is serialized to a JSON string
for the `tool_result.content` field. When `FunctionResponse.Parts` contains
`InlineData`, each part becomes a content block inside the `tool_result` array.

## Finish Reason Mapping

| Anthropic | genai |
|-----------|-------|
| `end_turn` | `FinishReasonStop` |
| `max_tokens` | `FinishReasonMaxTokens` |
| `stop_sequence` | `FinishReasonStop` |
| `tool_use` | `FinishReasonStop` |
| `refusal` | `FinishReasonSafety` |
| `""` | `FinishReasonUnspecified` |
| other | `FinishReasonOther` |
