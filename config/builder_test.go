package config_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ieshan/adk-go-pkg/config"
	"github.com/ieshan/adk-go-pkg/prompt"
	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/genai"
)

// testRegistry returns a *config.Registry pre-loaded with a "mock" model prefix and
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

	a, err := config.BuildWithPath(context.Background(), cfg, testRegistry(), nil, "")
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a == nil {
		t.Fatal("Build returned nil agent")
	}
	if a.Name() != "chat-bot" {
		t.Errorf("got %q, want %q", a.Name(), "chat-bot")
	}
}

// TestBuild_SequentialAgent verifies that a "sequential" agent with two LLM
// sub-agents is created correctly and reports the correct sub-agent count.
func TestBuild_SequentialAgent(t *testing.T) {
	cfg := &config.SequentialAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{
			Name: "pipeline",
			SubAgentEntries: []config.SubAgentEntry{
				{Inline: &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "step-1"}, Model: "mock/fast"}},
				{Inline: &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "step-2"}, Model: "mock/fast"}},
			},
		},
	}

	a, err := config.BuildWithPath(context.Background(), cfg, testRegistry(), nil, "")
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a.Name() != "pipeline" {
		t.Errorf("got %q, want %q", a.Name(), "pipeline")
	}
	if len(a.SubAgents()) != 2 {
		t.Errorf("got %d sub-agents, want 2", len(a.SubAgents()))
	}
}

// TestBuild_ParallelAgent verifies that a "parallel" agent with two sub-agents
// is constructed without error.
func TestBuild_ParallelAgent(t *testing.T) {
	cfg := &config.ParallelAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{
			Name: "fan-out",
			SubAgentEntries: []config.SubAgentEntry{
				{Inline: &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "worker-a"}, Model: "mock/fast"}},
				{Inline: &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "worker-b"}, Model: "mock/fast"}},
			},
		},
	}

	a, err := config.BuildWithPath(context.Background(), cfg, testRegistry(), nil, "")
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a.Name() != "fan-out" {
		t.Errorf("got %q, want %q", a.Name(), "fan-out")
	}
	if len(a.SubAgents()) != 2 {
		t.Errorf("got %d sub-agents, want 2", len(a.SubAgents()))
	}
}

// TestBuild_LoopAgent verifies that a "loop" agent is built with a positive
// MaxIterations value without error.
func TestBuild_LoopAgent(t *testing.T) {
	cfg := &config.LoopAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{
			Name: "refiner",
			SubAgentEntries: []config.SubAgentEntry{
				{Inline: &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "inner"}, Model: "mock/fast"}},
			},
		},
		MaxIterations: 3,
	}

	a, err := config.BuildWithPath(context.Background(), cfg, testRegistry(), nil, "")
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a.Name() != "refiner" {
		t.Errorf("got %q, want %q", a.Name(), "refiner")
	}
}

// TestBuild_NestedTree verifies a multi-level hierarchy: a root LLM agent that
// has a sequential sub-agent which itself has two LLM children.
func TestBuild_NestedTree(t *testing.T) {
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{
			Name: "root",
			SubAgentEntries: []config.SubAgentEntry{
				{Inline: &config.SequentialAgentConfig{
					BaseAgentConfig: config.BaseAgentConfig{
						Name: "seq",
						SubAgentEntries: []config.SubAgentEntry{
							{Inline: &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "child-a"}, Model: "mock/fast"}},
							{Inline: &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "child-b"}, Model: "mock/fast"}},
						},
					},
				}},
			},
		},
		Model: "mock/fast",
	}

	a, err := config.BuildWithPath(context.Background(), cfg, testRegistry(), nil, "")
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a.Name() != "root" {
		t.Errorf("got %q, want %q", a.Name(), "root")
	}
	if len(a.SubAgents()) != 1 {
		t.Fatalf("got %d sub-agents under root, want 1", len(a.SubAgents()))
	}
	seq := a.SubAgents()[0]
	if seq.Name() != "seq" {
		t.Errorf("got %q, want %q", seq.Name(), "seq")
	}
	if len(seq.SubAgents()) != 2 {
		t.Errorf("got %d children under seq, want 2", len(seq.SubAgents()))
	}
}

