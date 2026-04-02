// Package config provides types and utilities for loading, parsing, and translating
// agent configuration files in JSON or YAML format.
//
// # Overview
//
// An [AgentConfig] describes the shape of a single agent (or a hierarchy of sub-agents).
// Configuration files can be stored as .json, .yaml, or .yml files and loaded with [Load],
// or parsed directly from byte slices with [Parse].
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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/genai"
	"sigs.k8s.io/yaml"
)

// AgentConfig holds the declarative configuration for a single agent.
// It supports hierarchical definitions via [AgentConfig.SubAgents].
//
// Example YAML:
//
//	name: my-agent
//	type: llm
//	model: gemini/gemini-2.0-flash
//	instruction: "You are a helpful assistant."
//	tools:
//	  - name: search
//	generateConfig:
//	  temperature: 0.7
type AgentConfig struct {
	// Name is the unique identifier for this agent.
	Name string `json:"name" yaml:"name"`

	// Type describes the agent kind (e.g. "llm", "sequential", "loop").
	Type string `json:"type" yaml:"type"`

	// Model is an optional model reference in the form "prefix/model-id"
	// (e.g. "gemini/gemini-2.0-flash" or "openai/gpt-4o").
	Model string `json:"model,omitempty" yaml:"model,omitempty"`

	// Instruction is an optional system-level prompt for the agent.
	Instruction string `json:"instruction,omitempty" yaml:"instruction,omitempty"`

	// Description provides a human-readable summary of the agent's purpose.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Tools lists the tool references available to this agent.
	Tools []ToolRef `json:"tools,omitempty" yaml:"tools,omitempty"`

	// SubAgents holds nested child agent configurations.
	SubAgents []AgentConfig `json:"subAgents,omitempty" yaml:"subAgents,omitempty"`

	// GenerateConfig contains arbitrary generation parameters (temperature, topP, …)
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
		if err := yaml.Unmarshal(data, &cfg); err != nil {
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
