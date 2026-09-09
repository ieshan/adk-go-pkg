# OpenAI Model Provider

Package `model/openai` provides a drop-in `model.LLM` adapter for any
OpenAI-compatible API. It supports two endpoint flavours:

- **Chat Completions** (`/v1/chat/completions`) — the default, compatible with
  most OpenAI-compatible servers.
- **Responses** (`/v1/responses`) — opt-in via `Config.API`, supporting
  reasoning items, flat tool declarations, and typed SSE streaming events.

## Overview

The adapter translates between ADK-Go's `model.LLMRequest` / `model.LLMResponse`
types and the OpenAI wire format. Because it targets the OpenAI protocol (not a
vendor SDK), it works with any server that speaks that protocol.

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

    // API selects the OpenAI endpoint flavour. Defaults to
    // APIChatCompletions when empty, preserving current behaviour.
    API API
}
```

The `API` type has two constants:

```go
const (
    APIChatCompletions API = "chat/completions" // default
    APIResponses       API = "responses"
)
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
`TurnComplete: true`. Finish-reason chunks have `Partial: false` and arrive
before the `[DONE]` marker, which has `TurnComplete: true`.

> **Responses API note:** The Responses path uses typed semantic SSE events
> (e.g. `response.output_text.delta`, `response.completed`) with no `[DONE]`
> sentinel. Reasoning deltas (`response.reasoning_text.delta`,
> `response.reasoning_summary_text.delta`) yield `Partial: true` responses with
> `Thought: true` on the Part. The final response is built from the terminal
> `response.completed` / `response.incomplete` event.

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

### Tool Results

When submitting tool results via `FunctionResponse`, the following constraints apply:

- `ID` must be non-empty (matches the tool call ID)
- Exactly one `Part` is expected
- `InlineData` with `MIMEType: "application/json"` containing the tool result as JSON
- The `Response` field is ignored — only `InlineData` parts are used

> **Responses API note:** The Responses path uses `FunctionResponse.Response`
> (the `map[string]any` field) for function responses, not `InlineData` parts.
> Users who construct `FunctionResponse` with only `Parts` (no `Response` map)
> will get `"null"` as the output. The ADK has the same limitation.

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

This sends `response_format: { type: "json_schema", json_schema: { name: "response", strict: true, schema: {...} } }` to
the API. The `name` and `strict` fields are set automatically by the model adapter.
For unstructured JSON, set `ResponseMIMEType: "application/json"`
instead (sends `{ type: "json_object" }`).

> **Responses API note:** The Responses path uses `text.format` (not
> `response_format`). The schema name comes from `ResponseSchema.Title` (or
> `"response"` when empty). `enforceStrictOpenAISchema` is applied to the
> response schema to comply with OpenAI's strict mode requirements
> (`additionalProperties: false`, all properties in `required`).

### Images

The OpenAI model supports image inputs via:

- `InlineData`: base64-encoded data URL (e.g. `data:image/png;base64,...`)
- `FileData`: URI is sent as `image_url` in the OpenAI API request

> **Responses API note:** The Responses path (v1) does not support images.
> `InlineData` and `FileData` parts return `ErrUnsupportedContentPart`. A
> follow-up PR can add `input_image` items.

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
  errors. The response body is always closed via `defer` — even when reading
  the error body fails, the connection is properly released.
- Context cancellation during streaming yields a response with `Interrupted: true`.

### Finish Reason Mapping

#### Chat Completions

| OpenAI | genai |
|--------|-------|
| `stop` | `FinishReasonStop` |
| `length` | `FinishReasonMaxTokens` |
| `tool_calls` | `FinishReasonStop` |
| `content_filter` | `FinishReasonSafety` |
| `""` | `FinishReasonUnspecified` |
| other | `FinishReasonOther` |

#### Responses

The Responses path maps `incomplete_details.reason` from the response object:

| `incomplete_details.reason` | genai |
|--------|-------|
| `""` / nil (completed normally) | `FinishReasonStop` |
| `max_output_tokens` | `FinishReasonMaxTokens` |
| `content_filter` | `FinishReasonSafety` |
| other | `FinishReasonOther` |

Note: `response.failed` is a streaming event (not a status field on the
response object). It is handled in the stream translator (yields an error),
not in the finish reason mapping. For non-streaming, a failed response would
have empty output and yield `ErrNoTextOrToolContent`.

## Responses API

The Responses API (`/v1/responses`) is an alternative endpoint that supports
reasoning models, flat tool declarations, and typed SSE streaming. Select it
with `Config.API`:

```go
m, err := openai.New(openai.Config{
    Model:  "o3",
    APIKey: os.Getenv("OPENAI_API_KEY"),
    API:    openai.APIResponses,
})
```

### Request Mapping

