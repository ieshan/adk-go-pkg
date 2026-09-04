package eval

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
)

// requestIntercepterStateKey is the state key for the request intercepter UUID.
const requestIntercepterStateKey = "__request_intercepter_id__"

// RequestIntercepterPlugin captures LLM requests for AppDetails generation.
// It maintains a cache of requests keyed by UUID, allowing retrieval of the
// original request that produced a given response.
//
// It implements llmagent.BeforeModelCallback and llmagent.AfterModelCallback.
type RequestIntercepterPlugin struct {
	mu       sync.RWMutex
	requests map[string]*model.LLMRequest
}

// NewRequestIntercepterPlugin creates a new RequestIntercepterPlugin.
func NewRequestIntercepterPlugin() *RequestIntercepterPlugin {
	return &RequestIntercepterPlugin{
		requests: make(map[string]*model.LLMRequest),
	}
}

// BeforeModelCallback stores the LLM request with a UUID and puts the UUID
// into the agent context state for later retrieval.
func (p *RequestIntercepterPlugin) BeforeModelCallback(ctx agent.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
	if req == nil {
		return nil, nil
	}

	id := uuid.NewString()
	p.mu.Lock()
	p.requests[id] = req
	p.mu.Unlock()

	if state := ctx.State(); state != nil {
		if err := state.Set(requestIntercepterStateKey, id); err != nil {
			return nil, fmt.Errorf("request intercepter: set state key: %w", err)
		}
	}

	return nil, nil
}

// AfterModelCallback puts the UUID into the LLM response's CustomMetadata
// so the original request can be retrieved later via GetModelRequest.
func (p *RequestIntercepterPlugin) AfterModelCallback(ctx agent.Context, resp *model.LLMResponse, respErr error) (*model.LLMResponse, error) {
	if resp == nil {
		return resp, respErr
	}

	var idVal any
	if state := ctx.State(); state != nil {
		idVal, _ = state.Get(requestIntercepterStateKey)
	}
	id, _ := idVal.(string)
	if id == "" {
		return resp, respErr
	}

	if resp.CustomMetadata == nil {
		resp.CustomMetadata = make(map[string]any)
	}
	resp.CustomMetadata["request_intercepter_id"] = id

	return resp, respErr
}

// GetModelRequest retrieves the cached LLM request for the given response.
func (p *RequestIntercepterPlugin) GetModelRequest(resp *model.LLMResponse) *model.LLMRequest {
	if resp == nil || resp.CustomMetadata == nil {
		return nil
	}

	id, ok := resp.CustomMetadata["request_intercepter_id"].(string)
	if !ok || id == "" {
		return nil
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.requests[id]
}

// Clear removes all cached requests.
func (p *RequestIntercepterPlugin) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = make(map[string]*model.LLMRequest)
}

// Compile-time interface checks.
var _ llmagent.BeforeModelCallback = (*RequestIntercepterPlugin)(nil).BeforeModelCallback
var _ llmagent.AfterModelCallback = (*RequestIntercepterPlugin)(nil).AfterModelCallback

// EnsureRequestIntercepterPlugin combines RequestIntercepterPlugin with retry
// options. It ensures that every LLM request has default retry options applied
// and is captured for AppDetails generation.
type EnsureRequestIntercepterPlugin struct {
	*RequestIntercepterPlugin
}

// NewEnsureRequestIntercepterPlugin creates a new EnsureRequestIntercepterPlugin.
func NewEnsureRequestIntercepterPlugin() *EnsureRequestIntercepterPlugin {
	return &EnsureRequestIntercepterPlugin{
		RequestIntercepterPlugin: NewRequestIntercepterPlugin(),
	}
}

// BeforeModelCallback applies retry options and captures the request.
func (p *EnsureRequestIntercepterPlugin) BeforeModelCallback(ctx agent.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
	AddDefaultRetryOptionsIfNotPresent(req)
	return p.RequestIntercepterPlugin.BeforeModelCallback(ctx, req)
}

// AfterModelCallback delegates to the embedded RequestIntercepterPlugin.
func (p *EnsureRequestIntercepterPlugin) AfterModelCallback(ctx agent.Context, resp *model.LLMResponse, respErr error) (*model.LLMResponse, error) {
	return p.RequestIntercepterPlugin.AfterModelCallback(ctx, resp, respErr)
}

// Compile-time interface checks.
var _ llmagent.BeforeModelCallback = (*EnsureRequestIntercepterPlugin)(nil).BeforeModelCallback
var _ llmagent.AfterModelCallback = (*EnsureRequestIntercepterPlugin)(nil).AfterModelCallback
