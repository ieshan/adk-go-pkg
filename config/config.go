// Package config provides types and utilities for loading, parsing, and translating
// agent configuration files in JSON or YAML format.
//
// # Overview
//
// An [AgentConfig] describes the shape of a single agent (or a hierarchy of sub-agents).
// Configuration files can be stored as .json, .yaml, or .yml files and loaded with [Load],
// or parsed directly from byte slices with [Parse].
//
// # Agent Skills
//
// Agents can be configured with skillsets—specialized instruction sets stored in
// SKILL.md files with YAML frontmatter. Skills extend agent capabilities without
// requiring code changes.
//
// To configure skills, add a skillsets section to your agent config:
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
// See [SkillsetRef] for configuration details and [SkillFactory] for creating custom sources.
//
// # Loading a file
//
//	appCfg, err := config.Load("agents/my-agent.yaml")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(appCfg.AgentConfig.Name())
//
// # Parsing bytes
//
//	appCfg, err := config.Parse([]byte(`{"name":"bot","agent_class":"LlmAgent"}`), "json")
//
// # Translating generation config
//
//	gc, err := config.TranslateGenerateConfig(map[string]any{
//	    "temperature": 0.7,
//	    "maxOutputTokens": float64(1024),
//	})
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v4"
	"google.golang.org/genai"
)

