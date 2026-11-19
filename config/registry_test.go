package config_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/ieshan/adk-go-pkg/config"
	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/skilltoolset/skill"
)

func TestRegistry_ResolveModel_Prefixed(t *testing.T) {
	t.Parallel()
	r := config.NewRegistry()
	var received map[string]any
	r.RegisterModel("openai", func(cfg map[string]any) (model.LLM, error) {
		received = cfg
		return testutil.NewFakeLLM().WithName(cfg["model"].(string)), nil
	})

	llm, err := r.ResolveModel("openai/gpt-4o", map[string]any{"temperature": 0.5})
	if err != nil {
		t.Fatalf("ResolveModel: %v", err)
	}
	if llm.Name() != "gpt-4o" {
		t.Errorf("LLM.Name: got %q, want gpt-4o", llm.Name())
	}
	if received["model"] != "gpt-4o" {
		t.Errorf("received[model]: got %v, want gpt-4o", received["model"])
	}
	if received["temperature"] != 0.5 {
		t.Errorf("received[temperature]: got %v, want 0.5", received["temperature"])
	}
}

func TestRegistry_ResolveModel_ModelArgsDoNotOverrideRef(t *testing.T) {
	t.Parallel()
	r := config.NewRegistry()
	var received map[string]any
	r.RegisterModel("mock", func(cfg map[string]any) (model.LLM, error) {
		received = cfg
		return testutil.NewFakeLLM(), nil
	})
	if _, err := r.ResolveModel("mock/fast", map[string]any{"model": "override"}); err != nil {
		t.Fatalf("ResolveModel: %v", err)
	}
	if received["model"] != "fast" {
		t.Errorf("received[model]: got %v, want fast", received["model"])
	}
}

func TestRegistry_ResolveModel_Default(t *testing.T) {
	t.Parallel()
	t.Run("bare ref", func(t *testing.T) {
		t.Parallel()
		r := config.NewRegistry()
		var received map[string]any
		r.RegisterDefaultModel(func(cfg map[string]any) (model.LLM, error) {
			received = cfg
			return testutil.NewFakeLLM(), nil
		})
		if _, err := r.ResolveModel("gemini-2.5-flash", nil); err != nil {
			t.Fatalf("ResolveModel: %v", err)
		}
		if received["model"] != "gemini-2.5-flash" {
			t.Errorf("received[model]: got %v, want gemini-2.5-flash", received["model"])
		}
	})
	t.Run("empty ref has no model key", func(t *testing.T) {
		t.Parallel()
		r := config.NewRegistry()
		var received map[string]any
		r.RegisterDefaultModel(func(cfg map[string]any) (model.LLM, error) {
			received = cfg
			return testutil.NewFakeLLM(), nil
		})
		if _, err := r.ResolveModel("", map[string]any{"x": 1}); err != nil {
			t.Fatalf("ResolveModel: %v", err)
		}
		if _, ok := received["model"]; ok {
			t.Errorf("received has model key %v, want none", received["model"])
		}
		if received["x"] != 1 {
			t.Errorf("received[x]: got %v, want 1", received["x"])
		}
	})
	t.Run("unset default", func(t *testing.T) {
		t.Parallel()
		r := config.NewRegistry()
		_, err := r.ResolveModel("gemini-2.5-flash", nil)
		if !errors.Is(err, config.ErrNoDefaultModel) {
			t.Fatalf("got %v, want ErrNoDefaultModel", err)
		}
	})
}

func TestRegistry_ResolveTool(t *testing.T) {
	t.Parallel()
	r := config.NewRegistry()
	var received map[string]any
	r.RegisterTool("search", func(cfg map[string]any) (tool.Tool, error) {
		received = cfg
		return testutil.NewFakeTool("search"), nil
	})
	got, err := r.ResolveTool("search", map[string]any{"timeout": float64(30)})
	if err != nil {
		t.Fatalf("ResolveTool: %v", err)
	}
	if got.Name() != "search" || received["timeout"] != float64(30) {
		t.Errorf("got tool %q cfg %v", got.Name(), received)
	}
}

