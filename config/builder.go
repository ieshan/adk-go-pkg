package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/agent/workflowagents/loopagent"
	"google.golang.org/adk/v2/agent/workflowagents/parallelagent"
	"google.golang.org/adk/v2/agent/workflowagents/sequentialagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/skilltoolset"
	"google.golang.org/adk/v2/tool/skilltoolset/skill"
	"google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"

	"github.com/ieshan/adk-go-pkg/prompt"
)

// buildState holds the per-build caches used by the workflow builder. It is
// scoped to a single BuildWithPath/BuildAppWithPath call so nodes and agents are
// never shared across builds (or users).
type buildState struct {
	nodes   map[string]workflow.Node // resolved root-relative file path → node
	fnNodes map[string]workflow.Node // registered function name → node
	agents  map[string]agent.Agent   // resolved root-relative file path → agent
}

func newBuildState() *buildState {
	return &buildState{
		nodes:   make(map[string]workflow.Node),
		fnNodes: make(map[string]workflow.Node),
		agents:  make(map[string]agent.Agent),
	}
}

// resolveRefPath resolves ref relative to the directory of parentPath. Absolute
// refs are returned unchanged; the *os.Root then rejects them.
func resolveRefPath(parentPath, ref string) string {
	if filepath.IsAbs(ref) {
		return ref
	}
	return filepath.Join(filepath.Dir(parentPath), ref)
}

// BuildWithPath constructs a live [agent.Agent] tree from a declarative
// [AgentConfig] and a [Registry] that provides model and tool factories.
//
// The function is recursive: sub-agents are built depth-first before the parent
// so that the fully initialised child [agent.Agent] values are available when
// the parent constructor is called.
//
// Supported agent classes and their mapping:
//
//   - "LlmAgent"         → [llmagent.New] with resolved model and tools
//   - "SequentialAgent"  → [sequentialagent.New] with built sub-agents
//   - "ParallelAgent"    → [parallelagent.New] with built sub-agents
//   - "LoopAgent"        → [loopagent.New] with built sub-agents and MaxIterations
//   - "Workflow"         → a [workflow.Workflow] wrapped in [agent.New]
//
// The root parameter scopes all filesystem access for sub-agent config_path
// references and file-based instruction templates. Pass nil when the config
// tree contains no file references. The configPath parameter is the
// root-relative path of the config being built; each config's references resolve
// against its own directory. Pass an empty string when not loading from a file.
// A referenced file's own run_config/context_cache_config sections are ignored;
// only its agent tree is used.
//
// Example:
//
//	reg := config.NewRegistry()
//	reg.RegisterModel("gemini", geminiFactory)
//	reg.RegisterTool("search", searchFactory)
//	//	cfg, _ := config.Load(root, "agent.yaml")
//	a, err := config.BuildWithPath(ctx, cfg.AgentConfig, reg, root, "agent.yaml")
//	if err != nil {
//	    log.Fatal(err)
//	}
func BuildWithPath(ctx context.Context, cfg AgentConfig, reg *Registry, root *os.Root, configPath string) (agent.Agent, error) {
	return buildWithPath(ctx, cfg, reg, root, configPath, newBuildState())
}

func buildWithPath(ctx context.Context, cfg AgentConfig, reg *Registry, root *os.Root, configPath string, st *buildState) (agent.Agent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config.BuildWithPath: nil config")
	}

	var subAgents []agent.Agent
	entries := cfg.SubAgentEntries()
	if len(entries) > 0 {
		var err error
		subAgents, err = buildSubAgentEntries(ctx, entries, reg, root, configPath, st)
		if err != nil {
			return nil, err
		}
	}

	switch c := cfg.(type) {
	case *LLMAgentConfig:
		return buildLLMAgent(ctx, c, reg, root, configPath, subAgents)
	case *SequentialAgentConfig:
		return buildSequentialAgent(c, reg, subAgents)
	case *ParallelAgentConfig:
		return buildParallelAgent(c, reg, subAgents)
	case *LoopAgentConfig:
		return buildLoopAgent(c, reg, subAgents)
	case *WorkflowAgentConfig:
		return buildWorkflowAgent(ctx, c, reg, root, configPath, subAgents, st)
	default:
		return nil, fmt.Errorf("config.BuildWithPath: unknown agent config type %T", cfg)
	}
}

