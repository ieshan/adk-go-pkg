package config

import (
	"fmt"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
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

// Registry maps model prefixes and tool names to their respective factories.
// Use [NewRegistry] to create a ready-to-use instance.
//
// Example:
//
//	r := config.NewRegistry()
//	r.RegisterModel("openai", openaiFactory)
//	r.RegisterTool("search", searchFactory)
//
//	llm, err := r.ResolveModel("openai/gpt-4o", map[string]any{"temperature": 0.7})
//	tool, err := r.ResolveTool("search", nil)
type Registry struct {
	models map[string]ModelFactory
	tools  map[string]ToolFactory
}

// NewRegistry returns an initialised, empty [Registry].
func NewRegistry() *Registry {
	return &Registry{
		models: make(map[string]ModelFactory),
		tools:  make(map[string]ToolFactory),
	}
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
