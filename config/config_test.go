package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ieshan/adk-go-pkg/config"
	"google.golang.org/genai"
)

// TestLoad_JSON verifies that Load correctly reads and parses a JSON config file.
func TestLoad_JSON(t *testing.T) {
	data := []byte(`{
		"name": "my-agent",
		"agent_class": "LlmAgent",
		"model": "gemini/gemini-2.0-flash",
		"instruction": "You are helpful.",
		"description": "A helpful assistant",
		"tools": [
			{"name": "search", "args": {"timeout": 30}}
		]
	}`)

	dir := t.TempDir()
	path := filepath.Join(dir, "agent.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	appCfg, err := config.Load(root, "agent.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := appCfg.AgentConfig
	if got.Name() != "my-agent" {
		t.Errorf("Name: got %q, want %q", got.Name(), "my-agent")
	}
	if got.Type() != "llm" {
		t.Errorf("Type: got %q, want %q", got.Type(), "llm")
	}

	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("got %T, want *LLMAgentConfig", got)
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

// TestParse_SequentialAgentWithLLMFields verifies that Parse returns an error
// when a SequentialAgent contains LLM-only fields like model, instruction, etc.
func TestParse_SequentialAgentWithLLMFields(t *testing.T) {
	t.Parallel()
	yamlData := []byte(`
name: yaml-agent
agent_class: SequentialAgent
model: openai/gpt-4o
instruction: "Be concise."
description: "A concise agent"
tools:
  - name: calculator
    args:
      precision: 2
sub_agents:
  - name: sub1
    agent_class: LlmAgent
    model: gemini/gemini-pro
`)

	_, err := config.Parse(yamlData, "yaml")
	if err == nil {
		t.Fatal("got nil, want error for sequential agent with LLM fields")
	}
}

// TestLoad_YML verifies that Load accepts .yml extension as well.
func TestLoad_YML(t *testing.T) {
	yamlData := []byte("name: yml-agent\nagent_class: LlmAgent\n")

	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yml")
	if err := os.WriteFile(path, yamlData, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	appCfg, err := config.Load(root, "agent.yml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := appCfg.AgentConfig
	if got.Name() != "yml-agent" {
		t.Errorf("Name: got %q, want %q", got.Name(), "yml-agent")
	}
	if got.Type() != "llm" {
		t.Errorf("Type: got %q, want %q", got.Type(), "llm")
	}
}

// TestParse_UnknownType verifies that Parse returns an error for unknown agent types.
func TestParse_UnknownType(t *testing.T) {
	t.Parallel()
	data := []byte("name: unknown-agent\nagent_class: UnknownAgent\n")
	_, err := config.Parse(data, "yaml")
	if err == nil {
		t.Fatal("got nil, want error for unknown type")
	}
}

// TestParse_JSON verifies Parse with explicit "json" format.
func TestParse_JSON(t *testing.T) {
	t.Parallel()
	data := []byte(`{"name":"parse-json","agent_class":"LlmAgent","model":"gemini/gemini-pro"}`)

	appCfg, err := config.Parse(data, "json")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := appCfg.AgentConfig
	if got.Name() != "parse-json" {
		t.Errorf("Name: got %q, want %q", got.Name(), "parse-json")
	}

	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("got %T, want *LLMAgentConfig", got)
	}
	if llmCfg.Model != "gemini/gemini-pro" {
		t.Errorf("Model: got %q, want gemini/gemini-pro", llmCfg.Model)
	}
}

// TestParse_YAML verifies Parse with explicit "yaml" format.
func TestParse_YAML(t *testing.T) {
	t.Parallel()
	data := []byte("name: parse-yaml\nagent_class: LoopAgent\nmax_iterations: 10\n")

	appCfg, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := appCfg.AgentConfig
	if got.Name() != "parse-yaml" {
		t.Errorf("Name: got %q, want %q", got.Name(), "parse-yaml")
	}

	loopCfg, ok := got.(*config.LoopAgentConfig)
	if !ok {
		t.Fatalf("got %T, want *LoopAgentConfig", got)
	}
	if loopCfg.MaxIterations != 10 {
		t.Errorf("MaxIterations: got %d, want 10", loopCfg.MaxIterations)
	}
}

// TestLoad_UnknownExtension verifies that Load returns an error for unsupported extensions.
func TestLoad_UnknownExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.txt")
	if err := os.WriteFile(path, []byte("name: oops"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	_, err = config.Load(root, "agent.txt")
	if err == nil {
		t.Fatal("got nil, want error for .txt extension")
	}
}

// TestLoad_MissingFile verifies that Load returns an error when the file doesn't exist.
func TestLoad_MissingFile(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	_, err = config.Load(root, "nonexistent.json")
	if err == nil {
		t.Fatal("got nil, want error for missing file")
	}
}

// TestParse_UnknownFormat verifies that Parse returns an error for unknown format strings.
func TestParse_UnknownFormat(t *testing.T) {
	t.Parallel()
	_, err := config.Parse([]byte("{}"), "toml")
	if err == nil {
		t.Fatal("got nil, want error for unknown format")
	}
}

// TestAgentConfig_SubAgents verifies that nested sub-agents are parsed correctly.
func TestAgentConfig_SubAgents(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: root
agent_class: SequentialAgent
sub_agents:
  - name: child1
    agent_class: LlmAgent
    model: gemini/gemini-pro
    sub_agents:
      - name: grandchild
        agent_class: LlmAgent
        model: openai/gpt-4o
  - name: child2
    agent_class: LlmAgent
    model: gemini/gemini-flash
`)

	appCfg, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := appCfg.AgentConfig
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

// TestTranslateGenerateConfig_AllScalars verifies that every scalar field is
// mapped correctly.
func TestTranslateGenerateConfig_AllScalars(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		key   string
		val   any
		check func(*genai.GenerateContentConfig) bool
	}{
		{"temperature", "temperature", 0.7, func(g *genai.GenerateContentConfig) bool {
			return g.Temperature != nil && *g.Temperature == float32(0.7)
		}},
		{"topP", "topP", 0.9, func(g *genai.GenerateContentConfig) bool { return g.TopP != nil && *g.TopP == float32(0.9) }},
		{"topK", "topK", float64(40), func(g *genai.GenerateContentConfig) bool { return g.TopK != nil && *g.TopK == float32(40) }},
		{"maxOutputTokens", "maxOutputTokens", float64(512), func(g *genai.GenerateContentConfig) bool { return g.MaxOutputTokens == 512 }},
		{"candidateCount", "candidateCount", float64(3), func(g *genai.GenerateContentConfig) bool { return g.CandidateCount == 3 }},
		{"responseLogprobs", "responseLogprobs", true, func(g *genai.GenerateContentConfig) bool { return g.ResponseLogprobs == true }},
		{"logprobs", "logprobs", float64(5), func(g *genai.GenerateContentConfig) bool { return g.Logprobs != nil && *g.Logprobs == 5 }},
		{"presencePenalty", "presencePenalty", 0.5, func(g *genai.GenerateContentConfig) bool {
			return g.PresencePenalty != nil && *g.PresencePenalty == float32(0.5)
		}},
		{"frequencyPenalty", "frequencyPenalty", 0.6, func(g *genai.GenerateContentConfig) bool {
			return g.FrequencyPenalty != nil && *g.FrequencyPenalty == float32(0.6)
		}},
		{"seed", "seed", float64(42), func(g *genai.GenerateContentConfig) bool { return g.Seed != nil && *g.Seed == 42 }},
		{"audioTimestamp", "audioTimestamp", true, func(g *genai.GenerateContentConfig) bool { return g.AudioTimestamp == true }},
		{"cachedContent", "cachedContent", "projects/123/cache/abc", func(g *genai.GenerateContentConfig) bool { return g.CachedContent == "projects/123/cache/abc" }},
		{"enableEnhancedCivicAnswers", "enableEnhancedCivicAnswers", true, func(g *genai.GenerateContentConfig) bool {
			return g.EnableEnhancedCivicAnswers != nil && *g.EnableEnhancedCivicAnswers == true
		}},
		{"serviceTier", "serviceTier", "flex", func(g *genai.GenerateContentConfig) bool { return g.ServiceTier == genai.ServiceTierFlex }},
		{"mediaResolution", "mediaResolution", "MEDIA_RESOLUTION_LOW", func(g *genai.GenerateContentConfig) bool { return g.MediaResolution == genai.MediaResolutionLow }},
		{"responseMimeType", "responseMimeType", "application/json", func(g *genai.GenerateContentConfig) bool { return g.ResponseMIMEType == "application/json" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gc, err := config.TranslateGenerateConfig(map[string]any{tt.key: tt.val})
			if err != nil {
				t.Fatalf("TranslateGenerateConfig: %v", err)
			}
			if !tt.check(gc) {
				t.Errorf("check failed for %s", tt.name)
			}
		})
	}
}

// TestTranslateGenerateConfig_EmptyAndNil verifies empty inputs.
func TestTranslateGenerateConfig_EmptyAndNil(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"nil", "empty"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var m map[string]any
			if name == "empty" {
				m = map[string]any{}
			}
			gc, err := config.TranslateGenerateConfig(m)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gc == nil {
				t.Fatal("got nil GenerateContentConfig")
			}
		})
	}
}

// TestSkillsetRef_ParseYAML verifies YAML parsing of SkillsetRef with all fields.
func TestSkillsetRef_ParseYAML(t *testing.T) {
	t.Parallel()
	yamlData := []byte(`
name: skills-agent
agent_class: LlmAgent
skill_sets:
  - name: filesystem
    config:
      path: "./skills"
    preload: complete
    names:
      - "weather"
      - "cooking"
    system_instruction: "Use these skills for domain tasks"
`)

	appCfg, err := config.Parse(yamlData, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := appCfg.AgentConfig

	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("got %T, want *LLMAgentConfig", got)
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
	t.Parallel()
	jsonData := []byte(`{
		"name": "agent",
		"agent_class": "LlmAgent",
		"skill_sets": [
			{
				"name": "filesystem",
				"config": {"path": "/app/skills"},
				"preload": "frontmatters",
				"names": ["search", "calculator"]
			}
		]
	}`)

	appCfg, err := config.Parse(jsonData, "json")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := appCfg.AgentConfig

	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("got %T, want *LLMAgentConfig", got)
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
	t.Parallel()
	yamlData := []byte(`
name: agent
agent_class: LlmAgent
skill_sets:
  - name: filesystem
    config:
      path: "./skills"
`)

	appCfg, err := config.Parse(yamlData, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := appCfg.AgentConfig

	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("got %T, want *LLMAgentConfig", got)
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
	t.Parallel()
	yamlData := []byte(`
name: agent
agent_class: LlmAgent
skill_sets:
  - name: filesystem
    config:
      path: "./skills"
    names:
      - weather
      - cooking
`)

	appCfg, err := config.Parse(yamlData, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := appCfg.AgentConfig

	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("got %T, want *LLMAgentConfig", got)
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
	t.Parallel()
	yamlData := []byte(`
name: agent-with-skills
agent_class: LlmAgent
model: gemini/gemini-2.5-flash
skill_sets:
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

	appCfg, err := config.Parse(yamlData, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := appCfg.AgentConfig

	if got.Name() != "agent-with-skills" {
		t.Errorf("Name: got %q, want agent-with-skills", got.Name())
	}

	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("got %T, want *LLMAgentConfig", got)
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

// TestParse_LLMAgent_WithTransferFlags verifies parsing of transfer flags in JSON and YAML.
func TestParse_LLMAgent_WithTransferFlags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		format string
		data   []byte
	}{
		{name: "JSON", format: "json", data: []byte(`{"name":"transfer-agent","agent_class":"LlmAgent","model":"gemini/gemini-pro","disallow_transfer_to_parent":true,"disallow_transfer_to_peers":true}`)},
		{name: "YAML", format: "yaml", data: []byte(`
name: transfer-agent
agent_class: LlmAgent
model: gemini/gemini-pro
disallow_transfer_to_parent: true
disallow_transfer_to_peers: true
`)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			appCfg, err := config.Parse(tc.data, tc.format)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			got := appCfg.AgentConfig
			llmCfg, ok := got.(*config.LLMAgentConfig)
			if !ok {
				t.Fatalf("got %T, want *LLMAgentConfig", got)
			}
			if !llmCfg.DisallowTransferToParent {
				t.Errorf("DisallowTransferToParent: got false, want true")
			}
			if !llmCfg.DisallowTransferToPeers {
				t.Errorf("DisallowTransferToPeers: got false, want true")
			}
		})
	}
}

// TestParse_SequentialWithTransferFlags_ReturnsError verifies that transfer flags on sequential agent errors.
func TestParse_SequentialWithTransferFlags_ReturnsError(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: bad-seq
agent_class: SequentialAgent
disallow_transfer_to_parent: true
`)
	_, err := config.Parse(data, "yaml")
	if err == nil {
		t.Fatal("got nil, want error for disallow_transfer_to_parent on sequential agent")
	}
}

// TestParse_LoopWithModel_ReturnsError verifies that model on loop agent errors.
func TestParse_LoopWithModel_ReturnsError(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: bad-loop
agent_class: LoopAgent
model: openai/gpt-4o
`)
	_, err := config.Parse(data, "yaml")
	if err == nil {
		t.Fatal("got nil, want error for model on loop agent")
	}
}

// TestParse_LoopWithTools_ReturnsError verifies that tools on loop agent errors.
func TestParse_LoopWithTools_ReturnsError(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: bad-loop
agent_class: LoopAgent
tools:
  - name: search
`)
	_, err := config.Parse(data, "yaml")
	if err == nil {
		t.Fatal("got nil, want error for tools on loop agent")
	}
}

// TestParse_AgentRefConfig verifies sub-agent references parse correctly.
func TestParse_AgentRefConfig(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: root
agent_class: SequentialAgent
sub_agents:
  - config_path: "./sub.yaml"
  - code: "myapp.agents.sub_agent"
`)
	appCfg, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := appCfg.AgentConfig
	seq, ok := got.(*config.SequentialAgentConfig)
	if !ok {
		t.Fatalf("got %T, want *SequentialAgentConfig", got)
	}
	entries := seq.SubAgentEntries()
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Ref == nil || entries[0].Ref.ConfigPath != "./sub.yaml" {
		t.Errorf("entry[0]: got %+v, want config_path ref", entries[0])
	}
	if entries[1].Ref == nil || entries[1].Ref.Code != "myapp.agents.sub_agent" {
		t.Errorf("entry[1]: got %+v, want code ref", entries[1])
	}
}

// TestParse_AgentRefConfig_BothFieldsError verifies validation rejects both fields.
func TestParse_AgentRefConfig_BothFieldsError(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: root
agent_class: SequentialAgent
sub_agents:
  - config_path: "./sub.yaml"
    code: "myapp.agents.sub_agent"
`)
	_, err := config.Parse(data, "yaml")
	if err == nil {
		t.Fatal("got nil, want error when both config_path and code are set")
	}
}

// TestParse_LLMAgent_WithCallbacks verifies all callback fields parse.
func TestParse_LLMAgent_WithCallbacks(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: cb-agent
agent_class: LlmAgent
model: gemini/gemini-pro
instruction: "hi"
before_agent_callbacks:
  - name: myapp.security.before
after_agent_callbacks:
  - name: myapp.security.after
before_model_callbacks:
  - name: myapp.cb.before_model
after_model_callbacks:
  - name: myapp.cb.after_model
on_model_error_callbacks:
  - name: myapp.cb.on_model_err
before_tool_callbacks:
  - name: myapp.cb.before_tool
after_tool_callbacks:
  - name: myapp.cb.after_tool
on_tool_error_callbacks:
  - name: myapp.cb.on_tool_err
`)
	appCfg, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := appCfg.AgentConfig
	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("got %T, want *LLMAgentConfig", got)
	}
	if len(llmCfg.BeforeAgentCallbacks) != 1 || llmCfg.BeforeAgentCallbacks[0].Name != "myapp.security.before" {
		t.Errorf("BeforeAgentCallbacks: got %+v", llmCfg.BeforeAgentCallbacks)
	}
	if len(llmCfg.AfterModelCallbacks) != 1 || llmCfg.AfterModelCallbacks[0].Name != "myapp.cb.after_model" {
		t.Errorf("AfterModelCallbacks: got %+v", llmCfg.AfterModelCallbacks)
	}
	if len(llmCfg.OnModelErrorCallbacks) != 1 || llmCfg.OnModelErrorCallbacks[0].Name != "myapp.cb.on_model_err" {
		t.Errorf("OnModelErrorCallbacks: got %+v", llmCfg.OnModelErrorCallbacks)
	}
	if len(llmCfg.OnToolErrorCallbacks) != 1 || llmCfg.OnToolErrorCallbacks[0].Name != "myapp.cb.on_tool_err" {
		t.Errorf("OnToolErrorCallbacks: got %+v", llmCfg.OnToolErrorCallbacks)
	}
}

