package config_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ieshan/adk-go-pkg/config"
	"github.com/ieshan/adk-go-pkg/prompt"
	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model"
)

func TestBuild_AgentTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cfg  config.AgentConfig
	}{
		{
			name: "llm",
			cfg: &config.LLMAgentConfig{
				BaseAgentConfig: config.BaseAgentConfig{Name: "chat-bot"},
				Model:           "mock/fast",
			},
		},
		{
			name: "sequential",
			cfg: &config.SequentialAgentConfig{
				BaseAgentConfig: config.BaseAgentConfig{
					Name: "pipeline",
					SubAgentEntries: []config.SubAgentEntry{
						{Inline: &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "step-1"}, Model: "mock/fast"}},
						{Inline: &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "step-2"}, Model: "mock/fast"}},
					},
				},
			},
		},
		{
			name: "parallel",
			cfg: &config.ParallelAgentConfig{
				BaseAgentConfig: config.BaseAgentConfig{
					Name: "fan-out",
					SubAgentEntries: []config.SubAgentEntry{
						{Inline: &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "worker-a"}, Model: "mock/fast"}},
						{Inline: &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "worker-b"}, Model: "mock/fast"}},
					},
				},
			},
		},
		{
			name: "loop",
			cfg: &config.LoopAgentConfig{
				BaseAgentConfig: config.BaseAgentConfig{
					Name: "refiner",
					SubAgentEntries: []config.SubAgentEntry{
						{Inline: &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "inner"}, Model: "mock/fast"}},
					},
				},
				MaxIterations: 3,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, err := config.BuildWithPath(context.Background(), tc.cfg, newTestRegistry(), nil, "")
			if err != nil {
				t.Fatalf("BuildWithPath: %v", err)
			}
			if a.Name() != tc.cfg.Name() {
				t.Errorf("Name: got %q, want %q", a.Name(), tc.cfg.Name())
			}
		})
	}
}

func TestBuild_NestedTree(t *testing.T) {
	t.Parallel()
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
	a, err := config.BuildWithPath(context.Background(), cfg, newTestRegistry(), nil, "")
	if err != nil {
		t.Fatalf("BuildWithPath: %v", err)
	}
	if len(a.SubAgents()) != 1 || len(a.SubAgents()[0].SubAgents()) != 2 {
		t.Errorf("tree shape: got root sub=%d, seq sub=%d", len(a.SubAgents()), len(a.SubAgents()[0].SubAgents()))
	}
}

func TestBuild_NilConfig(t *testing.T) {
	t.Parallel()
	if _, err := config.BuildWithPath(context.Background(), nil, newTestRegistry(), nil, ""); err == nil {
		t.Fatal("got nil, want error for nil config")
	}
}

func TestBuild_ModelResolution(t *testing.T) {
	t.Parallel()
	t.Run("unregistered prefix", func(t *testing.T) {
		t.Parallel()
		cfg := &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "bot"}, Model: "unregistered/x"}
		if _, err := config.BuildWithPath(context.Background(), cfg, newRegistryNoDefault(), nil, ""); err == nil {
			t.Fatal("got nil, want error for unregistered prefix")
		}
	})
	t.Run("unprefixed uses default", func(t *testing.T) {
		t.Parallel()
		cfg := &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "bot"}, Model: "gemini-2.5-flash"}
		if _, err := config.BuildWithPath(context.Background(), cfg, newTestRegistry(), nil, ""); err != nil {
			t.Fatalf("BuildWithPath: %v", err)
		}
	})
	t.Run("omitted model uses default", func(t *testing.T) {
		t.Parallel()
		cfg := &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "bot"}}
		if _, err := config.BuildWithPath(context.Background(), cfg, newTestRegistry(), nil, ""); err != nil {
			t.Fatalf("BuildWithPath: %v", err)
		}
	})
	t.Run("omitted model without default", func(t *testing.T) {
		t.Parallel()
		cfg := &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "bot"}}
		_, err := config.BuildWithPath(context.Background(), cfg, newRegistryNoDefault(), nil, "")
		if !errors.Is(err, config.ErrNoDefaultModel) {
			t.Fatalf("got %v, want ErrNoDefaultModel", err)
		}
	})
}

func TestBuild_ProgrammaticGuards(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		cfg    *config.LLMAgentConfig
		target error
	}{
		{
			name: "model and model_code",
			cfg:  &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "b"}, Model: "mock/fast", ModelCode: &config.CodeConfig{Name: "x"}},
		},
		{
			name: "model_args and model_code",
			cfg:  &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "b"}, ModelArgs: map[string]any{"x": 1}, ModelCode: &config.CodeConfig{Name: "x"}},
		},
		{
			name: "bad include_contents",
			cfg:  &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "b"}, Model: "mock/fast", IncludeContents: "bogus"},
		},
		{
			name:   "callback args",
			cfg:    &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "b"}, Model: "mock/fast", BeforeModelCallbacks: []config.CodeConfig{{Name: "cb", Args: map[string]any{"x": 1}}}},
			target: config.ErrCallbackArgs,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.BuildWithPath(context.Background(), tc.cfg, newTestRegistry(), nil, "")
			if err == nil {
				t.Fatal("got nil, want error")
			}
			if tc.target != nil && !errors.Is(err, tc.target) {
				t.Errorf("got %v, want %v", err, tc.target)
			}
		})
	}
}