// TestBuild_NilConfig verifies that Build returns an error for nil
func TestBuild_NilConfig(t *testing.T) {
	_, err := config.BuildWithPath(context.Background(), nil, testRegistry(), nil, "")
	if err == nil {
		t.Fatal("got nil, want error for nil config")
	}
}

// TestBuild_ModelNotFound verifies that Build returns an error when the model
// prefix has no registered factory.
func TestBuild_ModelNotFound(t *testing.T) {
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{Name: "bot"},
		Model:           "unregistered/gpt-x",
	}
	_, err := config.BuildWithPath(context.Background(), cfg, testRegistry(), nil, "")
	if err == nil {
		t.Fatal("got nil, want error for unregistered model prefix")
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
	_, err := config.BuildWithPath(context.Background(), cfg, testRegistry(), nil, "")
	if err == nil {
		t.Fatal("got nil, want error for unregistered tool")
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

	a, err := config.BuildWithPath(context.Background(), cfg, testRegistry(), nil, "")
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a.Name() != "transfer-bot" {
		t.Errorf("got %q, want %q", a.Name(), "transfer-bot")
	}
}

// TestBuild_LLMAgent_DefaultTransferFlags verifies defaults are false when omitted.
func TestBuild_LLMAgent_DefaultTransferFlags(t *testing.T) {
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{Name: "default-bot"},
		Model:           "mock/fast",
	}

	a, err := config.BuildWithPath(context.Background(), cfg, testRegistry(), nil, "")
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a.Name() != "default-bot" {
		t.Errorf("got %q, want %q", a.Name(), "default-bot")
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
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	a, runCfg, liveRunCfg, cacheCfg, err := config.LoadAndBuild(context.Background(), root, "agent.yaml", testRegistry())
	if err != nil {
		t.Fatalf("LoadAndBuild returned unexpected error: %v", err)
	}
	if a.Name() != "file-agent" {
		t.Errorf("got %q, want %q", a.Name(), "file-agent")
	}
	if runCfg != nil {
		t.Errorf("got %+v, want nil runCfg when absent", runCfg)
	}
	if liveRunCfg != nil {
		t.Errorf("got %+v, want nil liveRunCfg when absent", liveRunCfg)
	}
	if cacheCfg != nil {
		t.Errorf("got %+v, want nil cacheCfg when absent", cacheCfg)
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
	if err := os.WriteFile(subPath, []byte(subContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	cfg := &config.SequentialAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{
			Name: "root",
			SubAgentEntries: []config.SubAgentEntry{
				{Ref: &config.AgentRefConfig{ConfigPath: "sub.yaml"}},
			},
		},
	}

	a, err := config.BuildWithPath(context.Background(), cfg, testRegistry(), root, "")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(a.SubAgents()) != 1 {
		t.Fatalf("got %d sub-agents, want 1", len(a.SubAgents()))
	}
	if a.SubAgents()[0].Name() != "sub" {
		t.Errorf("got %q, want %q", a.SubAgents()[0].Name(), "sub")
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

	cfg := &config.SequentialAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{
			Name: "root",
			SubAgentEntries: []config.SubAgentEntry{
				{Ref: &config.AgentRefConfig{Code: "myapp.agents.sub"}},
			},
		},
	}

	a, err := config.BuildWithPath(context.Background(), cfg, reg, nil, "")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(a.SubAgents()) != 1 {
		t.Fatalf("got %d sub-agents, want 1", len(a.SubAgents()))
	}
	if a.SubAgents()[0].Name() != "code-sub" {
		t.Errorf("got %q, want %q", a.SubAgents()[0].Name(), "code-sub")
	}
}

// TestBuild_LLMAgent_WithModelCode verifies modelCode resolution in Build.
func TestBuild_LLMAgent_WithModelCode(t *testing.T) {
	reg := testRegistry()
	fakeLLM := testutil.NewFakeLLM()
	reg.RegisterModelCode("myapp.models.custom", func(args map[string]any) (model.LLM, error) {
		return fakeLLM, nil
	})

	cfg := &config.LLMAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{Name: "code-model-bot"},
		ModelCode:       &config.CodeConfig{Name: "myapp.models.custom"},
		Instruction:     "hi",
	}

	a, err := config.BuildWithPath(context.Background(), cfg, reg, nil, "")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if a.Name() != "code-model-bot" {
		t.Errorf("got %q, want %q", a.Name(), "code-model-bot")
	}
}

// TestBuild_LLMAgent_ModelAndModelCodeError verifies Build rejects both.
func TestBuild_LLMAgent_ModelAndModelCodeError(t *testing.T) {
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{Name: "bad"},
		Model:           "mock/fast",
		ModelCode:       &config.CodeConfig{Name: "myapp.models.custom"},
		Instruction:     "hi",
	}
	_, err := config.BuildWithPath(context.Background(), cfg, testRegistry(), nil, "")
	if err == nil {
		t.Fatal("got nil, want error when both model and modelCode set")
	}
}

// TestBuild_LLMAgent_WithCallbacks verifies callbacks are resolved and wired.
func TestBuild_LLMAgent_WithCallbacks(t *testing.T) {
	reg := testRegistry()
	reg.RegisterBeforeModelCallback("my.cb", func(ctx agent.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
		return nil, nil
	})

	cfg := &config.LLMAgentConfig{
		BaseAgentConfig:      config.BaseAgentConfig{Name: "cb-bot"},
		Model:                "mock/fast",
		Instruction:          "hi",
		BeforeModelCallbacks: []config.CodeConfig{{Name: "my.cb"}},
	}

	a, err := config.BuildWithPath(context.Background(), cfg, reg, nil, "")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if a.Name() != "cb-bot" {
		t.Errorf("got %q, want %q", a.Name(), "cb-bot")
	}
}

// TestBuild_LLMAgent_WithOutputKey verifies OutputKey is passed through.
func TestBuild_LLMAgent_WithOutputKey(t *testing.T) {
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{Name: "key-bot"},
		Model:           "mock/fast",
		Instruction:     "hi",
		OutputKey:       "result",
		IncludeContents: "none",
	}
	a, err := config.BuildWithPath(context.Background(), cfg, testRegistry(), nil, "")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if a.Name() != "key-bot" {
		t.Errorf("got %q, want %q", a.Name(), "key-bot")
	}
}