| `LLMRequest` / Config field | Responses request field | Notes |
|---|---|---|
| `Config.SystemInstruction` | `instructions` | via `extractText` |
| `Contents` | `input` array of typed items | message, function_call, function_call_output |
| `Config.Temperature` | `temperature` | |
| `Config.TopP` | `top_p` | |
| `Config.MaxOutputTokens` | `max_output_tokens` | renamed from `max_tokens` |
| `Config.StopSequences` | **error** | `ErrStopSequencesNotSupported` |
| `Config.TopK` | **error** | `ErrTopKNotSupported` |
| `Config.CandidateCount > 1` | **error** | `ErrMultipleCandidatesNotSupported` |
| `Config.FrequencyPenalty` / `PresencePenalty` | **error** | `ErrPenaltiesNotSupported` |
| `Config.Labels` | **error** | `ErrLabelsNotSupported` |
| `Config.SafetySettings` | **error** | `ErrSafetySettingsNotSupported` |
| `Config.Tools` | `tools` (flat format) | `{"type":"function","name":...}` (not nested) |
| `Config.ToolConfig` | `tool_choice` | supports `allowed_tools` mode |
| `Config.ResponseSchema` | `text.format = {type:"json_schema",name,strict:true,schema}` | name from `Title` or `"response"` |
| `Config.ResponseJsonSchema` | `text.format = {type:"json_schema",name,strict:true,schema}` | `normalizeSchema` + `enforceStrictOpenAISchema` |
| `Config.ResponseMIMEType == "application/json"` (no schema) | `text.format = {type:"json_object"}` | |
| `Config.ThinkingConfig` | not mapped | v1 scope (matching ADK) |

### Content → Input Items

- User/system/developer text → `{type:"message", role, content:[{type:"input_text", text}]}`
- Assistant text → `{type:"message", role:"assistant", content:[{type:"output_text", text}]}` (Responses API rejects `input_text` for assistant role)
- `FunctionCall` Part → `{type:"function_call", call_id, name, arguments}` (arguments is JSON string)
- `FunctionResponse` Part → `{type:"function_call_output", call_id, output}` (output is JSON string from `fr.Response` map)
- `InlineData`/`FileData` → `ErrUnsupportedContentPart` (v1 scope)
- Empty/whitespace-only text parts are skipped

### Call ID Tracking

When `FunctionCall.ID` is empty, a deterministic ID is generated:
`adk-openai-call-N` (counter resets per request). The `callTracker` matches
`FunctionResponse` to pending `FunctionCall` by ID. If `FunctionResponse.ID`
is empty, the oldest pending call is used (FIFO).

### Tool Choice

| `ToolConfig.Mode` | Allowed names | `tool_choice` |
|---|---|---|
| AUTO | none | omitted (API decides) |
| AUTO | yes | `{type:"allowed_tools", mode:"auto", tools:[...]}` |
| NONE | — | `"none"` |
| ANY | none | `"required"` |
| ANY | yes | `{type:"allowed_tools", mode:"required", tools:[...]}` |

### Response Mapping

| Output item `type` | genai Part |
|---|---|
| `message` with `content[].type=="output_text"` | `&genai.Part{Text: content.Text}` |
| `message` with `content[].type=="refusal"` | `&genai.Part{Text: content.Refusal}` |
| `function_call` | `&genai.Part{FunctionCall: {ID: call_id, Name, Args: json.Unmarshal(arguments)}}` |
| `reasoning` with `content[].text` | `&genai.Part{Text: chunk.Text, Thought: true}` |
| unknown `type` | `ErrUnsupportedOutputItemType` |

### Usage Mapping

| Responses field | genai UsageMetadata |
|---|---|
| `input_tokens` | `PromptTokenCount` |
| `output_tokens` | `CandidatesTokenCount` |
| `total_tokens` | `TotalTokenCount` |
| `input_tokens_details.cached_tokens` | `CachedContentTokenCount` |
| `output_tokens_details.reasoning_tokens` | `ThoughtsTokenCount` |

### Metadata

Each response includes `CustomMetadata["openai_response_id"]` and
`CustomMetadata["openai_model"]`.

### `store` Default

The Responses API stores responses server-side by default (`store: true`). We
do not set `store` (matching the ADK). For OpenAI-compatible servers that don't
implement storage, this is a no-op. For OpenAI itself, responses are stored
server-side. Users who need stateless operation can set `store: false` via a
future Config field.

### `enforceStrictOpenAISchema`

For structured output with `strict: true`, the response schema is modified to
comply with OpenAI's strict mode requirements:

- `additionalProperties: false` on all object types
- All properties listed in `required` array (sorted alphabetically)
- Handles `$ref`, `$defs`, `anyOf`/`oneOf`/`allOf`, `items`, nested objects

This is applied **only** to the response `text.format` schema, not to tool
parameter schemas.