func TestBuild_SubAgentRefs(t *testing.T) {
	t.Parallel()
	t.Run("code", func(t *testing.T) {
		t.Parallel()
		reg := newTestRegistry()
		sub, err := testutil.NewFakeAgent("code-sub")
		if err != nil {
			t.Fatalf("NewFakeAgent: %v", err)
		}
		reg.RegisterAgent("myapp.agents.sub", sub)
		cfg := &config.SequentialAgentConfig{
			BaseAgentConfig: config.BaseAgentConfig{
				Name:            "root",
				SubAgentEntries: []config.SubAgentEntry{{Ref: &config.AgentRefConfig{Code: "myapp.agents.sub"}}},
			},
		}
		a, err := config.BuildWithPath(context.Background(), cfg, reg, nil, "")
		if err != nil {
			t.Fatalf("BuildWithPath: %v", err)
		}
		if len(a.SubAgents()) != 1 || a.SubAgents()[0].Name() != "code-sub" {
			t.Errorf("sub-agents: got %v", a.SubAgents())
		}
	})
	t.Run("file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, dir, "sub.yaml", "name: sub\nagent_class: LlmAgent\nmodel: mock/fast\n")
		root := openRoot(t, dir)
		cfg := &config.SequentialAgentConfig{
			BaseAgentConfig: config.BaseAgentConfig{
				Name:            "root",
				SubAgentEntries: []config.SubAgentEntry{{Ref: &config.AgentRefConfig{ConfigPath: "sub.yaml"}}},
			},
		}
		a, err := config.BuildWithPath(context.Background(), cfg, newTestRegistry(), root, "")
		if err != nil {
			t.Fatalf("BuildWithPath: %v", err)
		}
		if len(a.SubAgents()) != 1 || a.SubAgents()[0].Name() != "sub" {
			t.Errorf("sub-agents: got %v", a.SubAgents())
		}
	})
}

func TestBuild_ModelCode(t *testing.T) {
	t.Parallel()
	reg := newTestRegistry()
	fakeLLM := testutil.NewFakeLLM().WithName("code-model")
	reg.RegisterModelCode("myapp.models.custom", func(map[string]any) (model.LLM, error) { return fakeLLM, nil })
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{Name: "code-model-bot"},
		ModelCode:       &config.CodeConfig{Name: "myapp.models.custom"},
	}
	if _, err := config.BuildWithPath(context.Background(), cfg, reg, nil, ""); err != nil {
		t.Fatalf("BuildWithPath: %v", err)
	}
}

func TestBuild_DescriptionsWired(t *testing.T) {
	t.Parallel()
	for _, cfg := range []config.AgentConfig{
		&config.SequentialAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "s", Description: "seq desc"}},
		&config.ParallelAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "p", Description: "par desc"}},
		&config.LoopAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "l", Description: "loop desc"}},
	} {
		a, err := config.BuildWithPath(context.Background(), cfg, newTestRegistry(), nil, "")
		if err != nil {
			t.Fatalf("BuildWithPath(%s): %v", cfg.Name(), err)
		}
		if a.Description() != cfg.Description() {
			t.Errorf("%s description: got %q, want %q", cfg.Name(), a.Description(), cfg.Description())
		}
	}
}

func TestBuildApp_RunConfigMapping(t *testing.T) {
	t.Parallel()
	maxCalls := 750
	appCfg := &config.AppConfig{
		AgentConfig: &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "bot"}, Model: "mock/fast"},
		RunConfig: &config.RunConfig{
			StreamingMode:             config.StreamingModeSSE,
			SaveInputBlobsAsArtifacts: true,
			SaveLiveBlob:              true,
			MaxLLMCalls:               &maxCalls,
		},
	}
	_, runCfg, liveCfg, _, err := config.BuildAppWithPath(context.Background(), appCfg, newTestRegistry(), nil, "")
	if err != nil {
		t.Fatalf("BuildAppWithPath: %v", err)
	}
	if runCfg == nil || runCfg.StreamingMode != agent.StreamingModeSSE || !runCfg.SaveInputBlobsAsArtifacts || runCfg.MaxLLMCalls != 750 {
		t.Errorf("RunConfig: got %+v", runCfg)
	}
	if liveCfg == nil || !liveCfg.SaveLiveBlob || liveCfg.MaxLLMCalls != 750 {
		t.Errorf("LiveRunConfig: got %+v", liveCfg)
	}
}