// TestBuild_LLMAgent_WithSchemaRefs verifies that named schema references are resolved.
func TestBuild_LLMAgent_WithSchemaRefs(t *testing.T) {
	reg := testRegistry()
	inputSch := &genai.Schema{Type: genai.TypeObject, Description: "input"}
	outputSch := &genai.Schema{Type: genai.TypeString, Description: "output"}
	reg.RegisterSchema("myapp.schemas.input", config.StaticSchema(inputSch))
	reg.RegisterSchema("myapp.schemas.output", config.StaticSchema(outputSch))

	cfg := &config.LLMAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{Name: "schema-bot"},
		Model:           "mock/fast",
		InputSchema:     &config.SchemaRef{Ref: &config.CodeConfig{Name: "myapp.schemas.input"}},
		OutputSchema:    &config.SchemaRef{Ref: &config.CodeConfig{Name: "myapp.schemas.output"}},
	}

	a, err := config.BuildWithPath(context.Background(), cfg, reg, nil, "")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if a.Name() != "schema-bot" {
		t.Errorf("got %q, want %q", a.Name(), "schema-bot")
	}
}

// TestBuild_LLMAgent_WithInlineSchema verifies that inline schemas are passed through.
func TestBuild_LLMAgent_WithInlineSchema(t *testing.T) {
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{Name: "inline-schema-bot"},
		Model:           "mock/fast",
		InputSchema: &config.SchemaRef{Inline: &genai.Schema{
			Type:        genai.TypeObject,
			Description: "inline input",
		}},
		OutputSchema: &config.SchemaRef{Inline: &genai.Schema{
			Type:        genai.TypeString,
			Description: "inline output",
		}},
	}

	a, err := config.BuildWithPath(context.Background(), cfg, testRegistry(), nil, "")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if a.Name() != "inline-schema-bot" {
		t.Errorf("got %q, want %q", a.Name(), "inline-schema-bot")
	}
}

// TestBuild_LLMAgent_WithSchemaShorthand verifies that string-shorthand schema refs work.
func TestBuild_LLMAgent_WithSchemaShorthand(t *testing.T) {
	reg := testRegistry()
	reg.RegisterSchema("myapp.schemas.input", config.StaticSchema(&genai.Schema{Type: genai.TypeObject}))

	cfg := &config.LLMAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{Name: "shorthand-bot"},
		Model:           "mock/fast",
		InputSchema:     &config.SchemaRef{Ref: &config.CodeConfig{Name: "myapp.schemas.input"}},
	}

	a, err := config.BuildWithPath(context.Background(), cfg, reg, nil, "")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if a.Name() != "shorthand-bot" {
		t.Errorf("got %q, want %q", a.Name(), "shorthand-bot")
	}
}

