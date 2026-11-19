package config

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/workflow"
)

// buildWorkflowAgent builds a workflow.Workflow from a WorkflowAgentConfig and
// wraps it in an agent.Agent.
func buildWorkflowAgent(ctx context.Context, cfg *WorkflowAgentConfig, reg *Registry, root *os.Root, configPath string, subAgents []agent.Agent, st *buildState) (agent.Agent, error) {
	var collected []agent.Agent
	configDir := filepath.Dir(configPath)

	edges, err := parseEdges(ctx, reg, root, configDir, cfg.Edges, st, &collected)
	if err != nil {
		return nil, fmt.Errorf("config.Build [Workflow %q]: %w", cfg.Name(), err)
	}

	wf, err := workflow.New(cfg.Name(), edges, workflow.WithMaxConcurrency(cfg.MaxConcurrency))
	if err != nil {
		return nil, fmt.Errorf("config.Build [Workflow %q]: %w", cfg.Name(), err)
	}

	beforeCBs, err := resolveCallbacks(reg.ResolveBeforeAgentCallback, cfg.BeforeAgentCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [Workflow %q]: beforeAgentCallbacks: %w", cfg.Name(), err)
	}
	afterCBs, err := resolveCallbacks(reg.ResolveAfterAgentCallback, cfg.AfterAgentCallbacks)
	if err != nil {
		return nil, fmt.Errorf("config.Build [Workflow %q]: afterAgentCallbacks: %w", cfg.Name(), err)
	}

	allSubs := append(collected, subAgents...)
	a, err := agent.New(agent.Config{
		Name:                 cfg.Name(),
		Description:          cfg.Description(),
		SubAgents:            allSubs,
		Run:                  wf.Run,
		BeforeAgentCallbacks: beforeCBs,
		AfterAgentCallbacks:  afterCBs,
	})
	if err != nil {
		return nil, fmt.Errorf("config.Build [Workflow %q]: %w", cfg.Name(), err)
	}
	return a, nil
}

// parseEdges converts the declarative edges grammar into workflow.Edge values.
//
// Each top-level entry is a chain (a sequence). Within a chain, an item is a
// scalar node reference, a single-key {config_path: x}/{code: x} ref map, or a
// route map {route: target}.
func parseEdges(ctx context.Context, reg *Registry, root *os.Root, configDir string, raw []any, st *buildState, collected *[]agent.Agent) ([]workflow.Edge, error) {
	var edges []workflow.Edge

	for ci, chainRaw := range raw {
		chain, ok := chainRaw.([]any)
		if !ok {
			return nil, fmt.Errorf("edges[%d]: expected list, got %T", ci, chainRaw)
		}

		var chainNodes []workflow.Node
		var routeMaps []map[string]string

		for ii, item := range chain {
			switch t := item.(type) {
			case string:
				n, err := resolveNodeLike(ctx, reg, root, configDir, t, st, collected)
				if err != nil {
					return nil, err
				}
				chainNodes = append(chainNodes, n)
			case map[string]any:
				if ref, ok := singleRefMap(t); ok {
					n, err := resolveNodeLike(ctx, reg, root, configDir, ref, st, collected)
					if err != nil {
						return nil, err
					}
					chainNodes = append(chainNodes, n)
					continue
				}
				rm := make(map[string]string, len(t))
				for k, v := range t {
					s, ok := v.(string)
					if !ok {
						return nil, fmt.Errorf("edges[%d][%d]: route map value for %q must be a string, got %T", ci, ii, k, v)
					}
					rm[k] = s
				}
				routeMaps = append(routeMaps, rm)
			default:
				return nil, fmt.Errorf("edges[%d][%d]: unsupported edge item type %T", ci, ii, item)
			}
		}

		if len(routeMaps) == 0 {
			if len(chainNodes) < 2 {
				return nil, fmt.Errorf("edges[%d]: edge chain must have at least 2 nodes", ci)
			}
			edges = append(edges, workflow.Chain(chainNodes...)...)
			continue
		}

		if len(chainNodes) < 1 {
			return nil, fmt.Errorf("edges[%d]: routing chain must have a source node", ci)
		}
		if len(chainNodes) >= 2 {
			edges = append(edges, workflow.Chain(chainNodes...)...)
		}
		routerNode := chainNodes[len(chainNodes)-1]

		for _, rm := range routeMaps {
			for _, routeVal := range slices.Sorted(maps.Keys(rm)) {
				target, err := resolveNodeLike(ctx, reg, root, configDir, rm[routeVal], st, collected)
				if err != nil {
					return nil, err
				}
				var route workflow.Route
				if strings.EqualFold(routeVal, "default") {
					route = workflow.Default
				} else {
					route = workflow.StringRoute(routeVal)
				}
				edges = append(edges, workflow.Edge{From: routerNode, To: target, Route: route})
			}
		}
	}

	return edges, nil
}

