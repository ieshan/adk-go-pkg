package config_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ieshan/adk-go-pkg/config"
	"google.golang.org/adk/v2/agent"
)

func registerStringFn(r *config.Registry, name string) {
	config.RegisterNodeFunction(r, name, func(_ agent.Context, s string) (string, error) {
		return name + ":" + s, nil
	})
}

func buildWorkflowYAML(t *testing.T, reg *config.Registry, dir, yaml string) error {
	t.Helper()
	appCfg, err := config.Parse([]byte(yaml), "yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var root = openRoot(t, dir)
	_, err = config.BuildWithPath(context.Background(), appCfg.AgentConfig, reg, root, "wf.yaml")
	return err
}

func TestWorkflow_FunctionChain(t *testing.T) {
	t.Parallel()
	reg := newTestRegistry()
	registerStringFn(reg, "a")
	registerStringFn(reg, "b")

	err := buildWorkflowYAML(t, reg, t.TempDir(), `
name: wf
agent_class: Workflow
edges:
  - [START, a, b]
`)
	if err != nil {
		t.Fatalf("build workflow: %v", err)
	}
}

func TestWorkflow_RouteMap(t *testing.T) {
	t.Parallel()
	reg := newTestRegistry()
	for _, name := range []string{"router", "alpha", "fallback"} {
		registerStringFn(reg, name)
	}
	err := buildWorkflowYAML(t, reg, t.TempDir(), `
name: wf
agent_class: Workflow
edges:
  - [START, router, {ALPHA: alpha, default: fallback}]
`)
	if err != nil {
		t.Fatalf("build workflow: %v", err)
	}
}

func TestWorkflow_RefMapsInChain(t *testing.T) {
	t.Parallel()
	reg := newTestRegistry()
	registerStringFn(reg, "f")
	registerStringFn(reg, "g")

	err := buildWorkflowYAML(t, reg, t.TempDir(), `
name: wf
agent_class: Workflow
edges:
  - [START, {code: f}, g]
`)
	if err != nil {
		t.Fatalf("build workflow with code ref map: %v", err)
	}
}

func TestWorkflow_ConfigPathRefMap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "fn.yaml", "agent_class: FunctionNode\nname: fn\nfunc_code: f\n")
	reg := newTestRegistry()
	registerStringFn(reg, "f")
	registerStringFn(reg, "g")

	err := buildWorkflowYAML(t, reg, dir, `
name: wf
agent_class: Workflow
edges:
  - [START, {config_path: fn.yaml}, g]
`)
	if err != nil {
		t.Fatalf("build workflow with config_path ref map: %v", err)
	}
}

func TestWorkflow_JoinFanInDedup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "join.yaml", "agent_class: JoinNode\nname: join\n")
	reg := newTestRegistry()
	registerStringFn(reg, "a")
	registerStringFn(reg, "b")

	err := buildWorkflowYAML(t, reg, dir, `
name: wf
agent_class: Workflow
edges:
  - [START, a, join.yaml]
  - [START, b, join.yaml]
`)
	if err != nil {
		t.Fatalf("build workflow with join fan-in: %v", err)
	}
}

func TestWorkflow_FunctionFanOutSharesNode(t *testing.T) {
	t.Parallel()
	reg := newTestRegistry()
	for _, name := range []string{"f", "g", "h"} {
		registerStringFn(reg, name)
	}
	err := buildWorkflowYAML(t, reg, t.TempDir(), `
name: wf
agent_class: Workflow
edges:
  - [START, f, g]
  - [f, h]
`)
	if err != nil {
		t.Fatalf("build workflow with shared function: %v", err)
	}
}

func TestWorkflow_ToolNode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "tool.yaml", "agent_class: ToolNode\nname: toolnode\ntool_code: search\n")
	reg := newTestRegistry()
	registerStringFn(reg, "g")

	err := buildWorkflowYAML(t, reg, dir, `
name: wf
agent_class: Workflow
edges:
  - [START, tool.yaml, g]
`)
	if err != nil {
		t.Fatalf("build workflow with tool node: %v", err)
	}
}