// TestParseCallbacksOnNonLLMAgents verifies that before_agent_callbacks and
// after_agent_callbacks parse successfully on non-LLM agent types (sequential,
// parallel, and loop).
func TestParseCallbacksOnNonLLMAgents(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "sequential",
			data: []byte(`
name: test-agent
agent_class: SequentialAgent
before_agent_callbacks:
  - name: myapp.security.before
after_agent_callbacks:
  - name: myapp.security.after
`),
		},
		{
			name: "parallel",
			data: []byte(`
name: test-agent
agent_class: ParallelAgent
before_agent_callbacks:
  - name: myapp.security.before
after_agent_callbacks:
  - name: myapp.security.after
`),
		},
		{
			name: "loop",
			data: []byte(`
name: test-agent
agent_class: LoopAgent
max_iterations: 3
before_agent_callbacks:
  - name: myapp.security.before
after_agent_callbacks:
  - name: myapp.security.after
`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := config.Parse(tt.data, "yaml"); err != nil {
				t.Errorf("Parse: got unexpected error: %v", err)
			}
		})
	}
}

// TestParse_LLMAgent_WithModelCode verifies modelCode parsing and mutual exclusion.
func TestParse_LLMAgent_WithModelCode(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: code-model-agent
agent_class: LlmAgent
model_code:
  name: myapp.models.custom
  args:
    api_key: secret
instruction: "hi"
`)
	appCfg, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := appCfg.AgentConfig
	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("got %T, want *LLMAgentConfig", got)
	}
	if llmCfg.ModelCode == nil || llmCfg.ModelCode.Name != "myapp.models.custom" {
		t.Errorf("ModelCode: got %+v", llmCfg.ModelCode)
	}
	if llmCfg.ModelCode.Args["api_key"] != "secret" {
		t.Errorf("ModelCode.Args: got %+v", llmCfg.ModelCode.Args)
	}
}

// TestParse_ValidationErrors uses table-driven tests for field mutual exclusion and type checks.
func TestParse_ValidationErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		data    string
		wantErr string
	}{
		{
			name: "model and model_code both set",
			data: `name: bad
agent_class: LlmAgent
model: gemini/gemini-pro
model_code:
  name: myapp.models.custom
instruction: "hi"`,
			wantErr: "only one of model or model_code",
		},
		{
			name: "max_iterations on llm",
			data: `name: bad
agent_class: LlmAgent
model: gemini/gemini-pro
max_iterations: 5`,
			wantErr: "max_iterations",
		},
		{
			name: "model on sequential",
			data: `name: bad
agent_class: SequentialAgent
model: openai/gpt-4o`,
			wantErr: "model",
		},
		{
			name: "tools on parallel",
			data: `name: bad
agent_class: ParallelAgent
tools:
  - name: search`,
			wantErr: "tools",
		},
		{
			name: "model_code on loop",
			data: `name: bad
agent_class: LoopAgent
model_code:
  name: x`,
			wantErr: "model_code",
		},
		{
			name: "before_model_callbacks on sequential",
			data: `name: bad
agent_class: SequentialAgent
before_model_callbacks:
  - name: x`,
			wantErr: "before_model_callbacks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.Parse([]byte(tt.data), "yaml")
			if err == nil {
				t.Fatalf("got nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestParse_SchemaRef_Inline verifies parsing of an inline genai.Schema in JSON and YAML.
func TestParse_SchemaRef_Inline(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		format string
		data   []byte
	}{
		{name: "YAML", format: "yaml", data: []byte(`
name: inline-schema-agent
agent_class: LlmAgent
model: gemini/gemini-pro
input_schema:
  type: object
  description: "input desc"
  properties:
    name:
      type: string
output_schema:
  type: string
`)},
		{name: "JSON", format: "json", data: []byte(`{
		"name": "inline-json-agent",
		"agent_class": "LlmAgent",
		"model": "gemini/gemini-pro",
		"input_schema": {
			"type": "object",
			"description": "input desc"
		},
		"output_schema": {
			"type": "string"
		}
	}`)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			appCfg, err := config.Parse(tc.data, tc.format)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			got := appCfg.AgentConfig
			llmCfg, ok := got.(*config.LLMAgentConfig)
			if !ok {
				t.Fatalf("got %T, want *LLMAgentConfig", got)
			}
			if llmCfg.InputSchema == nil || llmCfg.InputSchema.Inline == nil {
				t.Fatalf("InputSchema.Inline is nil")
			}
			if llmCfg.InputSchema.Inline.Type != genai.Type("object") {
				t.Errorf("InputSchema.Type: got %v, want object", llmCfg.InputSchema.Inline.Type)
			}
			if llmCfg.InputSchema.Inline.Description != "input desc" {
				t.Errorf("InputSchema.Description: got %q", llmCfg.InputSchema.Inline.Description)
			}
			if llmCfg.OutputSchema == nil || llmCfg.OutputSchema.Inline == nil {
				t.Fatalf("OutputSchema.Inline is nil")
			}
			if llmCfg.OutputSchema.Inline.Type != genai.Type("string") {
				t.Errorf("OutputSchema.Type: got %v, want string", llmCfg.OutputSchema.Inline.Type)
			}
		})
	}
}

// TestParse_SchemaRef_NamedRef verifies parsing of a named CodeConfig reference in JSON and YAML.
func TestParse_SchemaRef_NamedRef(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		format string
		data   []byte
	}{
		{name: "YAML", format: "yaml", data: []byte(`
name: ref-schema-agent
agent_class: LlmAgent
model: gemini/gemini-pro
input_schema:
  name: myapp.schemas.input
  args:
    key: value
`)},
		{name: "JSON", format: "json", data: []byte(`{
		"name": "ref-json-agent",
		"agent_class": "LlmAgent",
		"model": "gemini/gemini-pro",
		"input_schema": {
			"name": "myapp.schemas.input",
			"args": {"key": "value"}
		}
	}`)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			appCfg, err := config.Parse(tc.data, tc.format)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			got := appCfg.AgentConfig
			llmCfg, ok := got.(*config.LLMAgentConfig)
			if !ok {
				t.Fatalf("got %T, want *LLMAgentConfig", got)
			}
			if llmCfg.InputSchema == nil || llmCfg.InputSchema.Ref == nil {
				t.Fatalf("InputSchema.Ref is nil")
			}
			if llmCfg.InputSchema.Ref.Name != "myapp.schemas.input" {
				t.Errorf("InputSchema.Ref.Name: got %q", llmCfg.InputSchema.Ref.Name)
			}
			if llmCfg.InputSchema.Ref.Args["key"] != "value" {
				t.Errorf("InputSchema.Ref.Args: got %+v", llmCfg.InputSchema.Ref.Args)
			}
		})
	}
}

// TestParse_SchemaRef_StringShorthand verifies parsing of a string shorthand reference in JSON and YAML.
func TestParse_SchemaRef_StringShorthand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		format string
		data   []byte
	}{
		{name: "YAML", format: "yaml", data: []byte(`
name: shorthand-schema-agent
agent_class: LlmAgent
model: gemini/gemini-pro
input_schema: myapp.schemas.input
`)},
		{name: "JSON", format: "json", data: []byte(`{
		"name": "shorthand-json-agent",
		"agent_class": "LlmAgent",
		"model": "gemini/gemini-pro",
		"input_schema": "myapp.schemas.input"
	}`)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			appCfg, err := config.Parse(tc.data, tc.format)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			got := appCfg.AgentConfig
			llmCfg, ok := got.(*config.LLMAgentConfig)
			if !ok {
				t.Fatalf("got %T, want *LLMAgentConfig", got)
			}
			if llmCfg.InputSchema == nil || llmCfg.InputSchema.Ref == nil {
				t.Fatalf("InputSchema.Ref is nil")
			}
			if llmCfg.InputSchema.Ref.Name != "myapp.schemas.input" {
				t.Errorf("InputSchema.Ref.Name: got %q", llmCfg.InputSchema.Ref.Name)
			}
		})
	}
}

// TestParse_SchemaRef_Invalid verifies error for invalid SchemaRef missing both type and name.
func TestParse_SchemaRef_Invalid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		format string
		data   []byte
	}{
		{
			name:   "yaml_missing_keys",
			format: "yaml",
			data: []byte(`
name: invalid-schema-agent
agent_class: LlmAgent
model: gemini/gemini-pro
input_schema:
  unknown: value
`),
		},
		{
			name:   "json_missing_keys",
			format: "json",
			data: []byte(`{
				"name": "invalid-json-agent",
				"agent_class": "LlmAgent",
				"model": "gemini/gemini-pro",
				"input_schema": {"unknown": "value"}
			}`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.Parse(tt.data, tt.format)
			if err == nil {
				t.Fatal("got nil, want error for invalid schemaRef")
			}
		})
	}
}

// TestParse_LLMAgent_AdvancedFields verifies static_instruction, schemas, output_key, include_contents.
func TestParse_LLMAgent_AdvancedFields(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: advanced
agent_class: LlmAgent
model: gemini/gemini-pro
instruction: "hi"
static_instruction: "static sys prompt"
input_schema:
  name: myapp.schemas.input
output_schema:
  name: myapp.schemas.output
output_key: result
include_contents: none
`)
	appCfg, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := appCfg.AgentConfig
	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("got %T, want *LLMAgentConfig", got)
	}
	if llmCfg.StaticInstruction != "static sys prompt" {
		t.Errorf("StaticInstruction: got %q", llmCfg.StaticInstruction)
	}
	if llmCfg.InputSchema == nil || llmCfg.InputSchema.Ref == nil || llmCfg.InputSchema.Ref.Name != "myapp.schemas.input" {
		t.Errorf("InputSchema: got %+v", llmCfg.InputSchema)
	}
	if llmCfg.OutputSchema == nil || llmCfg.OutputSchema.Ref == nil || llmCfg.OutputSchema.Ref.Name != "myapp.schemas.output" {
		t.Errorf("OutputSchema: got %+v", llmCfg.OutputSchema)
	}
	if llmCfg.OutputKey != "result" {
		t.Errorf("OutputKey: got %q", llmCfg.OutputKey)
	}
	if llmCfg.IncludeContents != "none" {
		t.Errorf("IncludeContents: got %q", llmCfg.IncludeContents)
	}
}

