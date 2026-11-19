package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ieshan/adk-go-pkg/config"
	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/genai"
)

func testSchema() *genai.Schema {
	return &genai.Schema{Type: genai.TypeObject}
}

// newTestRegistry returns a Registry with a "mock" model prefix, a "search"
// tool, and a default model factory.
func newTestRegistry() *config.Registry {
	r := config.NewRegistry()
	r.RegisterModel("mock", func(cfg map[string]any) (model.LLM, error) {
		name, _ := cfg["model"].(string)
		return testutil.NewFakeLLM().WithName(name), nil
	})
	r.RegisterTool("search", func(map[string]any) (tool.Tool, error) {
		return testutil.NewFakeTool("search"), nil
	})
	r.RegisterDefaultModel(func(cfg map[string]any) (model.LLM, error) {
		name, _ := cfg["model"].(string)
		if name == "" {
			name = "default"
		}
		return testutil.NewFakeLLM().WithName(name), nil
	})
	return r
}

// newRegistryNoDefault returns a Registry with no default model factory.
func newRegistryNoDefault() *config.Registry {
	r := config.NewRegistry()
	r.RegisterModel("mock", func(cfg map[string]any) (model.LLM, error) {
		name, _ := cfg["model"].(string)
		return testutil.NewFakeLLM().WithName(name), nil
	})
	return r
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func openRoot(t *testing.T, dir string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}
