package config

import (
	"context"
	"fmt"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/agent/workflowagents/loopagent"
	"google.golang.org/adk/agent/workflowagents/parallelagent"
	"google.golang.org/adk/agent/workflowagents/sequentialagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/skilltoolset"
	"google.golang.org/adk/tool/skilltoolset/skill"
	"google.golang.org/genai"
)

// Build constructs a live [agent.Agent] tree from a declarative [AgentConfig]
// and a [Registry] that provides model and tool factories.
//
// The function is recursive: sub-agents are built depth-first before the parent
// so that the fully initialised child [agent.Agent] values are available when
// the parent constructor is called.
//
// Supported agent types and their mapping:
//
//   - "llm"        → [llmagent.New] with resolved model and tools
//   - "sequential" → [sequentialagent.New] with built sub-agents
//   - "parallel"   → [parallelagent.New] with built sub-agents
//   - "loop"       → [loopagent.New] with built sub-agents and MaxIterations
//
// An error is returned for any unknown type, unresolvable model prefix,
// unresolvable tool name, or invalid MaxIterations value.
//
// Example:
//
//	reg := config.NewRegistry()
//	reg.RegisterModel("gemini", geminiFactory)
//	reg.RegisterTool("search", searchFactory)
//	//	cfg, _ := config.Load("agent.yaml")
//	a, err := config.Build(ctx, cfg, reg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// For sub-agent file references to resolve correctly, use BuildWithPath.
func Build(ctx context.Context, cfg AgentConfig, reg *Registry) (agent.Agent, error) {
	return BuildWithPath(ctx, cfg, reg, "")
}

// BuildWithPath is like Build but also accepts the config file path so that
// relative config_path references in sub-agents can be resolved.
func BuildWithPath(ctx context.Context, cfg AgentConfig, reg *Registry, configPath string) (agent.Agent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config.Build: nil config")
	}

	// Resolve sub-agent entries (inline + refs).
	var subAgents []agent.Agent
	entries := cfg.SubAgentEntries()
	if len(entries) > 0 {
		var err error
		subAgents, err = buildSubAgentEntries(ctx, entries, reg, configPath)
		if err != nil {
			return nil, err
		}
	}

	switch c := cfg.(type) {
	case *LLMAgentConfig:
		return buildLLMAgent(ctx, c, reg, subAgents)
	case *SequentialAgentConfig:
		return buildSequentialAgent(c, reg, subAgents)
	case *ParallelAgentConfig:
		return buildParallelAgent(c, reg, subAgents)
	case *LoopAgentConfig:
		return buildLoopAgent(c, reg, subAgents)
	default:
		return nil, fmt.Errorf("config.Build: unknown agent config type %T", cfg)
	}
}

func toAgentRunConfig(cfg *RunConfig) *agent.RunConfig {
	if cfg == nil {
		return nil
	}
	return &agent.RunConfig{
		StreamingMode:             agent.StreamingMode(cfg.StreamingMode),
		SaveInputBlobsAsArtifacts: cfg.SaveLiveBlob,
	}
}

func toAgentLiveRunConfig(cfg *LiveRunConfig) *agent.LiveRunConfig {
	if cfg == nil {
		return nil
	}
	return &agent.LiveRunConfig{
		MaxLLMCalls: cfg.MaxLLMCalls,
	}
}

// BuildApp is like Build but accepts an AppConfig and returns runtime configs
// ([agent.RunConfig], [agent.LiveRunConfig], and [ContextCacheConfig])
// alongside the built agent. It delegates to BuildAppWithPath with an empty path.
func BuildApp(ctx context.Context, appCfg *AppConfig, reg *Registry) (agent.Agent, *agent.RunConfig, *agent.LiveRunConfig, *ContextCacheConfig, error) {
	return BuildAppWithPath(ctx, appCfg, reg, "")
}

// BuildAppWithPath is like BuildApp but accepts the config file path so that
// relative config_path references in sub-agents can be resolved.
// It returns the built agent together with [agent.RunConfig], [agent.LiveRunConfig],
// and [ContextCacheConfig].
func BuildAppWithPath(ctx context.Context, appCfg *AppConfig, reg *Registry, configPath string) (agent.Agent, *agent.RunConfig, *agent.LiveRunConfig, *ContextCacheConfig, error) {
	ag, err := BuildWithPath(ctx, appCfg.AgentConfig, reg, configPath)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return ag, toAgentRunConfig(appCfg.RunConfig), toAgentLiveRunConfig(appCfg.LiveRunConfig), appCfg.ContextCacheConfig, nil
}