// TestParse_WithRunConfig verifies YAML/JSON with runConfig block populates appCfg.RunConfig; defaults applied when partial.
func TestParse_WithRunConfig(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: run-cfg-agent
agent_class: LlmAgent
model: gemini/gemini-pro
run_config:
  streaming_mode: sse
`)
	appCfg, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if appCfg.RunConfig == nil {
		t.Fatal("got nil RunConfig, want non-nil RunConfig")
	}
	if appCfg.RunConfig.StreamingMode != config.StreamingModeSSE {
		t.Errorf("StreamingMode: got %q, want %q", appCfg.RunConfig.StreamingMode, config.StreamingModeSSE)
	}
	if appCfg.ContextCacheConfig != nil {
		t.Errorf("got %+v, want nil ContextCacheConfig", appCfg.ContextCacheConfig)
	}
}

// TestParse_WithContextCacheConfig verifies YAML/JSON with contextCacheConfig block populates appCfg.ContextCacheConfig; defaults applied.
func TestParse_WithContextCacheConfig(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: cache-cfg-agent
agent_class: LlmAgent
model: gemini/gemini-pro
context_cache_config:
  min_tokens: 50
`)
	appCfg, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if appCfg.ContextCacheConfig == nil {
		t.Fatal("got nil ContextCacheConfig, want non-nil ContextCacheConfig")
	}
	if appCfg.ContextCacheConfig.CacheIntervals != 10 {
		t.Errorf("CacheIntervals default: got %d, want 10", appCfg.ContextCacheConfig.CacheIntervals)
	}
	if appCfg.ContextCacheConfig.TTLSeconds != 1800 {
		t.Errorf("TTLSeconds default: got %d, want 1800", appCfg.ContextCacheConfig.TTLSeconds)
	}
	if appCfg.ContextCacheConfig.MinTokens != 50 {
		t.Errorf("MinTokens: got %d, want 50", appCfg.ContextCacheConfig.MinTokens)
	}
	if appCfg.RunConfig != nil {
		t.Errorf("got %+v, want nil RunConfig", appCfg.RunConfig)
	}
}