// AgentConfig is the sealed interface for all declarative agent configurations.
// Only types defined in this package can implement it.
type AgentConfig interface {
	Type() string
	Name() string
	Description() string
	SubAgents() []AgentConfig
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

// LLMAgentConfig is the typed config for "llm" agents.
type LLMAgentConfig struct {
	BaseAgentConfig          `json:",inline" yaml:",inline"`
	Model                    string         `json:"model,omitempty" yaml:"model,omitempty"`
	ModelCode                *CodeConfig    `json:"model_code,omitempty" yaml:"model_code,omitempty"`
	Instruction              string         `json:"instruction,omitempty" yaml:"instruction,omitempty"`
	StaticInstruction        string         `json:"static_instruction,omitempty" yaml:"static_instruction,omitempty"`
	InputSchema              *SchemaRef     `json:"input_schema,omitempty" yaml:"input_schema,omitempty"`
	OutputSchema             *SchemaRef     `json:"output_schema,omitempty" yaml:"output_schema,omitempty"`
	OutputKey                string         `json:"output_key,omitempty" yaml:"output_key,omitempty"`
	IncludeContents          string         `json:"include_contents,omitempty" yaml:"include_contents,omitempty"`
	Tools                    []ToolRef      `json:"tools,omitempty" yaml:"tools,omitempty"`
	Skillsets                []SkillsetRef  `json:"skill_sets,omitempty" yaml:"skill_sets,omitempty"`
	GenerateConfig           map[string]any `json:"generate_content_config,omitempty" yaml:"generate_content_config,omitempty"`
	DisallowTransferToParent bool           `json:"disallow_transfer_to_parent,omitempty" yaml:"disallow_transfer_to_parent,omitempty"`
	DisallowTransferToPeers  bool           `json:"disallow_transfer_to_peers,omitempty" yaml:"disallow_transfer_to_peers,omitempty"`
	BeforeModelCallbacks     []CodeConfig   `json:"before_model_callbacks,omitempty" yaml:"before_model_callbacks,omitempty"`
	AfterModelCallbacks      []CodeConfig   `json:"after_model_callbacks,omitempty" yaml:"after_model_callbacks,omitempty"`
	OnModelErrorCallbacks    []CodeConfig   `json:"on_model_error_callbacks,omitempty" yaml:"on_model_error_callbacks,omitempty"`
	BeforeToolCallbacks      []CodeConfig   `json:"before_tool_callbacks,omitempty" yaml:"before_tool_callbacks,omitempty"`
	AfterToolCallbacks       []CodeConfig   `json:"after_tool_callbacks,omitempty" yaml:"after_tool_callbacks,omitempty"`
	OnToolErrorCallbacks     []CodeConfig   `json:"on_tool_error_callbacks,omitempty" yaml:"on_tool_error_callbacks,omitempty"`
}

func (c *LLMAgentConfig) Name() string        { return c.BaseAgentConfig.Name }
func (c *LLMAgentConfig) Description() string { return c.BaseAgentConfig.Description }
func (c *LLMAgentConfig) SubAgents() []AgentConfig {
	return inlineSubAgents(c.BaseAgentConfig.SubAgentEntries)
}
func (c *LLMAgentConfig) SubAgentEntries() []SubAgentEntry { return c.BaseAgentConfig.SubAgentEntries }
func (*LLMAgentConfig) Type() string                       { return "llm" }
func (*LLMAgentConfig) isAgentConfig()                     {}

// SequentialAgentConfig is the typed config for "sequential" agents.
type SequentialAgentConfig struct {
	BaseAgentConfig `json:",inline" yaml:",inline"`
}

func (c *SequentialAgentConfig) Name() string        { return c.BaseAgentConfig.Name }
func (c *SequentialAgentConfig) Description() string { return c.BaseAgentConfig.Description }
func (c *SequentialAgentConfig) SubAgents() []AgentConfig {
	return inlineSubAgents(c.BaseAgentConfig.SubAgentEntries)
}
func (c *SequentialAgentConfig) SubAgentEntries() []SubAgentEntry {
	return c.BaseAgentConfig.SubAgentEntries
}
func (*SequentialAgentConfig) Type() string   { return "sequential" }
func (*SequentialAgentConfig) isAgentConfig() {}

// ParallelAgentConfig is the typed config for "parallel" agents.
type ParallelAgentConfig struct {
	BaseAgentConfig `json:",inline" yaml:",inline"`
}

func (c *ParallelAgentConfig) Name() string        { return c.BaseAgentConfig.Name }
func (c *ParallelAgentConfig) Description() string { return c.BaseAgentConfig.Description }
func (c *ParallelAgentConfig) SubAgents() []AgentConfig {
	return inlineSubAgents(c.BaseAgentConfig.SubAgentEntries)
}
func (c *ParallelAgentConfig) SubAgentEntries() []SubAgentEntry {
	return c.BaseAgentConfig.SubAgentEntries
}
func (*ParallelAgentConfig) Type() string   { return "parallel" }
func (*ParallelAgentConfig) isAgentConfig() {}

// LoopAgentConfig is the typed config for "loop" agents.
type LoopAgentConfig struct {
	BaseAgentConfig `json:",inline" yaml:",inline"`
	MaxIterations   int `json:"max_iterations,omitempty" yaml:"max_iterations,omitempty"`
}

func (c *LoopAgentConfig) Name() string        { return c.BaseAgentConfig.Name }
func (c *LoopAgentConfig) Description() string { return c.BaseAgentConfig.Description }
func (c *LoopAgentConfig) SubAgents() []AgentConfig {
	return inlineSubAgents(c.BaseAgentConfig.SubAgentEntries)
}
func (c *LoopAgentConfig) SubAgentEntries() []SubAgentEntry { return c.BaseAgentConfig.SubAgentEntries }
func (*LoopAgentConfig) Type() string                       { return "loop" }
func (*LoopAgentConfig) isAgentConfig()                     {}

func inlineSubAgents(entries []SubAgentEntry) []AgentConfig {
	var agents []AgentConfig
	for _, e := range entries {
		if e.Inline != nil {
			agents = append(agents, e.Inline)
		}
	}
	return agents
}

// ToolRef identifies a tool by name and carries optional per-instance configuration.
//
// Example:
//
//	tools:
//	  - name: search
//	    args:
//	      maxResults: 5
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
//	skillsets:
//	  - name: filesystem
//	    config:
//	      path: "./skills"
//	    preload: complete
//	  - name: gcs
//	    config:
//	      bucket: "my-org-skills"
//	      prefix: "production/"
//	    preload: frontmatters
//	    systemInstruction: "Custom skill guidance for this agent."
//
// Specific skill loading (only selected skills):
//
//	skillsets:
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
	//	skillsets:
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
	ConfigPath string `json:"config_path,omitempty" yaml:"config_path,omitempty"`
	Code       string `json:"code,omitempty" yaml:"code,omitempty"`
}

// Validate returns an error if neither or both fields are set.
func (r *AgentRefConfig) Validate() error {
	hasPath := r.ConfigPath != ""
	hasCode := r.Code != ""
	if hasPath && hasCode {
		return fmt.Errorf("AgentRefConfig: only one of config_path or code may be set")
	}
	if !hasPath && !hasCode {
		return fmt.Errorf("AgentRefConfig: exactly one of config_path or code must be set")
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
// Strings are treated as CodeConfig name references. Objects with a
// "type" key are parsed as inline genai.Schema. Objects with a "name"
// key are parsed as CodeConfig references.
func (s *SchemaRef) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		s.Ref = &CodeConfig{Name: str}
		return nil
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}

	if _, hasType := m["type"]; hasType {
		var schema genai.Schema
		if err := json.Unmarshal(data, &schema); err != nil {
			return fmt.Errorf("schemaRef: invalid inline schema: %w", err)
		}
		s.Inline = &schema
		return nil
	}

	if _, hasName := m["name"]; hasName {
		var ref CodeConfig
		if err := json.Unmarshal(data, &ref); err != nil {
			return fmt.Errorf("schemaRef: invalid named ref: %w", err)
		}
		s.Ref = &ref
		return nil
	}

	return fmt.Errorf("schemaRef: must have 'type' (inline schema) or 'name' (reference) key")
}