func TestWorkflow_NodeFileErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		file    string
		content string
		target  error
	}{
		{
			name:    "description rejected",
			file:    "fn.yaml",
			content: "agent_class: FunctionNode\nname: fn\nfunc_code: f\ndescription: nope\n",
			target:  config.ErrUnknownField,
		},
		{
			name:    "bad func_code",
			file:    "fn.yaml",
			content: "agent_class: FunctionNode\nname: fn\nfunc_code: missing\n",
		},
		{
			name:    "missing name",
			file:    "fn.yaml",
			content: "agent_class: FunctionNode\nfunc_code: f\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeFile(t, dir, tc.file, tc.content)
			reg := newTestRegistry()
			registerStringFn(reg, "f")
			registerStringFn(reg, "g")

			err := buildWorkflowYAML(t, reg, dir, `
name: wf
agent_class: Workflow
edges:
  - [START, fn.yaml, g]
`)
			if err == nil {
				t.Fatal("got nil, want error")
			}
			if tc.target != nil && !errors.Is(err, tc.target) {
				t.Errorf("got %v, want %v", err, tc.target)
			}
		})
	}
}

func TestWorkflow_AgentNodeFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "agent.yaml", "name: node-agent\nagent_class: LlmAgent\nmodel: mock/fast\n")
	reg := newTestRegistry()
	registerStringFn(reg, "g")

	if err := buildWorkflowYAML(t, reg, dir, `
name: wf
agent_class: Workflow
edges:
  - [START, agent.yaml, g]
`); err != nil {
		t.Fatalf("build workflow with agent node file: %v", err)
	}
}

func TestWorkflow_MaxConcurrency(t *testing.T) {
	t.Parallel()
	reg := newTestRegistry()
	registerStringFn(reg, "a")
	registerStringFn(reg, "b")

	for _, v := range []string{"4", "-1"} {
		t.Run("max_concurrency="+v, func(t *testing.T) {
			t.Parallel()
			err := buildWorkflowYAML(t, reg, t.TempDir(), `
name: wf
agent_class: Workflow
max_concurrency: `+v+`
edges:
  - [START, a, b]
`)
			if err != nil {
				t.Fatalf("build workflow: %v", err)
			}
		})
	}
}

func TestWorkflow_MalformedEdges(t *testing.T) {
	t.Parallel()
	reg := newTestRegistry()
	registerStringFn(reg, "a")
	registerStringFn(reg, "b")

	tests := []struct {
		name  string
		edges string
	}{
		{"scalar top-level entry", "edges:\n  - a\n"},
		{"nested non-map item", "edges:\n  - [START, [a], b]\n"},
		{"non-string route value", "edges:\n  - [START, a, {A: 42}]\n"},
		{"single node chain", "edges:\n  - [START]\n"},
		{"route map without source", "edges:\n  - [{A: a}]\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := buildWorkflowYAML(t, reg, t.TempDir(), "name: wf\nagent_class: Workflow\n"+tc.edges)
			if err == nil {
				t.Fatal("got nil, want error")
			}
		})
	}
}

func TestBuild_WorkflowMergesProgrammaticSubAgents(t *testing.T) {
	t.Parallel()
	reg := newTestRegistry()
	registerStringFn(reg, "a")
	registerStringFn(reg, "b")
	cfg := &config.WorkflowAgentConfig{
		BaseAgentConfig: config.BaseAgentConfig{
			Name: "wf",
			SubAgentEntries: []config.SubAgentEntry{
				{Inline: &config.LLMAgentConfig{BaseAgentConfig: config.BaseAgentConfig{Name: "side"}, Model: "mock/fast"}},
			},
		},
		Edges: []any{[]any{"START", "a", "b"}},
	}
	a, err := config.BuildWithPath(context.Background(), cfg, reg, nil, "")
	if err != nil {
		t.Fatalf("BuildWithPath: %v", err)
	}
	if len(a.SubAgents()) != 1 || a.SubAgents()[0].Name() != "side" {
		t.Errorf("sub-agents: got %v, want [side]", a.SubAgents())
	}
}

func TestBuild_DuplicateSubAgentFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "sub.yaml", "name: sub\nagent_class: LlmAgent\nmodel: mock/fast\n")
	writeFile(t, dir, "root.yaml", `
name: root
agent_class: SequentialAgent
sub_agents:
  - config_path: sub.yaml
  - config_path: sub.yaml
`)
	root := openRoot(t, dir)
	_, _, _, _, err := config.LoadAndBuild(context.Background(), root, "root.yaml", newTestRegistry())
	if err == nil {
		t.Fatal("got nil, want duplicate-agent error")
	}
}
