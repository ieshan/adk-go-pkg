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
//	fmt.Println(cfg.Name)
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

// AgentConfig holds the declarative configuration for a single agent.
// It supports hierarchical definitions via [AgentConfig.SubAgents] and now skill integration
// via [AgentConfig.Skillsets].
//
// An agent with skills can discover and use specialized instruction sets
// defined in SKILL.md files. Skills extend agent capabilities without
// requiring code changes to the agent itself.
//
// Example YAML with skills:
//
//	name: my-agent
//	type: llm
//	model: gemini/gemini-2.5-flash
//	instruction: "You are a helpful assistant."
//	skillsets:
//	  - name: filesystem
//	    config:
//	      path: "./skills"
//	tools:
//	  - name: search
//
// See [SkillsetRef] for skill configuration details.
type AgentConfig struct {
	// Name is the unique identifier for this agent.
	Name string `json:"name" yaml:"name"`

	// Type describes the agent kind: "llm", "sequential", "parallel", or "loop".
	Type string `json:"type" yaml:"type"`

	// Model is an optional model reference in the form "prefix/model-id".
	// Required for "llm" type agents.
	Model string `json:"model,omitempty" yaml:"model,omitempty"`

	// Instruction is an optional system-level prompt for the agent.
	Instruction string `json:"instruction,omitempty" yaml:"instruction,omitempty"`

	// Description provides a human-readable summary of the agent's purpose.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Tools lists the tool references available to this agent.
	Tools []ToolRef `json:"tools,omitempty" yaml:"tools,omitempty"`

	// Skillsets lists skill source references available to this agent.
	// Optional. Each skillset becomes a tool.Toolset via skilltoolset.New()
	// and provides the agent with access to specialized instruction sets.
	//
	// Skillsets are resolved during Build() using the Registry's SkillFactory
	// registrations. The built-in "filesystem" factory reads skills from disk.
	// Custom factories can be registered for cloud storage sources.
	//
	// Skills and tools work together: tools provide executable capabilities,
	// while skills provide domain-specific instructions and knowledge.
	Skillsets []SkillsetRef `json:"skillsets,omitempty" yaml:"skillsets,omitempty"`

	// SubAgents holds nested child agent configurations.
	SubAgents []AgentConfig `json:"subAgents,omitempty" yaml:"subAgents,omitempty"`

	// GenerateConfig contains arbitrary generation parameters (temperature, topP, etc.)
	// that are translated to [genai.GenerateContentConfig] by [TranslateGenerateConfig].
	GenerateConfig map[string]any `json:"generateConfig,omitempty" yaml:"generateConfig,omitempty"`

	// MaxIterations limits the number of execution iterations for loop-type agents.
	MaxIterations int `json:"maxIterations,omitempty" yaml:"maxIterations,omitempty"`
}

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
func Load(path string) (*AgentConfig, error) {
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
func Parse(data []byte, format string) (*AgentConfig, error) {
	var cfg AgentConfig
	switch strings.ToLower(format) {
	case "json":
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("config.Parse JSON: %w", err)
		}
	case "yaml":
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(&cfg); err != nil {
			return nil, fmt.Errorf("config.Parse YAML: %w", err)
		}
	default:
		return nil, fmt.Errorf("config.Parse: unsupported format %q (use \"json\" or \"yaml\")", format)
	}
	return &cfg, nil
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
