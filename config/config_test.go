package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ieshan/adk-go-pkg/config"
)

// TestLoad_JSON verifies that Load correctly reads and parses a JSON config file.
func TestLoad_JSON(t *testing.T) {
	cfg := config.AgentConfig{
		Name:        "my-agent",
		Type:        "llm",
		Model:       "gemini/gemini-2.0-flash",
		Instruction: "You are helpful.",
		Description: "A helpful assistant",
		Tools: []config.ToolRef{
			{Name: "search", Config: map[string]any{"timeout": float64(30)}},
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	f, err := os.CreateTemp(t.TempDir(), "agent-*.json")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = f.Close()

	got, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Name != cfg.Name {
		t.Errorf("Name: got %q, want %q", got.Name, cfg.Name)
	}
	if got.Type != cfg.Type {
		t.Errorf("Type: got %q, want %q", got.Type, cfg.Type)
	}
	if got.Model != cfg.Model {
		t.Errorf("Model: got %q, want %q", got.Model, cfg.Model)
	}
	if got.Instruction != cfg.Instruction {
		t.Errorf("Instruction: got %q, want %q", got.Instruction, cfg.Instruction)
	}
	if got.Description != cfg.Description {
		t.Errorf("Description: got %q, want %q", got.Description, cfg.Description)
	}
	if len(got.Tools) != 1 || got.Tools[0].Name != "search" {
		t.Errorf("Tools: got %v, want [{search ...}]", got.Tools)
	}
}

// TestLoad_YAML verifies that Load correctly reads and parses a YAML config file.
func TestLoad_YAML(t *testing.T) {
	yamlData := []byte(`
name: yaml-agent
type: sequential
model: openai/gpt-4o
instruction: "Be concise."
description: "A concise agent"
tools:
  - name: calculator
    config:
      precision: 2
subAgents:
  - name: sub1
    type: llm
    model: gemini/gemini-pro
`)

	f, err := os.CreateTemp(t.TempDir(), "agent-*.yaml")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if _, err := f.Write(yamlData); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = f.Close()

	got, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Name != "yaml-agent" {
		t.Errorf("Name: got %q, want %q", got.Name, "yaml-agent")
	}
	if got.Type != "sequential" {
		t.Errorf("Type: got %q, want %q", got.Type, "sequential")
	}
	if got.Model != "openai/gpt-4o" {
		t.Errorf("Model: got %q, want %q", got.Model, "openai/gpt-4o")
	}
	if len(got.Tools) != 1 || got.Tools[0].Name != "calculator" {
		t.Errorf("Tools: got %v", got.Tools)
	}
	if len(got.SubAgents) != 1 || got.SubAgents[0].Name != "sub1" {
		t.Errorf("SubAgents: got %v", got.SubAgents)
	}
}

// TestLoad_YML verifies that Load accepts .yml extension as well.
func TestLoad_YML(t *testing.T) {
	yamlData := []byte("name: yml-agent\ntype: llm\n")

	f, err := os.CreateTemp(t.TempDir(), "agent-*.yml")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if _, err := f.Write(yamlData); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = f.Close()

	got, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Name != "yml-agent" {
		t.Errorf("Name: got %q, want %q", got.Name, "yml-agent")
	}
}

// TestParse_JSON verifies Parse with explicit "json" format.
func TestParse_JSON(t *testing.T) {
	data := []byte(`{"name":"parse-json","type":"llm","model":"gemini/gemini-pro","maxIterations":5}`)

	got, err := config.Parse(data, "json")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Name != "parse-json" {
		t.Errorf("Name: got %q, want %q", got.Name, "parse-json")
	}
	if got.MaxIterations != 5 {
		t.Errorf("MaxIterations: got %d, want 5", got.MaxIterations)
	}
}

// TestParse_YAML verifies Parse with explicit "yaml" format.
func TestParse_YAML(t *testing.T) {
	data := []byte("name: parse-yaml\ntype: loop\nmaxIterations: 10\n")

	got, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Name != "parse-yaml" {
		t.Errorf("Name: got %q, want %q", got.Name, "parse-yaml")
	}
	if got.MaxIterations != 10 {
		t.Errorf("MaxIterations: got %d, want 10", got.MaxIterations)
	}
}

// TestLoad_UnknownExtension verifies that Load returns an error for unsupported extensions.
func TestLoad_UnknownExtension(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "agent-*.txt")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	_, _ = f.Write([]byte("name: oops"))
	_ = f.Close()

	_, err = config.Load(f.Name())
	if err == nil {
		t.Fatal("expected error for .txt extension, got nil")
	}
}

// TestLoad_MissingFile verifies that Load returns an error when the file doesn't exist.
func TestLoad_MissingFile(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// TestParse_UnknownFormat verifies that Parse returns an error for unknown format strings.
func TestParse_UnknownFormat(t *testing.T) {
	_, err := config.Parse([]byte("{}"), "toml")
	if err == nil {
		t.Fatal("expected error for unknown format, got nil")
	}
}

// TestAgentConfig_SubAgents verifies that nested sub-agents are parsed correctly.
func TestAgentConfig_SubAgents(t *testing.T) {
	data := []byte(`
name: root
type: sequential
subAgents:
  - name: child1
    type: llm
    model: gemini/gemini-pro
    subAgents:
      - name: grandchild
        type: llm
        model: openai/gpt-4o
  - name: child2
    type: llm
    model: gemini/gemini-flash
`)

	got, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Name != "root" {
		t.Errorf("Name: got %q, want root", got.Name)
	}
	if len(got.SubAgents) != 2 {
		t.Fatalf("SubAgents len: got %d, want 2", len(got.SubAgents))
	}
	child1 := got.SubAgents[0]
	if child1.Name != "child1" {
		t.Errorf("child1 Name: got %q, want child1", child1.Name)
	}
	if len(child1.SubAgents) != 1 || child1.SubAgents[0].Name != "grandchild" {
		t.Errorf("grandchild: got %v", child1.SubAgents)
	}
	if got.SubAgents[1].Name != "child2" {
		t.Errorf("child2 Name: got %q, want child2", got.SubAgents[1].Name)
	}
}

// TestTranslateGenerateConfig verifies that common config keys are mapped to the
// correct fields in genai.GenerateContentConfig.
func TestTranslateGenerateConfig(t *testing.T) {
	m := map[string]any{
		"temperature":     0.7,
		"topP":            0.9,
		"topK":            float64(40),
		"maxOutputTokens": float64(512),
		"stopSequences":   []any{"STOP", "END"},
	}

	gc, err := config.TranslateGenerateConfig(m)
	if err != nil {
		t.Fatalf("TranslateGenerateConfig: %v", err)
	}
	if gc == nil {
		t.Fatal("got nil GenerateContentConfig")
	}
	if gc.Temperature == nil || *gc.Temperature != float32(0.7) {
		t.Errorf("Temperature: got %v, want 0.7", gc.Temperature)
	}
	if gc.TopP == nil || *gc.TopP != float32(0.9) {
		t.Errorf("TopP: got %v, want 0.9", gc.TopP)
	}
	if gc.TopK == nil || *gc.TopK != float32(40) {
		t.Errorf("TopK: got %v, want 40", gc.TopK)
	}
	if gc.MaxOutputTokens != 512 {
		t.Errorf("MaxOutputTokens: got %d, want 512", gc.MaxOutputTokens)
	}
	if len(gc.StopSequences) != 2 || gc.StopSequences[0] != "STOP" || gc.StopSequences[1] != "END" {
		t.Errorf("StopSequences: got %v, want [STOP END]", gc.StopSequences)
	}
}

// TestTranslateGenerateConfig_Empty verifies that a nil map returns a non-nil config.
func TestTranslateGenerateConfig_Empty(t *testing.T) {
	gc, err := config.TranslateGenerateConfig(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gc == nil {
		t.Fatal("got nil GenerateContentConfig for empty input")
	}
}

// TestTranslateGenerateConfig_CandidateCount verifies candidateCount is translated.
func TestTranslateGenerateConfig_CandidateCount(t *testing.T) {
	m := map[string]any{
		"candidateCount": float64(3),
	}
	gc, err := config.TranslateGenerateConfig(m)
	if err != nil {
		t.Fatalf("TranslateGenerateConfig: %v", err)
	}
	if gc.CandidateCount != 3 {
		t.Errorf("CandidateCount: got %d, want 3", gc.CandidateCount)
	}
}