// UnmarshalYAML implements custom YAML unmarshaling for SchemaRef using the
// yaml/v4 *yaml.Node interface.
func (s *SchemaRef) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		s.Ref = &CodeConfig{Name: node.Value}
		return nil
	case yaml.MappingNode:
		var hasType, hasName bool
		for i := 0; i < len(node.Content); i += 2 {
			switch node.Content[i].Value {
			case "type":
				hasType = true
			case "name":
				hasName = true
			}
		}
		if hasType {
			var schema genai.Schema
			if err := node.Decode(&schema); err != nil {
				return fmt.Errorf("schemaRef: invalid inline schema: %w", err)
			}
			s.Inline = &schema
			return nil
		}
		if hasName {
			var ref CodeConfig
			if err := node.Decode(&ref); err != nil {
				return fmt.Errorf("schemaRef: invalid named ref: %w", err)
			}
			s.Ref = &ref
			return nil
		}
		return fmt.Errorf("schemaRef: must have type (inline schema) or name (reference) key")
	default:
		return fmt.Errorf("schemaRef: expected scalar or mapping, got kind %d", node.Kind)
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
	StreamingModeNone StreamingMode = "none"
	StreamingModeSSE  StreamingMode = "sse"
	StreamingModeBIDI StreamingMode = "bidi"
)

// RunConfig configures runtime agent behavior.
type RunConfig struct {
	StreamingMode  StreamingMode  `json:"streaming_mode,omitempty" yaml:"streaming_mode,omitempty"`
	SaveLiveBlob   bool           `json:"save_live_blob,omitempty" yaml:"save_live_blob,omitempty"`
	CustomMetadata map[string]any `json:"custom_metadata,omitempty" yaml:"custom_metadata,omitempty"`
}

// SetDefaults applies Python-equivalent defaults.
func (r *RunConfig) SetDefaults() {
	if r.StreamingMode == "" {
		r.StreamingMode = StreamingModeNone
	}
}

// Validate currently enforces no constraints for RunConfig.
// It exists to satisfy the config validation pattern used by other types.
func (r *RunConfig) Validate() error {
	return nil
}

// LiveRunConfig configures live-session runtime behavior.
type LiveRunConfig struct {
	MaxLLMCalls int `json:"max_llm_calls,omitempty" yaml:"max_llm_calls,omitempty"`
}

// SetDefaults applies Python-equivalent defaults.
func (r *LiveRunConfig) SetDefaults() {
	if r.MaxLLMCalls == 0 {
		r.MaxLLMCalls = 500
	}
}

// Validate enforces Python-equivalent constraints.
func (r *LiveRunConfig) Validate() error {
	if r.MaxLLMCalls <= 0 {
		return fmt.Errorf("live_run_config.max_llm_calls must be > 0")
	}
	return nil
}

// ContextCacheConfig controls context caching across agents.
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
	AgentConfig        AgentConfig
	RunConfig          *RunConfig          // nil when absent in source
	LiveRunConfig      *LiveRunConfig      // nil when absent in source
	ContextCacheConfig *ContextCacheConfig // nil when absent in source
}

// Load reads an agent configuration from a file at path.
// The file format is inferred from the extension:
//   - .json  → JSON
//   - .yaml  → YAML
//   - .yml   → YAML
//
// Any other extension returns an error.
//
// Example:
//
//	appCfg, err := config.Load("config/agent.yaml")
func Load(path string) (*AppConfig, error) {
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

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config.Load: %w", err)
	}

	return Parse(data, format)
}

