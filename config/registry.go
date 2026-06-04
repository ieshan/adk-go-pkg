package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/skilltoolset/skill"
	"google.golang.org/genai"
)

// ModelFactory is a constructor function that receives a config map and returns
// an [model.LLM]. The map always contains at least the key "model" set to the
// remainder of the ref after the first "/".
//
// Example — registering an OpenAI-compatible factory:
//
//	r.RegisterModel("openai", func(cfg map[string]any) (model.LLM, error) {
//	    modelName := cfg["model"].(string)
//	    return myopenai.New(modelName, cfg)
//	})
type ModelFactory func(cfg map[string]any) (model.LLM, error)

// ToolFactory is a constructor function that receives a config map and returns
// a [tool.Tool].
//
// Example:
//
//	r.RegisterTool("search", func(cfg map[string]any) (tool.Tool, error) {
//	    return mysearch.New(cfg)
//	})
type ToolFactory func(cfg map[string]any) (tool.Tool, error)

// SkillFactory creates a skill.Source from configuration.
// The factory receives the merged config from SkillsetRef.Config.
//
// A skill.Source provides access to SKILL.md files organized in directories.
// Each directory is a "skill" containing frontmatter (YAML metadata) and
// markdown instructions. Sources may also provide resources in subdirectories
// (references/, assets/, scripts/).
//
// Built-in factories:
//   - "filesystem": Creates a FileSystemSource from an OS directory.
//     Config keys: {"path": "./skills"}
//
// Custom factories can provide skills from cloud storage, databases, etc.
//
// Example custom factory for GCS:
//
//	reg.RegisterSkill("gcs", func(cfg map[string]any) (skill.Source, error) {
//	    bucket := cfg["bucket"].(string)
//	    prefix := cfg["prefix"].(string)
//	    // Create GCS-based source...
//	    return gcsSource, nil
//	})
//
// The factory must return an error if required config keys are missing
// or if the source cannot be created.
type SkillFactory func(cfg map[string]any) (skill.Source, error)

// ModelCodeFactory creates a model.LLM from configuration arguments.
type ModelCodeFactory func(args map[string]any) (model.LLM, error)

// SchemaFactory creates a *genai.Schema from configuration arguments.
type SchemaFactory func(args map[string]any) (*genai.Schema, error)

// StaticSchema returns a SchemaFactory that always returns the given schema.
func StaticSchema(s *genai.Schema) SchemaFactory {
	return func(map[string]any) (*genai.Schema, error) { return s, nil }
}

// Registry maps model prefixes, tool names, and skill source names to their
// respective factories. Use [NewRegistry] to create a ready-to-use instance.
//
// The registry pattern enables declarative agent configuration where models,
// tools, and skills are referenced by name in YAML/JSON files and resolved at
// build time.
//
// Registry is safe for concurrent use.
//
// Example:
//
//	r := config.NewRegistry()
//	r.RegisterModel("openai", openaiFactory)
//	r.RegisterTool("search", searchFactory)
//	r.RegisterSkill("filesystem", filesystemSkillFactory) // built-in
//
//	agent, err := config.LoadAndBuild(ctx, "agent.yaml", r)
type Registry struct {
	mu                    sync.RWMutex
	models                map[string]ModelFactory
	tools                 map[string]ToolFactory
	skills                map[string]SkillFactory // Skill factories by registered name
	beforeModelCallbacks  map[string]llmagent.BeforeModelCallback
	afterModelCallbacks   map[string]llmagent.AfterModelCallback
	onModelErrorCallbacks map[string]llmagent.OnModelErrorCallback
	beforeToolCallbacks   map[string]llmagent.BeforeToolCallback
	afterToolCallbacks    map[string]llmagent.AfterToolCallback
	onToolErrorCallbacks  map[string]llmagent.OnToolErrorCallback
	beforeAgentCallbacks  map[string]agent.BeforeAgentCallback
	afterAgentCallbacks   map[string]agent.AfterAgentCallback
	modelCodes            map[string]ModelCodeFactory
	agents                map[string]agent.Agent
	schemas               map[string]SchemaFactory
}

