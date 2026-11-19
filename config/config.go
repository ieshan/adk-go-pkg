// Package config provides types and utilities for loading, parsing, and building
// agent configuration files in JSON or YAML format.
//
// # Overview
//
// An [AgentConfig] describes the shape of a single agent (or a hierarchy of
// sub-agents). Configuration files can be stored as .json, .yaml, or .yml files
// and loaded with [Load], or parsed directly from byte slices with [Parse].
//
// Parsing is strict: unknown fields are reported as [ErrUnknownField], and
// fields that exist in the adk-python spec but have no adk-go equivalent are
// reported as [ErrUnsupportedField]. This makes typos and non-portable
// constructs fail fast instead of being silently dropped.
//
// # Agent Skills
//
// Agents can be configured with skill sets—specialized instruction sets stored in
// SKILL.md files with YAML frontmatter. Skills extend agent capabilities without
// requiring code changes.
//
// To configure skills, add a skill_sets section to your agent config:
//
//	name: my-agent
//	agent_class: LlmAgent
//	skill_sets:
//	  - name: filesystem
//	    config:
//	      path: "./skills"
//	    preload: complete
//
// The built-in "filesystem" skill factory reads skills from disk. Register custom
// factories via [Registry.RegisterSkill] for cloud storage or other sources.
//
// # Loading a file
//
//	appCfg, err := config.Load(root, "agents/my-agent.yaml")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(appCfg.AgentConfig.Name())
//
// # Parsing bytes
//
//	appCfg, err := config.Parse([]byte(`{"name":"bot","agent_class":"LlmAgent"}`), "json")
//
// # Generation config
//
// The generate_content_config section is decoded directly into a
// [google.golang.org/genai.GenerateContentConfig]. Keys may be written in
// snake_case (recommended, matches adk-python), camelCase, or all-lowercase; the
// decoder normalizes them.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	jsonv2 "encoding/json/v2"

	"github.com/ieshan/adk-go-pkg/prompt"
	"go.yaml.in/yaml/v4"
	"google.golang.org/genai"
)

// Sentinel errors for configuration parsing. Wrap them with errors.Is.
var (
	// ErrUnknownField reports a key that is not part of the schema for the
	// agent class (or section) that contains it.
	ErrUnknownField = errors.New("unknown config field")
	// ErrUnsupportedField reports a real adk-python field that has no adk-go
	// equivalent and therefore cannot be honored.
	ErrUnsupportedField = errors.New("config field has no adk-go equivalent")
	// ErrCallbackArgs reports args on a callback code config, which adk-python
	// forbids (callback references are name-only).
	ErrCallbackArgs = errors.New("callback code configs do not take args")
	// ErrAmbiguousField reports two keys that normalize to the same field.
	ErrAmbiguousField = errors.New("config keys normalize to the same field")
	// ErrNoDefaultModel reports that a model reference needs the default model
	// factory but none was registered.
	ErrNoDefaultModel = errors.New("no default model factory registered")
)

// AgentConfig is the sealed interface for all declarative agent configurations.
// Only types defined in this package can implement it.
type AgentConfig interface {
	Name() string
	Description() string
	SubAgentEntries() []SubAgentEntry
	isAgentConfig() // sealed
}

// BaseAgentConfig holds fields common to every agent type.
// It is embedded (with json/yaml inline) into each concrete config.
type BaseAgentConfig struct {
	Name                 string          `json:"name" yaml:"name"`
	Description          string          `json:"description,omitempty" yaml:"description,omitempty"`
	SubAgentEntries      []SubAgentEntry `json:"sub_agents,omitempty" yaml:"sub_agents,omitempty"`
	BeforeAgentCallbacks []CodeConfig    `json:"before_agent_callbacks,omitempty" yaml:"before_agent_callbacks,omitempty"`
	AfterAgentCallbacks  []CodeConfig    `json:"after_agent_callbacks,omitempty" yaml:"after_agent_callbacks,omitempty"`
}

// LLMAgentConfig is the typed config for "LlmAgent" agents.
type LLMAgentConfig struct {
	BaseAgentConfig          `json:",inline" yaml:",inline"`
	Model                    string                       `json:"model,omitempty" yaml:"model,omitempty"`
	ModelCode                *CodeConfig                  `json:"model_code,omitempty" yaml:"model_code,omitempty"`
	ModelArgs                map[string]any               `json:"model_args,omitempty" yaml:"model_args,omitempty"`
	Instruction              string                       `json:"instruction,omitempty" yaml:"instruction,omitempty"`
	InstructionTemplate      *prompt.TemplateRef          `json:"instruction_template,omitempty" yaml:"instruction_template,omitempty"`
	GlobalInstruction        string                       `json:"global_instruction,omitempty" yaml:"global_instruction,omitempty"`
	InputSchema              *SchemaRef                   `json:"input_schema,omitempty" yaml:"input_schema,omitempty"`
	OutputSchema             *SchemaRef                   `json:"output_schema,omitempty" yaml:"output_schema,omitempty"`
	OutputKey                string                       `json:"output_key,omitempty" yaml:"output_key,omitempty"`
	IncludeContents          string                       `json:"include_contents,omitempty" yaml:"include_contents,omitempty"`
	Tools                    []ToolRef                    `json:"tools,omitempty" yaml:"tools,omitempty"`
	Skillsets                []SkillsetRef                `json:"skill_sets,omitempty" yaml:"skill_sets,omitempty"`
	GenerateContentConfig    *genai.GenerateContentConfig `json:"generate_content_config,omitempty" yaml:"generate_content_config,omitempty"`
	DisallowTransferToParent bool                         `json:"disallow_transfer_to_parent,omitempty" yaml:"disallow_transfer_to_parent,omitempty"`
	DisallowTransferToPeers  bool                         `json:"disallow_transfer_to_peers,omitempty" yaml:"disallow_transfer_to_peers,omitempty"`
	BeforeModelCallbacks     []CodeConfig                 `json:"before_model_callbacks,omitempty" yaml:"before_model_callbacks,omitempty"`
	AfterModelCallbacks      []CodeConfig                 `json:"after_model_callbacks,omitempty" yaml:"after_model_callbacks,omitempty"`
	OnModelErrorCallbacks    []CodeConfig                 `json:"on_model_error_callbacks,omitempty" yaml:"on_model_error_callbacks,omitempty"`
	BeforeToolCallbacks      []CodeConfig                 `json:"before_tool_callbacks,omitempty" yaml:"before_tool_callbacks,omitempty"`
	AfterToolCallbacks       []CodeConfig                 `json:"after_tool_callbacks,omitempty" yaml:"after_tool_callbacks,omitempty"`
	OnToolErrorCallbacks     []CodeConfig                 `json:"on_tool_error_callbacks,omitempty" yaml:"on_tool_error_callbacks,omitempty"`
}