// rawAgentConfig captures every possible field from JSON/YAML for polymorphic parsing.
type rawAgentConfig struct {
	Type        string           `json:"agent_class" yaml:"agent_class"`
	Name        string           `json:"name" yaml:"name"`
	Description string           `json:"description,omitempty" yaml:"description,omitempty"`
	SubAgents   []map[string]any `json:"sub_agents,omitempty" yaml:"sub_agents,omitempty"`

	// LLM-only fields
	Model                    string         `json:"model,omitempty" yaml:"model,omitempty"`
	ModelCode                *CodeConfig    `json:"model_code,omitempty" yaml:"model_code,omitempty"`
	Instruction              string         `json:"instruction,omitempty" yaml:"instruction,omitempty"`
	StaticInstruction        string         `json:"static_instruction,omitempty" yaml:"static_instruction,omitempty"`
	InputSchema              *SchemaRef     `json:"input_schema,omitempty" yaml:"input_schema,omitempty"`
	OutputSchema             *SchemaRef     `json:"output_schema,omitempty" yaml:"output_schema,omitempty"`
	OutputKey                string         `json:"output_key,omitempty" yaml:"output_key,omitempty"`
	IncludeContents          string         `json:"include_contents,omitempty" yaml:"include_contents,omitempty"`
	Tools                    []ToolRef      `json:"tools,omitempty" yaml:"tools,omitempty"`
	Skillsets                []SkillsetRef  `json:"skill_sets,omitempty" yaml:"skill_sets,omitempty"`
	GenerateConfig           map[string]any `json:"generate_content_config,omitempty" yaml:"generate_content_config,omitempty"`
	DisallowTransferToParent bool           `json:"disallow_transfer_to_parent,omitempty" yaml:"disallow_transfer_to_parent,omitempty"`
	DisallowTransferToPeers  bool           `json:"disallow_transfer_to_peers,omitempty" yaml:"disallow_transfer_to_peers,omitempty"`
	BeforeModelCallbacks     []CodeConfig   `json:"before_model_callbacks,omitempty" yaml:"before_model_callbacks,omitempty"`
	AfterModelCallbacks      []CodeConfig   `json:"after_model_callbacks,omitempty" yaml:"after_model_callbacks,omitempty"`
	OnModelErrorCallbacks    []CodeConfig   `json:"on_model_error_callbacks,omitempty" yaml:"on_model_error_callbacks,omitempty"`
	BeforeToolCallbacks      []CodeConfig   `json:"before_tool_callbacks,omitempty" yaml:"before_tool_callbacks,omitempty"`
	AfterToolCallbacks       []CodeConfig   `json:"after_tool_callbacks,omitempty" yaml:"after_tool_callbacks,omitempty"`
	OnToolErrorCallbacks     []CodeConfig   `json:"on_tool_error_callbacks,omitempty" yaml:"on_tool_error_callbacks,omitempty"`
	BeforeAgentCallbacks     []CodeConfig   `json:"before_agent_callbacks,omitempty" yaml:"before_agent_callbacks,omitempty"`
	AfterAgentCallbacks      []CodeConfig   `json:"after_agent_callbacks,omitempty" yaml:"after_agent_callbacks,omitempty"`

	// Loop-only fields
	MaxIterations int `json:"max_iterations,omitempty" yaml:"max_iterations,omitempty"`

	// Runtime-only fields
	RunConfig          *RunConfig          `json:"run_config,omitempty" yaml:"run_config,omitempty"`
	LiveRunConfig      *LiveRunConfig      `json:"live_run_config,omitempty" yaml:"live_run_config,omitempty"`
	ContextCacheConfig *ContextCacheConfig `json:"context_cache_config,omitempty" yaml:"context_cache_config,omitempty"`
}

// toAgentConfig converts a raw config to its typed AgentConfig, validating type-specific restrictions.
func toAgentConfig(raw rawAgentConfig) (AgentConfig, error) {
	entries, err := parseSubAgentEntries(raw.SubAgents)
	if err != nil {
		return nil, err
	}

	base := BaseAgentConfig{
		Name:                 raw.Name,
		Description:          raw.Description,
		SubAgentEntries:      entries,
		BeforeAgentCallbacks: raw.BeforeAgentCallbacks,
		AfterAgentCallbacks:  raw.AfterAgentCallbacks,
	}

	switch raw.Type {
	case "LlmAgent":
		if raw.MaxIterations != 0 {
			return nil, fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", raw.Type, raw.Name, "max_iterations")
		}
		if raw.Model != "" && raw.ModelCode != nil {
			return nil, fmt.Errorf("config.Parse [%s %q]: only one of model or model_code may be set", raw.Type, raw.Name)
		}
		return &LLMAgentConfig{
			BaseAgentConfig:          base,
			Model:                    raw.Model,
			ModelCode:                raw.ModelCode,
			Instruction:              raw.Instruction,
			StaticInstruction:        raw.StaticInstruction,
			InputSchema:              raw.InputSchema,
			OutputSchema:             raw.OutputSchema,
			OutputKey:                raw.OutputKey,
			IncludeContents:          raw.IncludeContents,
			Tools:                    raw.Tools,
			Skillsets:                raw.Skillsets,
			GenerateConfig:           raw.GenerateConfig,
			DisallowTransferToParent: raw.DisallowTransferToParent,
			DisallowTransferToPeers:  raw.DisallowTransferToPeers,
			BeforeModelCallbacks:     raw.BeforeModelCallbacks,
			AfterModelCallbacks:      raw.AfterModelCallbacks,
			OnModelErrorCallbacks:    raw.OnModelErrorCallbacks,
			BeforeToolCallbacks:      raw.BeforeToolCallbacks,
			AfterToolCallbacks:       raw.AfterToolCallbacks,
			OnToolErrorCallbacks:     raw.OnToolErrorCallbacks,
		}, nil
	case "SequentialAgent":
		if err := validateNoLLMFields(raw, "sequential"); err != nil {
			return nil, err
		}
		if raw.MaxIterations != 0 {
			return nil, fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", raw.Type, raw.Name, "max_iterations")
		}
		return &SequentialAgentConfig{BaseAgentConfig: base}, nil
	case "ParallelAgent":
		if err := validateNoLLMFields(raw, "parallel"); err != nil {
			return nil, err
		}
		if raw.MaxIterations != 0 {
			return nil, fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", raw.Type, raw.Name, "max_iterations")
		}
		return &ParallelAgentConfig{BaseAgentConfig: base}, nil
	case "LoopAgent":
		if err := validateNoLLMFields(raw, "loop"); err != nil {
			return nil, err
		}
		return &LoopAgentConfig{
			BaseAgentConfig: base,
			MaxIterations:   raw.MaxIterations,
		}, nil
	default:
		return nil, fmt.Errorf("config.Parse: unknown agent class %q", raw.Type)
	}
}

