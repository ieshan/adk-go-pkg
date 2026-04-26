package config_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/skilltoolset/skill"

	"github.com/ieshan/adk-go-pkg/config"
)

// TestRegistry_RegisterModel verifies that a registered ModelFactory is called with the
// correct arguments when resolved via a "prefix/model" reference string.
//
// The registry splits the ref on the first "/" — "openai/gpt-4o" → prefix "openai",
// remainder "gpt-4o". The factory receives {"model": "gpt-4o", ...generateConfig}.
func TestRegistry_RegisterModel(t *testing.T) {
	r := config.NewRegistry()

	var receivedCfg map[string]any
	r.RegisterModel("openai", func(cfg map[string]any) (model.LLM, error) {
		receivedCfg = cfg
		return &stubLLM{name: cfg["model"].(string)}, nil
	})

	llm, err := r.ResolveModel("openai/gpt-4o", map[string]any{"temperature": 0.5})
	if err != nil {
		t.Fatalf("ResolveModel: %v", err)
	}
	if llm == nil {
		t.Fatal("got nil LLM")
	}
	if llm.Name() != "gpt-4o" {
		t.Errorf("LLM.Name: got %q, want gpt-4o", llm.Name())
	}

	// Verify the factory received the expected config map.
	if receivedCfg == nil {
		t.Fatal("factory not called")
	}
	if receivedCfg["model"] != "gpt-4o" {
		t.Errorf("receivedCfg[model]: got %v, want gpt-4o", receivedCfg["model"])
	}
	if receivedCfg["temperature"] != 0.5 {
		t.Errorf("receivedCfg[temperature]: got %v, want 0.5", receivedCfg["temperature"])
	}
}

// TestRegistry_RegisterTool verifies that a registered ToolFactory can be resolved by name
// and that the factory receives the provided config.
func TestRegistry_RegisterTool(t *testing.T) {
	r := config.NewRegistry()

	var receivedCfg map[string]any
	r.RegisterTool("search", func(cfg map[string]any) (tool.Tool, error) {
		receivedCfg = cfg
		return &stubTool{name: "search"}, nil
	})

	got, err := r.ResolveTool("search", map[string]any{"timeout": float64(30)})
	if err != nil {
		t.Fatalf("ResolveTool: %v", err)
	}
	if got == nil {
		t.Fatal("got nil tool")
	}
	if got.Name() != "search" {
		t.Errorf("Tool.Name: got %q, want search", got.Name())
	}
	if receivedCfg == nil {
		t.Fatal("factory not called")
	}
	if receivedCfg["timeout"] != float64(30) {
		t.Errorf("receivedCfg[timeout]: got %v, want 30", receivedCfg["timeout"])
	}
}

// TestRegistry_ResolveModel_NotFound verifies an error is returned for an unregistered prefix.
func TestRegistry_ResolveModel_NotFound(t *testing.T) {
	r := config.NewRegistry()
	_, err := r.ResolveModel("anthropic/claude-3", nil)
	if err == nil {
		t.Fatal("expected error for unknown prefix, got nil")
	}
}