// Name returns the agent name.
func (c *LLMAgentConfig) Name() string { return c.BaseAgentConfig.Name }

// Description returns the agent description.
func (c *LLMAgentConfig) Description() string { return c.BaseAgentConfig.Description }

// SubAgentEntries returns the sub-agent entries for the LLM agent.
func (c *LLMAgentConfig) SubAgentEntries() []SubAgentEntry { return c.BaseAgentConfig.SubAgentEntries }

func (*LLMAgentConfig) isAgentConfig() {}

// SequentialAgentConfig is the typed config for "SequentialAgent" agents.
type SequentialAgentConfig struct {
	BaseAgentConfig `json:",inline" yaml:",inline"`
}

// Name returns the agent name.
func (c *SequentialAgentConfig) Name() string { return c.BaseAgentConfig.Name }

// Description returns the agent description.
func (c *SequentialAgentConfig) Description() string { return c.BaseAgentConfig.Description }

// SubAgentEntries returns the sub-agent entries for the sequential agent.
func (c *SequentialAgentConfig) SubAgentEntries() []SubAgentEntry {
	return c.BaseAgentConfig.SubAgentEntries
}

func (*SequentialAgentConfig) isAgentConfig() {}

// ParallelAgentConfig is the typed config for "ParallelAgent" agents.
type ParallelAgentConfig struct {
	BaseAgentConfig `json:",inline" yaml:",inline"`
}

// Name returns the agent name.
func (c *ParallelAgentConfig) Name() string { return c.BaseAgentConfig.Name }

// Description returns the agent description.
func (c *ParallelAgentConfig) Description() string { return c.BaseAgentConfig.Description }

// SubAgentEntries returns the sub-agent entries for the parallel agent.
func (c *ParallelAgentConfig) SubAgentEntries() []SubAgentEntry {
	return c.BaseAgentConfig.SubAgentEntries
}

func (*ParallelAgentConfig) isAgentConfig() {}

// LoopAgentConfig is the typed config for "LoopAgent" agents.
type LoopAgentConfig struct {
	BaseAgentConfig `json:",inline" yaml:",inline"`
	MaxIterations   int `json:"max_iterations,omitempty" yaml:"max_iterations,omitempty"`
}

// Name returns the agent name.
func (c *LoopAgentConfig) Name() string { return c.BaseAgentConfig.Name }

// Description returns the agent description.
func (c *LoopAgentConfig) Description() string { return c.BaseAgentConfig.Description }

// SubAgentEntries returns the sub-agent entries for the loop agent.
func (c *LoopAgentConfig) SubAgentEntries() []SubAgentEntry { return c.BaseAgentConfig.SubAgentEntries }

func (*LoopAgentConfig) isAgentConfig() {}

// WorkflowAgentConfig is the typed config for "Workflow" agents.
//
// A workflow declares its graph through edges rather than sub_agents. Each
// edges entry is a chain: a sequence whose items are node references (scalar
// names, START, or .yaml/.yml file references) and/or route maps.
type WorkflowAgentConfig struct {
	BaseAgentConfig `json:",inline" yaml:",inline"`
	Edges           []any `json:"edges,omitempty" yaml:"edges,omitempty"`
	MaxConcurrency  int   `json:"max_concurrency,omitempty" yaml:"max_concurrency,omitempty"`
}

// Name returns the workflow name.
func (c *WorkflowAgentConfig) Name() string { return c.BaseAgentConfig.Name }

// Description returns the workflow description.
func (c *WorkflowAgentConfig) Description() string { return c.BaseAgentConfig.Description }

// SubAgentEntries returns the sub-agent entries for the workflow (programmatic
// construction only; sub_agents is not accepted in a Workflow config file).
func (c *WorkflowAgentConfig) SubAgentEntries() []SubAgentEntry {
	return c.BaseAgentConfig.SubAgentEntries
}

func (*WorkflowAgentConfig) isAgentConfig() {}

// ToolRef identifies a tool by name and carries optional per-instance configuration.
//
// Example:
//
//	tools:
//	  - name: search
//	    args:
//	      maxResults: 5
//
// A bare string is shorthand for a name-only reference: "search" means
// {name: search}.
type ToolRef struct {
	// Name is the registered tool identifier.
	Name string `json:"name" yaml:"name"`

	// Args holds tool-specific settings passed to the [ToolFactory] at resolve time.
	Args map[string]any `json:"args,omitempty" yaml:"args,omitempty"`
}