// TestParse_WithBothRuntimeConfigs verifies both blocks present in same file.
func TestParse_WithBothRuntimeConfigs(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: both-cfg-agent
agent_class: LlmAgent
model: gemini/gemini-pro
run_config:
  streaming_mode: sse
live_run_config:
  max_llm_calls: 100
context_cache_config:
  cache_intervals: 5
  ttl_seconds: 600
`)
	appCfg, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if appCfg.RunConfig == nil {
		t.Fatal("got nil RunConfig, want non-nil RunConfig")
	}
	if appCfg.LiveRunConfig == nil {
		t.Fatal("got nil LiveRunConfig, want non-nil LiveRunConfig")
	}
	if appCfg.LiveRunConfig.MaxLLMCalls != 100 {
		t.Errorf("MaxLLMCalls: got %d, want 100", appCfg.LiveRunConfig.MaxLLMCalls)
	}
	if appCfg.ContextCacheConfig == nil {
		t.Fatal("got nil ContextCacheConfig, want non-nil ContextCacheConfig")
	}
	if appCfg.ContextCacheConfig.CacheIntervals != 5 {
		t.Errorf("CacheIntervals: got %d, want 5", appCfg.ContextCacheConfig.CacheIntervals)
	}
	if appCfg.ContextCacheConfig.TTLSeconds != 600 {
		t.Errorf("TTLSeconds: got %d, want 600", appCfg.ContextCacheConfig.TTLSeconds)
	}
}

// TestParse_RuntimeConfigAbsent verifies omitting both blocks yields nil pointers; no default objects created.
func TestParse_RuntimeConfigAbsent(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: no-runtime
agent_class: LlmAgent
model: gemini/gemini-pro
`)
	appCfg, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if appCfg.RunConfig != nil {
		t.Errorf("got %+v, want nil RunConfig", appCfg.RunConfig)
	}
	if appCfg.ContextCacheConfig != nil {
		t.Errorf("got %+v, want nil ContextCacheConfig", appCfg.ContextCacheConfig)
	}
}

