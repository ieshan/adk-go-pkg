package config_test

import (
	"context"
	"iter"

	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
)

// stubLLM is a minimal model.LLM implementation shared across config tests.
type stubLLM struct{ name string }

func (s *stubLLM) Name() string { return s.name }
func (s *stubLLM) GenerateContent(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {}
}

// stubTool is a minimal tool.Tool implementation shared across config tests.
type stubTool struct{ name string }

func (s *stubTool) Name() string        { return s.name }
func (s *stubTool) Description() string { return "stub tool" }
func (s *stubTool) IsLongRunning() bool { return false }

// Compile-time interface checks.
var _ model.LLM = (*stubLLM)(nil)
var _ tool.Tool = (*stubTool)(nil)
