package anthropic

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

const (
	defaultBaseURL    = "https://api.anthropic.com"
	defaultAPIVersion = "2023-06-01"
)

// Config holds the parameters needed to create an Anthropic-compatible LLM
// client.
//
// Only Model is required. All other fields have sensible defaults:
//   - BaseURL defaults to "https://api.anthropic.com".
//   - APIVersion defaults to "2023-06-01".
//   - AuthScheme defaults to "x-api-key".
//   - HTTPClient defaults to [http.DefaultClient].
type Config struct {
	// Model is the Anthropic model identifier to request, e.g.
	// "claude-opus-4" or "claude-sonnet-4".
	// This field is required; [New] returns an error if it is empty.
	Model string

	// APIKey is the authentication token. When AuthScheme is "x-api-key"
	// (the default) it is sent as the "x-api-key" header. When AuthScheme is
	// "bearer" it is sent as "Authorization: Bearer <APIKey>".
	APIKey string

	// BaseURL is the base URL of the Anthropic-compatible API server, without
	// a trailing slash. Defaults to "https://api.anthropic.com" when empty.
	BaseURL string

	// APIVersion is the Anthropic API version sent as the "anthropic-version"
	// header. Defaults to "2023-06-01" when empty. Some third-party providers
	// (e.g. AWS Bedrock) require a different version such as
	// "bedrock-2023-05-31".
	APIVersion string

	// AuthScheme selects how APIKey is transmitted.
	//   - "x-api-key" (default) → sends "x-api-key: <APIKey>".
	//   - "bearer"              → sends "Authorization: Bearer <APIKey>".
	AuthScheme string

	// HTTPClient is the HTTP client used to send requests. When nil,
	// [http.DefaultClient] is used.
	HTTPClient *http.Client

	// Headers contains additional HTTP headers sent with every request. These
	// are applied after standard headers, so they can be used to override
	// values if needed. Common use cases include tenant routing
	// ("X-Tenant-ID"), organisation IDs ("X-Org-ID"), provider-specific
	// tokens ("x-goog-api-client"), or beta feature flags
	// ("anthropic-beta: computer-use-2024-10-22").
	Headers map[string]string

	// CacheControl enables top-level automatic prompt caching on every
	// request. Anthropic places the cache breakpoint on the last cacheable
	// block automatically. Example:
	//
	//	map[string]any{"type": "ephemeral"}
	CacheControl map[string]any
}

// anthropicModel is the unexported concrete type that implements [model.LLM].
type anthropicModel struct {
	model        string
	apiKey       string
	baseURL      string
	apiVersion   string
	authScheme   string
	client       *http.Client
	headers      map[string]string
	cacheControl map[string]any
}

// New creates a [model.LLM] that communicates with an Anthropic-compatible
// Messages API.
//
// The returned LLM is safe for concurrent use. Config fields are copied at
// construction time; subsequent mutations to the Config do not affect the model.
//
// New returns an error when cfg.Model is empty, because every request to the
// Messages endpoint requires a model identifier.
func New(cfg Config) (model.LLM, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("anthropic: Config.Model must not be empty")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	apiVersion := cfg.APIVersion
	if apiVersion == "" {
		apiVersion = defaultAPIVersion
	}

	authScheme := cfg.AuthScheme
	if authScheme == "" {
		authScheme = "x-api-key"
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

	// Copy cache control defensively.
	var cc map[string]any
	if len(cfg.CacheControl) > 0 {
		cc = make(map[string]any, len(cfg.CacheControl))
		for k, v := range cfg.CacheControl {
			cc[k] = v
		}
	}

	return &anthropicModel{
		model:        cfg.Model,
		apiKey:       cfg.APIKey,
		baseURL:      baseURL,
		apiVersion:   apiVersion,
		authScheme:   authScheme,
		client:       client,
		headers:      hdrs,
		cacheControl: cc,
	}, nil
}

// Name returns the model identifier that was specified at construction time.
// It satisfies the [model.LLM] interface.
func (m *anthropicModel) Name() string {
	return m.model
}

// GenerateContent sends the request to the Anthropic Messages endpoint and
// yields [model.LLMResponse] values. It satisfies the [model.LLM] interface.
//
// When stream is false, the endpoint is called without streaming; the response
// body is read in full, unmarshalled as a [messageResponse], and translated
// using [translateResponse]. A single response is yielded with TurnComplete set
// to true.
//
// When stream is true, the response body is treated as an SSE stream and
// forwarded to [parseStream]; each SSE event produces one yield. The caller
// must consume the full iterator or cancel ctx to release resources.
//
// Non-2xx HTTP status codes cause a single error to be yielded and the
// iterator stops.
func (m *anthropicModel) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		// 1. Build the message request.
		msgReq, err := buildMessageRequest(req, m.model, stream, m.cacheControl)
		if err != nil {
			yield(nil, fmt.Errorf("anthropic: build request: %w", err))
			return
		}

		// 2. Marshal to JSON.
		body, err := json.Marshal(msgReq)
		if err != nil {
			yield(nil, fmt.Errorf("anthropic: marshal request: %w", err))
			return
		}

		// 3. Create HTTP POST to {baseURL}/v1/messages.
		url := m.baseURL + "/v1/messages"
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			yield(nil, fmt.Errorf("anthropic: create HTTP request: %w", err))
			return
		}

		// 4. Set standard and custom headers.
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("anthropic-version", m.apiVersion)

		if m.apiKey != "" {
			if m.authScheme == "bearer" {
				httpReq.Header.Set("Authorization", "Bearer "+m.apiKey)
			} else {
				httpReq.Header.Set("x-api-key", m.apiKey)
			}
		}

		for k, v := range m.headers {
			httpReq.Header.Set(k, v)
		}

		// 5. Execute the request.
		resp, err := m.client.Do(httpReq)
		if err != nil {
			yield(nil, fmt.Errorf("anthropic: HTTP request: %w", err))
			return
		}
		defer func() { _ = resp.Body.Close() }()

		// 6. Check status code.
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			errBody, _ := io.ReadAll(resp.Body)
			yield(nil, fmt.Errorf("anthropic: HTTP %d: %s", resp.StatusCode, string(errBody)))
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
			yield(nil, fmt.Errorf("anthropic: read response body: %w", err))
			return
		}

		var msgResp messageResponse
		if err = json.Unmarshal(rawBody, &msgResp); err != nil {
			yield(nil, fmt.Errorf("anthropic: unmarshal response: %w", err))
			return
		}

		llmResp := translateResponse(&msgResp)
		llmResp.TurnComplete = true
		yield(llmResp, nil)
	}
}

// Compile-time interface check.
var _ model.LLM = (*anthropicModel)(nil)