func parseSubAgentEntries(maps []map[string]any) ([]SubAgentEntry, error) {
	var entries []SubAgentEntry
	for i, m := range maps {
		if m == nil {
			continue
		}
		// Check for reference keys
		if _, hasPath := m["config_path"]; hasPath {
			var ref AgentRefConfig
			b, _ := json.Marshal(m)
			if err := json.Unmarshal(b, &ref); err != nil {
				return nil, fmt.Errorf("sub-agent %d: invalid AgentRefConfig: %w", i, err)
			}
			if err := ref.Validate(); err != nil {
				return nil, fmt.Errorf("sub-agent %d: %w", i, err)
			}
			entries = append(entries, SubAgentEntry{Ref: &ref})
			continue
		}
		if _, hasCode := m["code"]; hasCode {
			var ref AgentRefConfig
			b, _ := json.Marshal(m)
			if err := json.Unmarshal(b, &ref); err != nil {
				return nil, fmt.Errorf("sub-agent %d: invalid AgentRefConfig: %w", i, err)
			}
			if err := ref.Validate(); err != nil {
				return nil, fmt.Errorf("sub-agent %d: %w", i, err)
			}
			entries = append(entries, SubAgentEntry{Ref: &ref})
			continue
		}
		// Inline agent
		b, _ := json.Marshal(m)
		var raw rawAgentConfig
		if err := json.Unmarshal(b, &raw); err != nil {
			return nil, fmt.Errorf("sub-agent %d: invalid inline agent: %w", i, err)
		}
		agent, err := toAgentConfig(raw)
		if err != nil {
			return nil, fmt.Errorf("sub-agent %d: %w", i, err)
		}
		entries = append(entries, SubAgentEntry{Inline: agent})
	}
	return entries, nil
}

// validateNoLLMFields returns an error if any LLM-only field is set on a non-LLM agent.
func validateNoLLMFields(raw rawAgentConfig, typ string) error {
	if raw.Model != "" {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "model")
	}
	if raw.Instruction != "" {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "instruction")
	}
	if len(raw.Tools) > 0 {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "tools")
	}
	if len(raw.Skillsets) > 0 {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "skill_sets")
	}
	if len(raw.GenerateConfig) > 0 {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "generate_content_config")
	}
	if raw.DisallowTransferToParent {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "disallow_transfer_to_parent")
	}
	if raw.DisallowTransferToPeers {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "disallow_transfer_to_peers")
	}
	if raw.ModelCode != nil {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "model_code")
	}
	if raw.StaticInstruction != "" {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "static_instruction")
	}
	if raw.InputSchema != nil {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "input_schema")
	}
	if raw.OutputSchema != nil {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "output_schema")
	}
	if raw.OutputKey != "" {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "output_key")
	}
	if raw.IncludeContents != "" && raw.IncludeContents != "default" {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "include_contents")
	}
	if len(raw.BeforeModelCallbacks) > 0 {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "before_model_callbacks")
	}
	if len(raw.AfterModelCallbacks) > 0 {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "after_model_callbacks")
	}
	if len(raw.BeforeToolCallbacks) > 0 {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "before_tool_callbacks")
	}
	if len(raw.AfterToolCallbacks) > 0 {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "after_tool_callbacks")
	}
	if len(raw.OnModelErrorCallbacks) > 0 {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "on_model_error_callbacks")
	}
	if len(raw.OnToolErrorCallbacks) > 0 {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "on_tool_error_callbacks")
	}
	if len(raw.BeforeAgentCallbacks) > 0 {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "before_agent_callbacks")
	}
	if len(raw.AfterAgentCallbacks) > 0 {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "after_agent_callbacks")
	}
	return nil
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
	var raw rawAgentConfig
	switch strings.ToLower(format) {
	case "json":
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("config.Parse JSON: %w", err)
		}
	case "yaml":
		dec := yaml.NewDecoder(bytes.NewReader(data))
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("config.Parse YAML: %w", err)
		}
	default:
		return nil, fmt.Errorf("config.Parse: unsupported format %q (use \"json\" or \"yaml\")", format)
	}

	agentCfg, err := toAgentConfig(raw)
	if err != nil {
		return nil, err
	}

	appCfg := &AppConfig{
		AgentConfig:        agentCfg,
		RunConfig:          raw.RunConfig,
		LiveRunConfig:      raw.LiveRunConfig,
		ContextCacheConfig: raw.ContextCacheConfig,
	}

	if appCfg.RunConfig != nil {
		appCfg.RunConfig.SetDefaults()
		if err := appCfg.RunConfig.Validate(); err != nil {
			return nil, fmt.Errorf("config.Parse: %w", err)
		}
	}

	if appCfg.LiveRunConfig != nil {
		appCfg.LiveRunConfig.SetDefaults()
		if err := appCfg.LiveRunConfig.Validate(); err != nil {
			return nil, fmt.Errorf("config.Parse: %w", err)
		}
	}

	if appCfg.ContextCacheConfig != nil {
		appCfg.ContextCacheConfig.SetDefaults()
		if err := appCfg.ContextCacheConfig.Validate(); err != nil {
			return nil, fmt.Errorf("config.Parse: %w", err)
		}
	}

	return appCfg, nil
}

