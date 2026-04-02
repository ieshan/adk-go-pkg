// Package openai provides adapters and helpers for integrating ADK-Go agents
// with OpenAI-compatible APIs.
//
// The primary entry point is [New], which constructs a [model.LLM] that speaks
// the OpenAI Chat Completions API protocol. Any server that is compatible with
// that protocol (e.g. local LLM servers, Azure OpenAI, Together AI, etc.) can
// be targeted by setting [Config.BaseURL].
//
// # Basic usage
//
//	m, err := openai.New(openai.Config{
//	    Model:  "gpt-4o",
//	    APIKey: os.Getenv("OPENAI_API_KEY"),
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	req := &model.LLMRequest{
//	    Contents: []*genai.Content{
//	        {Role: "user", Parts: []*genai.Part{{Text: "Hello!"}}},
//	    },
//	}
//
//	for resp, err := range m.GenerateContent(ctx, req, false) {
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    fmt.Println(resp.Content.Parts[0].Text)
//	}
//
// # Streaming
//
// Pass stream=true to [model.LLM.GenerateContent] to enable server-sent-event
// streaming. Each partial delta is yielded with Partial=true; the final sentinel
// is yielded with TurnComplete=true.
//
//	for resp, err := range m.GenerateContent(ctx, req, true) {
//	    if err != nil { break }
//	    if resp.TurnComplete { break }
//	    if resp.Partial {
//	        fmt.Print(resp.Content.Parts[0].Text)
//	    }
//	}
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"

	"google.golang.org/adk/model"
)

const defaultBaseURL = "https://api.openai.com/v1"

// Config holds the parameters needed to create an OpenAI-compatible LLM client.
//
// Only Model is required. All other fields have sensible defaults:
//   - BaseURL defaults to "https://api.openai.com/v1".
//   - HTTPClient defaults to [http.DefaultClient].
//   - APIKey may be empty when the target server does not require authentication
//     (e.g. a local inference server).
type Config struct {
	// Model is the model identifier to request, e.g. "gpt-4o" or "gpt-4o-mini".
	// This field is required; [New] returns an error if it is empty.
	Model string

	// APIKey is sent as "Authorization: Bearer <APIKey>" on every request.
	// Leave empty only when the target server does not require authentication.
	APIKey string

	// BaseURL is the base URL of the OpenAI-compatible API server, without a
	// trailing slash. Defaults to "https://api.openai.com/v1" when empty.
	BaseURL string

	// HTTPClient is the HTTP client used to send requests. When nil,
	// [http.DefaultClient] is used.
	HTTPClient *http.Client

	// Headers contains additional HTTP headers that are sent with every request.
	// These are applied after the standard Content-Type and Authorization headers,
	// so they can be used to override those values if needed.
	Headers map[string]string
}

// openaiModel is the unexported concrete type that implements [model.LLM].
type openaiModel struct {
	model   string
	apiKey  string
	baseURL string
	client  *http.Client
	headers map[string]string
}

// New creates a [model.LLM] that communicates with an OpenAI-compatible Chat
// Completions API.
//
// The returned LLM is safe for concurrent use. Config fields are copied at
// construction time; subsequent mutations to the Config do not affect the model.
//
// New returns an error when cfg.Model is empty, because every request to the
// Chat Completions endpoint requires a model identifier.
//
// Example — pointing at a local server:
//
//	m, err := openai.New(openai.Config{
//	    Model:   "llama3",
//	    BaseURL: "http://localhost:11434/v1",
//	})
func New(cfg Config) (model.LLM, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("openai: Config.Model must not be empty")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	// Copy headers defensively so external mutations don't affect the model.
	var hdrs map[string]string
	if len(cfg.Headers) > 0 {
		hdrs = make(map[string]string, len(cfg.Headers))
		for k, v := range cfg.Headers {
			hdrs[k] = v
		}
	}

	return &openaiModel{
		model:   cfg.Model,
		apiKey:  cfg.APIKey,
		baseURL: baseURL,
		client:  client,
		headers: hdrs,
	}, nil
}

// Name returns the model identifier that was specified at construction time.
// It satisfies the [model.LLM] interface.
func (m *openaiModel) Name() string {
	return m.model
}

// GenerateContent sends the request to the OpenAI Chat Completions endpoint and
// yields [model.LLMResponse] values. It satisfies the [model.LLM] interface.
//
// When stream is false, the endpoint is called without streaming; the response
// body is read in full, unmarshalled as a [chatResponse], and translated using
// [translateResponse]. A single response is yielded with TurnComplete set to true.
//
// When stream is true, the response body is treated as an SSE stream and
// forwarded to [parseStream]; each SSE event produces one yield. The caller
// must consume the full iterator or cancel ctx to release resources.
//
// Non-2xx HTTP status codes cause a single error to be yielded and the iterator
// stops.
func (m *openaiModel) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		// 1. Build the chat request.
		chatReq, err := buildChatRequest(req, m.model, stream)
		if err != nil {
			yield(nil, fmt.Errorf("openai: build request: %w", err))
			return
		}

		// 2. Marshal to JSON.
		body, err := json.Marshal(chatReq)
		if err != nil {
			yield(nil, fmt.Errorf("openai: marshal request: %w", err))
			return
		}

		// 3. Create HTTP POST to {baseURL}/chat/completions.
		url := m.baseURL + "/chat/completions"
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			yield(nil, fmt.Errorf("openai: create HTTP request: %w", err))
			return
		}

		// 4. Set standard and custom headers.
		httpReq.Header.Set("Content-Type", "application/json")
		if m.apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+m.apiKey)
		}
		for k, v := range m.headers {
			httpReq.Header.Set(k, v)
		}

		// 5. Execute the request.
		resp, err := m.client.Do(httpReq)
		if err != nil {
			yield(nil, fmt.Errorf("openai: HTTP request: %w", err))
			return
		}
		defer func() { _ = resp.Body.Close() }()

		// 6. Check status code.
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			errBody, _ := io.ReadAll(resp.Body)
			yield(nil, fmt.Errorf("openai: HTTP %d: %s", resp.StatusCode, string(errBody)))
			return
		}

		// 7. Streaming path — delegate to parseStream.
		if stream {
			for r, e := range parseStream(ctx, resp.Body) {
				if !yield(r, e) {
					return
				}
			}
			return
		}

		// 8. Non-streaming path — read body, unmarshal, translate, yield once.
		rawBody, err := io.ReadAll(resp.Body)
		if err != nil {
			yield(nil, fmt.Errorf("openai: read response body: %w", err))
			return
		}

		var chatResp chatResponse
		if err := json.Unmarshal(rawBody, &chatResp); err != nil {
			yield(nil, fmt.Errorf("openai: unmarshal response: %w", err))
			return
		}

		llmResp := translateResponse(&chatResp)
		llmResp.TurnComplete = true
		yield(llmResp, nil)
	}
}
