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
//	type: llm
//	skillsets:
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
//	cfg, err := config.Load("agents/my-agent.yaml")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(cfg.Name())
//
// # Parsing bytes
//
//	cfg, err := config.Parse([]byte(`{"name":"bot","type":"llm"}`), "json")
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
	isAgentConfig() // sealed
}

// BaseAgentConfig holds fields common to every agent type.
// It is embedded (with json/yaml inline) into each concrete config.
type BaseAgentConfig struct {
	Name        string        `json:"name" yaml:"name"`
	Description string        `json:"description,omitempty" yaml:"description,omitempty"`
	SubAgents   []AgentConfig `json:"subAgents,omitempty" yaml:"subAgents,omitempty"`
}

// LLMAgentConfig is the typed config for "llm" agents.
type LLMAgentConfig struct {
	BaseAgentConfig          `json:",inline" yaml:",inline"`
	Model                    string         `json:"model,omitempty" yaml:"model,omitempty"`
	Instruction              string         `json:"instruction,omitempty" yaml:"instruction,omitempty"`
	Tools                    []ToolRef      `json:"tools,omitempty" yaml:"tools,omitempty"`
	Skillsets                []SkillsetRef  `json:"skillsets,omitempty" yaml:"skillsets,omitempty"`
	GenerateConfig           map[string]any `json:"generateConfig,omitempty" yaml:"generateConfig,omitempty"`
	DisallowTransferToParent bool           `json:"disallowTransferToParent,omitempty" yaml:"disallowTransferToParent,omitempty"`
	DisallowTransferToPeers  bool           `json:"disallowTransferToPeers,omitempty" yaml:"disallowTransferToPeers,omitempty"`
}

func (c *LLMAgentConfig) Name() string             { return c.BaseAgentConfig.Name }
func (c *LLMAgentConfig) Description() string      { return c.BaseAgentConfig.Description }
func (c *LLMAgentConfig) SubAgents() []AgentConfig { return c.BaseAgentConfig.SubAgents }
func (*LLMAgentConfig) Type() string               { return "llm" }
func (*LLMAgentConfig) isAgentConfig()             {}

// SequentialAgentConfig is the typed config for "sequential" agents.
type SequentialAgentConfig struct {
	BaseAgentConfig `json:",inline" yaml:",inline"`
}

func (c *SequentialAgentConfig) Name() string             { return c.BaseAgentConfig.Name }
func (c *SequentialAgentConfig) Description() string      { return c.BaseAgentConfig.Description }
func (c *SequentialAgentConfig) SubAgents() []AgentConfig { return c.BaseAgentConfig.SubAgents }
func (*SequentialAgentConfig) Type() string               { return "sequential" }
func (*SequentialAgentConfig) isAgentConfig()             {}

// ParallelAgentConfig is the typed config for "parallel" agents.
type ParallelAgentConfig struct {
	BaseAgentConfig `json:",inline" yaml:",inline"`
}

func (c *ParallelAgentConfig) Name() string             { return c.BaseAgentConfig.Name }
func (c *ParallelAgentConfig) Description() string      { return c.BaseAgentConfig.Description }
func (c *ParallelAgentConfig) SubAgents() []AgentConfig { return c.BaseAgentConfig.SubAgents }
func (*ParallelAgentConfig) Type() string               { return "parallel" }
func (*ParallelAgentConfig) isAgentConfig()             {}

// LoopAgentConfig is the typed config for "loop" agents.
type LoopAgentConfig struct {
	BaseAgentConfig `json:",inline" yaml:",inline"`
	MaxIterations   int `json:"maxIterations,omitempty" yaml:"maxIterations,omitempty"`
}

func (c *LoopAgentConfig) Name() string             { return c.BaseAgentConfig.Name }
func (c *LoopAgentConfig) Description() string      { return c.BaseAgentConfig.Description }
func (c *LoopAgentConfig) SubAgents() []AgentConfig { return c.BaseAgentConfig.SubAgents }
func (*LoopAgentConfig) Type() string               { return "loop" }
func (*LoopAgentConfig) isAgentConfig()             {}