// NewRegistry returns an initialised [Registry] with built-in skill factories registered.
func NewRegistry() *Registry {
	r := &Registry{
		models:                make(map[string]ModelFactory),
		tools:                 make(map[string]ToolFactory),
		skills:                make(map[string]SkillFactory),
		beforeModelCallbacks:  make(map[string]llmagent.BeforeModelCallback),
		afterModelCallbacks:   make(map[string]llmagent.AfterModelCallback),
		onModelErrorCallbacks: make(map[string]llmagent.OnModelErrorCallback),
		beforeToolCallbacks:   make(map[string]llmagent.BeforeToolCallback),
		afterToolCallbacks:    make(map[string]llmagent.AfterToolCallback),
		onToolErrorCallbacks:  make(map[string]llmagent.OnToolErrorCallback),
		beforeAgentCallbacks:  make(map[string]agent.BeforeAgentCallback),
		afterAgentCallbacks:   make(map[string]agent.AfterAgentCallback),
		modelCodes:            make(map[string]ModelCodeFactory),
		agents:                make(map[string]agent.Agent),
		schemas:               make(map[string]SchemaFactory),
	}

	r.RegisterSkill("filesystem", func(cfg map[string]any) (skill.Source, error) {
		path, ok := cfg["path"].(string)
		if !ok {
			return nil, fmt.Errorf("filesystem skill factory requires 'path' config key")
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve skill path: %w", err)
		}
		return skill.NewFileSystemSource(os.DirFS(absPath)), nil
	})

	return r
}

// RegisterModel associates a [ModelFactory] with the given prefix.
// When [ResolveModel] is called with a ref like "prefix/model-id",
// this factory will be invoked.
//
// Registering the same prefix twice overwrites the previous factory.
func (r *Registry) RegisterModel(prefix string, factory ModelFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models[prefix] = factory
}

// HasModel reports whether a factory has been registered for prefix.
func (r *Registry) HasModel(prefix string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.models[prefix]
	return ok
}

// RegisterTool associates a [ToolFactory] with the given name.
// When [ResolveTool] is called with that name the factory is invoked.
//
// Registering the same name twice overwrites the previous factory.
func (r *Registry) RegisterTool(name string, factory ToolFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[name] = factory
}

// RegisterSkill associates a SkillFactory with the given name.
// When Build encounters a SkillsetRef with this name, the factory is invoked
// with the merged configuration from SkillsetRef.Config.
//
// Registering the same name twice overwrites the previous factory.
//
// Example:
//
//	reg.RegisterSkill("filesystem", func(cfg map[string]any) (skill.Source, error) {
//	    path, ok := cfg["path"].(string)
//	    if !ok {
//	        return nil, fmt.Errorf("filesystem skill factory requires 'path' config")
//	    }
//	    absPath, err := filepath.Abs(path)
//	    if err != nil {
//	        return nil, fmt.Errorf("resolve skill path: %w", err)
//	    }
//	    return skill.NewFileSystemSource(os.DirFS(absPath)), nil
//	})
func (r *Registry) RegisterSkill(name string, factory SkillFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[name] = factory
}

// ResolveSkill looks up a SkillFactory by name and creates a skill.Source.
//
// The factory is called with the provided cfg map (from SkillsetRef.Config).
// Returns an error if no factory is registered for the given name or if
// the factory returns an error.
//
// Example:
//
//	source, err := r.ResolveSkill("filesystem", map[string]any{
//	    "path": "./my-skills",
//	})
//	if err != nil {
//	    log.Fatalf("Failed to create skill source: %v", err)
//	}
//	// Use source with skilltoolset.New()
func (r *Registry) ResolveSkill(name string, cfg map[string]any) (skill.Source, error) {
	r.mu.RLock()
	factory, ok := r.skills[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("config.Registry.ResolveSkill: no factory registered for skill %q", name)
	}
	return factory(cfg)
}