// TestBuild_LLMAgent_MissingSchemaRef verifies error for unregistered schema reference.
func TestBuild_LLMAgent_MissingSchemaRef(t *testing.T) {
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{Name: "missing-schema-bot"},
		Model:           "mock/fast",
		InputSchema:     &config.SchemaRef{Ref: &config.CodeConfig{Name: "myapp.schemas.missing"}},
	}

	_, err := config.BuildWithPath(context.Background(), cfg, testRegistry(), nil, "")
	if err == nil {
		t.Fatal("got nil, want error for missing schema registration")
	}
}

// TestRegistry_SchemaResolution verifies RegisterSchema and ResolveSchema with StaticSchema.
func TestRegistry_SchemaResolution(t *testing.T) {
	reg := config.NewRegistry()
	schema := &genai.Schema{Type: genai.TypeObject, Description: "test"}
	reg.RegisterSchema("myapp.test", config.StaticSchema(schema))

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
		t.Fatal("got nil, want error for missing schema")
	}
}

// TestBuild_Sequential_WithCallbacks verifies agent-level callbacks.
func TestBuild_Sequential_WithCallbacks(t *testing.T) {
	reg := testRegistry()
	reg.RegisterBeforeAgentCallback("seq.cb", func(ctx agent.Context) (*genai.Content, error) {
		return nil, nil
	})

	cfg := &config.SequentialAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{
			Name:                 "seq-cb",
			BeforeAgentCallbacks: []config.CodeConfig{{Name: "seq.cb"}},
			SubAgentEntries: []config.SubAgentEntry{
				{Inline: &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "child"}, Model: "mock/fast"}},
			},
		},
	}

	a, err := config.BuildWithPath(context.Background(), cfg, reg, nil, "")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if a.Name() != "seq-cb" {
		t.Errorf("got %q, want %q", a.Name(), "seq-cb")
	}
}

// TestBuildApp_ReturnsAgent verifies BuildApp returns a non-nil agent for an LLM config.
func TestBuildApp_ReturnsAgent(t *testing.T) {
	appCfg := &config.AppConfig{
		AgentConfig: &config.LLMAgentConfig{
			BaseAgentConfig: config.BaseAgentConfig{Name: "app-bot"},
			Model:           "mock/fast",
		},
	}

	a, runCfg, liveRunCfg, cacheCfg, err := config.BuildAppWithPath(context.Background(), appCfg, testRegistry(), nil, "")
	if err != nil {
		t.Fatalf("BuildApp: %v", err)
	}
	if a == nil {
		t.Fatal("got nil agent, want non-nil agent")
	}
	if a.Name() != "app-bot" {
		t.Errorf("got %q, want %q", a.Name(), "app-bot")
	}
	if runCfg != nil {
		t.Errorf("got %+v, want nil runCfg", runCfg)
	}
	if liveRunCfg != nil {
		t.Errorf("got %+v, want nil liveRunCfg", liveRunCfg)
	}
	if cacheCfg != nil {
		t.Errorf("got %+v, want nil cacheCfg", cacheCfg)
	}
}

// TestBuildApp_ReturnsRunConfig verifies BuildApp translates RunConfig to agent.RunConfig.
func TestBuildApp_ReturnsRunConfig(t *testing.T) {
	appCfg := &config.AppConfig{
		AgentConfig: &config.LLMAgentConfig{
			BaseAgentConfig: config.BaseAgentConfig{Name: "run-cfg-bot"},
			Model:           "mock/fast",
		},
		RunConfig: &config.RunConfig{
			StreamingMode: config.StreamingModeSSE,
			SaveLiveBlob:  true,
		},
	}

	a, runCfg, liveRunCfg, cacheCfg, err := config.BuildAppWithPath(context.Background(), appCfg, testRegistry(), nil, "")
	if err != nil {
		t.Fatalf("BuildApp: %v", err)
	}
	if a.Name() != "run-cfg-bot" {
		t.Errorf("got %q, want %q", a.Name(), "run-cfg-bot")
	}
	if runCfg == nil {
		t.Fatal("got nil runCfg, want non-nil runCfg")
	}
	if runCfg.StreamingMode != agent.StreamingModeSSE {
		t.Errorf("StreamingMode: got %v, want %v", runCfg.StreamingMode, agent.StreamingModeSSE)
	}
	if !runCfg.SaveInputBlobsAsArtifacts {
		t.Errorf("SaveInputBlobsAsArtifacts: got false, want true")
	}
	if liveRunCfg != nil {
		t.Errorf("got %+v, want nil liveRunCfg", liveRunCfg)
	}
	if cacheCfg != nil {
		t.Errorf("got %+v, want nil cacheCfg", cacheCfg)
	}
}