// TranslateGenerateConfig converts a generic key–value map into a
// [*genai.GenerateContentConfig].
//
// Scalars:
//
//	"temperature"                → Temperature  (*float32)
//	"topP"                       → TopP         (*float32)
//	"topK"                       → TopK         (*float32)
//	"maxOutputTokens"            → MaxOutputTokens (*int32)
//	"candidateCount"             → CandidateCount  (*int32)
//	"responseLogprobs"           → ResponseLogprobs (bool)
//	"logprobs"                   → Logprobs     (*int32)
//	"presencePenalty"            → PresencePenalty (*float32)
//	"frequencyPenalty"           → FrequencyPenalty (*float32)
//	"seed"                       → Seed         (*int32)
//	"audioTimestamp"             → AudioTimestamp (bool)
//	"cachedContent"              → CachedContent (string)
//	"enableEnhancedCivicAnswers" → EnableEnhancedCivicAnswers (*bool)
//	"serviceTier"                → ServiceTier (ServiceTier)
//	"mediaResolution"            → MediaResolution (MediaResolution)
//	"responseMimeType"           → ResponseMIMEType (string)
//
// Slices and maps:
//
//	"stopSequences"      → StopSequences ([]string)
//	"responseModalities" → ResponseModalities ([]string)
//	"labels"             → Labels (map[string]string)
//
// Complex / nested:
//
//	"responseSchema"      → ResponseSchema (*Schema)
//	"responseJsonSchema"  → ResponseJsonSchema (any)
//	"safetySettings"      → SafetySettings ([]*SafetySetting)
//	"tools"               → Tools ([]*Tool)
//	"toolConfig"          → ToolConfig (*ToolConfig)
//	"thinkingConfig"      → ThinkingConfig (*ThinkingConfig)
//	"speechConfig"        → SpeechConfig (*SpeechConfig)
//	"imageConfig"         → ImageConfig (*ImageConfig)
//	"routingConfig"       → RoutingConfig (*GenerationConfigRoutingConfig)
//	"modelSelectionConfig"→ ModelSelectionConfig (*ModelSelectionConfig)
//	"modelArmorConfig"    → ModelArmorConfig (*ModelArmorConfig)
//	"httpOptions"         → HTTPOptions (*HTTPOptions)
//	"systemInstruction"   → SystemInstruction (*Content)
//
// Unknown keys are silently ignored. A nil map returns an empty config without error.
//
// Example:
//
//	gc, err := config.TranslateGenerateConfig(map[string]any{
//	    "temperature":     0.7,
//	    "maxOutputTokens": float64(1024),
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
func TranslateGenerateConfig(m map[string]any) (*genai.GenerateContentConfig, error) {
	gc := &genai.GenerateContentConfig{}
	if m == nil {
		return gc, nil
	}

	if v, ok := m["temperature"]; ok {
		f, err := toFloat32(v)
		if err != nil {
			return nil, fmt.Errorf("config.TranslateGenerateConfig temperature: %w", err)
		}
		gc.Temperature = &f
	}

	if v, ok := m["topP"]; ok {
		f, err := toFloat32(v)
		if err != nil {
			return nil, fmt.Errorf("config.TranslateGenerateConfig topP: %w", err)
		}
		gc.TopP = &f
	}

	if v, ok := m["topK"]; ok {
		f, err := toFloat32(v)
		if err != nil {
			return nil, fmt.Errorf("config.TranslateGenerateConfig topK: %w", err)
		}
		gc.TopK = &f
	}

	if v, ok := m["maxOutputTokens"]; ok {
		i, err := toInt32(v)
		if err != nil {
			return nil, fmt.Errorf("config.TranslateGenerateConfig maxOutputTokens: %w", err)
		}
		gc.MaxOutputTokens = i
	}

	if v, ok := m["candidateCount"]; ok {
		i, err := toInt32(v)
		if err != nil {
			return nil, fmt.Errorf("config.TranslateGenerateConfig candidateCount: %w", err)
		}
		gc.CandidateCount = i
	}

	if v, ok := m["stopSequences"]; ok {
		ss, err := toStringSlice(v)
		if err != nil {
			return nil, fmt.Errorf("config.TranslateGenerateConfig stopSequences: %w", err)
		}
		gc.StopSequences = ss
	}

	if v, ok := m["responseMimeType"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("config.TranslateGenerateConfig responseMimeType: expected string, got %T", v)
		}
		gc.ResponseMIMEType = s
	}

	if v, ok := m["responseLogprobs"]; ok {
		b, err := toBool(v)
		if err != nil {
			return nil, fmt.Errorf("config.TranslateGenerateConfig responseLogprobs: %w", err)
		}
		gc.ResponseLogprobs = b
	}

	if v, ok := m["logprobs"]; ok {
		i, err := toInt32(v)
		if err != nil {
			return nil, fmt.Errorf("config.TranslateGenerateConfig logprobs: %w", err)
		}
		gc.Logprobs = &i
	}

	if v, ok := m["presencePenalty"]; ok {
		f, err := toFloat32(v)
		if err != nil {
			return nil, fmt.Errorf("config.TranslateGenerateConfig presencePenalty: %w", err)
		}
		gc.PresencePenalty = &f
	}

	if v, ok := m["frequencyPenalty"]; ok {
		f, err := toFloat32(v)
		if err != nil {
			return nil, fmt.Errorf("config.TranslateGenerateConfig frequencyPenalty: %w", err)
		}
		gc.FrequencyPenalty = &f
	}

	if v, ok := m["seed"]; ok {
		i, err := toInt32(v)
		if err != nil {
			return nil, fmt.Errorf("config.TranslateGenerateConfig seed: %w", err)
		}
		gc.Seed = &i
	}

	if v, ok := m["audioTimestamp"]; ok {
		b, err := toBool(v)
		if err != nil {
			return nil, fmt.Errorf("config.TranslateGenerateConfig audioTimestamp: %w", err)
		}
		gc.AudioTimestamp = b
	}

	if v, ok := m["cachedContent"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("config.TranslateGenerateConfig cachedContent: expected string, got %T", v)
		}
		gc.CachedContent = s
	}

	if v, ok := m["enableEnhancedCivicAnswers"]; ok {
		b, err := toBool(v)
		if err != nil {
			return nil, fmt.Errorf("config.TranslateGenerateConfig enableEnhancedCivicAnswers: %w", err)
		}
		gc.EnableEnhancedCivicAnswers = &b
	}

	if v, ok := m["serviceTier"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("config.TranslateGenerateConfig serviceTier: expected string, got %T", v)
		}
		gc.ServiceTier = genai.ServiceTier(s)
	}

	if v, ok := m["mediaResolution"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("config.TranslateGenerateConfig mediaResolution: expected string, got %T", v)
		}
		gc.MediaResolution = genai.MediaResolution(s)
	}

	if v, ok := m["responseModalities"]; ok {
		ss, err := toStringSlice(v)
		if err != nil {
			return nil, fmt.Errorf("config.TranslateGenerateConfig responseModalities: %w", err)
		}
		gc.ResponseModalities = ss
	}

	if v, ok := m["labels"]; ok {
		m, err := toStringMap(v)
		if err != nil {
			return nil, fmt.Errorf("config.TranslateGenerateConfig labels: %w", err)
		}
		gc.Labels = m
	}

	if v, ok := m["responseSchema"]; ok {
		if err := toStruct("responseSchema", v, &gc.ResponseSchema); err != nil {
			return nil, err
		}
	}

	if v, ok := m["responseJsonSchema"]; ok {
		gc.ResponseJsonSchema = v
	}

	if v, ok := m["safetySettings"]; ok {
		if err := toStruct("safetySettings", v, &gc.SafetySettings); err != nil {
			return nil, err
		}
	}

	if v, ok := m["tools"]; ok {
		if err := toStruct("tools", v, &gc.Tools); err != nil {
			return nil, err
		}
	}

	if v, ok := m["toolConfig"]; ok {
		if err := toStruct("toolConfig", v, &gc.ToolConfig); err != nil {
			return nil, err
		}
	}

	if v, ok := m["thinkingConfig"]; ok {
		if err := toStruct("thinkingConfig", v, &gc.ThinkingConfig); err != nil {
			return nil, err
		}
	}

	if v, ok := m["speechConfig"]; ok {
		if err := toStruct("speechConfig", v, &gc.SpeechConfig); err != nil {
			return nil, err
		}
	}

	if v, ok := m["imageConfig"]; ok {
		if err := toStruct("imageConfig", v, &gc.ImageConfig); err != nil {
			return nil, err
		}
	}

	if v, ok := m["routingConfig"]; ok {
		if err := toStruct("routingConfig", v, &gc.RoutingConfig); err != nil {
			return nil, err
		}
	}

	if v, ok := m["modelSelectionConfig"]; ok {
		if err := toStruct("modelSelectionConfig", v, &gc.ModelSelectionConfig); err != nil {
			return nil, err
		}
	}

	if v, ok := m["modelArmorConfig"]; ok {
		if err := toStruct("modelArmorConfig", v, &gc.ModelArmorConfig); err != nil {
			return nil, err
		}
	}

	if v, ok := m["httpOptions"]; ok {
		if err := toStruct("httpOptions", v, &gc.HTTPOptions); err != nil {
			return nil, err
		}
	}

	if v, ok := m["systemInstruction"]; ok {
		c, err := toContent(v)
		if err != nil {
			return nil, err
		}
		gc.SystemInstruction = c
	}

	return gc, nil
}