// ResolveModel looks up a [ModelFactory] by splitting ref on the first "/"
// (prefix = everything before the slash, remainder = everything after), then
// calls the factory with a merged config map:
//
//	{"model": remainder, ...generateConfig}
//
// If ref contains no "/" or the derived prefix has no registered factory,
// an error is returned.
//
// Example:
//
//	llm, err := r.ResolveModel("openai/gpt-4o", map[string]any{"temperature": 0.5})
func (r *Registry) ResolveModel(ref string, generateConfig map[string]any) (model.LLM, error) {
	idx := strings.Index(ref, "/")
	if idx < 0 {
		return nil, fmt.Errorf("config.Registry.ResolveModel: ref %q has no prefix (expected \"prefix/model\")", ref)
	}

	prefix := ref[:idx]
	remainder := ref[idx+1:]

	r.mu.RLock()
	factory, ok := r.models[prefix]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("config.Registry.ResolveModel: no factory registered for prefix %q", prefix)
	}

	cfg := make(map[string]any, len(generateConfig)+1)
	for k, v := range generateConfig {
		cfg[k] = v
	}
	cfg["model"] = remainder

	return factory(cfg)
}

// ResolveTool looks up a [ToolFactory] by name and calls it with the provided cfg.
// An error is returned if no factory is registered for name.
//
// Example:
//
//	t, err := r.ResolveTool("search", map[string]any{"maxResults": 5})
func (r *Registry) ResolveTool(name string, cfg map[string]any) (tool.Tool, error) {
	r.mu.RLock()
	factory, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("config.Registry.ResolveTool: no factory registered for tool %q", name)
	}
	return factory(cfg)
}

// --- Typed code-reference registries (Option D: Hybrid) ---

// RegisterBeforeModelCallback associates a before-model callback with name.
func (r *Registry) RegisterBeforeModelCallback(name string, cb llmagent.BeforeModelCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.beforeModelCallbacks[name] = cb
}

// ResolveBeforeModelCallback looks up a before-model callback by name.
// Returns an error if no callback is registered for the given name.
func (r *Registry) ResolveBeforeModelCallback(name string) (llmagent.BeforeModelCallback, error) {
	r.mu.RLock()
	cb, ok := r.beforeModelCallbacks[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no BeforeModelCallback registered for %q", name)
	}
	return cb, nil
}

// RegisterAfterModelCallback associates an after-model callback with name.
func (r *Registry) RegisterAfterModelCallback(name string, cb llmagent.AfterModelCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.afterModelCallbacks[name] = cb
}

// ResolveAfterModelCallback looks up an after-model callback by name.
// Returns an error if no callback is registered for the given name.
func (r *Registry) ResolveAfterModelCallback(name string) (llmagent.AfterModelCallback, error) {
	r.mu.RLock()
	cb, ok := r.afterModelCallbacks[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no AfterModelCallback registered for %q", name)
	}
	return cb, nil
}

// RegisterOnModelErrorCallback associates an on-model-error callback with name.
func (r *Registry) RegisterOnModelErrorCallback(name string, cb llmagent.OnModelErrorCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onModelErrorCallbacks[name] = cb
}

// ResolveOnModelErrorCallback looks up an on-model-error callback by name.
// Returns an error if no callback is registered for the given name.
func (r *Registry) ResolveOnModelErrorCallback(name string) (llmagent.OnModelErrorCallback, error) {
	r.mu.RLock()
	cb, ok := r.onModelErrorCallbacks[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no OnModelErrorCallback registered for %q", name)
	}
	return cb, nil
}

// RegisterBeforeToolCallback associates a before-tool callback with name.
func (r *Registry) RegisterBeforeToolCallback(name string, cb llmagent.BeforeToolCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.beforeToolCallbacks[name] = cb
}

// ResolveBeforeToolCallback looks up a before-tool callback by name.
// Returns an error if no callback is registered for the given name.
func (r *Registry) ResolveBeforeToolCallback(name string) (llmagent.BeforeToolCallback, error) {
	r.mu.RLock()
	cb, ok := r.beforeToolCallbacks[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no BeforeToolCallback registered for %q", name)
	}
	return cb, nil
}