// LoadAndBuild is a convenience function that combines Load and BuildAppWithPath.
// It reads the agent configuration from path, constructs the live agent tree,
// and returns runtime configs ([agent.RunConfig], [agent.LiveRunConfig],
// and [ContextCacheConfig]) alongside the agent.
func LoadAndBuild(ctx context.Context, path string, reg *Registry) (agent.Agent, *agent.RunConfig, *agent.LiveRunConfig, *ContextCacheConfig, error) {
	appCfg, err := Load(path)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("config.LoadAndBuild: %w", err)
	}
	return BuildAppWithPath(ctx, appCfg, reg, path)
}

func buildSubAgentEntries(ctx context.Context, entries []SubAgentEntry, reg *Registry, parentPath string) ([]agent.Agent, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	agents := make([]agent.Agent, 0, len(entries))
	for _, entry := range entries {
		var cfg AgentConfig
		if entry.Inline != nil {
			cfg = entry.Inline
		} else if entry.Ref != nil {
			if entry.Ref.Code != "" {
				a, err := reg.ResolveAgent(entry.Ref.Code)
				if err != nil {
					return nil, fmt.Errorf("config.Build: resolve sub-agent code ref %q: %w", entry.Ref.Code, err)
				}
				agents = append(agents, a)
				continue
			}
			var err error
			cfg, err = ResolveAgentRef(entry.Ref, parentPath)
			if err != nil {
				return nil, fmt.Errorf("config.Build: resolve sub-agent ref: %w", err)
			}
		} else {
			return nil, fmt.Errorf("config.Build: empty SubAgentEntry")
		}
		a, err := BuildWithPath(ctx, cfg, reg, parentPath)
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, nil
}

func buildLLMAgent(ctx context.Context, cfg *LLMAgentConfig, reg *Registry, subAgents []agent.Agent) (agent.Agent, error) {
	var llm model.LLM
	var err error

	if cfg.ModelCode != nil {
		if cfg.Model != "" {
			return nil, fmt.Errorf("config.Build [llm %q]: only one of model or model_code may be set", cfg.Name())
		}
		llm, err = reg.ResolveModelCode(cfg.ModelCode.Name, cfg.ModelCode.Args)
		if err != nil {
			return nil, fmt.Errorf("config.Build [llm %q]: resolve model_code: %w", cfg.Name(), err)
		}
	} else {
		llm, err = reg.ResolveModel(cfg.Model, cfg.GenerateConfig)
		if err != nil {
			return nil, fmt.Errorf("config.Build [llm %q]: %w", cfg.Name(), err)
		}
	}

	tools := make([]tool.Tool, 0, len(cfg.Tools))
	for _, ref := range cfg.Tools {
		t, err := reg.ResolveTool(ref.Name, ref.Args)
		if err != nil {
			return nil, fmt.Errorf("config.Build [llm %q]: %w", cfg.Name(), err)
		}
		tools = append(tools, t)
	}

	var toolsets []tool.Toolset
	for _, ref := range cfg.Skillsets {
		source, err := reg.ResolveSkill(ref.Name, ref.Config)
		if err != nil {
			return nil, fmt.Errorf("config.Build [llm %q]: skillset %q: %w", cfg.Name(), ref.Name, err)
		}
		if len(ref.Names) > 0 {
			source = NewFilteredSource(source, ref.Names)
		}
		source, err = applyPreload(ctx, source, ref.Preload)
		if err != nil {
			return nil, fmt.Errorf("config.Build [llm %q]: skillset %q preload: %w", cfg.Name(), ref.Name, err)
		}
		skillToolset, err := skilltoolset.New(ctx, skilltoolset.Config{
			Source:            source,
			SystemInstruction: ref.SystemInstruction,
		})
		if err != nil {
			return nil, fmt.Errorf("config.Build [llm %q]: skillset %q: %w", cfg.Name(), ref.Name, err)
		}
		toolsets = append(toolsets, skillToolset)
	}

	var gc *genai.GenerateContentConfig
	if len(cfg.GenerateConfig) > 0 {
		gc, err = TranslateGenerateConfig(cfg.GenerateConfig)
		if err != nil {
			return nil, fmt.Errorf("config.Build [llm %q]: %w", cfg.Name(), err)
		}
	}

	// Resolve callbacks via typed registry
	beforeModelCBs, err := resolveCallbacks(reg.ResolveBeforeModelCallback, cfg.BeforeModelCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [llm %q]: beforeModelCallbacks: %w", cfg.Name(), err)
	}
	afterModelCBs, err := resolveCallbacks(reg.ResolveAfterModelCallback, cfg.AfterModelCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [llm %q]: afterModelCallbacks: %w", cfg.Name(), err)
	}
	onModelErrorCBs, err := resolveCallbacks(reg.ResolveOnModelErrorCallback, cfg.OnModelErrorCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [llm %q]: onModelErrorCallbacks: %w", cfg.Name(), err)
	}
	beforeToolCBs, err := resolveCallbacks(reg.ResolveBeforeToolCallback, cfg.BeforeToolCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [llm %q]: beforeToolCallbacks: %w", cfg.Name(), err)
	}
	afterToolCBs, err := resolveCallbacks(reg.ResolveAfterToolCallback, cfg.AfterToolCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [llm %q]: afterToolCallbacks: %w", cfg.Name(), err)
	}
	onToolErrorCBs, err := resolveCallbacks(reg.ResolveOnToolErrorCallback, cfg.OnToolErrorCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [llm %q]: onToolErrorCallbacks: %w", cfg.Name(), err)
	}

	beforeAgentCBs, err := resolveCallbacks(reg.ResolveBeforeAgentCallback, cfg.BeforeAgentCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [llm %q]: beforeAgentCallbacks: %w", cfg.Name(), err)
	}
	afterAgentCBs, err := resolveCallbacks(reg.ResolveAfterAgentCallback, cfg.AfterAgentCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [llm %q]: afterAgentCallbacks: %w", cfg.Name(), err)
	}

	var inputSchema, outputSchema *genai.Schema
	if cfg.InputSchema != nil {
		if cfg.InputSchema.Inline != nil {
			inputSchema = cfg.InputSchema.Inline
		} else if cfg.InputSchema.Ref != nil {
			inputSchema, err = reg.ResolveSchema(cfg.InputSchema.Ref.Name, cfg.InputSchema.Ref.Args)
			if err != nil {
				return nil, fmt.Errorf("config.Build [llm %q]: inputSchema: %w", cfg.Name(), err)
			}
		}
	}
	if cfg.OutputSchema != nil {
		if cfg.OutputSchema.Inline != nil {
			outputSchema = cfg.OutputSchema.Inline
		} else if cfg.OutputSchema.Ref != nil {
			outputSchema, err = reg.ResolveSchema(cfg.OutputSchema.Ref.Name, cfg.OutputSchema.Ref.Args)
			if err != nil {
				return nil, fmt.Errorf("config.Build [llm %q]: outputSchema: %w", cfg.Name(), err)
			}
		}
	}

	return llmagent.New(llmagent.Config{
		Name:                     cfg.Name(),
		Description:              cfg.Description(),
		Model:                    llm,
		Tools:                    tools,
		Toolsets:                 toolsets,
		Instruction:              cfg.Instruction,
		GlobalInstruction:        cfg.StaticInstruction,
		SubAgents:                subAgents,
		GenerateContentConfig:    gc,
		DisallowTransferToParent: cfg.DisallowTransferToParent,
		DisallowTransferToPeers:  cfg.DisallowTransferToPeers,
		InputSchema:              inputSchema,
		OutputSchema:             outputSchema,
		OutputKey:                cfg.OutputKey,
		IncludeContents:          llmagent.IncludeContents(cfg.IncludeContents),
		BeforeModelCallbacks:     beforeModelCBs,
		AfterModelCallbacks:      afterModelCBs,
		OnModelErrorCallbacks:    onModelErrorCBs,
		BeforeToolCallbacks:      beforeToolCBs,
		AfterToolCallbacks:       afterToolCBs,
		OnToolErrorCallbacks:     onToolErrorCBs,
		BeforeAgentCallbacks:     beforeAgentCBs,
		AfterAgentCallbacks:      afterAgentCBs,
	})
}

// resolveCallbacks maps a list of CodeConfig names to resolved callbacks via a resolver function.
func resolveCallbacks[T any](resolveFn func(string) (T, error), configs []CodeConfig) ([]T, error) {
	if len(configs) == 0 {
		return nil, nil
	}
	result := make([]T, 0, len(configs))
	for _, cc := range configs {
		cb, err := resolveFn(cc.Name)
		if err != nil {
			return nil, fmt.Errorf("resolve callback %q: %w", cc.Name, err)
		}
		result = append(result, cb)
	}
	return result, nil
}

// applyPreload wraps a skill.Source with a preload proxy based on the strategy.
//
// Preload strategies optimize skill access patterns by loading data into memory
// at initialization time rather than on-demand. This trades memory for latency.
//
// Strategies:
//   - "": Returns base source unchanged. Skills loaded on-demand. Lowest memory,
//     highest latency for first access to each skill.
//   - "complete": Loads all skill data (frontmatters, instructions, resources)
//     into memory. Fastest access after initialization. Highest memory usage.
//     Best for small skill sets (<100MB) where fast response is critical.
//   - "frontmatters": Loads only skill frontmatters (metadata) into memory.
//     Balanced option. Skill instructions/resources loaded on-demand.
//     Best when listing skills is frequent but loading individual skills is rare.
//
// The context is used during the initial load. Cancellation during preload will
// return an error, but the returned source (if any) should not be used.
//
// Returns an error if the strategy is unrecognized or if preload fails.
func applyPreload(ctx context.Context, base skill.Source, strategy string) (skill.Source, error) {
	switch strategy {
	case "":
		return base, nil
	case "complete":
		source, _, err := skill.WithCompletePreloadSource(ctx, base)
		return source, err
	case "frontmatters":
		source, _, err := skill.WithFrontmatterPreloadSource(ctx, base)
		return source, err
	default:
		return nil, fmt.Errorf("unknown preload strategy %q (valid: '', 'complete', 'frontmatters')", strategy)
	}
}

func buildSequentialAgent(cfg *SequentialAgentConfig, reg *Registry, subAgents []agent.Agent) (agent.Agent, error) {
	beforeCBs, err := resolveCallbacks(reg.ResolveBeforeAgentCallback, cfg.BeforeAgentCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [sequential %q]: beforeAgentCallbacks: %w", cfg.Name(), err)
	}
	afterCBs, err := resolveCallbacks(reg.ResolveAfterAgentCallback, cfg.AfterAgentCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [sequential %q]: afterAgentCallbacks: %w", cfg.Name(), err)
	}
	a, err := sequentialagent.New(sequentialagent.Config{
		AgentConfig: agent.Config{
			Name:                 cfg.Name(),
			SubAgents:            subAgents,
			BeforeAgentCallbacks: beforeCBs,
			AfterAgentCallbacks:  afterCBs,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("config.Build [sequential %q]: %w", cfg.Name(), err)
	}
	return a, nil
}

func buildParallelAgent(cfg *ParallelAgentConfig, reg *Registry, subAgents []agent.Agent) (agent.Agent, error) {
	beforeCBs, err := resolveCallbacks(reg.ResolveBeforeAgentCallback, cfg.BeforeAgentCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [parallel %q]: beforeAgentCallbacks: %w", cfg.Name(), err)
	}
	afterCBs, err := resolveCallbacks(reg.ResolveAfterAgentCallback, cfg.AfterAgentCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [parallel %q]: afterAgentCallbacks: %w", cfg.Name(), err)
	}
	a, err := parallelagent.New(parallelagent.Config{
		AgentConfig: agent.Config{
			Name:                 cfg.Name(),
			SubAgents:            subAgents,
			BeforeAgentCallbacks: beforeCBs,
			AfterAgentCallbacks:  afterCBs,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("config.Build [parallel %q]: %w", cfg.Name(), err)
	}
	return a, nil
}

func buildLoopAgent(cfg *LoopAgentConfig, reg *Registry, subAgents []agent.Agent) (agent.Agent, error) {
	if cfg.MaxIterations < 0 {
		return nil, fmt.Errorf("config.Build [loop %q]: MaxIterations must be >= 0, got %d", cfg.Name(), cfg.MaxIterations)
	}
	beforeCBs, err := resolveCallbacks(reg.ResolveBeforeAgentCallback, cfg.BeforeAgentCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [loop %q]: beforeAgentCallbacks: %w", cfg.Name(), err)
	}
	afterCBs, err := resolveCallbacks(reg.ResolveAfterAgentCallback, cfg.AfterAgentCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [loop %q]: afterAgentCallbacks: %w", cfg.Name(), err)
	}
	a, err := loopagent.New(loopagent.Config{
		AgentConfig: agent.Config{
			Name:                 cfg.Name(),
			SubAgents:            subAgents,
			BeforeAgentCallbacks: beforeCBs,
			AfterAgentCallbacks:  afterCBs,
		},
		MaxIterations: uint(cfg.MaxIterations),
	})
	if err != nil {
		return nil, fmt.Errorf("config.Build [loop %q]: %w", cfg.Name(), err)
	}
	return a, nil
}