// SkillsetRef identifies a skill source by name with optional configuration.
// It is similar to ToolRef but for skill sources that become SkillToolsets
// via the skilltoolset package.
//
// Skillsets provide agents with access to specialized instruction sets stored
// as SKILL.md files with YAML frontmatter. The agent can list available skills,
// load skill instructions, and access skill resources (references/, assets/).
//
// Example YAML configurations:
//
// Wildcard loading (all skills from source):
//
//	skill_sets:
//	  - name: filesystem
//	    config:
//	      path: "./skills"
//	    preload: complete
//	  - name: gcs
//	    config:
//	      bucket: "my-org-skills"
//	      prefix: "production/"
//	    preload: frontmatters
//	    system_instruction: "Custom skill guidance for this agent."
//
// Specific skill loading (only selected skills):
//
//	skill_sets:
//	  - name: filesystem
//	    config:
//	      path: "./skills"
//	    names: ["weather", "cooking"]  // Only these 2 skills visible
//	    preload: frontmatters
//
// The Name field references a registered SkillFactory in the Registry.
// Built-in factories include "filesystem". Additional factories can be
// registered via Registry.RegisterSkill for custom sources (GCS, S3, etc.).
//
// Use the Names field to restrict an agent to specific skills from a source,
// improving both security (agent can't see other skills) and performance
// (filtered skills are not loaded during preload).
type SkillsetRef struct {
	// Name is the registered SkillFactory identifier.
	// Required. Must match a factory registered with Registry.RegisterSkill.
	// Built-in: "filesystem".
	Name string `json:"name" yaml:"name"`

	// Config holds skill-source-specific settings passed to the SkillFactory.
	// Optional. Recognized keys depend on the source type:
	//
	//   - "filesystem": {"path": "./skills"} - relative or absolute path
	//   - "gcs": {"bucket": "my-bucket", "prefix": "skills/"} - GCS bucket
	//
	// The "path" key is required for filesystem sources.
	Config map[string]any `json:"config,omitempty" yaml:"config,omitempty"`

	// Preload strategy for optimizing skill access at initialization time.
	// Optional. Default is "" (no preload).
	//
	// Valid values:
	//   - "" (empty): No preload, query skills on-demand (lowest memory, slowest access)
	//   - "complete": Load all skills (frontmatters, instructions, resources) into memory.
	//                 Fastest access after init. Highest memory usage. Longest init time.
	//   - "frontmatters": Load only skill frontmatters into memory. Balanced option.
	//                     Skill instructions/resources loaded on-demand.
	//
	// Use "complete" for small skill sets or when fast response is critical.
	// Use "frontmatters" for large skill sets where listing skills is frequent.
	// Use "" (default) for very large skill sets where memory is constrained.
	Preload string `json:"preload,omitempty" yaml:"preload,omitempty"`

	// Names is a list of specific skill names to include from the source.
	// Optional. If empty, all skills from the source are available (wildcard).
	// If specified, only these exact skill names will be visible to the agent.
	//
	// Use this to restrict an agent to a subset of skills from a larger source.
	// For example, to only allow "weather" and "cooking" skills from a folder
	// containing 20+ skills.
	//
	// Example:
	//
	//	skill_sets:
	//	  - name: filesystem
	//	    config:
	//	      path: "./skills"
	//	    names: ["weather", "cooking"]  // Only 2 skills visible
	//
	// The filtering happens at the source level - filtered skills are completely
	// hidden (do not appear in list_skills output) and are not preloaded.
	// This is more efficient than wildcard loading with runtime filtering.
	Names []string `json:"names,omitempty" yaml:"names,omitempty"`

	// SystemInstruction overrides the default skill system instruction.
	// Optional. If empty, skilltoolset uses its default instruction that
	// explains skill usage patterns (list skills, load skill, load resources).
	//
	// Custom instructions should explain:
	//   - When to use skills vs other tools
	//   - Any skill-specific conventions
	//   - Resource handling preferences
	SystemInstruction string `json:"system_instruction,omitempty" yaml:"system_instruction,omitempty"`
}

// AgentRefConfig references another agent by config file path or code name.
type AgentRefConfig struct {
	// ConfigPath is a root-relative path to an agent config file. It is
	// resolved relative to the referencing config file's own directory inside
	// the *os.Root passed to BuildWithPath/LoadAndBuild.
	//
	// A referenced file's own run_config/context_cache_config sections are
	// ignored; only its agent tree is used.
	ConfigPath string `json:"config_path,omitempty" yaml:"config_path,omitempty"`
	// Code is a registered agent name in the Registry, resolved at Build time.
	Code string `json:"code,omitempty" yaml:"code,omitempty"`
}

// Validate returns an error if neither or both fields are set.
func (r *AgentRefConfig) Validate() error {
	hasPath := r.ConfigPath != ""
	hasCode := r.Code != ""
	if hasPath && hasCode {
		return fmt.Errorf("agentRefConfig: only one of config_path or code may be set")
	}
	if !hasPath && !hasCode {
		return fmt.Errorf("agentRefConfig: exactly one of config_path or code must be set")
	}
	return nil
}

// CodeConfig references a Go value (callback, model factory, schema, agent)
// by a registered name. Args are passed to factory functions.
type CodeConfig struct {
	Name string         `json:"name" yaml:"name"`
	Args map[string]any `json:"args,omitempty" yaml:"args,omitempty"`
}

// SchemaRef is a tagged union that holds either an inline *genai.Schema
// or a named *CodeConfig reference. It is used for InputSchema and
// OutputSchema fields in LLMAgentConfig.
type SchemaRef struct {
	Inline *genai.Schema
	Ref    *CodeConfig
}

// UnmarshalJSON implements custom JSON unmarshaling for SchemaRef.
func (s *SchemaRef) UnmarshalJSON(data []byte) error {
	var v any
	if err := jsonv2.Unmarshal(data, &v); err != nil {
		return err
	}
	ref, err := schemaRefFromValue(v)
	if err != nil {
		return err
	}
	*s = *ref
	return nil
}

// UnmarshalYAML implements custom YAML unmarshaling for SchemaRef.
func (s *SchemaRef) UnmarshalYAML(node *yaml.Node) error {
	var v any
	if err := node.Decode(&v); err != nil {
		return err
	}
	ref, err := schemaRefFromValue(normalizeValue(v))
	if err != nil {
		return err
	}
	*s = *ref
	return nil
}

// schemaRefFromValue tri-classifies a schema value:
//   - scalar string → named reference;
//   - map containing "name" → named reference (with optional args);
//   - other map → inline genai.Schema decoded with the normalizing decoder.
func schemaRefFromValue(v any) (*SchemaRef, error) {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil, fmt.Errorf("schema: %q must be a non-empty string", "name")
		}
		return &SchemaRef{Ref: &CodeConfig{Name: t}}, nil
	case map[string]any:
		if _, hasName := t["name"]; hasName {
			if err := checkKeys(t, keySet("name", "args"), nil, "schema"); err != nil {
				return nil, err
			}
			name, _ := asString(t["name"])
			if name == "" {
				return nil, fmt.Errorf("schema: %q must be a non-empty string", "name")
			}
			ref := &CodeConfig{Name: name}
			if a, ok := t["args"]; ok {
				args, ok := asMap(a)
				if !ok {
					return nil, fmt.Errorf("schema: args: expected map, got %T", a)
				}
				ref.Args = args
			}
			return &SchemaRef{Ref: ref}, nil
		}
		sch, err := decodeGenai[genai.Schema](t, "schema")
		if err != nil {
			return nil, err
		}
		return &SchemaRef{Inline: sch}, nil
	default:
		return nil, fmt.Errorf("schema: expected string or map, got %T", v)
	}
}