// RegisterAfterToolCallback associates an after-tool callback with name.
func (r *Registry) RegisterAfterToolCallback(name string, cb llmagent.AfterToolCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.afterToolCallbacks[name] = cb
}

// ResolveAfterToolCallback looks up an after-tool callback by name.
// Returns an error if no callback is registered for the given name.
func (r *Registry) ResolveAfterToolCallback(name string) (llmagent.AfterToolCallback, error) {
	r.mu.RLock()
	cb, ok := r.afterToolCallbacks[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no AfterToolCallback registered for %q", name)
	}
	return cb, nil
}

// RegisterOnToolErrorCallback associates an on-tool-error callback with name.
func (r *Registry) RegisterOnToolErrorCallback(name string, cb llmagent.OnToolErrorCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onToolErrorCallbacks[name] = cb
}

// ResolveOnToolErrorCallback looks up an on-tool-error callback by name.
// Returns an error if no callback is registered for the given name.
func (r *Registry) ResolveOnToolErrorCallback(name string) (llmagent.OnToolErrorCallback, error) {
	r.mu.RLock()
	cb, ok := r.onToolErrorCallbacks[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no OnToolErrorCallback registered for %q", name)
	}
	return cb, nil
}

// RegisterBeforeAgentCallback associates a before-agent callback with name.
func (r *Registry) RegisterBeforeAgentCallback(name string, cb agent.BeforeAgentCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.beforeAgentCallbacks[name] = cb
}

// ResolveBeforeAgentCallback looks up a before-agent callback by name.
// Returns an error if no callback is registered for the given name.
func (r *Registry) ResolveBeforeAgentCallback(name string) (agent.BeforeAgentCallback, error) {
	r.mu.RLock()
	cb, ok := r.beforeAgentCallbacks[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no BeforeAgentCallback registered for %q", name)
	}
	return cb, nil
}

// RegisterAfterAgentCallback associates an after-agent callback with name.
func (r *Registry) RegisterAfterAgentCallback(name string, cb agent.AfterAgentCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.afterAgentCallbacks[name] = cb
}

// ResolveAfterAgentCallback looks up an after-agent callback by name.
// Returns an error if no callback is registered for the given name.
func (r *Registry) ResolveAfterAgentCallback(name string) (agent.AfterAgentCallback, error) {
	r.mu.RLock()
	cb, ok := r.afterAgentCallbacks[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no AfterAgentCallback registered for %q", name)
	}
	return cb, nil
}

// RegisterModelCode associates a ModelCodeFactory with the given name.
func (r *Registry) RegisterModelCode(name string, factory ModelCodeFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modelCodes[name] = factory
}

// ResolveModelCode looks up a ModelCodeFactory by name and calls it with args.
// Returns an error if no factory is registered for the given name.
func (r *Registry) ResolveModelCode(name string, args map[string]any) (model.LLM, error) {
	r.mu.RLock()
	factory, ok := r.modelCodes[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no ModelCode factory registered for %q", name)
	}
	return factory(args)
}

// RegisterAgent associates a pre-built agent with the given name.
func (r *Registry) RegisterAgent(name string, a agent.Agent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[name] = a
}

// ResolveAgent looks up a pre-built agent by name.
// Returns an error if no agent is registered for the given name.
func (r *Registry) ResolveAgent(name string) (agent.Agent, error) {
	r.mu.RLock()
	a, ok := r.agents[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no Agent registered for %q", name)
	}
	return a, nil
}

// RegisterSchema associates a SchemaFactory with the given name.
func (r *Registry) RegisterSchema(name string, factory SchemaFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.schemas[name] = factory
}

// ResolveSchema looks up a SchemaFactory by name and calls it with the provided args.
func (r *Registry) ResolveSchema(name string, args map[string]any) (*genai.Schema, error) {
	r.mu.RLock()
	factory, ok := r.schemas[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("config.Registry.ResolveSchema: no factory registered for schema %q", name)
	}
	return factory(args)
}
