package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAgentRef_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	subContent := "name: sub-agent\nagent_class: LlmAgent\nmodel: gemini/gemini-pro\n"
	_ = os.WriteFile(filepath.Join(dir, "sub.yaml"), []byte(subContent), 0644)

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer func() { _ = root.Close() }()

	ref := &AgentRefConfig{ConfigPath: "sub.yaml"}
	cfg, err := ResolveAgentRef(ref, root, "parent.yaml")
	if err != nil {
		t.Fatalf("ResolveAgentRef: %v", err)
	}
	if cfg.Name() != "sub-agent" {
		t.Errorf("expected sub-agent, got %q", cfg.Name())
	}
}

func TestResolveAgentRef_RelativePath(t *testing.T) {
	dir := t.TempDir()
	subContent := "name: sub-agent\nagent_class: LlmAgent\nmodel: gemini/gemini-pro\n"
	_ = os.WriteFile(filepath.Join(dir, "sub.yaml"), []byte(subContent), 0644)

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer func() { _ = root.Close() }()

	ref := &AgentRefConfig{ConfigPath: "sub.yaml"}
	cfg, err := ResolveAgentRef(ref, root, "parent.yaml")
	if err != nil {
		t.Fatalf("ResolveAgentRef: %v", err)
	}
	if cfg.Name() != "sub-agent" {
		t.Errorf("expected sub-agent, got %q", cfg.Name())
	}
}

func TestResolveAgentRef_MissingFile(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer func() { _ = root.Close() }()

	ref := &AgentRefConfig{ConfigPath: "nonexistent.yaml"}
	_, err = ResolveAgentRef(ref, root, "parent.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestResolveAgentRef_BothFieldsError(t *testing.T) {
	ref := &AgentRefConfig{ConfigPath: "a.yaml", Code: "x"}
	_, err := ResolveAgentRef(ref, nil, "")
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestResolveAgentRef_NeitherFieldError(t *testing.T) {
	ref := &AgentRefConfig{}
	_, err := ResolveAgentRef(ref, nil, "")
	if err == nil {
		t.Fatal("expected validation error")
	}
}