// singleRefMap reports the target of a single-key {config_path: x} or
// {code: x} map. Such a map is a node reference, not a route map.
func singleRefMap(m map[string]any) (string, bool) {
	if len(m) != 1 {
		return "", false
	}
	for k, v := range m {
		if k != "config_path" && k != "code" {
			return "", false
		}
		s, ok := v.(string)
		if !ok || s == "" {
			return "", false
		}
		return s, true
	}
	return "", false
}

// resolveNodeLike maps a scalar reference to a workflow.Node. START is the
// sentinel, a .yaml/.yml reference is a file inside configDir, and anything else
// is a registered node function name.
func resolveNodeLike(ctx context.Context, reg *Registry, root *os.Root, configDir, ref string, st *buildState, collected *[]agent.Agent) (workflow.Node, error) {
	if ref == "START" {
		return workflow.Start, nil
	}
	if strings.HasSuffix(ref, ".yaml") || strings.HasSuffix(ref, ".yml") {
		return resolveNodeFile(ctx, reg, root, configDir, ref, st, collected)
	}

	if n, ok := st.fnNodes[ref]; ok {
		return n, nil
	}
	fn, err := reg.ResolveNodeFunction(ref)
	if err != nil {
		return nil, fmt.Errorf("node or function reference %q not found in registries: %w", ref, err)
	}
	n := workflow.NewFunctionNode[any, any](ref, fn, workflow.NodeConfig{})
	st.fnNodes[ref] = n
	return n, nil
}

// resolveNodeFile loads a .yaml/.yml file reference. The file may declare a
// FunctionNode, JoinNode, ToolNode, or a regular agent class.
func resolveNodeFile(ctx context.Context, reg *Registry, root *os.Root, configDir, ref string, st *buildState, collected *[]agent.Agent) (workflow.Node, error) {
	fullPath := filepath.Join(configDir, ref)
	if n, ok := st.nodes[fullPath]; ok {
		return n, nil
	}
	if root == nil {
		return nil, fmt.Errorf("node file %q: root is required for file references", ref)
	}

	data, err := root.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("node file %q: %w", ref, err)
	}
	m, err := decodeMap(data, "yaml")
	if err != nil {
		return nil, fmt.Errorf("node file %q: %w", ref, err)
	}

	class := "LlmAgent"
	if v, ok := m["agent_class"]; ok {
		s, _ := asString(v)
		if s != "" {
			class = s
		}
	}
	name, _ := asString(m["name"])
	nctx := fmt.Sprintf("config.Build: node file %q [%s]", ref, class)
	if name == "" {
		return nil, fmt.Errorf("%s: %q is required", nctx, "name")
	}

	switch class {
	case "FunctionNode":
		n, err := parseFunctionNode(m, name, reg, nctx)
		if err != nil {
			return nil, err
		}
		st.nodes[fullPath] = n
		return n, nil
	case "JoinNode":
		if err := checkKeys(m, keySet("agent_class", "name"), nil, nctx); err != nil {
			return nil, err
		}
		n := workflow.NewJoinNode(name)
		st.nodes[fullPath] = n
		return n, nil
	case "ToolNode":
		n, err := parseToolNode(m, name, reg, nctx)
		if err != nil {
			return nil, err
		}
		st.nodes[fullPath] = n
		return n, nil
	default:
		return resolveAgentNodeFile(ctx, m, reg, root, fullPath, st, collected)
	}
}

