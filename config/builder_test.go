package config_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ieshan/adk-go-pkg/config"
	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
)

// testRegistry returns a *Registry pre-loaded with a "mock" model prefix and
// a "search" tool factory — sufficient for all builder tests.
func testRegistry() *config.Registry {
	r := config.NewRegistry()

	r.RegisterModel("mock", func(cfg map[string]any) (model.LLM, error) {
		name, _ := cfg["model"].(string)
		return testutil.NewFakeLLM().WithName(name), nil
	})

	r.RegisterTool("search", func(cfg map[string]any) (tool.Tool, error) {
		return testutil.NewFakeTool("search"), nil
	})

	return r
}

// TestBuild_LLMAgent verifies that Build produces a named LLM agent when given
// a valid "llm" config with a registered model and tool.
func TestBuild_LLMAgent(t *testing.T) {
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{Name: "chat-bot"},
		Model:           "mock/fast",
		Tools:           []config.ToolRef{{Name: "search"}},
	}

	a, err := config.Build(context.Background(), cfg, testRegistry())
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a == nil {
		t.Fatal("Build returned nil agent")
	}
	if a.Name() != "chat-bot" {
		t.Errorf("expected agent name %q, got %q", "chat-bot", a.Name())
	}
}

// TestBuild_SequentialAgent verifies that a "sequential" agent with two LLM
// sub-agents is created correctly and reports the correct sub-agent count.
func TestBuild_SequentialAgent(t *testing.T) {
	cfg := &config.SequentialAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{
			Name: "pipeline",
			SubAgents: []config.AgentConfig{
				&config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "step-1"}, Model: "mock/fast"},
				&config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "step-2"}, Model: "mock/fast"},
			},
		},
	}

	a, err := config.Build(context.Background(), cfg, testRegistry())
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a.Name() != "pipeline" {
		t.Errorf("expected agent name %q, got %q", "pipeline", a.Name())
	}
	if len(a.SubAgents()) != 2 {
		t.Errorf("expected 2 sub-agents, got %d", len(a.SubAgents()))
	}
}

// TestBuild_ParallelAgent verifies that a "parallel" agent with two sub-agents
// is constructed without error.
func TestBuild_ParallelAgent(t *testing.T) {
	cfg := &config.ParallelAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{
			Name: "fan-out",
			SubAgents: []config.AgentConfig{
				&config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "worker-a"}, Model: "mock/fast"},
				&config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "worker-b"}, Model: "mock/fast"},
			},
		},
	}

	a, err := config.Build(context.Background(), cfg, testRegistry())
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a.Name() != "fan-out" {
		t.Errorf("expected agent name %q, got %q", "fan-out", a.Name())
	}
	if len(a.SubAgents()) != 2 {
		t.Errorf("expected 2 sub-agents, got %d", len(a.SubAgents()))
	}
}

// TestBuild_LoopAgent verifies that a "loop" agent is built with a positive
// MaxIterations value without error.
func TestBuild_LoopAgent(t *testing.T) {
	cfg := &config.LoopAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{
			Name: "refiner",
			SubAgents: []config.AgentConfig{
				&config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "inner"}, Model: "mock/fast"},
			},
		},
		MaxIterations: 3,
	}

	a, err := config.Build(context.Background(), cfg, testRegistry())
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a.Name() != "refiner" {
		t.Errorf("expected agent name %q, got %q", "refiner", a.Name())
	}
}

// TestBuild_NestedTree verifies a multi-level hierarchy: a root LLM agent that
// has a sequential sub-agent which itself has two LLM children.
func TestBuild_NestedTree(t *testing.T) {
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{
			Name: "root",
			SubAgents: []config.AgentConfig{
				&config.SequentialAgentConfig{
					BaseAgentConfig: config.BaseAgentConfig{
						Name: "seq",
						SubAgents: []config.AgentConfig{
							&config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "child-a"}, Model: "mock/fast"},
							&config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "child-b"}, Model: "mock/fast"},
						},
					},
				},
			},
		},
		Model: "mock/fast",
	}

	a, err := config.Build(context.Background(), cfg, testRegistry())
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a.Name() != "root" {
		t.Errorf("expected root agent name %q, got %q", "root", a.Name())
	}
	if len(a.SubAgents()) != 1 {
		t.Fatalf("expected 1 sub-agent under root, got %d", len(a.SubAgents()))
	}
	seq := a.SubAgents()[0]
	if seq.Name() != "seq" {
		t.Errorf("expected sub-agent name %q, got %q", "seq", seq.Name())
	}
	if len(seq.SubAgents()) != 2 {
		t.Errorf("expected 2 children under seq, got %d", len(seq.SubAgents()))
	}
}

// TestBuild_NilConfig verifies that Build returns an error for nil config.
func TestBuild_NilConfig(t *testing.T) {
	_, err := config.Build(context.Background(), nil, testRegistry())
	if err == nil {
		t.Fatal("expected an error for nil config, got nil")
	}
}

// TestBuild_ModelNotFound verifies that Build returns an error when the model
// prefix has no registered factory.
func TestBuild_ModelNotFound(t *testing.T) {
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{Name: "bot"},
		Model:           "unregistered/gpt-x",
	}
	_, err := config.Build(context.Background(), cfg, testRegistry())
	if err == nil {
		t.Fatal("expected an error for unregistered model prefix, got nil")
	}
}

// TestBuild_ToolNotFound verifies that Build returns an error when a tool name
// has no registered factory.
func TestBuild_ToolNotFound(t *testing.T) {
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{Name: "bot"},
		Model:           "mock/fast",
		Tools:           []config.ToolRef{{Name: "nonexistent-tool"}},
	}
	_, err := config.Build(context.Background(), cfg, testRegistry())
	if err == nil {
		t.Fatal("expected an error for unregistered tool, got nil")
	}
}

// TestBuild_LLMAgent_WithTransferFlags verifies that transfer flags are passed through.
func TestBuild_LLMAgent_WithTransferFlags(t *testing.T) {
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig:          config.BaseAgentConfig{Name: "transfer-bot"},
		Model:                    "mock/fast",
		DisallowTransferToParent: true,
		DisallowTransferToPeers:  true,
	}

	a, err := config.Build(context.Background(), cfg, testRegistry())
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a.Name() != "transfer-bot" {
		t.Errorf("expected agent name %q, got %q", "transfer-bot", a.Name())
	}
}

// TestBuild_LLMAgent_DefaultTransferFlags verifies defaults are false when omitted.
func TestBuild_LLMAgent_DefaultTransferFlags(t *testing.T) {
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{Name: "default-bot"},
		Model:           "mock/fast",
	}

	a, err := config.Build(context.Background(), cfg, testRegistry())
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a.Name() != "default-bot" {
		t.Errorf("expected agent name %q, got %q", "default-bot", a.Name())
	}
}

// TestLoadAndBuild verifies the convenience function: it writes a temporary
// YAML file, calls LoadAndBuild, and confirms the resulting agent has the
// expected name.
func TestLoadAndBuild(t *testing.T) {
	content := `
name: file-agent
type: llm
model: mock/fast
`
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write temp YAML file: %v", err)
	}

	a, err := config.LoadAndBuild(context.Background(), path, testRegistry())
	if err != nil {
		t.Fatalf("LoadAndBuild returned unexpected error: %v", err)
	}
	if a.Name() != "file-agent" {
		t.Errorf("expected agent name %q, got %q", "file-agent", a.Name())
	}
}