// TestBuildApp_ReturnsLiveRunConfig verifies BuildApp translates LiveRunConfig to agent.LiveRunConfig.
func TestBuildApp_ReturnsLiveRunConfig(t *testing.T) {
	appCfg := &config.AppConfig{
		AgentConfig: &config.LLMAgentConfig{
			BaseAgentConfig: config.BaseAgentConfig{Name: "live-run-cfg-bot"},
			Model:           "mock/fast",
		},
		LiveRunConfig: &config.LiveRunConfig{
			MaxLLMCalls: 750,
		},
	}

	_, _, liveRunCfg, _, err := config.BuildAppWithPath(context.Background(), appCfg, testRegistry(), nil, "")
	if err != nil {
		t.Fatalf("BuildApp: %v", err)
	}
	if liveRunCfg == nil {
		t.Fatal("got nil liveRunCfg, want non-nil liveRunCfg")
	}
	if liveRunCfg.MaxLLMCalls != 750 {
		t.Errorf("MaxLLMCalls: got %d, want 750", liveRunCfg.MaxLLMCalls)
	}
}

// TestBuildApp_ReturnsContextCacheConfig verifies BuildApp passes through ContextCacheConfig.
func TestBuildApp_ReturnsContextCacheConfig(t *testing.T) {
	appCfg := &config.AppConfig{
		AgentConfig: &config.LLMAgentConfig{
			BaseAgentConfig: config.BaseAgentConfig{Name: "cache-bot"},
			Model:           "mock/fast",
		},
		ContextCacheConfig: &config.ContextCacheConfig{
			CacheIntervals: 5,
			TTLSeconds:     600,
			MinTokens:      100,
		},
	}

	_, _, _, cacheCfg, err := config.BuildAppWithPath(context.Background(), appCfg, testRegistry(), nil, "")
	if err != nil {
		t.Fatalf("BuildApp: %v", err)
	}
	if cacheCfg == nil {
		t.Fatal("got nil cacheCfg, want non-nil cacheCfg")
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
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{Name: "rich-bot"},
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

	a, err := config.BuildWithPath(context.Background(), cfg, testRegistry(), nil, "")
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a == nil {
		t.Fatal("Build returned nil agent")
	}
	if a.Name() != "rich-bot" {
		t.Errorf("got %q, want %q", a.Name(), "rich-bot")
	}
}

// TestBuild_LLMAgent_WithInstructionTemplate_Inline verifies that an inline
// InstructionTemplate is resolved and produces a non-nil agent.
func TestBuild_LLMAgent_WithInstructionTemplate_Inline(t *testing.T) {
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig:     config.BaseAgentConfig{Name: "tmpl-inline-bot"},
		Model:               "mock/fast",
		InstructionTemplate: &prompt.TemplateRef{Inline: "You are {{.Agent.Name}}."},
	}

	a, err := config.BuildWithPath(context.Background(), cfg, testRegistry(), nil, "")
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a == nil {
		t.Fatal("Build returned nil agent")
	}
	if a.Name() != "tmpl-inline-bot" {
		t.Errorf("got %q, want %q", a.Name(), "tmpl-inline-bot")
	}
}

// TestBuild_LLMAgent_WithInstructionTemplate_Name verifies that a named
// InstructionTemplate is resolved from the registry.
func TestBuild_LLMAgent_WithInstructionTemplate_Name(t *testing.T) {
	reg := testRegistry()
	tr := reg.TemplateRegistry()
	if err := tr.Register("greeting", "Hello from {{.Agent.Name}}."); err != nil {
		t.Fatalf("RegisterString: %v", err)
	}

	cfg := &config.LLMAgentConfig{
		BaseAgentConfig:     config.BaseAgentConfig{Name: "tmpl-name-bot"},
		Model:               "mock/fast",
		InstructionTemplate: &prompt.TemplateRef{Name: "greeting"},
	}

	a, err := config.BuildWithPath(context.Background(), cfg, reg, nil, "")
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a == nil {
		t.Fatal("Build returned nil agent")
	}
}