// SubAgentEntry is a tagged union: either an inline AgentConfig or a reference.
type SubAgentEntry struct {
	Inline AgentConfig
	Ref    *AgentRefConfig
}

// StreamingMode defines agent streaming behavior.
type StreamingMode string

const (
	// StreamingModeNone disables streaming.
	StreamingModeNone StreamingMode = "none"
	// StreamingModeSSE enables server-sent events streaming.
	StreamingModeSSE StreamingMode = "sse"
	// StreamingModeBIDI enables bidirectional streaming. It is accepted at parse
	// time but rejected by BuildAppWithPath because agent.StreamingMode has no
	// bidi constant.
	StreamingModeBIDI StreamingMode = "bidi"
)

// RunConfig mirrors the adk-python RunConfig fields that have adk-go targets.
//
// It is a spec-aligned extension: adk-python passes RunConfig programmatically
// to the Runner rather than declaring it in agent YAML. The field names match
// google.adk.agents.run_config.RunConfig so values mean the same thing.
type RunConfig struct {
	// StreamingMode is one of "", "none", "sse", or "bidi".
	StreamingMode StreamingMode `json:"streaming_mode,omitempty" yaml:"streaming_mode,omitempty"`
	// SaveInputBlobsAsArtifacts maps to agent.RunConfig.SaveInputBlobsAsArtifacts.
	SaveInputBlobsAsArtifacts bool `json:"save_input_blobs_as_artifacts,omitempty" yaml:"save_input_blobs_as_artifacts,omitempty"`
	// SaveLiveBlob maps to agent.LiveRunConfig.SaveLiveBlob.
	SaveLiveBlob bool `json:"save_live_blob,omitempty" yaml:"save_live_blob,omitempty"`
	// MaxLLMCalls is a pointer so "unset" is distinguishable from an explicit
	// value. Mapping to adk-go semantics:
	//   - nil  → 0 (adk-go default, 500 or ADK_MAX_LLM_CALLS);
	//   - <= 0 → -1 (adk-go unbounded, matching adk-python's "<= 0 is unbounded");
	//   - > 0  → the value.
	MaxLLMCalls *int `json:"max_llm_calls,omitempty" yaml:"max_llm_calls,omitempty"`
}

// ContextCacheConfig controls context caching across agents.
//
// It is a spec-aligned extension mirroring adk-python's ContextCacheConfig; the
// three supported fields match its programmatic field names.
type ContextCacheConfig struct {
	CacheIntervals int `json:"cache_intervals,omitempty" yaml:"cache_intervals,omitempty"`
	TTLSeconds     int `json:"ttl_seconds,omitempty" yaml:"ttl_seconds,omitempty"`
	MinTokens      int `json:"min_tokens,omitempty" yaml:"min_tokens,omitempty"`
}

// SetDefaults applies Python-equivalent defaults.
func (c *ContextCacheConfig) SetDefaults() {
	if c.CacheIntervals == 0 {
		c.CacheIntervals = 10
	}
	if c.TTLSeconds == 0 {
		c.TTLSeconds = 1800
	}
}

// Validate enforces Python-equivalent constraints.
func (c *ContextCacheConfig) Validate() error {
	if c.CacheIntervals < 1 || c.CacheIntervals > 100 {
		return fmt.Errorf("context_cache_config.cache_intervals must be in [1,100]")
	}
	if c.TTLSeconds <= 0 {
		return fmt.Errorf("context_cache_config.ttl_seconds must be > 0")
	}
	if c.MinTokens < 0 {
		return fmt.Errorf("context_cache_config.min_tokens must be >= 0")
	}
	return nil
}

// AppConfig is the top-level application configuration.
// It holds the root agent tree plus optional runtime settings.
type AppConfig struct {
	AgentConfig AgentConfig
	// RunConfig is nil when the run_config section is absent.
	RunConfig *RunConfig
	// ContextCacheConfig is nil when the context_cache_config section is absent.
	ContextCacheConfig *ContextCacheConfig
}

// Load reads an agent configuration from a file at path beneath root.
// The file format is inferred from the extension:
//   - .json  → JSON
//   - .yaml  → YAML
//   - .yml   → YAML
//
// Any other extension returns an error. path is interpreted relative to root;
// root must not be nil. The *os.Root enforces filesystem boundary protection,
// rejecting absolute paths and traversal that escapes the root.
//
// Example:
//
//	appCfg, err := config.Load(root, "config/agent.yaml")
func Load(root *os.Root, path string) (*AppConfig, error) {
	if root == nil {
		return nil, errors.New("config.Load: root must not be nil")
	}
	ext := strings.ToLower(filepath.Ext(path))
	var format string
	switch ext {
	case ".json":
		format = "json"
	case ".yaml", ".yml":
		format = "yaml"
	default:
		return nil, fmt.Errorf("config.Load: unsupported file extension %q (use .json, .yaml, or .yml)", ext)
	}

	data, err := root.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config.Load: %w", err)
	}

	return Parse(data, format)
}