func TestRegistry_NodeFunctions(t *testing.T) {
	t.Parallel()
	r := config.NewRegistry()
	config.RegisterNodeFunction(r, "upper", func(_ agent.Context, s string) (string, error) {
		return strings.ToUpper(s), nil
	})
	config.RegisterNodeFunction(r, "id", func(_ agent.Context, v any) (any, error) {
		return v, nil
	})

	upper, err := r.ResolveNodeFunction("upper")
	if err != nil {
		t.Fatalf("ResolveNodeFunction(upper): %v", err)
	}
	out, err := upper(nil, "hi")
	if err != nil || out != "HI" {
		t.Errorf("upper: got %v, %v want HI", out, err)
	}
	// Non-string input to a string function is converted with fmt.Sprint.
	out, err = upper(nil, 42)
	if err != nil || out != "42" {
		t.Errorf("upper(42): got %v, %v want 42", out, err)
	}

	id, err := r.ResolveNodeFunction("id")
	if err != nil {
		t.Fatalf("ResolveNodeFunction(id): %v", err)
	}
	out, err = id(nil, 7)
	if err != nil || out != 7 {
		t.Errorf("id: got %v, %v want 7", out, err)
	}

	if _, err := r.ResolveNodeFunction("missing"); err == nil {
		t.Error("missing node function: got nil, want error")
	}
}

func TestRegistry_BuiltinSkills(t *testing.T) {
	t.Parallel()
	r := config.NewRegistry()
	_, err := r.ResolveSkill("filesystem", map[string]any{})
	if !errors.Is(err, config.ErrMissingPathConfig) {
		t.Fatalf("got %v, want ErrMissingPathConfig", err)
	}
}

func TestRegistry_SkillRegistration(t *testing.T) {
	t.Parallel()
	r := config.NewRegistry()
	r.RegisterSkill("memory", func(map[string]any) (skill.Source, error) { return stubSkillSource{}, nil })
	if _, err := r.ResolveSkill("memory", nil); err != nil {
		t.Fatalf("ResolveSkill: %v", err)
	}
	if _, err := r.ResolveSkill("missing", nil); err == nil {
		t.Error("missing skill: got nil, want error")
	}
}

func TestRegistry_MissingResolutions(t *testing.T) {
	t.Parallel()
	r := config.NewRegistry()
	tests := []struct {
		name    string
		resolve func() error
	}{
		{"BeforeModelCallback", func() error { _, e := r.ResolveBeforeModelCallback("missing"); return e }},
		{"AfterModelCallback", func() error { _, e := r.ResolveAfterModelCallback("missing"); return e }},
		{"OnModelErrorCallback", func() error { _, e := r.ResolveOnModelErrorCallback("missing"); return e }},
		{"BeforeToolCallback", func() error { _, e := r.ResolveBeforeToolCallback("missing"); return e }},
		{"AfterToolCallback", func() error { _, e := r.ResolveAfterToolCallback("missing"); return e }},
		{"OnToolErrorCallback", func() error { _, e := r.ResolveOnToolErrorCallback("missing"); return e }},
		{"BeforeAgentCallback", func() error { _, e := r.ResolveBeforeAgentCallback("missing"); return e }},
		{"AfterAgentCallback", func() error { _, e := r.ResolveAfterAgentCallback("missing"); return e }},
		{"ModelCode", func() error { _, e := r.ResolveModelCode("missing", nil); return e }},
		{"Agent", func() error { _, e := r.ResolveAgent("missing"); return e }},
		{"Schema", func() error { _, e := r.ResolveSchema("missing", nil); return e }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.resolve(); err == nil {
				t.Fatal("got nil, want error")
			}
		})
	}
}

func TestRegistry_ResolveAndInvoke(t *testing.T) {
	t.Parallel()
	r := config.NewRegistry()
	var called bool
	r.RegisterBeforeModelCallback("my.cb", func(agent.Context, *model.LLMRequest) (*model.LLMResponse, error) {
		called = true
		return nil, nil
	})
	cb, err := r.ResolveBeforeModelCallback("my.cb")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	_, _ = cb(nil, nil)
	if !called {
		t.Error("resolved callback was not the registered function")
	}
}

type stubSkillSource struct{}

func (stubSkillSource) ListFrontmatters(context.Context) ([]*skill.Frontmatter, error) {
	return nil, nil
}
func (stubSkillSource) ListResources(context.Context, string, string) ([]string, error) {
	return nil, nil
}
func (stubSkillSource) LoadFrontmatter(context.Context, string) (*skill.Frontmatter, error) {
	return nil, nil
}
func (stubSkillSource) LoadInstructions(context.Context, string) (string, error) { return "", nil }
func (stubSkillSource) LoadResource(context.Context, string, string) (io.ReadCloser, error) {
	return nil, nil
}