// ToolRef identifies a tool by name and carries optional per-instance configuration.
//
// Example:
//
//	tools:
//	  - name: search
//	    config:
//	      maxResults: 5
type ToolRef struct {
	// Name is the registered tool identifier.
	Name string `json:"name" yaml:"name"`

	// Config holds tool-specific settings passed to the [ToolFactory] at resolve time.
	Config map[string]any `json:"config,omitempty" yaml:"config,omitempty"`
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
	SystemInstruction string `json:"systemInstruction,omitempty" yaml:"systemInstruction,omitempty"`
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
//	cfg, err := config.Load("config/agent.yaml")
func Load(path string) (AgentConfig, error) {
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
	Type        string           `json:"type" yaml:"type"`
	Name        string           `json:"name" yaml:"name"`
	Description string           `json:"description,omitempty" yaml:"description,omitempty"`
	SubAgents   []rawAgentConfig `json:"subAgents,omitempty" yaml:"subAgents,omitempty"`

	// LLM-only fields
	Model                    string         `json:"model,omitempty" yaml:"model,omitempty"`
	Instruction              string         `json:"instruction,omitempty" yaml:"instruction,omitempty"`
	Tools                    []ToolRef      `json:"tools,omitempty" yaml:"tools,omitempty"`
	Skillsets                []SkillsetRef  `json:"skillsets,omitempty" yaml:"skillsets,omitempty"`
	GenerateConfig           map[string]any `json:"generateConfig,omitempty" yaml:"generateConfig,omitempty"`
	DisallowTransferToParent bool           `json:"disallowTransferToParent,omitempty" yaml:"disallowTransferToParent,omitempty"`
	DisallowTransferToPeers  bool           `json:"disallowTransferToPeers,omitempty" yaml:"disallowTransferToPeers,omitempty"`

	// Loop-only fields
	MaxIterations int `json:"maxIterations,omitempty" yaml:"maxIterations,omitempty"`
}

// toAgentConfig converts a raw config to its typed AgentConfig, validating type-specific restrictions.
func toAgentConfig(raw rawAgentConfig) (AgentConfig, error) {
	// Recursively convert sub-agents.
	subAgents := make([]AgentConfig, len(raw.SubAgents))
	for i, sub := range raw.SubAgents {
		converted, err := toAgentConfig(sub)
		if err != nil {
			return nil, err
		}
		subAgents[i] = converted
	}

	base := BaseAgentConfig{
		Name:        raw.Name,
		Description: raw.Description,
		SubAgents:   subAgents,
	}

	switch raw.Type {
	case "llm":
		if raw.MaxIterations != 0 {
			return nil, fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", raw.Type, raw.Name, "maxIterations")
		}
		return &LLMAgentConfig{
			BaseAgentConfig:          base,
			Model:                    raw.Model,
			Instruction:              raw.Instruction,
			Tools:                    raw.Tools,
			Skillsets:                raw.Skillsets,
			GenerateConfig:           raw.GenerateConfig,
			DisallowTransferToParent: raw.DisallowTransferToParent,
			DisallowTransferToPeers:  raw.DisallowTransferToPeers,
		}, nil
	case "sequential":
		if err := validateNoLLMFields(raw, "sequential"); err != nil {
			return nil, err
		}
		if raw.MaxIterations != 0 {
			return nil, fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", raw.Type, raw.Name, "maxIterations")
		}
		return &SequentialAgentConfig{BaseAgentConfig: base}, nil
	case "parallel":
		if err := validateNoLLMFields(raw, "parallel"); err != nil {
			return nil, err
		}
		if raw.MaxIterations != 0 {
			return nil, fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", raw.Type, raw.Name, "maxIterations")
		}
		return &ParallelAgentConfig{BaseAgentConfig: base}, nil
	case "loop":
		if err := validateNoLLMFields(raw, "loop"); err != nil {
			return nil, err
		}
		return &LoopAgentConfig{
			BaseAgentConfig: base,
			MaxIterations:   raw.MaxIterations,
		}, nil
	default:
		return nil, fmt.Errorf("config.Parse: unknown agent type %q", raw.Type)
	}
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
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "skillsets")
	}
	if len(raw.GenerateConfig) > 0 {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "generateConfig")
	}
	if raw.DisallowTransferToParent {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "disallowTransferToParent")
	}
	if raw.DisallowTransferToPeers {
		return fmt.Errorf("config.Parse [%s %q]: field %q is not allowed for this agent type", typ, raw.Name, "disallowTransferToPeers")
	}
	return nil
}

// Parse decodes raw bytes into an [AgentConfig].
// The format parameter must be "json" or "yaml" (case-insensitive).
// YAML parsing uses strict validation - unknown fields will produce errors.
//
// Example — JSON:
//
//	cfg, err := config.Parse([]byte(`{"name":"bot","type":"llm"}`), "json")
//
// Example — YAML:
//
//	cfg, err := config.Parse([]byte("name: bot\ntype: llm\n"), "yaml")
func Parse(data []byte, format string) (AgentConfig, error) {
	var raw rawAgentConfig
	switch strings.ToLower(format) {
	case "json":
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("config.Parse JSON: %w", err)
		}
	case "yaml":
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("config.Parse YAML: %w", err)
		}
	default:
		return nil, fmt.Errorf("config.Parse: unsupported format %q (use \"json\" or \"yaml\")", format)
	}
	return toAgentConfig(raw)
}

// TranslateGenerateConfig converts a generic key–value map into a
// [*genai.GenerateContentConfig].
//
// Recognised keys and their target fields:
//
//	"temperature"     → Temperature  (*float32)
//	"topP"            → TopP         (*float32)
//	"topK"            → TopK         (*float32)
//	"maxOutputTokens" → MaxOutputTokens (int32)
//	"candidateCount"  → CandidateCount  (int32)
//	"stopSequences"   → StopSequences   ([]string)
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