// Parse decodes raw bytes into an [*AppConfig].
// The format parameter must be "json" or "yaml" (case-insensitive).
//
// Example — JSON:
//
//	appCfg, err := config.Parse([]byte(`{"name":"bot","agent_class":"LlmAgent"}`), "json")
//
// Example — YAML:
//
//	appCfg, err := config.Parse([]byte("name: bot\nagent_class: LlmAgent\n"), "yaml")
func Parse(data []byte, format string) (*AppConfig, error) {
	m, err := decodeMap(data, format)
	if err != nil {
		return nil, err
	}

	appCfg := &AppConfig{}

	if v, ok := m["run_config"]; ok {
		appCfg.RunConfig, err = parseRunConfig(v, "config.Parse: run_config")
		if err != nil {
			return nil, err
		}
	}

	if v, ok := m["context_cache_config"]; ok {
		appCfg.ContextCacheConfig, err = parseContextCacheConfig(v, "config.Parse: context_cache_config")
		if err != nil {
			return nil, err
		}
		appCfg.ContextCacheConfig.SetDefaults()
		if err = appCfg.ContextCacheConfig.Validate(); err != nil {
			return nil, fmt.Errorf("config.Parse: %w", err)
		}
	}

	appCfg.AgentConfig, err = parseAgentMap(m, "config.Parse", true)
	if err != nil {
		return nil, err
	}

	return appCfg, nil
}

func decodeMap(data []byte, format string) (map[string]any, error) {
	switch strings.ToLower(format) {
	case "json":
		var m map[string]any
		if err := jsonv2.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("config.Parse JSON: %w", err)
		}
		if m == nil {
			return nil, errors.New("config.Parse JSON: empty document")
		}
		return normalizeValue(m).(map[string]any), nil
	case "yaml":
		var m map[string]any
		dec := yaml.NewDecoder(bytes.NewReader(data))
		if err := dec.Decode(&m); err != nil {
			return nil, fmt.Errorf("config.Parse YAML: %w", err)
		}
		if m == nil {
			return nil, errors.New("config.Parse YAML: empty document")
		}
		return normalizeValue(m).(map[string]any), nil
	default:
		return nil, fmt.Errorf("config.Parse: unsupported format %q (use \"json\" or \"yaml\")", format)
	}
}

// normalizeValue converts map[any]any (which YAML may produce for non-string
// keys) into map[string]any recursively. String maps and slices are traversed
// in place only when needed.
func normalizeValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			t[k] = normalizeValue(val)
		}
		return t
	case map[any]any:
		m := make(map[string]any, len(t))
		for k, val := range t {
			m[fmt.Sprint(k)] = normalizeValue(val)
		}
		return m
	case []any:
		for i, val := range t {
			t[i] = normalizeValue(val)
		}
		return t
	default:
		return v
	}
}

// --- Key validation helpers ---

func keySet(keys ...string) map[string]bool {
	s := make(map[string]bool, len(keys))
	for _, k := range keys {
		s[k] = true
	}
	return s
}

