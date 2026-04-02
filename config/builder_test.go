package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ieshan/adk-go-pkg/config"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
)

// testRegistry returns a *Registry pre-loaded with a "mock" model prefix and
// a "search" tool factory — sufficient for all builder tests.
func testRegistry() *config.Registry {
	r := config.NewRegistry()

	r.RegisterModel("mock", func(cfg map[string]any) (model.LLM, error) {
		name, _ := cfg["model"].(string)
		return &stubLLM{name: name}, nil
	})

	r.RegisterTool("search", func(cfg map[string]any) (tool.Tool, error) {
		return &stubTool{name: "search"}, nil
	})

	return r
}

// TestBuild_LLMAgent verifies that Build produces a named LLM agent when given
// a valid "llm" config with a registered model and tool.
func TestBuild_LLMAgent(t *testing.T) {
	cfg := &config.AgentConfig{
		Name:  "chat-bot",
		Type:  "llm",
		Model: "mock/fast",
		Tools: []config.ToolRef{{Name: "search"}},
	}

	a, err := config.Build(cfg, testRegistry())
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
	cfg := &config.AgentConfig{
		Name: "pipeline",
		Type: "sequential",
		SubAgents: []config.AgentConfig{
			{Name: "step-1", Type: "llm", Model: "mock/fast"},
			{Name: "step-2", Type: "llm", Model: "mock/fast"},
		},
	}

	a, err := config.Build(cfg, testRegistry())
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
	cfg := &config.AgentConfig{
		Name: "fan-out",
		Type: "parallel",
		SubAgents: []config.AgentConfig{
			{Name: "worker-a", Type: "llm", Model: "mock/fast"},
			{Name: "worker-b", Type: "llm", Model: "mock/fast"},
		},
	}

	a, err := config.Build(cfg, testRegistry())
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
	cfg := &config.AgentConfig{
		Name:          "refiner",
		Type:          "loop",
		MaxIterations: 3,
		SubAgents: []config.AgentConfig{
			{Name: "inner", Type: "llm", Model: "mock/fast"},
		},
	}

	a, err := config.Build(cfg, testRegistry())
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
	cfg := &config.AgentConfig{
		Name:  "root",
		Type:  "llm",
		Model: "mock/fast",
		SubAgents: []config.AgentConfig{
			{
				Name: "seq",
				Type: "sequential",
				SubAgents: []config.AgentConfig{
					{Name: "child-a", Type: "llm", Model: "mock/fast"},
					{Name: "child-b", Type: "llm", Model: "mock/fast"},
				},
			},
		},
	}

	a, err := config.Build(cfg, testRegistry())
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

// TestBuild_UnknownType verifies that Build returns a descriptive error when
// the agent type is not recognised.
func TestBuild_UnknownType(t *testing.T) {
	cfg := &config.AgentConfig{
		Name: "weird",
		Type: "unknown",
	}
	_, err := config.Build(cfg, testRegistry())
	if err == nil {
		t.Fatal("expected an error for unknown agent type, got nil")
	}
}

// TestBuild_ModelNotFound verifies that Build returns an error when the model
// prefix has no registered factory.
func TestBuild_ModelNotFound(t *testing.T) {
	cfg := &config.AgentConfig{
		Name:  "bot",
		Type:  "llm",
		Model: "unregistered/gpt-x",
	}
	_, err := config.Build(cfg, testRegistry())
	if err == nil {
		t.Fatal("expected an error for unregistered model prefix, got nil")
	}
}

// TestBuild_ToolNotFound verifies that Build returns an error when a tool name
// has no registered factory.
func TestBuild_ToolNotFound(t *testing.T) {
	cfg := &config.AgentConfig{
		Name:  "bot",
		Type:  "llm",
		Model: "mock/fast",
		Tools: []config.ToolRef{{Name: "nonexistent-tool"}},
	}
	_, err := config.Build(cfg, testRegistry())
	if err == nil {
		t.Fatal("expected an error for unregistered tool, got nil")
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

	a, err := config.LoadAndBuild(path, testRegistry())
	if err != nil {
		t.Fatalf("LoadAndBuild returned unexpected error: %v", err)
	}
	if a.Name() != "file-agent" {
		t.Errorf("expected agent name %q, got %q", "file-agent", a.Name())
	}
}