// TestParse_RuntimeConfigValidation verifies invalid runtime config values return errors.
func TestParse_RuntimeConfigValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "cache_intervals too high",
			data: `name: bad
agent_class: LlmAgent
model: gemini/gemini-pro
context_cache_config:
  cache_intervals: 101`,
			want: "cache_intervals",
		},
		{
			name: "ttl_seconds negative",
			data: `name: bad
agent_class: LlmAgent
model: gemini/gemini-pro
context_cache_config:
  ttl_seconds: -1`,
			want: "ttl_seconds",
		},
		{
			name: "min_tokens negative",
			data: `name: bad
agent_class: LlmAgent
model: gemini/gemini-pro
context_cache_config:
  min_tokens: -5`,
			want: "min_tokens",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.Parse([]byte(tt.data), "yaml")
			if err == nil {
				t.Fatalf("got nil, want error containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.want)
			}
		})
	}
}

// TestLoad_RuntimeConfigs verifies file load round-trip for both runtime config blocks.
func TestLoad_RuntimeConfigs(t *testing.T) {
	content := []byte(`
name: loaded-runtime
agent_class: LlmAgent
model: gemini/gemini-pro
run_config:
  streaming_mode: bidi
live_run_config:
  max_llm_calls: 200
context_cache_config:
  cache_intervals: 20
  ttl_seconds: 3600
  min_tokens: 50
`)
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	appCfg, err := config.Load(root, "agent.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if appCfg.RunConfig == nil {
		t.Fatal("got nil RunConfig, want non-nil RunConfig")
	}
	if appCfg.RunConfig.StreamingMode != config.StreamingModeBIDI {
		t.Errorf("StreamingMode: got %q, want %q", appCfg.RunConfig.StreamingMode, config.StreamingModeBIDI)
	}
	if appCfg.LiveRunConfig == nil {
		t.Fatal("got nil LiveRunConfig, want non-nil LiveRunConfig")
	}
	if appCfg.LiveRunConfig.MaxLLMCalls != 200 {
		t.Errorf("MaxLLMCalls: got %d, want 200", appCfg.LiveRunConfig.MaxLLMCalls)
	}
	if appCfg.ContextCacheConfig == nil {
		t.Fatal("got nil ContextCacheConfig, want non-nil ContextCacheConfig")
	}
	if appCfg.ContextCacheConfig.CacheIntervals != 20 {
		t.Errorf("CacheIntervals: got %d, want 20", appCfg.ContextCacheConfig.CacheIntervals)
	}
	if appCfg.ContextCacheConfig.TTLSeconds != 3600 {
		t.Errorf("TTLSeconds: got %d, want 3600", appCfg.ContextCacheConfig.TTLSeconds)
	}
	if appCfg.ContextCacheConfig.MinTokens != 50 {
		t.Errorf("MinTokens: got %d, want 50", appCfg.ContextCacheConfig.MinTokens)
	}
}