// checkKeys reports keys in m that are neither allowed nor unsupported.
// Unsupported keys produce ErrUnsupportedField; everything else ErrUnknownField.
func checkKeys(m map[string]any, allowed, unsupported map[string]bool, ctx string) error {
	for k := range m {
		if allowed[k] {
			continue
		}
		if unsupported != nil && unsupported[k] {
			return fmt.Errorf("%s: %w: %q", ctx, ErrUnsupportedField, k)
		}
		return fmt.Errorf("%s: %w %q (valid: %s)", ctx, ErrUnknownField, k, strings.Join(slices.Sorted(maps.Keys(allowed)), ", "))
	}
	return nil
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func asStringSlice(v any) ([]string, bool) {
	list, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

func asInt(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int32:
		return int(t), true
	case int64:
		return int(t), true
	case float64:
		if t != float64(int(t)) {
			return 0, false
		}
		return int(t), true
	case float32:
		if t != float32(int(t)) {
			return 0, false
		}
		return int(t), true
	default:
		return 0, false
	}
}

func asBool(v any) (bool, bool) {
	b, ok := v.(bool)
	return b, ok
}

// mustString assigns m[key] to dst, erroring on a non-string value.
// A missing key leaves dst untouched.
func mustString(m map[string]any, key, ctx string, dst *string) error {
	v, ok := m[key]
	if !ok {
		return nil
	}
	s, ok := asString(v)
	if !ok {
		return fmt.Errorf("%s: %s: expected string, got %T", ctx, key, v)
	}
	*dst = s
	return nil
}

// mustBool assigns m[key] to dst, erroring on a non-bool value.
// A missing key leaves dst untouched.
func mustBool(m map[string]any, key, ctx string, dst *bool) error {
	v, ok := m[key]
	if !ok {
		return nil
	}
	b, ok := asBool(v)
	if !ok {
		return fmt.Errorf("%s: %s: expected bool, got %T", ctx, key, v)
	}
	*dst = b
	return nil
}

// --- Agent parsing ---

var (
	commonAgentKeys = keySet(
		"agent_class", "name", "description", "sub_agents",
		"before_agent_callbacks", "after_agent_callbacks",
	)
	llmAgentKeys = keySet(
		"model", "model_code", "model_args", "instruction", "global_instruction",
		"instruction_template", "input_schema", "output_schema", "output_key",
		"include_contents", "tools", "skill_sets", "generate_content_config",
		"disallow_transfer_to_parent", "disallow_transfer_to_peers",
		"before_model_callbacks", "after_model_callbacks", "on_model_error_callbacks",
		"before_tool_callbacks", "after_tool_callbacks", "on_tool_error_callbacks",
	)
	workflowAgentKeys = keySet(
		"agent_class", "name", "description",
		"before_agent_callbacks", "after_agent_callbacks",
		"edges", "max_concurrency",
	)
	agentUnsupportedKeys = keySet("static_instruction")
)

func agentKeys(class string, topLevel bool) map[string]bool {
	allowed := make(map[string]bool)
	var base map[string]bool
	switch class {
	case "LlmAgent":
		base = commonAgentKeys
		for k := range llmAgentKeys {
			allowed[k] = true
		}
	case "Workflow":
		base = workflowAgentKeys
	case "SequentialAgent", "ParallelAgent":
		base = commonAgentKeys
	case "LoopAgent":
		base = commonAgentKeys
		allowed["max_iterations"] = true
	}
	for k := range base {
		allowed[k] = true
	}
	if topLevel {
		allowed["run_config"] = true
		allowed["context_cache_config"] = true
	}
	return allowed
}

func agentCtx(class, name string) string {
	if name == "" {
		return fmt.Sprintf("config.Parse [%s]", class)
	}
	return fmt.Sprintf("config.Parse [%s %q]", class, name)
}

// parseAgentMap validates and converts a raw agent map into its typed config.
func parseAgentMap(m map[string]any, ctx string, topLevel bool) (AgentConfig, error) {
	class := "LlmAgent"
	if v, ok := m["agent_class"]; ok {
		s, ok := asString(v)
		if !ok {
			return nil, fmt.Errorf("%s: agent_class: expected string, got %T", ctx, v)
		}
		if s != "" {
			class = s
		}
	}
	switch class {
	case "LlmAgent", "SequentialAgent", "ParallelAgent", "LoopAgent", "Workflow":
	default:
		return nil, fmt.Errorf("%s: unknown agent class %q", ctx, class)
	}

	name, _ := asString(m["name"])
	if name == "" {
		return nil, fmt.Errorf("%s: %q is required", ctx, "name")
	}
	actx := agentCtx(class, name)

	if err := checkKeys(m, agentKeys(class, topLevel), agentUnsupportedKeys, actx); err != nil {
		return nil, err
	}

	base, err := parseBase(m, actx)
	if err != nil {
		return nil, err
	}

	switch class {
	case "SequentialAgent":
		return &SequentialAgentConfig{BaseAgentConfig: *base}, nil
	case "ParallelAgent":
		return &ParallelAgentConfig{BaseAgentConfig: *base}, nil
	case "LoopAgent":
		return parseLoopAgent(m, actx, base)
	case "Workflow":
		return parseWorkflowAgent(m, actx, base)
	default:
		return parseLLMAgent(m, actx, base)
	}
}

func parseBase(m map[string]any, ctx string) (*BaseAgentConfig, error) {
	base := &BaseAgentConfig{}
	base.Name, _ = asString(m["name"])
	if err := mustString(m, "description", ctx, &base.Description); err != nil {
		return nil, err
	}

	if v, ok := m["sub_agents"]; ok {
		entries, err := parseSubAgentEntries(v, ctx)
		if err != nil {
			return nil, err
		}
		base.SubAgentEntries = entries
	}
	if v, ok := m["before_agent_callbacks"]; ok {
		cbs, err := parseCallbacks(v, ctx+".before_agent_callbacks")
		if err != nil {
			return nil, err
		}
		base.BeforeAgentCallbacks = cbs
	}
	if v, ok := m["after_agent_callbacks"]; ok {
		cbs, err := parseCallbacks(v, ctx+".after_agent_callbacks")
		if err != nil {
			return nil, err
		}
		base.AfterAgentCallbacks = cbs
	}
	return base, nil
}

func parseLLMAgent(m map[string]any, ctx string, base *BaseAgentConfig) (*LLMAgentConfig, error) {
	c := &LLMAgentConfig{BaseAgentConfig: *base}

	if err := mustString(m, "model", ctx, &c.Model); err != nil {
		return nil, err
	}

	if v, ok := m["model_code"]; ok {
		cc, err := parseCodeConfig(v, ctx+".model_code")
		if err != nil {
			return nil, err
		}
		c.ModelCode = cc
	}
	if c.Model != "" && c.ModelCode != nil {
		return nil, fmt.Errorf("%s: only one of model or model_code may be set", ctx)
	}
	if v, ok := m["model_args"]; ok {
		args, ok := asMap(v)
		if !ok {
			return nil, fmt.Errorf("%s: model_args: expected map, got %T", ctx, v)
		}
		if c.ModelCode != nil {
			return nil, fmt.Errorf("%s: model_args cannot be combined with model_code", ctx)
		}
		c.ModelArgs = args
	}

	if err := mustString(m, "instruction", ctx, &c.Instruction); err != nil {
		return nil, err
	}
	if err := mustString(m, "global_instruction", ctx, &c.GlobalInstruction); err != nil {
		return nil, err
	}
	if err := mustString(m, "output_key", ctx, &c.OutputKey); err != nil {
		return nil, err
	}

	if v, ok := m["instruction_template"]; ok {
		ref, err := parseTemplateRef(v, ctx+".instruction_template")
		if err != nil {
			return nil, err
		}
		c.InstructionTemplate = ref
	}
	if v, ok := m["input_schema"]; ok {
		ref, err := schemaRefFromValue(v)
		if err != nil {
			return nil, fmt.Errorf("%s: input_schema: %w", ctx, err)
		}
		c.InputSchema = ref
	}
	if v, ok := m["output_schema"]; ok {
		ref, err := schemaRefFromValue(v)
		if err != nil {
			return nil, fmt.Errorf("%s: output_schema: %w", ctx, err)
		}
		c.OutputSchema = ref
	}

	if v, ok := m["include_contents"]; ok {
		s, ok := asString(v)
		if !ok {
			return nil, fmt.Errorf("%s: include_contents: expected string, got %T", ctx, v)
		}
		switch s {
		case "", "none", "default":
		default:
			return nil, fmt.Errorf("%s: include_contents must be one of \"default\" or \"none\", got %q", ctx, s)
		}
		c.IncludeContents = s
	}

	if v, ok := m["tools"]; ok {
		tools, err := parseTools(v, ctx+".tools")
		if err != nil {
			return nil, err
		}
		c.Tools = tools
	}
	if v, ok := m["skill_sets"]; ok {
		skills, err := parseSkillsets(v, ctx+".skill_sets")
		if err != nil {
			return nil, err
		}
		c.Skillsets = skills
	}
	if v, ok := m["generate_content_config"]; ok {
		gm, ok := asMap(v)
		if !ok {
			return nil, fmt.Errorf("%s: generate_content_config: expected map, got %T", ctx, v)
		}
		gc, err := decodeGenerateConfig(gm)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", ctx, err)
		}
		c.GenerateContentConfig = gc
	}

	if err := mustBool(m, "disallow_transfer_to_parent", ctx, &c.DisallowTransferToParent); err != nil {
		return nil, err
	}
	if err := mustBool(m, "disallow_transfer_to_peers", ctx, &c.DisallowTransferToPeers); err != nil {
		return nil, err
	}

	callbackFields := []struct {
		key  string
		dest *[]CodeConfig
	}{
		{"before_model_callbacks", &c.BeforeModelCallbacks},
		{"after_model_callbacks", &c.AfterModelCallbacks},
		{"on_model_error_callbacks", &c.OnModelErrorCallbacks},
		{"before_tool_callbacks", &c.BeforeToolCallbacks},
		{"after_tool_callbacks", &c.AfterToolCallbacks},
		{"on_tool_error_callbacks", &c.OnToolErrorCallbacks},
	}
	for _, f := range callbackFields {
		if v, ok := m[f.key]; ok {
			cbs, err := parseCallbacks(v, ctx+"."+f.key)
			if err != nil {
				return nil, err
			}
			*f.dest = cbs
		}
	}

	return c, nil
}