// TestRegistry_ResolveTool_NotFound verifies an error is returned for an unregistered tool name.
func TestRegistry_ResolveTool_NotFound(t *testing.T) {
	r := config.NewRegistry()
	_, err := r.ResolveTool("unknown-tool", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
}

// TestRegistry_ResolveModel_NoPrefixSlash verifies that a ref with no "/" yields an
// error when no matching prefix is registered.
func TestRegistry_ResolveModel_NoPrefixSlash(t *testing.T) {
	r := config.NewRegistry()
	r.RegisterModel("openai", func(cfg map[string]any) (model.LLM, error) {
		return &stubLLM{}, nil
	})

	// "gpt-4o" has no "/" — no matching prefix factory.
	_, err := r.ResolveModel("gpt-4o", nil)
	if err == nil {
		t.Fatal("expected error for ref without slash and no matching prefix, got nil")
	}
}

// TestRegistry_MultipleModels verifies multiple model prefixes can coexist and route correctly.
func TestRegistry_MultipleModels(t *testing.T) {
	r := config.NewRegistry()

	r.RegisterModel("gemini", func(cfg map[string]any) (model.LLM, error) {
		return &stubLLM{name: "gemini:" + cfg["model"].(string)}, nil
	})
	r.RegisterModel("openai", func(cfg map[string]any) (model.LLM, error) {
		return &stubLLM{name: "openai:" + cfg["model"].(string)}, nil
	})

	llm1, err := r.ResolveModel("gemini/gemini-2.0-flash", nil)
	if err != nil {
		t.Fatalf("ResolveModel gemini: %v", err)
	}
	if llm1.Name() != "gemini:gemini-2.0-flash" {
		t.Errorf("llm1 Name: got %q", llm1.Name())
	}

	llm2, err := r.ResolveModel("openai/gpt-4o", nil)
	if err != nil {
		t.Fatalf("ResolveModel openai: %v", err)
	}
	if llm2.Name() != "openai:gpt-4o" {
		t.Errorf("llm2 Name: got %q", llm2.Name())
	}
}

// TestRegistry_MultipleTools verifies multiple tool factories can coexist.
func TestRegistry_MultipleTools(t *testing.T) {
	r := config.NewRegistry()

	r.RegisterTool("search", func(cfg map[string]any) (tool.Tool, error) {
		return &stubTool{name: "search"}, nil
	})
	r.RegisterTool("calculator", func(cfg map[string]any) (tool.Tool, error) {
		return &stubTool{name: "calculator"}, nil
	})

	t1, err := r.ResolveTool("search", nil)
	if err != nil {
		t.Fatalf("ResolveTool search: %v", err)
	}
	if t1.Name() != "search" {
		t.Errorf("t1.Name: got %q", t1.Name())
	}

	t2, err := r.ResolveTool("calculator", nil)
	if err != nil {
		t.Fatalf("ResolveTool calculator: %v", err)
	}
	if t2.Name() != "calculator" {
		t.Errorf("t2.Name: got %q", t2.Name())
	}
}

// TestRegistry_RegisterSkill verifies skill factory registration and resolution.
func TestRegistry_RegisterSkill(t *testing.T) {
	r := config.NewRegistry()

	var receivedCfg map[string]any
	r.RegisterSkill("memory", func(cfg map[string]any) (skill.Source, error) {
		receivedCfg = cfg
		return stubSkillSource{}, nil
	})

	source, err := r.ResolveSkill("memory", map[string]any{"data": "test"})
	if err != nil {
		t.Fatalf("ResolveSkill: %v", err)
	}
	if source == nil {
		t.Fatal("got nil skill source")
	}
	if receivedCfg == nil {
		t.Fatal("factory not called")
	}
	if receivedCfg["data"] != "test" {
		t.Errorf("receivedCfg[data]: got %v, want test", receivedCfg["data"])
	}
}

// TestRegistry_RegisterSkill_Overwrite verifies overwriting existing skill factory.
func TestRegistry_RegisterSkill_Overwrite(t *testing.T) {
	r := config.NewRegistry()

	var firstCalled, secondCalled bool
	r.RegisterSkill("test", func(cfg map[string]any) (skill.Source, error) {
		firstCalled = true
		return stubSkillSource{}, nil
	})
	r.RegisterSkill("test", func(cfg map[string]any) (skill.Source, error) {
		secondCalled = true
		return stubSkillSource{}, nil
	})

	_, err := r.ResolveSkill("test", nil)
	if err != nil {
		t.Fatalf("ResolveSkill: %v", err)
	}

	if firstCalled {
		t.Error("first factory was called, expected second to overwrite")
	}
	if !secondCalled {
		t.Error("second factory was not called")
	}
}

// TestRegistry_ResolveSkill_NotFound verifies error for unregistered skill name.
func TestRegistry_ResolveSkill_NotFound(t *testing.T) {
	r := config.NewRegistry()
	_, err := r.ResolveSkill("unknown-skill", nil)
	if err == nil {
		t.Fatal("expected error for unknown skill, got nil")
	}
}

// TestRegistry_ResolveSkill_FactoryError verifies error propagation from factory.
func TestRegistry_ResolveSkill_FactoryError(t *testing.T) {
	r := config.NewRegistry()

	r.RegisterSkill("failing", func(cfg map[string]any) (skill.Source, error) {
		return stubSkillSource{}, fmt.Errorf("factory error")
	})

	_, err := r.ResolveSkill("failing", nil)
	if err == nil {
		t.Fatal("expected error from factory, got nil")
	}
}

// TestRegistry_ResolveSkill_MultipleSkills verifies multiple skill factories coexist.
func TestRegistry_ResolveSkill_MultipleSkills(t *testing.T) {
	r := config.NewRegistry()

	r.RegisterSkill("memory", func(cfg map[string]any) (skill.Source, error) {
		return stubSkillSource{name: "memory"}, nil
	})
	r.RegisterSkill("gcs", func(cfg map[string]any) (skill.Source, error) {
		return stubSkillSource{name: "gcs"}, nil
	})

	s1, err := r.ResolveSkill("memory", nil)
	if err != nil {
		t.Fatalf("ResolveSkill memory: %v", err)
	}
	if s1.(stubSkillSource).name != "memory" {
		t.Errorf("s1.name: got %q", s1.(stubSkillSource).name)
	}

	s2, err := r.ResolveSkill("gcs", nil)
	if err != nil {
		t.Fatalf("ResolveSkill gcs: %v", err)
	}
	if s2.(stubSkillSource).name != "gcs" {
		t.Errorf("s2.name: got %q", s2.(stubSkillSource).name)
	}
}

// TestNewRegistry_BuiltinSkills verifies built-in filesystem factory is registered.
func TestNewRegistry_BuiltinSkills(t *testing.T) {
	r := config.NewRegistry()

	// The filesystem factory should be registered by default.
	// It will fail without a valid path, but we can verify it's registered
	// by checking that it attempts to resolve the path.
	_, err := r.ResolveSkill("filesystem", map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
	// Error should mention the missing path requirement.
	if !strings.Contains(err.Error(), "path") {
		t.Errorf("error should mention 'path', got: %v", err)
	}
}

// stubSkillSource is a test double for skill.Source.
type stubSkillSource struct {
	name string
}

// Ensure stubSkillSource implements skill.Source interface.
// We only need to stub the interface, not implement all methods fully.
func (s stubSkillSource) ListFrontmatters(ctx context.Context) ([]*skill.Frontmatter, error) {
	return nil, nil
}
func (s stubSkillSource) ListResources(ctx context.Context, name, subpath string) ([]string, error) {
	return nil, nil
}
func (s stubSkillSource) LoadFrontmatter(ctx context.Context, name string) (*skill.Frontmatter, error) {
	return nil, nil
}
func (s stubSkillSource) LoadInstructions(ctx context.Context, name string) (string, error) {
	return "", nil
}
func (s stubSkillSource) LoadResource(ctx context.Context, name, resourcePath string) (io.ReadCloser, error) {
	return nil, nil
}
