package agui

import (
	"fmt"
	"time"
)

// Config for the AG-UI HTTP handler.
type Config struct {
	// Agent processes runs. Required.
	Agent Agent

	// Middlewares applied in order before the agent. Optional.
	Middlewares []Middleware

	// ToolMode controls client tool result flow. Default: ToolModeNextRun.
	ToolMode ToolMode

	// ToolTimeout is the max wait for inline tool results. Default: 5 minutes.
	ToolTimeout time.Duration

	// ToolResultHandler for inline mode. Created automatically if nil and ToolMode is ToolModeInline.
	ToolResultHandler *ToolResultHandler

	// OnError is an optional callback for handler errors.
	OnError func(err error)
}

func (c *Config) validate() error {
	if c.Agent == nil {
		return fmt.Errorf("agui: Agent is required")
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.ToolTimeout == 0 {
		c.ToolTimeout = 5 * time.Minute
	}
	if c.ToolMode == ToolModeInline && c.ToolResultHandler == nil {
		c.ToolResultHandler = NewToolResultHandler()
	}
}
