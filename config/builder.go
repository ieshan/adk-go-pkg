package config

import (
	"context"
	"fmt"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/agent/workflowagents/loopagent"
	"google.golang.org/adk/agent/workflowagents/parallelagent"
	"google.golang.org/adk/agent/workflowagents/sequentialagent"
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
//
//	cfg, _ := config.Load("agent.yaml")
//	a, err := config.Build(cfg, reg)
//	if err != nil {
//	    log.Fatal(err)
//	}
func Build(cfg *AgentConfig, reg *Registry) (agent.Agent, error) {
	// Build sub-agents depth-first.
	subAgents, err := buildSubAgents(cfg.SubAgents, reg)
	if err != nil {
		return nil, err
	}

	switch cfg.Type {
	case "llm":
		return buildLLMAgent(cfg, reg, subAgents)
	case "sequential":
		return buildSequentialAgent(cfg, subAgents)
	case "parallel":
		return buildParallelAgent(cfg, subAgents)
	case "loop":
		return buildLoopAgent(cfg, subAgents)
	default:
		return nil, fmt.Errorf("config.Build: unknown agent type %q for agent %q", cfg.Type, cfg.Name)
	}
}

// LoadAndBuild is a convenience function that combines [Load] and [Build].
// It reads the agent configuration from path and immediately constructs the
// live agent tree using the provided [Registry].
//
// Example:
//
//	reg := config.NewRegistry()
//	reg.RegisterModel("gemini", geminiFactory)
//
//	a, err := config.LoadAndBuild("agents/root.yaml", reg)
//	if err != nil {
//	    log.Fatal(err)
//	}
func LoadAndBuild(path string, reg *Registry) (agent.Agent, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, fmt.Errorf("config.LoadAndBuild: %w", err)
	}
	return Build(cfg, reg)
}

// buildSubAgents recursively builds each entry in the configs slice.
func buildSubAgents(configs []AgentConfig, reg *Registry) ([]agent.Agent, error) {
	if len(configs) == 0 {
		return nil, nil
	}
	agents := make([]agent.Agent, 0, len(configs))
	for i := range configs {
		a, err := Build(&configs[i], reg)
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, nil
}

// buildLLMAgent resolves the model, tools, and skillsets then delegates to llmagent.New.
func buildLLMAgent(cfg *AgentConfig, reg *Registry, subAgents []agent.Agent) (agent.Agent, error) {
	llm, err := reg.ResolveModel(cfg.Model, cfg.GenerateConfig)
	if err != nil {
		return nil, fmt.Errorf("config.Build [llm %q]: %w", cfg.Name, err)
	}

	tools := make([]tool.Tool, 0, len(cfg.Tools))
	for _, ref := range cfg.Tools {
		t, err := reg.ResolveTool(ref.Name, ref.Config)
		if err != nil {
			return nil, fmt.Errorf("config.Build [llm %q]: %w", cfg.Name, err)
		}
		tools = append(tools, t)
	}

	// Resolve skillsets to tool.Toolset instances.
	var toolsets []tool.Toolset
	for _, ref := range cfg.Skillsets {
		source, err := reg.ResolveSkill(ref.Name, ref.Config)
		if err != nil {
			return nil, fmt.Errorf("config.Build [llm %q]: skillset %q: %w", cfg.Name, ref.Name, err)
		}

		// Apply name-based filtering if specific skills are requested.
		// This happens before preload so filtered skills are not loaded.
		if len(ref.Names) > 0 {
			source = NewFilteredSource(source, ref.Names)
		}

		// Apply preload proxy if specified.
		// When Names is set, only the selected skills are preloaded (more efficient).
		ctx := context.Background() // Use background for init; consider making this configurable.
		source, err = applyPreload(ctx, source, ref.Preload)
		if err != nil {
			return nil, fmt.Errorf("config.Build [llm %q]: skillset %q preload: %w", cfg.Name, ref.Name, err)
		}

		skillToolset, err := skilltoolset.New(ctx, skilltoolset.Config{
			Source:            source,
			SystemInstruction: ref.SystemInstruction,
		})
		if err != nil {
			return nil, fmt.Errorf("config.Build [llm %q]: skillset %q: %w", cfg.Name, ref.Name, err)
		}
		toolsets = append(toolsets, skillToolset)
	}

	// Translate the optional generate config map.
	var gc *genai.GenerateContentConfig
	if len(cfg.GenerateConfig) > 0 {
		gc, err = TranslateGenerateConfig(cfg.GenerateConfig)
		if err != nil {
			return nil, fmt.Errorf("config.Build [llm %q]: %w", cfg.Name, err)
		}
	}

	return llmagent.New(llmagent.Config{
		Name:                  cfg.Name,
		Description:           cfg.Description,
		Model:                 llm,
		Tools:                 tools,
		Toolsets:              toolsets,
		Instruction:           cfg.Instruction,
		SubAgents:             subAgents,
		GenerateContentConfig: gc,
	})
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

// buildSequentialAgent delegates to sequentialagent.New.
func buildSequentialAgent(cfg *AgentConfig, subAgents []agent.Agent) (agent.Agent, error) {
	a, err := sequentialagent.New(sequentialagent.Config{
		AgentConfig: agent.Config{
			Name:      cfg.Name,
			SubAgents: subAgents,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("config.Build [sequential %q]: %w", cfg.Name, err)
	}
	return a, nil
}

// buildParallelAgent delegates to parallelagent.New.
func buildParallelAgent(cfg *AgentConfig, subAgents []agent.Agent) (agent.Agent, error) {
	a, err := parallelagent.New(parallelagent.Config{
		AgentConfig: agent.Config{
			Name:      cfg.Name,
			SubAgents: subAgents,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("config.Build [parallel %q]: %w", cfg.Name, err)
	}
	return a, nil
}

// buildLoopAgent validates MaxIterations then delegates to loopagent.New.
// MaxIterations must be >= 0; a value of 0 means "run indefinitely".
func buildLoopAgent(cfg *AgentConfig, subAgents []agent.Agent) (agent.Agent, error) {
	if cfg.MaxIterations < 0 {
		return nil, fmt.Errorf("config.Build [loop %q]: MaxIterations must be >= 0, got %d", cfg.Name, cfg.MaxIterations)
	}
	a, err := loopagent.New(loopagent.Config{
		AgentConfig: agent.Config{
			Name:      cfg.Name,
			SubAgents: subAgents,
		},
		MaxIterations: uint(cfg.MaxIterations),
	})
	if err != nil {
		return nil, fmt.Errorf("config.Build [loop %q]: %w", cfg.Name, err)
	}
	return a, nil
}