// mappedMaxLLMCalls maps the config value to adk-go semantics: nil → 0 (default),
// <= 0 → -1 (unbounded), > 0 → the value.
func mappedMaxLLMCalls(v *int) int {
	if v == nil {
		return 0
	}
	if *v <= 0 {
		return -1
	}
	return *v
}

func toAgentRunConfig(cfg *RunConfig) *agent.RunConfig {
	if cfg == nil {
		return nil
	}
	mode := agent.StreamingMode(cfg.StreamingMode)
	if cfg.StreamingMode == "" {
		mode = agent.StreamingModeNone
	}
	return &agent.RunConfig{
		StreamingMode:             mode,
		SaveInputBlobsAsArtifacts: cfg.SaveInputBlobsAsArtifacts,
		MaxLLMCalls:               mappedMaxLLMCalls(cfg.MaxLLMCalls),
	}
}

func toAgentLiveRunConfig(cfg *RunConfig) *agent.LiveRunConfig {
	if cfg == nil {
		return nil
	}
	return &agent.LiveRunConfig{
		SaveLiveBlob: cfg.SaveLiveBlob,
		MaxLLMCalls:  mappedMaxLLMCalls(cfg.MaxLLMCalls),
	}
}

// BuildAppWithPath accepts an AppConfig and returns runtime configs
// ([agent.RunConfig], [agent.LiveRunConfig], and [ContextCacheConfig])
// alongside the built agent.
//
// The returned [agent.LiveRunConfig] is non-nil whenever the config declares a
// run_config section, even if it only sets streaming_mode: it is derived from the
// same section. Its MaxLLMCalls is 0 (the adk-go default) when unset, so do not
// read liveRunCfg != nil as "live behavior was configured".
//
// A nested config file's own run_config/context_cache_config sections are
// ignored; only the root AppConfig's runtime settings are returned.
//
// root and configPath have the same semantics as in [BuildWithPath].
func BuildAppWithPath(ctx context.Context, appCfg *AppConfig, reg *Registry, root *os.Root, configPath string) (agent.Agent, *agent.RunConfig, *agent.LiveRunConfig, *ContextCacheConfig, error) {
	if appCfg.RunConfig != nil && appCfg.RunConfig.StreamingMode == StreamingModeBIDI {
		return nil, nil, nil, nil, fmt.Errorf("config: streaming_mode %q is not supported by ADK-Go (supported: %q, %q)", StreamingModeBIDI, StreamingModeNone, StreamingModeSSE)
	}
	ag, err := buildWithPath(ctx, appCfg.AgentConfig, reg, root, configPath, newBuildState())
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return ag, toAgentRunConfig(appCfg.RunConfig), toAgentLiveRunConfig(appCfg.RunConfig), appCfg.ContextCacheConfig, nil
}

// LoadAndBuild is a convenience function that combines Load and BuildAppWithPath.
// It reads the agent configuration from path beneath root, constructs the live
// agent tree, and returns runtime configs ([agent.RunConfig],
// [agent.LiveRunConfig], and [ContextCacheConfig]) alongside the agent.
// root must not be nil.
func LoadAndBuild(ctx context.Context, root *os.Root, path string, reg *Registry) (agent.Agent, *agent.RunConfig, *agent.LiveRunConfig, *ContextCacheConfig, error) {
	appCfg, err := Load(root, path)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("config.LoadAndBuild: %w", err)
	}
	return BuildAppWithPath(ctx, appCfg, reg, root, path)
}

