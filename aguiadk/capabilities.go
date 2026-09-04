package aguiadk

import (
	"github.com/ieshan/adk-go-pkg/agui"

	"google.golang.org/adk/v2/agent"
)

// InferCapabilities inspects an ADK agent and bridge configuration to produce
// an AgentCapabilities descriptor for AG-UI discovery. It checks the
// agent's sub-agent tree, the bridge's client-tool configuration, and the
// HITL/interrupt configuration to produce an accurate capabilities snapshot.
func InferCapabilities(a agent.Agent, cfg Config) *agui.AgentCapabilities {
	if a == nil {
		return nil
	}
	caps := &agui.AgentCapabilities{
		Identity: &agui.IdentityCapabilities{
			Name:        a.Name(),
			Type:        "adk-go",
			Description: a.Description(),
		},
		Transport: &agui.TransportCapabilities{
			Streaming: true,
		},
		State: &agui.StateCapabilities{
			Snapshots: true,
			Deltas:    true,
		},
		Messages: &agui.MessageCapabilities{
			Snapshots:     cfg.EmitMessagesSnapshot,
			StreamingText: true,
		},
		Tools: &agui.ToolCapabilities{
			Supported:   true,
			ServerTools: true,
		},
		Reasoning: &agui.ReasoningCapabilities{
			Supported: true,
			Streaming: true,
		},
	}

	// Client tool support.
	if cfg.ClientTools != nil {
		caps.Tools.ClientTools = true
	}

	// HITL / interrupt support: the bridge supports interrupts when client
	// tools are configured in hand-back mode, or when a custom approval mode
	// function is configured (which implies the approval flow is active).
	if cfg.ClientTools != nil && cfg.ClientTools.Mode == ClientToolModeHandBack {
		caps.HumanInTheLoop = &agui.HumanInTheLoopCapabilities{Interrupts: true}
	} else if cfg.ApprovalModeFunc != nil {
		caps.HumanInTheLoop = &agui.HumanInTheLoopCapabilities{Interrupts: true}
	}

	// Sub-agent support: if the agent has sub-agents, advertise activity
	// tracking.
	if len(a.SubAgents()) > 0 {
		caps.Activities = &agui.ActivityCapabilities{
			Snapshots: true,
			Deltas:    cfg.EmitActivityDeltas,
		}
	}

	return caps
}
