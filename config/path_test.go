package config_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ieshan/adk-go-pkg/config"
)

func TestPath_NestedConfigPathUsesChildDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "root.yaml", `
name: root
agent_class: SequentialAgent
sub_agents:
  - config_path: sub/child.yaml
`)
	writeFile(t, dir, "sub/child.yaml", `
name: child
agent_class: SequentialAgent
sub_agents:
  - config_path: grand.yaml
`)
	writeFile(t, dir, "sub/grand.yaml", "name: grand\nagent_class: LlmAgent\nmodel: mock/fast\n")
	root := openRoot(t, dir)

	a, _, _, _, err := config.LoadAndBuild(context.Background(), root, "root.yaml", newTestRegistry())
	if err != nil {
		t.Fatalf("LoadAndBuild: %v", err)
	}
	if len(a.SubAgents()) != 1 || a.SubAgents()[0].Name() != "child" {
		t.Fatalf("root sub-agents: got %v", a.SubAgents())
	}
	grand := a.SubAgents()[0].SubAgents()
	if len(grand) != 1 || grand[0].Name() != "grand" {
		t.Errorf("child sub-agents: got %v, want grand", grand)
	}
}

func TestPath_InstructionTemplateRelativeToConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "agents/prompts/system.tmpl", "You are {{.Agent.Name}}.")
	writeFile(t, dir, "agents/bot.yaml", `
name: bot
agent_class: LlmAgent
model: mock/fast
instruction_template:
  path: prompts/system.tmpl
`)
	root := openRoot(t, dir)

	if _, _, _, _, err := config.LoadAndBuild(context.Background(), root, "agents/bot.yaml", newTestRegistry()); err != nil {
		t.Fatalf("LoadAndBuild: %v", err)
	}
}

func TestPath_Containment(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "sub/child.yaml", "name: child\nagent_class: LlmAgent\nmodel: mock/fast\n")
	root := openRoot(t, dir)

	outside := filepath.Join(t.TempDir(), "secret.yaml")
	writeFile(t, filepath.Dir(outside), filepath.Base(outside), "name: x\nagent_class: LlmAgent\n")
	if err := os.Symlink(outside, filepath.Join(dir, "link.yaml")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	tests := []struct {
		name string
		ref  string
	}{
		{"absolute", "/etc/passwd"},
		{"parent escape", "../../etc/passwd"},
		{"symlink escape", "link.yaml"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &config.SequentialAgentConfig{
				BaseAgentConfig: config.BaseAgentConfig{
					Name:            "root",
					SubAgentEntries: []config.SubAgentEntry{{Ref: &config.AgentRefConfig{ConfigPath: tc.ref}}},
				},
			}
			if _, err := config.BuildWithPath(context.Background(), cfg, newTestRegistry(), root, "sub/child.yaml"); err == nil {
				t.Fatal("got nil, want containment error")
			}
		})
	}
}