func TestBuildApp_MaxLLMCallsMapping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   *int
		want int
	}{
		{"unset", nil, 0},
		{"zero", new(0), -1},
		{"negative", new(-5), -1},
		{"positive", new(10), 10},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			appCfg := &config.AppConfig{
				AgentConfig: &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "bot"}, Model: "mock/fast"},
				RunConfig:   &config.RunConfig{MaxLLMCalls: tc.in},
			}
			_, runCfg, liveCfg, _, err := config.BuildAppWithPath(context.Background(), appCfg, newTestRegistry(), nil, "")
			if err != nil {
				t.Fatalf("BuildAppWithPath: %v", err)
			}
			if runCfg.MaxLLMCalls != tc.want {
				t.Errorf("RunConfig.MaxLLMCalls: got %d, want %d", runCfg.MaxLLMCalls, tc.want)
			}
			if liveCfg.MaxLLMCalls != tc.want {
				t.Errorf("LiveRunConfig.MaxLLMCalls: got %d, want %d", liveCfg.MaxLLMCalls, tc.want)
			}
		})
	}
}

func TestBuildApp_LiveConfigDerivedFromRunConfig(t *testing.T) {
	t.Parallel()
	appCfg := &config.AppConfig{
		AgentConfig: &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "bot"}, Model: "mock/fast"},
		RunConfig:   &config.RunConfig{StreamingMode: config.StreamingModeNone},
	}
	_, runCfg, liveCfg, _, err := config.BuildAppWithPath(context.Background(), appCfg, newTestRegistry(), nil, "")
	if err != nil {
		t.Fatalf("BuildAppWithPath: %v", err)
	}
	if runCfg == nil || liveCfg == nil {
		t.Fatalf("got run=%v live=%v, want both non-nil", runCfg, liveCfg)
	}
	if liveCfg.SaveLiveBlob || liveCfg.MaxLLMCalls != 0 {
		t.Errorf("LiveRunConfig: got %+v, want zero values", liveCfg)
	}
}

func TestBuildApp_AbsentRunConfig(t *testing.T) {
	t.Parallel()
	appCfg := &config.AppConfig{
		AgentConfig: &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "bot"}, Model: "mock/fast"},
	}
	_, runCfg, liveCfg, cacheCfg, err := config.BuildAppWithPath(context.Background(), appCfg, newTestRegistry(), nil, "")
	if err != nil {
		t.Fatalf("BuildAppWithPath: %v", err)
	}
	if runCfg != nil || liveCfg != nil || cacheCfg != nil {
		t.Errorf("got run=%v live=%v cache=%v, want all nil", runCfg, liveCfg, cacheCfg)
	}
}

func TestBuildApp_RejectsBidiStreaming(t *testing.T) {
	t.Parallel()
	appCfg := &config.AppConfig{
		AgentConfig: &config.SequentialAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "root"}},
		RunConfig:   &config.RunConfig{StreamingMode: config.StreamingModeBIDI},
	}
	_, _, _, _, err := config.BuildAppWithPath(context.Background(), appCfg, newTestRegistry(), nil, "")
	if err == nil {
		t.Fatal("got nil, want error for bidi streaming")
	}
}

func TestLoadAndBuild(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "agent.yaml", "name: file-agent\nagent_class: LlmAgent\nmodel: mock/fast\n")
	root := openRoot(t, dir)

	a, runCfg, liveCfg, cacheCfg, err := config.LoadAndBuild(context.Background(), root, "agent.yaml", newTestRegistry())
	if err != nil {
		t.Fatalf("LoadAndBuild: %v", err)
	}
	if a.Name() != "file-agent" {
		t.Errorf("Name: got %q", a.Name())
	}
	if runCfg != nil || liveCfg != nil || cacheCfg != nil {
		t.Errorf("got run=%v live=%v cache=%v, want all nil", runCfg, liveCfg, cacheCfg)
	}
}

func TestBuild_SchemaRefs(t *testing.T) {
	t.Parallel()
	reg := newTestRegistry()
	reg.RegisterSchema("myapp.schemas.input", config.StaticSchema(testSchema()))
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{Name: "schema-bot"},
		Model:           "mock/fast",
		InputSchema:     &config.SchemaRef{Ref: &config.CodeConfig{Name: "myapp.schemas.input"}},
		OutputSchema:    &config.SchemaRef{Inline: testSchema()},
	}
	if _, err := config.BuildWithPath(context.Background(), cfg, reg, nil, ""); err != nil {
		t.Fatalf("BuildWithPath: %v", err)
	}
}

func TestBuild_InstructionTemplatePrecedence(t *testing.T) {
	t.Parallel()
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig:     config.BaseAgentConfig{Name: "bot"},
		Model:               "mock/fast",
		Instruction:         "static",
		InstructionTemplate: &prompt.TemplateRef{Inline: "templated"},
	}
	if _, err := config.BuildWithPath(context.Background(), cfg, newTestRegistry(), nil, ""); err != nil {
		t.Fatalf("BuildWithPath: %v", err)
	}
}

func TestBuild_InvalidInstructionTemplate(t *testing.T) {
	t.Parallel()
	cfg := &config.LLMAgentConfig{
		BaseAgentConfig:     config.BaseAgentConfig{Name: "bot"},
		Model:               "mock/fast",
		InstructionTemplate: &prompt.TemplateRef{Inline: "a", Name: "b"},
	}
	if _, err := config.BuildWithPath(context.Background(), cfg, newTestRegistry(), nil, ""); err == nil {
		t.Fatal("got nil, want error for invalid template ref")
	}
}