// toFloat32 converts numeric values to float32.
func toFloat32(v any) (float32, error) {
	switch t := v.(type) {
	case float32:
		return t, nil
	case float64:
		return float32(t), nil
	case int:
		return float32(t), nil
	case int32:
		return float32(t), nil
	case int64:
		return float32(t), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to float32", v)
	}
}

// toInt32 converts numeric values to int32.
func toInt32(v any) (int32, error) {
	switch t := v.(type) {
	case int32:
		return t, nil
	case int:
		return int32(t), nil
	case int64:
		return int32(t), nil
	case float32:
		return int32(t), nil
	case float64:
		return int32(t), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to int32", v)
	}
}

// toStringSlice converts []any (where each element is a string) to []string.
func toStringSlice(v any) ([]string, error) {
	switch t := v.(type) {
	case []string:
		return t, nil
	case []any:
		ss := make([]string, 0, len(t))
		for i, elem := range t {
			s, ok := elem.(string)
			if !ok {
				return nil, fmt.Errorf("element %d is %T, want string", i, elem)
			}
			ss = append(ss, s)
		}
		return ss, nil
	default:
		return nil, fmt.Errorf("cannot convert %T to []string", v)
	}
}

// toBool converts a value to bool.
func toBool(v any) (bool, error) {
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("expected bool, got %T", v)
	}
	return b, nil
}

