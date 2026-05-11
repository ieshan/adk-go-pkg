package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ieshan/adk-go-pkg/config"
)

// TestLoad_JSON verifies that Load correctly reads and parses a JSON config file.
func TestLoad_JSON(t *testing.T) {
	data := []byte(`{
		"name": "my-agent",
		"type": "llm",
		"model": "gemini/gemini-2.0-flash",
		"instruction": "You are helpful.",
		"description": "A helpful assistant",
		"tools": [
			{"name": "search", "config": {"timeout": 30}}
		]
	}`)

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
	if got.Name() != "my-agent" {
		t.Errorf("Name: got %q, want %q", got.Name(), "my-agent")
	}
	if got.Type() != "llm" {
		t.Errorf("Type: got %q, want %q", got.Type(), "llm")
	}

	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("expected *LLMAgentConfig, got %T", got)
	}
	if llmCfg.Model != "gemini/gemini-2.0-flash" {
		t.Errorf("Model: got %q, want %q", llmCfg.Model, "gemini/gemini-2.0-flash")
	}
	if llmCfg.Instruction != "You are helpful." {
		t.Errorf("Instruction: got %q, want %q", llmCfg.Instruction, "You are helpful.")
	}
	if llmCfg.BaseAgentConfig.Description != "A helpful assistant" {
		t.Errorf("Description: got %q, want %q", llmCfg.BaseAgentConfig.Description, "A helpful assistant")
	}
	if len(llmCfg.Tools) != 1 || llmCfg.Tools[0].Name != "search" {
		t.Errorf("Tools: got %v, want [{search ...}]", llmCfg.Tools)
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

	_, err := config.Parse(yamlData, "yaml")
	if err == nil {
		t.Fatal("expected error for sequential agent with LLM fields, got nil")
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
	if got.Name() != "yml-agent" {
		t.Errorf("Name: got %q, want %q", got.Name(), "yml-agent")
	}
	if got.Type() != "llm" {
		t.Errorf("Type: got %q, want %q", got.Type(), "llm")
	}
}

// TestParse_UnknownType verifies that Parse returns an error for unknown agent types.
func TestParse_UnknownType(t *testing.T) {
	data := []byte("name: unknown-agent\ntype: unknown\n")
	_, err := config.Parse(data, "yaml")
	if err == nil {
		t.Fatal("expected error for unknown type, got nil")
	}
}

// TestParse_JSON verifies Parse with explicit "json" format.
func TestParse_JSON(t *testing.T) {
	data := []byte(`{"name":"parse-json","type":"llm","model":"gemini/gemini-pro"}`)

	got, err := config.Parse(data, "json")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Name() != "parse-json" {
		t.Errorf("Name: got %q, want %q", got.Name(), "parse-json")
	}

	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("expected *LLMAgentConfig, got %T", got)
	}
	if llmCfg.Model != "gemini/gemini-pro" {
		t.Errorf("Model: got %q, want gemini/gemini-pro", llmCfg.Model)
	}
}

