package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveAgentRef resolves an AgentRefConfig into an AgentConfig.
// If ConfigPath is set, loads from file relative to the parent config's
// directory beneath root. root must not be nil for ConfigPath references.
// If Code is set, this function returns an error — Code refs are resolved
// at Build time via Registry.ResolveAgent in BuildWithPath.
func ResolveAgentRef(ref *AgentRefConfig, root *os.Root, parentConfigPath string) (AgentConfig, error) {
	if err := ref.Validate(); err != nil {
		return nil, fmt.Errorf("config.ResolveAgentRef: %w", err)
	}
	if ref.ConfigPath != "" {
		if root == nil {
			return nil, fmt.Errorf("config.ResolveAgentRef: root required for ConfigPath refs")
		}
		dir := filepath.Dir(parentConfigPath)
		path := filepath.Join(dir, ref.ConfigPath)
		appCfg, err := Load(root, path)
		if err != nil {
			return nil, fmt.Errorf("config.ResolveAgentRef: %w", err)
		}
		return appCfg.AgentConfig, nil
	}
	// Code refs resolved at Build time via Registry.ResolveAgent
	return nil, fmt.Errorf("config.ResolveAgentRef: code refs require Build with Registry")
}
