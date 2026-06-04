package config

import (
	"fmt"
	"path/filepath"
)

// ResolveAgentRef resolves an AgentRefConfig into an AgentConfig.
// If ConfigPath is set, loads from file (relative to parent config dir).
// If Code is set, resolves via Registry (requires pre-registered agent).
func ResolveAgentRef(ref *AgentRefConfig, parentConfigPath string) (AgentConfig, error) {
	if err := ref.Validate(); err != nil {
		return nil, fmt.Errorf("config.ResolveAgentRef: %w", err)
	}
	if ref.ConfigPath != "" {
		var path string
		if filepath.IsAbs(ref.ConfigPath) {
			path = ref.ConfigPath
		} else {
			dir := filepath.Dir(parentConfigPath)
			path = filepath.Join(dir, ref.ConfigPath)
		}
		appCfg, err := Load(path)
		if err != nil {
			return nil, fmt.Errorf("config.ResolveAgentRef: %w", err)
		}
		return appCfg.AgentConfig, nil
	}
	// Code refs resolved at Build time via Registry.ResolveAgent
	return nil, fmt.Errorf("config.ResolveAgentRef: code refs require Build with Registry")
}