// toStringMap converts a value to map[string]string.
func toStringMap(v any) (map[string]string, error) {
	switch t := v.(type) {
	case map[string]string:
		return t, nil
	case map[string]any:
		m := make(map[string]string, len(t))
		for k, val := range t {
			s, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("key %q is %T, want string", k, val)
			}
			m[k] = s
		}
		return m, nil
	default:
		return nil, fmt.Errorf("expected map[string]string, got %T", v)
	}
}

// toStruct unmarshals a generic value into a typed struct via JSON round-trip.
func toStruct(key string, v any, dest any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("config.TranslateGenerateConfig %s: %w", key, err)
	}
	if err := json.Unmarshal(b, dest); err != nil {
		return fmt.Errorf("config.TranslateGenerateConfig %s: %w", key, err)
	}
	return nil
}

// toContent converts a string, map, or array of parts into a *genai.Content.
func toContent(v any) (*genai.Content, error) {
	switch t := v.(type) {
	case string:
		return genai.NewContentFromText(t, "system"), nil
	case map[string]any:
		var c genai.Content
		if err := toStruct("systemInstruction", t, &c); err != nil {
			return nil, err
		}
		return &c, nil
	case []any:
		var parts []*genai.Part
		if err := toStruct("systemInstruction", t, &parts); err != nil {
			return nil, err
		}
		return &genai.Content{Role: "system", Parts: parts}, nil
	default:
		return nil, fmt.Errorf("config.TranslateGenerateConfig systemInstruction: expected string, map, or array, got %T", v)
	}
}