func buildSubAgentEntries(ctx context.Context, entries []SubAgentEntry, reg *Registry, root *os.Root, parentPath string, st *buildState) ([]agent.Agent, error) {
	agents := make([]agent.Agent, 0, len(entries))
	for _, entry := range entries {
		if entry.Inline != nil {
			a, err := buildWithPath(ctx, entry.Inline, reg, root, parentPath, st)
			if err != nil {
				return nil, err
			}
			agents = append(agents, a)
			continue
		}
		if entry.Ref == nil {
			return nil, fmt.Errorf("config.BuildWithPath: empty SubAgentEntry")
		}
		if err := entry.Ref.Validate(); err != nil {
			return nil, fmt.Errorf("config.BuildWithPath: %w", err)
		}
		if entry.Ref.Code != "" {
			a, err := reg.ResolveAgent(entry.Ref.Code)
			if err != nil {
				return nil, fmt.Errorf("config.BuildWithPath: resolve sub-agent code ref %q: %w", entry.Ref.Code, err)
			}
			agents = append(agents, a)
			continue
		}

		childPath := resolveRefPath(parentPath, entry.Ref.ConfigPath)
		if a, ok := st.agents[childPath]; ok {
			agents = append(agents, a)
			continue
		}
		appCfg, err := Load(root, childPath)
		if err != nil {
			return nil, fmt.Errorf("config.BuildWithPath: resolve sub-agent ref %q: %w", entry.Ref.ConfigPath, err)
		}
		a, err := buildWithPath(ctx, appCfg.AgentConfig, reg, root, childPath, st)
		if err != nil {
			return nil, err
		}
		st.agents[childPath] = a
		agents = append(agents, a)
	}
	return agents, nil
}