// TestBuild_LLMAgent_WithInstructionTemplate_Path verifies that a file-based
// InstructionTemplate is loaded and resolved.
func TestBuild_LLMAgent_WithInstructionTemplate_Path(t *testing.T) {
	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "instruction.tmpl")
	if err := os.WriteFile(tmplPath, []byte("You are a helpful assistant."), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	cfg := &config.LLMAgentConfig{
		BaseAgentConfig:     config.BaseAgentConfig{Name: "tmpl-path-bot"},
		Model:               "mock/fast",
		InstructionTemplate: &prompt.TemplateRef{Path: "instruction.tmpl"},
	}

	a, err := config.BuildWithPath(context.Background(), cfg, testRegistry(), root, "")
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a == nil {
		t.Fatal("Build returned nil agent")
	}
}

// TestBuild_LLMAgent_InstructionTemplatePrecedence verifies that
// InstructionTemplate takes precedence over Instruction.
func TestBuild_LLMAgent_InstructionTemplatePrecedence(t *testing.T) {
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig:     config.BaseAgentConfig{Name: "precedence-bot"},
		Model:               "mock/fast",
		Instruction:         "static instruction",
		InstructionTemplate: &prompt.TemplateRef{Inline: "templated instruction"},
	}

	a, err := config.BuildWithPath(context.Background(), cfg, testRegistry(), nil, "")
	if err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if a == nil {
		t.Fatal("Build returned nil agent")
	}
}

// TestBuild_LLMAgent_InvalidInstructionTemplate verifies that an invalid
// TemplateRef (multiple fields set) returns an error.
func TestBuild_LLMAgent_InvalidInstructionTemplate(t *testing.T) {
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{Name: "bad-tmpl-bot"},
		Model:           "mock/fast",
		InstructionTemplate: &prompt.TemplateRef{
			Inline: "inline",
			Name:   "name",
		},
	}

	_, err := config.BuildWithPath(context.Background(), cfg, testRegistry(), nil, "")
	if err == nil {
		t.Fatal("got nil, want error for invalid InstructionTemplate")
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
	if err := os.WriteFile(subPath, []byte(subContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	appCfg := &config.AppConfig{
		AgentConfig: &config.SequentialAgentConfig{
			BaseAgentConfig: config.BaseAgentConfig{
				Name: "root",
				SubAgentEntries: []config.SubAgentEntry{
					{Ref: &config.AgentRefConfig{ConfigPath: "sub.yaml"}},
				},
			},
		},
	}

	a, _, _, _, err := config.BuildAppWithPath(context.Background(), appCfg, testRegistry(), root, "root.yaml")
	if err != nil {
		t.Fatalf("BuildAppWithPath: %v", err)
	}
	if len(a.SubAgents()) != 1 {
		t.Fatalf("got %d sub-agents, want 1", len(a.SubAgents()))
	}
	if a.SubAgents()[0].Name() != "sub" {
		t.Errorf("got %q, want %q", a.SubAgents()[0].Name(), "sub")
	}
}

// TestBuildApp_RejectsBidiStreaming verifies that BuildAppWithPath returns an
// error when RunConfig.StreamingMode is set to StreamingModeBIDI, which is not
// supported by ADK-Go.
func TestBuildApp_RejectsBidiStreaming(t *testing.T) {
	t.Parallel()
	appCfg := &config.AppConfig{
		AgentConfig: &config.SequentialAgentConfig{
			BaseAgentConfig: config.BaseAgentConfig{
				Name: "bidi-root",
				SubAgentEntries: []config.SubAgentEntry{
					{Inline: &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "child"}, Model: "mock/fast"}},
				},
			},
		},
		RunConfig: &config.RunConfig{StreamingMode: config.StreamingModeBIDI},
	}

	_, _, _, _, err := config.BuildAppWithPath(context.Background(), appCfg, testRegistry(), nil, "")
	if err == nil {
		t.Fatal("got nil, want error for bidi streaming mode")
	}
}