// TestParse_WithLiveRunConfig verifies YAML/JSON with live_run_config block populates appCfg.LiveRunConfig; defaults applied when partial.
func TestParse_WithLiveRunConfig(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: live-cfg-agent
agent_class: LlmAgent
model: gemini/gemini-pro
live_run_config:
  max_llm_calls: 300
`)
	appCfg, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if appCfg.LiveRunConfig == nil {
		t.Fatal("got nil LiveRunConfig, want non-nil LiveRunConfig")
	}
	if appCfg.LiveRunConfig.MaxLLMCalls != 300 {
		t.Errorf("MaxLLMCalls: got %d, want 300", appCfg.LiveRunConfig.MaxLLMCalls)
	}

	// Default case: no live_run_config block
	data2 := []byte(`
name: live-default-agent
agent_class: LlmAgent
model: gemini/gemini-pro
`)
	appCfg2, err := config.Parse(data2, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if appCfg2.LiveRunConfig != nil {
		t.Errorf("got %+v, want nil LiveRunConfig", appCfg2.LiveRunConfig)
	}
}

// TestParse_LiveRunConfigValidation verifies invalid live_run_config values return errors.
func TestParse_LiveRunConfigValidation(t *testing.T) {
	t.Parallel()
	data := []byte(`name: bad
agent_class: LlmAgent
model: gemini/gemini-pro
live_run_config:
  max_llm_calls: -1`)
	_, err := config.Parse([]byte(data), "yaml")
	if err == nil {
		t.Fatal("got nil, want error for negative max_llm_calls")
	}
	if !errors.Is(err, config.ErrMaxLLMCallsInvalid) {
		t.Errorf("error %v does not wrap config.ErrMaxLLMCallsInvalid", err)
	}
}

// TestParse_FullPythonStyleConfig verifies a comprehensive YAML config using every snake_case key.
func TestParse_FullPythonStyleConfig(t *testing.T) {
	t.Parallel()
	data := []byte(`
name: full-agent
agent_class: LlmAgent
description: "A full-featured agent"
model: gemini/gemini-pro
instruction: "You are helpful."
static_instruction: "Static prompt"
input_schema:
  type: object
output_schema:
  type: string
output_key: result
include_contents: none
tools:
  - name: search
    args:
      timeout: 30
skill_sets:
  - name: filesystem
    config:
      path: "./skills"
    preload: complete
generate_content_config:
  temperature: 0.7
disallow_transfer_to_parent: true
disallow_transfer_to_peers: true
before_model_callbacks:
  - name: myapp.cb.before_model
after_model_callbacks:
  - name: myapp.cb.after_model
on_model_error_callbacks:
  - name: myapp.cb.on_model_err
before_tool_callbacks:
  - name: myapp.cb.before_tool
after_tool_callbacks:
  - name: myapp.cb.after_tool
on_tool_error_callbacks:
  - name: myapp.cb.on_tool_err
before_agent_callbacks:
  - name: myapp.cb.before_agent
after_agent_callbacks:
  - name: myapp.cb.after_agent
run_config:
  streaming_mode: sse
  save_live_blob: true
  custom_metadata:
    key: value
context_cache_config:
  cache_intervals: 5
  ttl_seconds: 600
  min_tokens: 50
sub_agents:
  - name: child
    agent_class: LlmAgent
    model: gemini/gemini-pro
`)
	appCfg, err := config.Parse(data, "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := appCfg.AgentConfig
	if got.Name() != "full-agent" {
		t.Errorf("Name: got %q, want full-agent", got.Name())
	}
	llmCfg, ok := got.(*config.LLMAgentConfig)
	if !ok {
		t.Fatalf("got %T, want *LLMAgentConfig", got)
	}
	if llmCfg.StaticInstruction != "Static prompt" {
		t.Errorf("StaticInstruction: got %q", llmCfg.StaticInstruction)
	}
	if llmCfg.OutputKey != "result" {
		t.Errorf("OutputKey: got %q", llmCfg.OutputKey)
	}
	if llmCfg.IncludeContents != "none" {
		t.Errorf("IncludeContents: got %q", llmCfg.IncludeContents)
	}
	if len(llmCfg.Tools) != 1 || llmCfg.Tools[0].Name != "search" {
		t.Errorf("Tools: got %v", llmCfg.Tools)
	}
	if llmCfg.Tools[0].Args["timeout"] != 30 {
		t.Errorf("Tool.Args: got %+v", llmCfg.Tools[0].Args)
	}
	if len(llmCfg.Skillsets) != 1 || llmCfg.Skillsets[0].Name != "filesystem" {
		t.Errorf("Skillsets: got %v", llmCfg.Skillsets)
	}
	if !llmCfg.DisallowTransferToParent {
		t.Errorf("DisallowTransferToParent: got false, want true")
	}
	if !llmCfg.DisallowTransferToPeers {
		t.Errorf("DisallowTransferToPeers: got false, want true")
	}
	if len(llmCfg.BeforeModelCallbacks) != 1 {
		t.Errorf("BeforeModelCallbacks: got %d", len(llmCfg.BeforeModelCallbacks))
	}
	if len(llmCfg.OnToolErrorCallbacks) != 1 {
		t.Errorf("OnToolErrorCallbacks: got %d", len(llmCfg.OnToolErrorCallbacks))
	}
	if len(llmCfg.BeforeAgentCallbacks) != 1 {
		t.Errorf("BeforeAgentCallbacks: got %d", len(llmCfg.BeforeAgentCallbacks))
	}
	if appCfg.RunConfig == nil || appCfg.RunConfig.StreamingMode != config.StreamingModeSSE {
		t.Errorf("RunConfig: got %+v", appCfg.RunConfig)
	}
	if appCfg.ContextCacheConfig == nil || appCfg.ContextCacheConfig.MinTokens != 50 {
		t.Errorf("ContextCacheConfig: got %+v", appCfg.ContextCacheConfig)
	}
	if len(got.SubAgents()) != 1 || got.SubAgents()[0].Name() != "child" {
		t.Errorf("SubAgents: got %v", got.SubAgents())
	}
}

// TestParse_MissingAgentClass verifies that missing agent_class returns an error.
func TestParse_MissingAgentClass(t *testing.T) {
	t.Parallel()
	data := []byte("name: missing-class-agent\nmodel: gemini/gemini-pro\n")
	_, err := config.Parse(data, "yaml")
	if err == nil {
		t.Fatal("got nil, want error for missing agent_class")
	}
}

// TestParse_EdgeCases verifies Parse behavior on empty, comment-only, and
// syntactically invalid inputs.
func TestParse_EdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		data    string
		format  string
		wantErr bool
	}{
		{
			name:    "empty input yaml",
			data:    "",
			format:  "yaml",
			wantErr: true,
		},
		{
			name:    "empty input json",
			data:    "",
			format:  "json",
			wantErr: true,
		},
		{
			name:    "only comments yaml",
			data:    "# this is a comment\n# another comment\n",
			format:  "yaml",
			wantErr: true,
		},
		{
			name:    "invalid yaml syntax",
			data:    "name: [unclosed\n",
			format:  "yaml",
			wantErr: true,
		},
		{
			name:    "invalid json syntax",
			data:    `{"name": "bad"`,
			format:  "json",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.Parse([]byte(tt.data), tt.format)
			if tt.wantErr {
				if err == nil {
					t.Fatal("got nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
		})
	}
}

// TestTranslateGenerateConfig_StringSliceAndMapFields verifies slice and map fields.
func TestTranslateGenerateConfig_StringSliceAndMapFields(t *testing.T) {
	t.Parallel()
	m := map[string]any{
		"stopSequences":      []any{"STOP", "END"},
		"responseModalities": []any{"TEXT", "IMAGE"},
		"labels":             map[string]any{"env": "prod"},
	}
	gc, err := config.TranslateGenerateConfig(m)
	if err != nil {
		t.Fatalf("TranslateGenerateConfig: %v", err)
	}
	if len(gc.StopSequences) != 2 || gc.StopSequences[0] != "STOP" || gc.StopSequences[1] != "END" {
		t.Errorf("StopSequences: got %v", gc.StopSequences)
	}
	if len(gc.ResponseModalities) != 2 || gc.ResponseModalities[0] != "TEXT" || gc.ResponseModalities[1] != "IMAGE" {
		t.Errorf("ResponseModalities: got %v", gc.ResponseModalities)
	}
	if len(gc.Labels) != 1 || gc.Labels["env"] != "prod" {
		t.Errorf("Labels: got %v", gc.Labels)
	}
}

// TestTranslateGenerateConfig_NestedStructs verifies representative complex fields via toStruct.
func TestTranslateGenerateConfig_NestedStructs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input map[string]any
		check func(*genai.GenerateContentConfig) bool
	}{
		{
			name: "safetySettings",
			input: map[string]any{
				"safetySettings": []any{
					map[string]any{"category": "HARM_CATEGORY_HARASSMENT", "threshold": "BLOCK_LOW_AND_ABOVE"},
				},
			},
			check: func(g *genai.GenerateContentConfig) bool {
				return len(g.SafetySettings) == 1 &&
					g.SafetySettings[0].Category == genai.HarmCategoryHarassment &&
					g.SafetySettings[0].Threshold == genai.HarmBlockThresholdBlockLowAndAbove
			},
		},
		{
			name:  "thinkingConfig",
			input: map[string]any{"thinkingConfig": map[string]any{"includeThoughts": true}},
			check: func(g *genai.GenerateContentConfig) bool {
				return g.ThinkingConfig != nil && g.ThinkingConfig.IncludeThoughts == true
			},
		},
		{
			name:  "toolConfig",
			input: map[string]any{"toolConfig": map[string]any{"functionCallingConfig": map[string]any{"mode": "ANY"}}},
			check: func(g *genai.GenerateContentConfig) bool {
				return g.ToolConfig != nil && g.ToolConfig.FunctionCallingConfig != nil &&
					g.ToolConfig.FunctionCallingConfig.Mode == genai.FunctionCallingConfigModeAny
			},
		},
		{
			name:  "tools",
			input: map[string]any{"tools": []any{map[string]any{"googleSearch": map[string]any{}}}},
			check: func(g *genai.GenerateContentConfig) bool {
				return len(g.Tools) == 1 && g.Tools[0].GoogleSearch != nil
			},
		},
		{
			name:  "imageConfig",
			input: map[string]any{"imageConfig": map[string]any{"aspectRatio": "16:9"}},
			check: func(g *genai.GenerateContentConfig) bool {
				return g.ImageConfig != nil && g.ImageConfig.AspectRatio == "16:9"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gc, err := config.TranslateGenerateConfig(tt.input)
			if err != nil {
				t.Fatalf("TranslateGenerateConfig: %v", err)
			}
			if !tt.check(gc) {
				t.Errorf("check failed for %s", tt.name)
			}
		})
	}
}

// TestTranslateGenerateConfig_SystemInstruction verifies polymorphic systemInstruction.
func TestTranslateGenerateConfig_SystemInstruction(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		val   any
		check func(*genai.GenerateContentConfig) bool
	}{
		{
			name: "string",
			val:  "Be concise",
			check: func(g *genai.GenerateContentConfig) bool {
				return g.SystemInstruction != nil &&
					g.SystemInstruction.Role == "system" &&
					len(g.SystemInstruction.Parts) == 1 &&
					g.SystemInstruction.Parts[0].Text == "Be concise"
			},
		},
		{
			name: "map",
			val: map[string]any{
				"role":  "system",
				"parts": []any{map[string]any{"text": "hi"}},
			},
			check: func(g *genai.GenerateContentConfig) bool {
				return g.SystemInstruction != nil &&
					g.SystemInstruction.Role == "system" &&
					len(g.SystemInstruction.Parts) == 1 &&
					g.SystemInstruction.Parts[0].Text == "hi"
			},
		},
		{
			name: "array",
			val:  []any{map[string]any{"text": "hi"}},
			check: func(g *genai.GenerateContentConfig) bool {
				return g.SystemInstruction != nil &&
					g.SystemInstruction.Role == "system" &&
					len(g.SystemInstruction.Parts) == 1 &&
					g.SystemInstruction.Parts[0].Text == "hi"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gc, err := config.TranslateGenerateConfig(map[string]any{"systemInstruction": tt.val})
			if err != nil {
				t.Fatalf("TranslateGenerateConfig: %v", err)
			}
			if !tt.check(gc) {
				t.Errorf("check failed for %s", tt.name)
			}
		})
	}
}

// TestTranslateGenerateConfig_ResponseJsonSchema verifies pass-through behavior.
func TestTranslateGenerateConfig_ResponseJsonSchema(t *testing.T) {
	t.Parallel()
	m := map[string]any{
		"responseJsonSchema": map[string]any{"type": "object"},
	}
	gc, err := config.TranslateGenerateConfig(m)
	if err != nil {
		t.Fatalf("TranslateGenerateConfig: %v", err)
	}
	v, ok := gc.ResponseJsonSchema.(map[string]any)
	if !ok || v["type"] != "object" {
		t.Errorf("ResponseJsonSchema: got %v", gc.ResponseJsonSchema)
	}
}

// TestTranslateGenerateConfig_TypeErrors verifies meaningful errors on type mismatches.
func TestTranslateGenerateConfig_TypeErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   map[string]any
		wantErr string
	}{
		{
			name:    "seed string",
			input:   map[string]any{"seed": "abc"},
			wantErr: "config.TranslateGenerateConfig seed:",
		},
		{
			name:    "responseLogprobs int",
			input:   map[string]any{"responseLogprobs": 1},
			wantErr: "config.TranslateGenerateConfig responseLogprobs:",
		},
		{
			name:    "labels non-string value",
			input:   map[string]any{"labels": map[string]any{"k": 123}},
			wantErr: "config.TranslateGenerateConfig labels:",
		},
		{
			name:    "serviceTier int",
			input:   map[string]any{"serviceTier": 123},
			wantErr: "config.TranslateGenerateConfig serviceTier:",
		},
		{
			name:    "safetySettings not array",
			input:   map[string]any{"safetySettings": "not-an-array"},
			wantErr: "config.TranslateGenerateConfig safetySettings:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.TranslateGenerateConfig(tt.input)
			if err == nil {
				t.Fatal("got nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestTranslateGenerateConfig_UnknownFieldsIgnored verifies unknown keys are silently ignored.
func TestTranslateGenerateConfig_UnknownFieldsIgnored(t *testing.T) {
	t.Parallel()
	m := map[string]any{
		"fictitiousField": "value",
		"temperature":     0.5,
	}
	gc, err := config.TranslateGenerateConfig(m)
	if err != nil {
		t.Fatalf("TranslateGenerateConfig: %v", err)
	}
	if gc.Temperature == nil || *gc.Temperature != float32(0.5) {
		t.Errorf("Temperature: got %v, want 0.5", gc.Temperature)
	}
}

// FuzzParse verifies that config.Parse never panics on arbitrary YAML input.
// Valid YAML should parse without error (or produce a config validation error);
// invalid YAML should return an error (no panic).
func FuzzParse(f *testing.F) {
	// Seed: valid YAML.
	f.Add([]byte("name: bot\nagent_class: LlmAgent\n"))
	// Seed: malformed YAML.
	f.Add([]byte(":\n  :\n   bad\n"))
	// Seed: empty bytes.
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, err := config.Parse(data, "yaml")
		// The function must not panic — reaching here is the primary assertion.
		// Both error and non-error outcomes are acceptable as long as no panic
		// occurred. We don't assert on the specific error because config
		// validation may accept or reject various inputs.
		_ = err
	})
}