func buildLLMAgent(ctx context.Context, cfg *LLMAgentConfig, reg *Registry, root *os.Root, configPath string, subAgents []agent.Agent) (agent.Agent, error) {
	var llm model.LLM
	var err error

	switch cfg.IncludeContents {
	case "", "default", "none":
	default:
		return nil, fmt.Errorf("config.Build [LlmAgent %q]: include_contents must be \"default\" or \"none\", got %q", cfg.Name(), cfg.IncludeContents)
	}

	if cfg.ModelCode != nil {
		if cfg.Model != "" {
			return nil, fmt.Errorf("config.Build [LlmAgent %q]: only one of model or model_code may be set", cfg.Name())
		}
		if cfg.ModelArgs != nil {
			return nil, fmt.Errorf("config.Build [LlmAgent %q]: model_args cannot be combined with model_code", cfg.Name())
		}
		llm, err = reg.ResolveModelCode(cfg.ModelCode.Name, cfg.ModelCode.Args)
		if err != nil {
			return nil, fmt.Errorf("config.Build [LlmAgent %q]: resolve model_code: %w", cfg.Name(), err)
		}
	} else {
		llm, err = reg.ResolveModel(cfg.Model, cfg.ModelArgs)
		if err != nil {
			return nil, fmt.Errorf("config.Build [LlmAgent %q]: %w", cfg.Name(), err)
		}
	}

	tools := make([]tool.Tool, 0, len(cfg.Tools))
	for _, ref := range cfg.Tools {
		t, toolErr := reg.ResolveTool(ref.Name, ref.Args)
		if toolErr != nil {
			return nil, fmt.Errorf("config.Build [LlmAgent %q]: %w", cfg.Name(), toolErr)
		}
		tools = append(tools, t)
	}

	var toolsets []tool.Toolset
	for _, ref := range cfg.Skillsets {
		source, skillErr := reg.ResolveSkill(ref.Name, ref.Config)
		if skillErr != nil {
			return nil, fmt.Errorf("config.Build [LlmAgent %q]: skillset %q: %w", cfg.Name(), ref.Name, skillErr)
		}
		if len(ref.Names) > 0 {
			source = NewFilteredSource(source, ref.Names)
		}
		source, skillErr = applyPreload(ctx, source, ref.Preload)
		if skillErr != nil {
			return nil, fmt.Errorf("config.Build [LlmAgent %q]: skillset %q preload: %w", cfg.Name(), ref.Name, skillErr)
		}
		skillToolset, skillErr := skilltoolset.New(ctx, skilltoolset.Config{
			Source:            source,
			SystemInstruction: ref.SystemInstruction,
		})
		if skillErr != nil {
			return nil, fmt.Errorf("config.Build [LlmAgent %q]: skillset %q: %w", cfg.Name(), ref.Name, skillErr)
		}
		toolsets = append(toolsets, skillToolset)
	}

	beforeModelCBs, err := resolveCallbacks(reg.ResolveBeforeModelCallback, cfg.BeforeModelCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [LlmAgent %q]: beforeModelCallbacks: %w", cfg.Name(), err)
	}
	afterModelCBs, err := resolveCallbacks(reg.ResolveAfterModelCallback, cfg.AfterModelCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [LlmAgent %q]: afterModelCallbacks: %w", cfg.Name(), err)
	}
	onModelErrorCBs, err := resolveCallbacks(reg.ResolveOnModelErrorCallback, cfg.OnModelErrorCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [LlmAgent %q]: onModelErrorCallbacks: %w", cfg.Name(), err)
	}
	beforeToolCBs, err := resolveCallbacks(reg.ResolveBeforeToolCallback, cfg.BeforeToolCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [LlmAgent %q]: beforeToolCallbacks: %w", cfg.Name(), err)
	}
	afterToolCBs, err := resolveCallbacks(reg.ResolveAfterToolCallback, cfg.AfterToolCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [LlmAgent %q]: afterToolCallbacks: %w", cfg.Name(), err)
	}
	onToolErrorCBs, err := resolveCallbacks(reg.ResolveOnToolErrorCallback, cfg.OnToolErrorCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [LlmAgent %q]: onToolErrorCallbacks: %w", cfg.Name(), err)
	}
	beforeAgentCBs, err := resolveCallbacks(reg.ResolveBeforeAgentCallback, cfg.BeforeAgentCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [LlmAgent %q]: beforeAgentCallbacks: %w", cfg.Name(), err)
	}
	afterAgentCBs, err := resolveCallbacks(reg.ResolveAfterAgentCallback, cfg.AfterAgentCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [LlmAgent %q]: afterAgentCallbacks: %w", cfg.Name(), err)
	}

	var instructionProvider llmagent.InstructionProvider
	if cfg.InstructionTemplate != nil && cfg.InstructionTemplate.IsSet() {
		if validateErr := cfg.InstructionTemplate.Validate(); validateErr != nil {
			return nil, fmt.Errorf("config.Build [LlmAgent %q]: instruction_template: %w", cfg.Name(), validateErr)
		}
		var engine *prompt.TemplateEngine
		if reg.TemplateRegistry() != nil {
			engine = reg.TemplateRegistry().Engine()
		} else {
			engine = prompt.New()
		}
		loader := prompt.NewLoader(engine, root)

		ref := *cfg.InstructionTemplate
		if ref.Path != "" {
			ref.Path = resolveRefPath(configPath, ref.Path)
		}
		tmpl, resolveErr := ref.Resolve(reg.TemplateRegistry(), loader)
		if resolveErr != nil {
			return nil, fmt.Errorf("config.Build [LlmAgent %q]: resolve instruction_template: %w", cfg.Name(), resolveErr)
		}
		instructionProvider = prompt.NewInstructionProviderFromTemplate(tmpl)
	}

	inputSchema, err := resolveSchemaRef(cfg.InputSchema, reg)
	if err != nil {
		return nil, fmt.Errorf("config.Build [LlmAgent %q]: inputSchema: %w", cfg.Name(), err)
	}
	outputSchema, err := resolveSchemaRef(cfg.OutputSchema, reg)
	if err != nil {
		return nil, fmt.Errorf("config.Build [LlmAgent %q]: outputSchema: %w", cfg.Name(), err)
	}

	return llmagent.New(llmagent.Config{
		Name:                     cfg.Name(),
		Description:              cfg.Description(),
		Model:                    llm,
		Tools:                    tools,
		Toolsets:                 toolsets,
		Instruction:              cfg.Instruction,
		GlobalInstruction:        cfg.GlobalInstruction,
		SubAgents:                subAgents,
		GenerateContentConfig:    cfg.GenerateContentConfig,
		DisallowTransferToParent: cfg.DisallowTransferToParent,
		DisallowTransferToPeers:  cfg.DisallowTransferToPeers,
		InputSchema:              inputSchema,
		OutputSchema:             outputSchema,
		OutputKey:                cfg.OutputKey,
		InstructionProvider:      instructionProvider,
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

func resolveSchemaRef(ref *SchemaRef, reg *Registry) (*genai.Schema, error) {
	if ref == nil {
		return nil, nil
	}
	if ref.Inline != nil {
		return ref.Inline, nil
	}
	if ref.Ref != nil {
		return reg.ResolveSchema(ref.Ref.Name, ref.Ref.Args)
	}
	return nil, fmt.Errorf("empty schema reference")
}

// resolveCallbacks maps a list of CodeConfig names to resolved callbacks via a resolver function.
func resolveCallbacks[T any](resolveFn func(string) (T, error), configs []CodeConfig) ([]T, error) {
	if len(configs) == 0 {
		return nil, nil
	}
	result := make([]T, 0, len(configs))
	for _, cc := range configs {
		if cc.Args != nil {
			return nil, fmt.Errorf("callback %q: %w", cc.Name, ErrCallbackArgs)
		}
		cb, err := resolveFn(cc.Name)
		if err != nil {
			return nil, fmt.Errorf("resolve callback %q: %w", cc.Name, err)
		}
		result = append(result, cb)
	}
	return result, nil
}

// applyPreload wraps a skill.Source with a preload proxy based on the strategy.
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
		return nil, fmt.Errorf("config.Build [SequentialAgent %q]: beforeAgentCallbacks: %w", cfg.Name(), err)
	}
	afterCBs, err := resolveCallbacks(reg.ResolveAfterAgentCallback, cfg.AfterAgentCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [SequentialAgent %q]: afterAgentCallbacks: %w", cfg.Name(), err)
	}
	a, err := sequentialagent.New(sequentialagent.Config{
		AgentConfig: agent.Config{
			Name:                 cfg.Name(),
			Description:          cfg.Description(),
			SubAgents:            subAgents,
			BeforeAgentCallbacks: beforeCBs,
			AfterAgentCallbacks:  afterCBs,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("config.Build [SequentialAgent %q]: %w", cfg.Name(), err)
	}
	return a, nil
}

func buildParallelAgent(cfg *ParallelAgentConfig, reg *Registry, subAgents []agent.Agent) (agent.Agent, error) {
	beforeCBs, err := resolveCallbacks(reg.ResolveBeforeAgentCallback, cfg.BeforeAgentCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [ParallelAgent %q]: beforeAgentCallbacks: %w", cfg.Name(), err)
	}
	afterCBs, err := resolveCallbacks(reg.ResolveAfterAgentCallback, cfg.AfterAgentCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [ParallelAgent %q]: afterAgentCallbacks: %w", cfg.Name(), err)
	}
	a, err := parallelagent.New(parallelagent.Config{
		AgentConfig: agent.Config{
			Name:                 cfg.Name(),
			Description:          cfg.Description(),
			SubAgents:            subAgents,
			BeforeAgentCallbacks: beforeCBs,
			AfterAgentCallbacks:  afterCBs,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("config.Build [ParallelAgent %q]: %w", cfg.Name(), err)
	}
	return a, nil
}

func buildLoopAgent(cfg *LoopAgentConfig, reg *Registry, subAgents []agent.Agent) (agent.Agent, error) {
	if cfg.MaxIterations < 0 {
		return nil, fmt.Errorf("config.Build [LoopAgent %q]: MaxIterations must be >= 0, got %d", cfg.Name(), cfg.MaxIterations)
	}
	beforeCBs, err := resolveCallbacks(reg.ResolveBeforeAgentCallback, cfg.BeforeAgentCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [LoopAgent %q]: beforeAgentCallbacks: %w", cfg.Name(), err)
	}
	afterCBs, err := resolveCallbacks(reg.ResolveAfterAgentCallback, cfg.AfterAgentCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [LoopAgent %q]: afterAgentCallbacks: %w", cfg.Name(), err)
	}
	a, err := loopagent.New(loopagent.Config{
		AgentConfig: agent.Config{
			Name:                 cfg.Name(),
			Description:          cfg.Description(),
			SubAgents:            subAgents,
			BeforeAgentCallbacks: beforeCBs,
			AfterAgentCallbacks:  afterCBs,
		},
		MaxIterations: uint(cfg.MaxIterations),
	})
	if err != nil {
		return nil, fmt.Errorf("config.Build [LoopAgent %q]: %w", cfg.Name(), err)
	}
	return a, nil
}