// TestParse_YAML verifies Parse with explicit "yaml" format.
func TestParse_YAML(t *testing.T) {
	data := []byte("name: parse-yaml\ntype: loop\nmaxIterations: 10\n")

	got, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Name() != "parse-yaml" {
		t.Errorf("Name: got %q, want %q", got.Name(), "parse-yaml")
	}

	loopCfg, ok := got.(*config.LoopAgentConfig)
	if !ok {
		t.Fatalf("expected *LoopAgentConfig, got %T", got)
	}
	if loopCfg.MaxIterations != 10 {
		t.Errorf("MaxIterations: got %d, want 10", loopCfg.MaxIterations)
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
	if got.Name() != "root" {
		t.Errorf("Name: got %q, want root", got.Name())
	}
	if got.Type() != "sequential" {
		t.Errorf("Type: got %q, want sequential", got.Type())
	}
	if len(got.SubAgents()) != 2 {
		t.Fatalf("SubAgents len: got %d, want 2", len(got.SubAgents()))
	}
	child1 := got.SubAgents()[0]
	if child1.Name() != "child1" {
		t.Errorf("child1 Name: got %q, want child1", child1.Name())
	}
	if len(child1.SubAgents()) != 1 || child1.SubAgents()[0].Name() != "grandchild" {
		t.Errorf("grandchild: got %v", child1.SubAgents())
	}
	if got.SubAgents()[1].Name() != "child2" {
		t.Errorf("child2 Name: got %q, want child2", got.SubAgents()[1].Name())
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

// TestSkillsetRef_ParseYAML verifies YAML parsing of SkillsetRef with all fields.
func TestSkillsetRef_ParseYAML(t *testing.T) {
	yamlData := []byte(`
name: skills-agent
type: llm
skillsets:
  - name: filesystem
    config:
      path: "./skills"
    preload: complete
    names:
      - "weather"
      - "cooking"
    systemInstruction: "Use these skills for domain tasks"
`)

	got, err := config.Parse(yamlData, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("expected *LLMAgentConfig, got %T", got)
	}
	if len(llmCfg.Skillsets) != 1 {
		t.Fatalf("Skillsets: got %d, want 1", len(llmCfg.Skillsets))
	}

	s := llmCfg.Skillsets[0]
	if s.Name != "filesystem" {
		t.Errorf("Name: got %q, want filesystem", s.Name)
	}
	if s.Preload != "complete" {
		t.Errorf("Preload: got %q, want complete", s.Preload)
	}
	if s.SystemInstruction != "Use these skills for domain tasks" {
		t.Errorf("SystemInstruction: got %q, want 'Use these skills for domain tasks'", s.SystemInstruction)
	}
	if len(s.Names) != 2 || s.Names[0] != "weather" || s.Names[1] != "cooking" {
		t.Errorf("Names: got %v, want [weather cooking]", s.Names)
	}

	path, ok := s.Config["path"].(string)
	if !ok || path != "./skills" {
		t.Errorf("Config[path]: got %v, want ./skills", s.Config["path"])
	}
}

// TestSkillsetRef_ParseJSON verifies JSON parsing of SkillsetRef.
func TestSkillsetRef_ParseJSON(t *testing.T) {
	jsonData := []byte(`{
		"name": "agent",
		"type": "llm",
		"skillsets": [
			{
				"name": "filesystem",
				"config": {"path": "/app/skills"},
				"preload": "frontmatters",
				"names": ["search", "calculator"]
			}
		]
	}`)

	got, err := config.Parse(jsonData, "json")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("expected *LLMAgentConfig, got %T", got)
	}
	if len(llmCfg.Skillsets) != 1 {
		t.Fatalf("Skillsets: got %d, want 1", len(llmCfg.Skillsets))
	}

	s := llmCfg.Skillsets[0]
	if s.Name != "filesystem" {
		t.Errorf("Name: got %q, want filesystem", s.Name)
	}
	if s.Preload != "frontmatters" {
		t.Errorf("Preload: got %q, want frontmatters", s.Preload)
	}
	if len(s.Names) != 2 || s.Names[0] != "search" || s.Names[1] != "calculator" {
		t.Errorf("Names: got %v, want [search calculator]", s.Names)
	}
}

// TestSkillsetRef_Parse_Minimal verifies parsing with only required Name field.
func TestSkillsetRef_Parse_Minimal(t *testing.T) {
	yamlData := []byte(`
name: agent
type: llm
skillsets:
  - name: filesystem
    config:
      path: "./skills"
`)

	got, err := config.Parse(yamlData, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("expected *LLMAgentConfig, got %T", got)
	}
	if len(llmCfg.Skillsets) != 1 {
		t.Fatalf("Skillsets: got %d, want 1", len(llmCfg.Skillsets))
	}

	s := llmCfg.Skillsets[0]
	if s.Name != "filesystem" {
		t.Errorf("Name: got %q, want filesystem", s.Name)
	}
	if s.Preload != "" {
		t.Errorf("Preload: got %q, want empty", s.Preload)
	}
	if s.SystemInstruction != "" {
		t.Errorf("SystemInstruction: got %q, want empty", s.SystemInstruction)
	}
	if len(s.Names) != 0 {
		t.Errorf("Names: got %v, want empty", s.Names)
	}
}

// TestSkillsetRef_Parse_WithNames verifies parsing of Names field for specific skill loading.
func TestSkillsetRef_Parse_WithNames(t *testing.T) {
	yamlData := []byte(`
name: agent
type: llm
skillsets:
  - name: filesystem
    config:
      path: "./skills"
    names:
      - weather
      - cooking
`)

	got, err := config.Parse(yamlData, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("expected *LLMAgentConfig, got %T", got)
	}
	if len(llmCfg.Skillsets) != 1 {
		t.Fatalf("Skillsets: got %d, want 1", len(llmCfg.Skillsets))
	}

	s := llmCfg.Skillsets[0]
	if len(s.Names) != 2 {
		t.Errorf("Names len: got %d, want 2", len(s.Names))
	}
	if s.Names[0] != "weather" {
		t.Errorf("Names[0]: got %q, want weather", s.Names[0])
	}
	if s.Names[1] != "cooking" {
		t.Errorf("Names[1]: got %q, want cooking", s.Names[1])
	}
}

// TestAgentConfig_Parse_WithSkillsets verifies AgentConfig parsing including skillsets.
func TestAgentConfig_Parse_WithSkillsets(t *testing.T) {
	yamlData := []byte(`
name: agent-with-skills
type: llm
model: gemini/gemini-2.5-flash
skillsets:
  - name: filesystem
    config:
      path: "./local-skills"
    preload: complete
  - name: gcs
    config:
      bucket: "my-bucket"
      prefix: "skills/"
    preload: frontmatters
    names:
      - shared-skill
`)

	got, err := config.Parse(yamlData, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got.Name() != "agent-with-skills" {
		t.Errorf("Name: got %q, want agent-with-skills", got.Name())
	}

	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("expected *LLMAgentConfig, got %T", got)
	}
	if len(llmCfg.Skillsets) != 2 {
		t.Fatalf("Skillsets: got %d, want 2", len(llmCfg.Skillsets))
	}

	// Check first skillset
	s1 := llmCfg.Skillsets[0]
	if s1.Name != "filesystem" {
		t.Errorf("Skillset[0].Name: got %q, want filesystem", s1.Name)
	}
	if s1.Preload != "complete" {
		t.Errorf("Skillset[0].Preload: got %q, want complete", s1.Preload)
	}

	// Check second skillset
	s2 := llmCfg.Skillsets[1]
	if s2.Name != "gcs" {
		t.Errorf("Skillset[1].Name: got %q, want gcs", s2.Name)
	}
	if s2.Preload != "frontmatters" {
		t.Errorf("Skillset[1].Preload: got %q, want frontmatters", s2.Preload)
	}
	if len(s2.Names) != 1 || s2.Names[0] != "shared-skill" {
		t.Errorf("Skillset[1].Names: got %v, want [shared-skill]", s2.Names)
	}
}

// TestParse_LLMAgent_WithTransferFlags_JSON verifies JSON parsing of transfer flags.
func TestParse_LLMAgent_WithTransferFlags_JSON(t *testing.T) {
	data := []byte(`{"name":"transfer-agent","type":"llm","model":"gemini/gemini-pro","disallowTransferToParent":true,"disallowTransferToPeers":true}`)

	got, err := config.Parse(data, "json")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("expected *LLMAgentConfig, got %T", got)
	}
	if !llmCfg.DisallowTransferToParent {
		t.Errorf("DisallowTransferToParent: got false, want true")
	}
	if !llmCfg.DisallowTransferToPeers {
		t.Errorf("DisallowTransferToPeers: got false, want true")
	}
}

// TestParse_LLMAgent_WithTransferFlags_YAML verifies YAML parsing of transfer flags.
func TestParse_LLMAgent_WithTransferFlags_YAML(t *testing.T) {
	data := []byte(`
name: transfer-agent
type: llm
model: gemini/gemini-pro
disallowTransferToParent: true
disallowTransferToPeers: true
`)

	got, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("expected *LLMAgentConfig, got %T", got)
	}
	if !llmCfg.DisallowTransferToParent {
		t.Errorf("DisallowTransferToParent: got false, want true")
	}
	if !llmCfg.DisallowTransferToPeers {
		t.Errorf("DisallowTransferToPeers: got false, want true")
	}
}

