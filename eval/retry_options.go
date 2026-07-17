package eval

import (
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

// AddDefaultRetryOptionsIfNotPresent adds default HTTP retry options to an
// LLM request if they are not already present.
//
// The Python ADK sets HttpRetryOptions on the GenerateContentConfig to
// protect eval runs from transient model provider outages. The Go genai
// SDK does not currently expose HttpRetryOptions on GenerateContentConfig,
// so this function ensures the config is initialized but cannot set retry
// parameters. When the Go SDK adds retry configuration support, this
// function should be updated to set the same defaults as the Python version.
func AddDefaultRetryOptionsIfNotPresent(req *model.LLMRequest) {
	if req == nil {
		return
	}
	if req.Config == nil {
		req.Config = &genai.GenerateContentConfig{}
	}
}

// EnsureRetryOptionsPlugin is a model plugin that adds default retry options
// to LLM requests before they are sent.
type EnsureRetryOptionsPlugin struct{}

// BeforeModelCallback adds default retry options if not present.
func (p *EnsureRetryOptionsPlugin) BeforeModelCallback(ctx agent.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
	AddDefaultRetryOptionsIfNotPresent(req)
	return nil, nil
}

// AfterModelCallback is a no-op for this plugin.
func (p *EnsureRetryOptionsPlugin) AfterModelCallback(ctx agent.Context, resp *model.LLMResponse, respErr error) (*model.LLMResponse, error) {
	return resp, respErr
}

// Compile-time interface checks.
var _ llmagent.BeforeModelCallback = (*EnsureRetryOptionsPlugin)(nil).BeforeModelCallback
var _ llmagent.AfterModelCallback = (*EnsureRetryOptionsPlugin)(nil).AfterModelCallback