func parseLoopAgent(m map[string]any, ctx string, base *BaseAgentConfig) (*LoopAgentConfig, error) {
	c := &LoopAgentConfig{BaseAgentConfig: *base}
	if v, ok := m["max_iterations"]; ok {
		i, ok := asInt(v)
		if !ok {
			return nil, fmt.Errorf("%s: max_iterations: expected integer, got %T", ctx, v)
		}
		if i < 0 {
			return nil, fmt.Errorf("%s: max_iterations must be >= 0, got %d", ctx, i)
		}
		c.MaxIterations = i
	}
	return c, nil
}

func parseWorkflowAgent(m map[string]any, ctx string, base *BaseAgentConfig) (*WorkflowAgentConfig, error) {
	c := &WorkflowAgentConfig{BaseAgentConfig: *base}
	if v, ok := m["edges"]; ok {
		edges, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("%s: edges: expected list, got %T", ctx, v)
		}
		c.Edges = edges
	}
	if v, ok := m["max_concurrency"]; ok {
		i, ok := asInt(v)
		if !ok {
			return nil, fmt.Errorf("%s: max_concurrency: expected integer, got %T", ctx, v)
		}
		c.MaxConcurrency = i
	}
	return c, nil
}

func parseSubAgentEntries(v any, ctx string) ([]SubAgentEntry, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: sub_agents: expected list, got %T", ctx, v)
	}
	entries := make([]SubAgentEntry, 0, len(list))
	for i, item := range list {
		ectx := fmt.Sprintf("%s: sub_agents[%d]", ctx, i)
		switch t := item.(type) {
		case string:
			ref := &AgentRefConfig{ConfigPath: t}
			if err := ref.Validate(); err != nil {
				return nil, fmt.Errorf("%s: %w", ectx, err)
			}
			entries = append(entries, SubAgentEntry{Ref: ref})
		case map[string]any:
			_, hasPath := t["config_path"]
			_, hasCode := t["code"]
			if hasPath || hasCode {
				if err := checkKeys(t, keySet("config_path", "code"), nil, ectx); err != nil {
					return nil, err
				}
				ref := &AgentRefConfig{}
				if err := mustString(t, "config_path", ectx, &ref.ConfigPath); err != nil {
					return nil, err
				}
				if err := mustString(t, "code", ectx, &ref.Code); err != nil {
					return nil, err
				}
				if err := ref.Validate(); err != nil {
					return nil, fmt.Errorf("%s: %w", ectx, err)
				}
				entries = append(entries, SubAgentEntry{Ref: ref})
				continue
			}
			inline, err := parseAgentMap(t, ectx, false)
			if err != nil {
				return nil, err
			}
			entries = append(entries, SubAgentEntry{Inline: inline})
		default:
			return nil, fmt.Errorf("%s: expected string or map, got %T", ectx, item)
		}
	}
	return entries, nil
}

func parseTools(v any, ctx string) ([]ToolRef, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected list, got %T", ctx, v)
	}
	tools := make([]ToolRef, 0, len(list))
	for i, item := range list {
		ictx := fmt.Sprintf("%s[%d]", ctx, i)
		switch t := item.(type) {
		case string:
			tools = append(tools, ToolRef{Name: t})
		case map[string]any:
			if err := checkKeys(t, keySet("name", "args"), nil, ictx); err != nil {
				return nil, err
			}
			name, _ := asString(t["name"])
			if name == "" {
				return nil, fmt.Errorf("%s: %q is required", ictx, "name")
			}
			ref := ToolRef{Name: name}
			if a, ok := t["args"]; ok {
				args, ok := asMap(a)
				if !ok {
					return nil, fmt.Errorf("%s: args: expected map, got %T", ictx, a)
				}
				ref.Args = args
			}
			tools = append(tools, ref)
		default:
			return nil, fmt.Errorf("%s: expected string or map, got %T", ictx, item)
		}
	}
	return tools, nil
}

func parseCallbacks(v any, ctx string) ([]CodeConfig, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected list, got %T", ctx, v)
	}
	cbs := make([]CodeConfig, 0, len(list))
	for i, item := range list {
		ictx := fmt.Sprintf("%s[%d]", ctx, i)
		switch t := item.(type) {
		case string:
			cbs = append(cbs, CodeConfig{Name: t})
		case map[string]any:
			if _, hasArgs := t["args"]; hasArgs {
				return nil, fmt.Errorf("%s: %w", ictx, ErrCallbackArgs)
			}
			if err := checkKeys(t, keySet("name"), nil, ictx); err != nil {
				return nil, err
			}
			name, _ := asString(t["name"])
			if name == "" {
				return nil, fmt.Errorf("%s: %q is required", ictx, "name")
			}
			cbs = append(cbs, CodeConfig{Name: name})
		default:
			return nil, fmt.Errorf("%s: expected string or map, got %T", ictx, item)
		}
	}
	return cbs, nil
}

