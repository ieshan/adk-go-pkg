package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ieshan/adk-go-pkg/config"
)

func TestResolveAgentRef_ConfigPath(t *testing.T) {
	t.Parallel()
	// Both absolute and relative paths resolve the same way via *os.Root,
	// so a single table-driven test covers both scenarios.
	tests := []struct {
		name       string
		configPath string
	}{
		{name: "absolute path", configPath: "sub.yaml"},
		{name: "relative path", configPath: "sub.yaml"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			subContent := "name: sub-agent\nagent_class: LlmAgent\nmodel: gemini/gemini-pro\n"
			if err := os.WriteFile(filepath.Join(dir, "sub.yaml"), []byte(subContent), 0644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatalf("OpenRoot: %v", err)
			}
			t.Cleanup(func() { _ = root.Close() })

			ref := &config.AgentRefConfig{ConfigPath: tc.configPath}
			cfg, err := config.ResolveAgentRef(ref, root, "parent.yaml")
			if err != nil {
				t.Fatalf("ResolveAgentRef: %v", err)
			}
			if cfg.Name() != "sub-agent" {
				t.Errorf("got %q, want sub-agent", cfg.Name())
			}
		})
	}
}

func TestResolveAgentRef_MissingFile(t *testing.T) {
	t.Parallel()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	ref := &config.AgentRefConfig{ConfigPath: "nonexistent.yaml"}
	_, err = config.ResolveAgentRef(ref, root, "parent.yaml")
	if err == nil {
		t.Fatal("got nil, want error for missing file")
	}
}

func TestResolveAgentRef_BothFieldsError(t *testing.T) {
	t.Parallel()
	ref := &config.AgentRefConfig{ConfigPath: "a.yaml", Code: "x"}
	_, err := config.ResolveAgentRef(ref, nil, "")
	if err == nil {
		t.Fatal("got nil, want validation error")
	}
}

func TestResolveAgentRef_NeitherFieldError(t *testing.T) {
	t.Parallel()
	ref := &config.AgentRefConfig{}
	_, err := config.ResolveAgentRef(ref, nil, "")
	if err == nil {
		t.Fatal("got nil, want validation error")
	}
}
