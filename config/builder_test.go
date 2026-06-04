package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

// testRegistry returns a *Registry pre-loaded with a "mock" model prefix and
// a "search" tool factory — sufficient for all builder tests.
func testRegistry() *Registry {
	r := NewRegistry()

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
	cfg := &LLMAgentConfig{
		BaseAgentConfig: BaseAgentConfig{Name: "chat-bot"},
		Model:           "mock/fast",
		Tools:           []ToolRef{{Name: "search"}},
	}

	a, err := Build(context.Background(), cfg, testRegistry())
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
	cfg := &SequentialAgentConfig{
		BaseAgentConfig: BaseAgentConfig{
			Name: "pipeline",
			SubAgentEntries: []SubAgentEntry{
				{Inline: &LLMAgentConfig{BaseAgentConfig: BaseAgentConfig{Name: "step-1"}, Model: "mock/fast"}},
				{Inline: &LLMAgentConfig{BaseAgentConfig: BaseAgentConfig{Name: "step-2"}, Model: "mock/fast"}},
			},
		},
	}

	a, err := Build(context.Background(), cfg, testRegistry())
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
	cfg := &ParallelAgentConfig{
		BaseAgentConfig: BaseAgentConfig{
			Name: "fan-out",
			SubAgentEntries: []SubAgentEntry{
				{Inline: &LLMAgentConfig{BaseAgentConfig: BaseAgentConfig{Name: "worker-a"}, Model: "mock/fast"}},
				{Inline: &LLMAgentConfig{BaseAgentConfig: BaseAgentConfig{Name: "worker-b"}, Model: "mock/fast"}},
			},
		},
	}

	a, err := Build(context.Background(), cfg, testRegistry())
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
	cfg := &LoopAgentConfig{
		BaseAgentConfig: BaseAgentConfig{
			Name: "refiner",
			SubAgentEntries: []SubAgentEntry{
				{Inline: &LLMAgentConfig{BaseAgentConfig: BaseAgentConfig{Name: "inner"}, Model: "mock/fast"}},
			},
		},
		MaxIterations: 3,
	}

	a, err := Build(context.Background(), cfg, testRegistry())
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
	cfg := &LLMAgentConfig{
		BaseAgentConfig: BaseAgentConfig{
			Name: "root",
			SubAgentEntries: []SubAgentEntry{
				{Inline: &SequentialAgentConfig{
					BaseAgentConfig: BaseAgentConfig{
						Name: "seq",
						SubAgentEntries: []SubAgentEntry{
							{Inline: &LLMAgentConfig{BaseAgentConfig: BaseAgentConfig{Name: "child-a"}, Model: "mock/fast"}},
							{Inline: &LLMAgentConfig{BaseAgentConfig: BaseAgentConfig{Name: "child-b"}, Model: "mock/fast"}},
						},
					},
				}},
			},
		},
		Model: "mock/fast",
	}

	a, err := Build(context.Background(), cfg, testRegistry())
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

// TestBuild_NilConfig verifies that Build returns an error for nil
func TestBuild_NilConfig(t *testing.T) {
	_, err := Build(context.Background(), nil, testRegistry())
	if err == nil {
		t.Fatal("expected an error for nil config, got nil")
	}
}

// TestBuild_ModelNotFound verifies that Build returns an error when the model
// prefix has no registered factory.
func TestBuild_ModelNotFound(t *testing.T) {
	cfg := &LLMAgentConfig{
		BaseAgentConfig: BaseAgentConfig{Name: "bot"},
		Model:           "unregistered/gpt-x",
	}
	_, err := Build(context.Background(), cfg, testRegistry())
	if err == nil {
		t.Fatal("expected an error for unregistered model prefix, got nil")
	}
}

// TestBuild_ToolNotFound verifies that Build returns an error when a tool name
// has no registered factory.
func TestBuild_ToolNotFound(t *testing.T) {
	cfg := &LLMAgentConfig{
		BaseAgentConfig: BaseAgentConfig{Name: "bot"},
		Model:           "mock/fast",
		Tools:           []ToolRef{{Name: "nonexistent-tool"}},
	}
	_, err := Build(context.Background(), cfg, testRegistry())
	if err == nil {
		t.Fatal("expected an error for unregistered tool, got nil")
	}
}

// TestBuild_LLMAgent_WithTransferFlags verifies that transfer flags are passed through.
func TestBuild_LLMAgent_WithTransferFlags(t *testing.T) {
	cfg := &LLMAgentConfig{
		BaseAgentConfig:          BaseAgentConfig{Name: "transfer-bot"},
		Model:                    "mock/fast",
		DisallowTransferToParent: true,
		DisallowTransferToPeers:  true,
	}

	a, err := Build(context.Background(), cfg, testRegistry())
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a.Name() != "transfer-bot" {
		t.Errorf("expected agent name %q, got %q", "transfer-bot", a.Name())
	}
}

// TestBuild_LLMAgent_DefaultTransferFlags verifies defaults are false when omitted.
func TestBuild_LLMAgent_DefaultTransferFlags(t *testing.T) {
	cfg := &LLMAgentConfig{
		BaseAgentConfig: BaseAgentConfig{Name: "default-bot"},
		Model:           "mock/fast",
	}

	a, err := Build(context.Background(), cfg, testRegistry())
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
agent_class: LlmAgent
model: mock/fast
`
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write temp YAML file: %v", err)
	}

	a, runCfg, liveRunCfg, cacheCfg, err := LoadAndBuild(context.Background(), path, testRegistry())
	if err != nil {
		t.Fatalf("LoadAndBuild returned unexpected error: %v", err)
	}
	if a.Name() != "file-agent" {
		t.Errorf("expected agent name %q, got %q", "file-agent", a.Name())
	}
	if runCfg != nil {
		t.Errorf("expected nil runCfg when absent, got %+v", runCfg)
	}
	if liveRunCfg != nil {
		t.Errorf("expected nil liveRunCfg when absent, got %+v", liveRunCfg)
	}
	if cacheCfg != nil {
		t.Errorf("expected nil cacheCfg when absent, got %+v", cacheCfg)
	}
}

// TestBuild_SubAgentFromFile verifies Build loads sub-agents from config_path refs.
func TestBuild_SubAgentFromFile(t *testing.T) {
	dir := t.TempDir()
	subContent := `
name: sub
agent_class: LlmAgent
model: mock/fast
`
	subPath := filepath.Join(dir, "sub.yaml")
	_ = os.WriteFile(subPath, []byte(subContent), 0644)

	cfg := &SequentialAgentConfig{
		BaseAgentConfig: BaseAgentConfig{
			Name: "root",
			SubAgentEntries: []SubAgentEntry{
				{Ref: &AgentRefConfig{ConfigPath: subPath}},
			},
		},
	}

	a, err := BuildWithPath(context.Background(), cfg, testRegistry(), dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(a.SubAgents()) != 1 {
		t.Fatalf("expected 1 sub-agent, got %d", len(a.SubAgents()))
	}
	if a.SubAgents()[0].Name() != "sub" {
		t.Errorf("expected sub-agent name %q, got %q", "sub", a.SubAgents()[0].Name())
	}
}

// TestBuild_SubAgentFromCode verifies Build resolves code refs via Registry.
func TestBuild_SubAgentFromCode(t *testing.T) {
	reg := testRegistry()
	fakeSub, err := testutil.NewFakeAgent("code-sub")
	if err != nil {
		t.Fatalf("NewFakeAgent: %v", err)
	}
	reg.RegisterAgent("myapp.agents.sub", fakeSub)

	cfg := &SequentialAgentConfig{
		BaseAgentConfig: BaseAgentConfig{
			Name: "root",
			SubAgentEntries: []SubAgentEntry{
				{Ref: &AgentRefConfig{Code: "myapp.agents.sub"}},
			},
		},
	}

	a, err := Build(context.Background(), cfg, reg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(a.SubAgents()) != 1 {
		t.Fatalf("expected 1 sub-agent, got %d", len(a.SubAgents()))
	}
	if a.SubAgents()[0].Name() != "code-sub" {
		t.Errorf("expected name %q, got %q", "code-sub", a.SubAgents()[0].Name())
	}
}

// TestBuild_LLMAgent_WithModelCode verifies modelCode resolution in Build.
func TestBuild_LLMAgent_WithModelCode(t *testing.T) {
	reg := testRegistry()
	fakeLLM := testutil.NewFakeLLM()
	reg.RegisterModelCode("myapp.models.custom", func(args map[string]any) (model.LLM, error) {
		return fakeLLM, nil
	})

	cfg := &LLMAgentConfig{
		BaseAgentConfig: BaseAgentConfig{Name: "code-model-bot"},
		ModelCode:       &CodeConfig{Name: "myapp.models.custom"},
		Instruction:     "hi",
	}

	a, err := Build(context.Background(), cfg, reg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if a.Name() != "code-model-bot" {
		t.Errorf("expected name %q, got %q", "code-model-bot", a.Name())
	}
}

// TestBuild_LLMAgent_ModelAndModelCodeError verifies Build rejects both.
func TestBuild_LLMAgent_ModelAndModelCodeError(t *testing.T) {
	cfg := &LLMAgentConfig{
		BaseAgentConfig: BaseAgentConfig{Name: "bad"},
		Model:           "mock/fast",
		ModelCode:       &CodeConfig{Name: "myapp.models.custom"},
		Instruction:     "hi",
	}
	_, err := Build(context.Background(), cfg, testRegistry())
	if err == nil {
		t.Fatal("expected error when both model and modelCode set")
	}
}

// TestBuild_LLMAgent_WithCallbacks verifies callbacks are resolved and wired.
func TestBuild_LLMAgent_WithCallbacks(t *testing.T) {
	reg := testRegistry()
	reg.RegisterBeforeModelCallback("my.cb", func(ctx agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
		return nil, nil
	})

	cfg := &LLMAgentConfig{
		BaseAgentConfig:      BaseAgentConfig{Name: "cb-bot"},
		Model:                "mock/fast",
		Instruction:          "hi",
		BeforeModelCallbacks: []CodeConfig{{Name: "my.cb"}},
	}

	a, err := Build(context.Background(), cfg, reg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if a.Name() != "cb-bot" {
		t.Errorf("expected name %q, got %q", "cb-bot", a.Name())
	}
}

// TestBuild_LLMAgent_WithOutputKey verifies OutputKey is passed through.
func TestBuild_LLMAgent_WithOutputKey(t *testing.T) {
	cfg := &LLMAgentConfig{
		BaseAgentConfig: BaseAgentConfig{Name: "key-bot"},
		Model:           "mock/fast",
		Instruction:     "hi",
		OutputKey:       "result",
		IncludeContents: "none",
	}
	a, err := Build(context.Background(), cfg, testRegistry())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if a.Name() != "key-bot" {
		t.Errorf("expected name %q, got %q", "key-bot", a.Name())
	}
}

// TestBuild_LLMAgent_WithSchemaRefs verifies that named schema references are resolved.
func TestBuild_LLMAgent_WithSchemaRefs(t *testing.T) {
	reg := testRegistry()
	inputSch := &genai.Schema{Type: genai.TypeObject, Description: "input"}
	outputSch := &genai.Schema{Type: genai.TypeString, Description: "output"}
	reg.RegisterSchema("myapp.schemas.input", StaticSchema(inputSch))
	reg.RegisterSchema("myapp.schemas.output", StaticSchema(outputSch))

	cfg := &LLMAgentConfig{
		BaseAgentConfig: BaseAgentConfig{Name: "schema-bot"},
		Model:           "mock/fast",
		InputSchema:     &SchemaRef{Ref: &CodeConfig{Name: "myapp.schemas.input"}},
		OutputSchema:    &SchemaRef{Ref: &CodeConfig{Name: "myapp.schemas.output"}},
	}

	a, err := Build(context.Background(), cfg, reg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if a.Name() != "schema-bot" {
		t.Errorf("expected name %q, got %q", "schema-bot", a.Name())
	}
}

// TestBuild_LLMAgent_WithInlineSchema verifies that inline schemas are passed through.
func TestBuild_LLMAgent_WithInlineSchema(t *testing.T) {
	cfg := &LLMAgentConfig{
		BaseAgentConfig: BaseAgentConfig{Name: "inline-schema-bot"},
		Model:           "mock/fast",
		InputSchema: &SchemaRef{Inline: &genai.Schema{
			Type:        genai.TypeObject,
			Description: "inline input",
		}},
		OutputSchema: &SchemaRef{Inline: &genai.Schema{
			Type:        genai.TypeString,
			Description: "inline output",
		}},
	}

	a, err := Build(context.Background(), cfg, testRegistry())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if a.Name() != "inline-schema-bot" {
		t.Errorf("expected name %q, got %q", "inline-schema-bot", a.Name())
	}
}

// TestBuild_LLMAgent_WithSchemaShorthand verifies that string-shorthand schema refs work.
func TestBuild_LLMAgent_WithSchemaShorthand(t *testing.T) {
	reg := testRegistry()
	reg.RegisterSchema("myapp.schemas.input", StaticSchema(&genai.Schema{Type: genai.TypeObject}))

	cfg := &LLMAgentConfig{
		BaseAgentConfig: BaseAgentConfig{Name: "shorthand-bot"},
		Model:           "mock/fast",
		InputSchema:     &SchemaRef{Ref: &CodeConfig{Name: "myapp.schemas.input"}},
	}

	a, err := Build(context.Background(), cfg, reg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if a.Name() != "shorthand-bot" {
		t.Errorf("expected name %q, got %q", "shorthand-bot", a.Name())
	}
}

// TestBuild_LLMAgent_MissingSchemaRef verifies error for unregistered schema reference.
func TestBuild_LLMAgent_MissingSchemaRef(t *testing.T) {
	cfg := &LLMAgentConfig{
		BaseAgentConfig: BaseAgentConfig{Name: "missing-schema-bot"},
		Model:           "mock/fast",
		InputSchema:     &SchemaRef{Ref: &CodeConfig{Name: "myapp.schemas.missing"}},
	}

	_, err := Build(context.Background(), cfg, testRegistry())
	if err == nil {
		t.Fatal("expected error for missing schema registration, got nil")
	}
}

// TestRegistry_SchemaResolution verifies RegisterSchema and ResolveSchema with StaticSchema.
func TestRegistry_SchemaResolution(t *testing.T) {
	reg := NewRegistry()
	schema := &genai.Schema{Type: genai.TypeObject, Description: "test"}
	reg.RegisterSchema("myapp.test", StaticSchema(schema))

	got, err := reg.ResolveSchema("myapp.test", nil)
	if err != nil {
		t.Fatalf("ResolveSchema: %v", err)
	}
	if got.Type != genai.TypeObject {
		t.Errorf("Type: got %v, want %v", got.Type, genai.TypeObject)
	}
	if got.Description != "test" {
		t.Errorf("Description: got %q", got.Description)
	}

	_, err = reg.ResolveSchema("myapp.missing", nil)
	if err == nil {
		t.Fatal("expected error for missing schema, got nil")
	}
}

// TestBuild_Sequential_WithCallbacks verifies agent-level callbacks.
func TestBuild_Sequential_WithCallbacks(t *testing.T) {
	reg := testRegistry()
	reg.RegisterBeforeAgentCallback("seq.cb", func(ctx agent.CallbackContext) (*genai.Content, error) {
		return nil, nil
	})

	cfg := &SequentialAgentConfig{
		BaseAgentConfig: BaseAgentConfig{
			Name:                 "seq-cb",
			BeforeAgentCallbacks: []CodeConfig{{Name: "seq.cb"}},
			SubAgentEntries: []SubAgentEntry{
				{Inline: &LLMAgentConfig{BaseAgentConfig: BaseAgentConfig{Name: "child"}, Model: "mock/fast"}},
			},
		},
	}

	a, err := Build(context.Background(), cfg, reg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if a.Name() != "seq-cb" {
		t.Errorf("expected name %q, got %q", "seq-cb", a.Name())
	}
}

// TestBuildApp_ReturnsAgent verifies BuildApp returns a non-nil agent for an LLM config.
func TestBuildApp_ReturnsAgent(t *testing.T) {
	appCfg := &AppConfig{
		AgentConfig: &LLMAgentConfig{
			BaseAgentConfig: BaseAgentConfig{Name: "app-bot"},
			Model:           "mock/fast",
		},
	}

	a, runCfg, liveRunCfg, cacheCfg, err := BuildApp(context.Background(), appCfg, testRegistry())
	if err != nil {
		t.Fatalf("BuildApp: %v", err)
	}
	if a == nil {
		t.Fatal("expected non-nil agent")
	}
	if a.Name() != "app-bot" {
		t.Errorf("expected name %q, got %q", "app-bot", a.Name())
	}
	if runCfg != nil {
		t.Errorf("expected nil runCfg, got %+v", runCfg)
	}
	if liveRunCfg != nil {
		t.Errorf("expected nil liveRunCfg, got %+v", liveRunCfg)
	}
	if cacheCfg != nil {
		t.Errorf("expected nil cacheCfg, got %+v", cacheCfg)
	}
}

// TestBuildApp_ReturnsRunConfig verifies BuildApp translates RunConfig to agent.RunConfig.
func TestBuildApp_ReturnsRunConfig(t *testing.T) {
	appCfg := &AppConfig{
		AgentConfig: &LLMAgentConfig{
			BaseAgentConfig: BaseAgentConfig{Name: "run-cfg-bot"},
			Model:           "mock/fast",
		},
		RunConfig: &RunConfig{
			StreamingMode: StreamingModeSSE,
			SaveLiveBlob:  true,
		},
	}

	a, runCfg, liveRunCfg, cacheCfg, err := BuildApp(context.Background(), appCfg, testRegistry())
	if err != nil {
		t.Fatalf("BuildApp: %v", err)
	}
	if a.Name() != "run-cfg-bot" {
		t.Errorf("expected name %q, got %q", "run-cfg-bot", a.Name())
	}
	if runCfg == nil {
		t.Fatal("expected non-nil runCfg")
	}
	if runCfg.StreamingMode != agent.StreamingModeSSE {
		t.Errorf("StreamingMode: got %v, want %v", runCfg.StreamingMode, agent.StreamingModeSSE)
	}
	if !runCfg.SaveInputBlobsAsArtifacts {
		t.Errorf("SaveInputBlobsAsArtifacts: got false, want true")
	}
	if liveRunCfg != nil {
		t.Errorf("expected nil liveRunCfg, got %+v", liveRunCfg)
	}
	if cacheCfg != nil {
		t.Errorf("expected nil cacheCfg, got %+v", cacheCfg)
	}
}

// TestBuildApp_NilRunConfig verifies BuildApp returns nil *agent.RunConfig when input has none.
func TestBuildApp_NilRunConfig(t *testing.T) {
	appCfg := &AppConfig{
		AgentConfig: &LLMAgentConfig{
			BaseAgentConfig: BaseAgentConfig{Name: "nil-run-bot"},
			Model:           "mock/fast",
		},
	}

	_, runCfg, _, _, err := BuildApp(context.Background(), appCfg, testRegistry())
	if err != nil {
		t.Fatalf("BuildApp: %v", err)
	}
	if runCfg != nil {
		t.Errorf("expected nil runCfg, got %+v", runCfg)
	}
}

// TestBuildApp_ReturnsLiveRunConfig verifies BuildApp translates LiveRunConfig to agent.LiveRunConfig.
func TestBuildApp_ReturnsLiveRunConfig(t *testing.T) {
	appCfg := &AppConfig{
		AgentConfig: &LLMAgentConfig{
			BaseAgentConfig: BaseAgentConfig{Name: "live-run-cfg-bot"},
			Model:           "mock/fast",
		},
		LiveRunConfig: &LiveRunConfig{
			MaxLLMCalls: 750,
		},
	}

	_, _, liveRunCfg, _, err := BuildApp(context.Background(), appCfg, testRegistry())
	if err != nil {
		t.Fatalf("BuildApp: %v", err)
	}
	if liveRunCfg == nil {
		t.Fatal("expected non-nil liveRunCfg")
	}
	if liveRunCfg.MaxLLMCalls != 750 {
		t.Errorf("MaxLLMCalls: got %d, want 750", liveRunCfg.MaxLLMCalls)
	}
}

// TestBuildApp_ReturnsContextCacheConfig verifies BuildApp passes through ContextCacheConfig.
func TestBuildApp_ReturnsContextCacheConfig(t *testing.T) {
	appCfg := &AppConfig{
		AgentConfig: &LLMAgentConfig{
			BaseAgentConfig: BaseAgentConfig{Name: "cache-bot"},
			Model:           "mock/fast",
		},
		ContextCacheConfig: &ContextCacheConfig{
			CacheIntervals: 5,
			TTLSeconds:     600,
			MinTokens:      100,
		},
	}

	_, _, _, cacheCfg, err := BuildApp(context.Background(), appCfg, testRegistry())
	if err != nil {
		t.Fatalf("BuildApp: %v", err)
	}
	if cacheCfg == nil {
		t.Fatal("expected non-nil cacheCfg")
	}
	if cacheCfg.CacheIntervals != 5 {
		t.Errorf("CacheIntervals: got %d, want 5", cacheCfg.CacheIntervals)
	}
	if cacheCfg.TTLSeconds != 600 {
		t.Errorf("TTLSeconds: got %d, want 600", cacheCfg.TTLSeconds)
	}
	if cacheCfg.MinTokens != 100 {
		t.Errorf("MinTokens: got %d, want 100", cacheCfg.MinTokens)
	}
}

// TestBuild_LLMAgent_WithRichGenerateConfig verifies that Build handles a realistic
// mixed GenerateConfig without error.
func TestBuild_LLMAgent_WithRichGenerateConfig(t *testing.T) {
	cfg := &LLMAgentConfig{
		BaseAgentConfig: BaseAgentConfig{Name: "rich-bot"},
		Model:           "mock/fast",
		GenerateConfig: map[string]any{
			"temperature": 0.7,
			"safetySettings": []any{
				map[string]any{"category": "HARM_CATEGORY_HARASSMENT", "threshold": "BLOCK_LOW_AND_ABOVE"},
			},
			"thinkingConfig":    map[string]any{"includeThoughts": true},
			"systemInstruction": "Be helpful",
		},
	}

	a, err := Build(context.Background(), cfg, testRegistry())
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a == nil {
		t.Fatal("Build returned nil agent")
	}
	if a.Name() != "rich-bot" {
		t.Errorf("expected agent name %q, got %q", "rich-bot", a.Name())
	}
}

// TestBuildAppWithPath_SubAgentResolution verifies BuildAppWithPath still resolves sub-agent config_path refs.
func TestBuildAppWithPath_SubAgentResolution(t *testing.T) {
	dir := t.TempDir()
	subContent := `
name: sub
agent_class: LlmAgent
model: mock/fast
`
	subPath := filepath.Join(dir, "sub.yaml")
	_ = os.WriteFile(subPath, []byte(subContent), 0644)

	appCfg := &AppConfig{
		AgentConfig: &SequentialAgentConfig{
			BaseAgentConfig: BaseAgentConfig{
				Name: "root",
				SubAgentEntries: []SubAgentEntry{
					{Ref: &AgentRefConfig{ConfigPath: "sub.yaml"}},
				},
			},
		},
	}

	rootPath := filepath.Join(dir, "root.yaml")
	a, _, _, _, err := BuildAppWithPath(context.Background(), appCfg, testRegistry(), rootPath)
	if err != nil {
		t.Fatalf("BuildAppWithPath: %v", err)
	}
	if len(a.SubAgents()) != 1 {
		t.Fatalf("expected 1 sub-agent, got %d", len(a.SubAgents()))
	}
	if a.SubAgents()[0].Name() != "sub" {
		t.Errorf("expected sub-agent name %q, got %q", "sub", a.SubAgents()[0].Name())
	}
}