func parseSkillsets(v any, ctx string) ([]SkillsetRef, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected list, got %T", ctx, v)
	}
	sets := make([]SkillsetRef, 0, len(list))
	for i, item := range list {
		ictx := fmt.Sprintf("%s[%d]", ctx, i)
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: expected map, got %T", ictx, item)
		}
		if err := checkKeys(m, keySet("name", "config", "preload", "names", "system_instruction"), nil, ictx); err != nil {
			return nil, err
		}
		name, _ := asString(m["name"])
		if name == "" {
			return nil, fmt.Errorf("%s: %q is required", ictx, "name")
		}
		ref := SkillsetRef{Name: name}
		if c, ok := m["config"]; ok {
			cfg, ok := asMap(c)
			if !ok {
				return nil, fmt.Errorf("%s: config: expected map, got %T", ictx, c)
			}
			ref.Config = cfg
		}
		if err := mustString(m, "preload", ictx, &ref.Preload); err != nil {
			return nil, err
		}
		if err := mustString(m, "system_instruction", ictx, &ref.SystemInstruction); err != nil {
			return nil, err
		}
		if n, ok := m["names"]; ok {
			names, ok := asStringSlice(n)
			if !ok {
				return nil, fmt.Errorf("%s: names: expected list of strings", ictx)
			}
			ref.Names = names
		}
		sets = append(sets, ref)
	}
	return sets, nil
}

// parseCodeConfig parses a {name, args?} factory reference (model_code).
// Callback references are name-only and use parseCallbacks instead.
func parseCodeConfig(v any, ctx string) (*CodeConfig, error) {
	m, ok := asMap(v)
	if !ok {
		return nil, fmt.Errorf("%s: expected map, got %T", ctx, v)
	}
	if err := checkKeys(m, keySet("name", "args"), nil, ctx); err != nil {
		return nil, err
	}
	name, _ := asString(m["name"])
	if name == "" {
		return nil, fmt.Errorf("%s: %q is required", ctx, "name")
	}
	cc := &CodeConfig{Name: name}
	if a, ok := m["args"]; ok {
		args, ok := asMap(a)
		if !ok {
			return nil, fmt.Errorf("%s: args: expected map, got %T", ctx, a)
		}
		cc.Args = args
	}
	return cc, nil
}

func parseTemplateRef(v any, ctx string) (*prompt.TemplateRef, error) {
	m, ok := asMap(v)
	if !ok {
		return nil, fmt.Errorf("%s: expected map, got %T", ctx, v)
	}
	if err := checkKeys(m, keySet("name", "inline", "path"), nil, ctx); err != nil {
		return nil, err
	}
	ref := &prompt.TemplateRef{}
	for _, f := range []struct {
		key string
		dst *string
	}{
		{"name", &ref.Name},
		{"inline", &ref.Inline},
		{"path", &ref.Path},
	} {
		if err := mustString(m, f.key, ctx, f.dst); err != nil {
			return nil, err
		}
	}
	if err := ref.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", ctx, err)
	}
	return ref, nil
}

// --- Runtime section parsing ---

var (
	supportedRunConfigKeys = keySet(
		"streaming_mode", "save_input_blobs_as_artifacts", "save_live_blob", "max_llm_calls",
	)
	unsupportedRunConfigKeys = keySet(
		"custom_metadata", "save_live_audio", "speech_config", "http_options", "labels",
		"service_tier", "response_modalities", "avatar_config", "support_cfc",
		"output_audio_transcription", "input_audio_transcription", "realtime_input_config",
		"explicit_vad_signal", "translation_config", "enable_affective_dialog", "proactivity",
		"session_resumption", "history_config", "context_window_compression",
		"tool_thread_pool_config", "telemetry", "get_session_config", "model_input_context",
		"include_thoughts_from_other_agents",
	)
)

func parseRunConfig(v any, ctx string) (*RunConfig, error) {
	m, ok := asMap(v)
	if !ok {
		return nil, fmt.Errorf("%s: expected map, got %T", ctx, v)
	}
	if err := checkKeys(m, supportedRunConfigKeys, unsupportedRunConfigKeys, ctx); err != nil {
		return nil, err
	}
	rc := &RunConfig{}
	var sm string
	if err := mustString(m, "streaming_mode", ctx, &sm); err != nil {
		return nil, err
	}
	rc.StreamingMode = StreamingMode(sm)
	if err := mustBool(m, "save_input_blobs_as_artifacts", ctx, &rc.SaveInputBlobsAsArtifacts); err != nil {
		return nil, err
	}
	if err := mustBool(m, "save_live_blob", ctx, &rc.SaveLiveBlob); err != nil {
		return nil, err
	}
	if v, ok := m["max_llm_calls"]; ok {
		i, ok := asInt(v)
		if !ok {
			return nil, fmt.Errorf("%s: max_llm_calls: expected integer, got %T", ctx, v)
		}
		rc.MaxLLMCalls = &i
	}
	if err := validateRunConfig(rc); err != nil {
		return nil, fmt.Errorf("%s: %w", ctx, err)
	}
	return rc, nil
}

func validateRunConfig(rc *RunConfig) error {
	switch rc.StreamingMode {
	case "", StreamingModeNone, StreamingModeSSE, StreamingModeBIDI:
		return nil
	default:
		return fmt.Errorf("streaming_mode must be one of \"\", \"none\", \"sse\", or \"bidi\", got %q", rc.StreamingMode)
	}
}

func parseContextCacheConfig(v any, ctx string) (*ContextCacheConfig, error) {
	m, ok := asMap(v)
	if !ok {
		return nil, fmt.Errorf("%s: expected map, got %T", ctx, v)
	}
	if err := checkKeys(m, keySet("cache_intervals", "ttl_seconds", "min_tokens"), keySet("create_http_options"), ctx); err != nil {
		return nil, err
	}
	c := &ContextCacheConfig{}
	if v, ok := m["cache_intervals"]; ok {
		i, ok := asInt(v)
		if !ok {
			return nil, fmt.Errorf("%s: cache_intervals: expected integer, got %T", ctx, v)
		}
		c.CacheIntervals = i
	}
	if v, ok := m["ttl_seconds"]; ok {
		i, ok := asInt(v)
		if !ok {
			return nil, fmt.Errorf("%s: ttl_seconds: expected integer, got %T", ctx, v)
		}
		c.TTLSeconds = i
	}
	if v, ok := m["min_tokens"]; ok {
		i, ok := asInt(v)
		if !ok {
			return nil, fmt.Errorf("%s: min_tokens: expected integer, got %T", ctx, v)
		}
		c.MinTokens = i
	}
	return c, nil
}
