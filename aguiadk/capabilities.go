package aguiadk

import (
	"github.com/ieshan/adk-go-pkg/agui"

	"google.golang.org/adk/v2/agent"
)

// InferCapabilities inspects an ADK agent and bridge configuration to produce
// an AgentCapabilities descriptor for AG-UI discovery. It checks if the agent
// has immediate sub-agents, the bridge's client-tool configuration, and the
// client-tool mode to produce an accurate capabilities snapshot.
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
			Streaming:   true,
		},
		Reasoning: &agui.ReasoningCapabilities{
			Supported: true,
			Streaming: true,
			Encrypted: true,
		},
	}

	// Client tool support.
	if cfg.ClientTools != nil {
		caps.Tools.ClientTools = true
	}

	// HITL / interrupt support: NextRun mode emits AG-UI interrupts when
	// approval is required (ApprovalModeFunc is nil or returns false).
	// HandBack mode does a plain RUN_FINISHED (no interrupt protocol).
	// Inline mode handles results within the SSE connection (no interrupt).
	if cfg.ClientTools != nil && cfg.ClientTools.Mode == ClientToolModeNextRun {
		caps.HumanInTheLoop = &agui.HumanInTheLoopCapabilities{Interrupts: true}
	}

	// Sub-agent support: if the agent has immediate sub-agents, advertise
	// activity tracking.
	if len(a.SubAgents()) > 0 {
		caps.Activities = &agui.ActivityCapabilities{
			Snapshots: true,
			Deltas:    cfg.EmitActivityDeltas,
		}
	}

	return caps
}