func parseFunctionNode(m map[string]any, name string, reg *Registry, ctx string) (workflow.Node, error) {
	if err := checkKeys(m, keySet("agent_class", "name", "func_code", "rerun_on_resume", "parallel_worker"), nil, ctx); err != nil {
		return nil, err
	}
	code, _ := asString(m["func_code"])
	if code == "" {
		return nil, fmt.Errorf("%s: %q is required", ctx, "func_code")
	}
	fn, err := reg.ResolveNodeFunction(code)
	if err != nil {
		return nil, fmt.Errorf("%s: resolve func_code %q: %w", ctx, code, err)
	}
	nc, err := nodeConfigFromMap(m, ctx)
	if err != nil {
		return nil, err
	}
	n := workflow.NewFunctionNode[any, any](name, fn, nc)
	return n, nil
}

func parseToolNode(m map[string]any, name string, reg *Registry, ctx string) (workflow.Node, error) {
	if err := checkKeys(m, keySet("agent_class", "name", "tool_code", "args", "rerun_on_resume", "parallel_worker"), nil, ctx); err != nil {
		return nil, err
	}
	code, _ := asString(m["tool_code"])
	if code == "" {
		return nil, fmt.Errorf("%s: %q is required", ctx, "tool_code")
	}
	var args map[string]any
	if v, ok := m["args"]; ok {
		args, _ = asMap(v)
		if args == nil {
			return nil, fmt.Errorf("%s: args: expected map, got %T", ctx, v)
		}
	}
	t, err := reg.ResolveTool(code, args)
	if err != nil {
		return nil, fmt.Errorf("%s: resolve tool_code %q: %w", ctx, code, err)
	}
	nc, err := nodeConfigFromMap(m, ctx)
	if err != nil {
		return nil, err
	}
	n, err := workflow.NewNamedToolNode(name, t, nc)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ctx, err)
	}
	return n, nil
}

func nodeConfigFromMap(m map[string]any, ctx string) (workflow.NodeConfig, error) {
	var cfg workflow.NodeConfig
	if err := mustBool(m, "parallel_worker", ctx, &cfg.ParallelWorker); err != nil {
		return cfg, err
	}
	var rerun bool
	if err := mustBool(m, "rerun_on_resume", ctx, &rerun); err != nil {
		return cfg, err
	}
	if _, ok := m["rerun_on_resume"]; ok {
		cfg.RerunOnResume = new(rerun)
	}
	return cfg, nil
}

// resolveAgentNodeFile builds a regular agent from a node file and wraps it in
// an AgentNode. A cache hit reuses the existing instance without re-adding it to
// the workflow's collected sub-agents.
func resolveAgentNodeFile(ctx context.Context, m map[string]any, reg *Registry, root *os.Root, fullPath string, st *buildState, collected *[]agent.Agent) (workflow.Node, error) {
	a, ok := st.agents[fullPath]
	if ok {
		n, err := workflow.NewAgentNode(a, workflow.NodeConfig{})
		if err != nil {
			return nil, err
		}
		st.nodes[fullPath] = n
		return n, nil
	}

	cfg, err := parseAgentMap(m, fmt.Sprintf("config.Build: node file %q", fullPath), true)
	if err != nil {
		return nil, err
	}
	a, err = buildWithPath(ctx, cfg, reg, root, fullPath, st)
	if err != nil {
		return nil, err
	}
	st.agents[fullPath] = a
	*collected = append(*collected, a)

	n, err := workflow.NewAgentNode(a, workflow.NodeConfig{})
	if err != nil {
		return nil, err
	}
	st.nodes[fullPath] = n
	return n, nil
}