// TestParse_LLMAgent_WithMaxIterations_ReturnsError verifies that maxIterations on LLM agent errors.
func TestParse_LLMAgent_WithMaxIterations_ReturnsError(t *testing.T) {
	data := []byte(`
name: bad-llm
type: llm
model: gemini/gemini-pro
maxIterations: 5
`)
	_, err := config.Parse(data, "yaml")
	if err == nil {
		t.Fatal("expected error for maxIterations on llm agent, got nil")
	}
}

// TestParse_SequentialWithModel_ReturnsError verifies that model on sequential agent errors.
func TestParse_SequentialWithModel_ReturnsError(t *testing.T) {
	data := []byte(`
name: bad-seq
type: sequential
model: openai/gpt-4o
`)
	_, err := config.Parse(data, "yaml")
	if err == nil {
		t.Fatal("expected error for model on sequential agent, got nil")
	}
}

// TestParse_SequentialWithTransferFlags_ReturnsError verifies that transfer flags on sequential agent errors.
func TestParse_SequentialWithTransferFlags_ReturnsError(t *testing.T) {
	data := []byte(`
name: bad-seq
type: sequential
disallowTransferToParent: true
`)
	_, err := config.Parse(data, "yaml")
	if err == nil {
		t.Fatal("expected error for disallowTransferToParent on sequential agent, got nil")
	}
}

// TestParse_ParallelWithTools_ReturnsError verifies that tools on parallel agent errors.
func TestParse_ParallelWithTools_ReturnsError(t *testing.T) {
	data := []byte(`
name: bad-parallel
type: parallel
tools:
  - name: search
`)
	_, err := config.Parse(data, "yaml")
	if err == nil {
		t.Fatal("expected error for tools on parallel agent, got nil")
	}
}

// TestParse_LoopWithModel_ReturnsError verifies that model on loop agent errors.
func TestParse_LoopWithModel_ReturnsError(t *testing.T) {
	data := []byte(`
name: bad-loop
type: loop
model: openai/gpt-4o
`)
	_, err := config.Parse(data, "yaml")
	if err == nil {
		t.Fatal("expected error for model on loop agent, got nil")
	}
}

// TestParse_LoopWithTools_ReturnsError verifies that tools on loop agent errors.
func TestParse_LoopWithTools_ReturnsError(t *testing.T) {
	data := []byte(`
name: bad-loop
type: loop
tools:
  - name: search
`)
	_, err := config.Parse(data, "yaml")
	if err == nil {
		t.Fatal("expected error for tools on loop agent, got nil")
	}
}
