package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/skilltoolset/skill"
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

// Registry maps model prefixes, tool names, and skill source names to their
// respective factories. Use [NewRegistry] to create a ready-to-use instance.
//
// The registry pattern enables declarative agent configuration where models,
// tools, and skills are referenced by name in YAML/JSON files and resolved at
// build time.
//
// Example:
//
//	r := config.NewRegistry()
//	r.RegisterModel("openai", openaiFactory)
//	r.RegisterTool("search", searchFactory)
//	r.RegisterSkill("filesystem", filesystemSkillFactory) // NEW
//
//	agent, err := config.LoadAndBuild("agent.yaml", r)
type Registry struct {
	models map[string]ModelFactory
	tools  map[string]ToolFactory
	skills map[string]SkillFactory // Skill factories by registered name
}

// NewRegistry returns an initialised [Registry] with built-in skill factories registered.
func NewRegistry() *Registry {
	r := &Registry{
		models: make(map[string]ModelFactory),
		tools:  make(map[string]ToolFactory),
		skills: make(map[string]SkillFactory),
	}

	// Register built-in filesystem skill factory.
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
	r.models[prefix] = factory
}

// RegisterTool associates a [ToolFactory] with the given name.
// When [ResolveTool] is called with that name the factory is invoked.
//
// Registering the same name twice overwrites the previous factory.
func (r *Registry) RegisterTool(name string, factory ToolFactory) {
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
	factory, ok := r.skills[name]
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

	factory, ok := r.models[prefix]
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
	factory, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("config.Registry.ResolveTool: no factory registered for tool %q", name)
	}
	return factory(cfg)
}
