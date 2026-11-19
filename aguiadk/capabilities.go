package aguiadk

import (
	"github.com/ieshan/adk-go-pkg/agui"

	"google.golang.org/adk/v2/agent"
)

// InferCapabilities inspects an ADK agent and bridge configuration to produce
// an AgentCapabilities descriptor for AG-UI discovery, matching the 1.0
// AgentCapabilities schema. Categories with no schema equivalent (e.g.
// MESSAGES_SNAPSHOT and activity streaming) are reported under Custom so a 1.0
// client does not strip them.
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
		Transport: &agui.TransportCapabilities{Streaming: true},
		State:     &agui.StateCapabilities{Snapshots: true, Deltas: true},
		Tools:     &agui.ToolsCapabilities{Supported: true},
		// Agent-level declaration: the bridge emits reasoning spans,
		// messages, and encrypted values whenever the model supplies
		// thought parts/signatures, so all three are advertised
		// unconditionally — a model that never thinks simply produces none.
		Reasoning: &agui.ReasoningCapabilities{Supported: true, Streaming: true, Encrypted: true},
		// Intentionally undeclared (per the schema, an omitted category
		// means "unknown", not "unsupported"): output (the bridge does not
		// offer structured-output negotiation), execution (no code-execution
		// tool is inferred from an ADK agent), multimodal.output (no output
		// media is produced beyond text), transport.websocket/httpBinary/
		// resumable (only SSE is served).
	}

	// Multimodal input: media parts are always accepted at the protocol
	// level; provider gating in the bridge forwards them for "" and
	// "openai", and degrades other providers to text fallback.
	if cfg.Provider == "" || cfg.Provider == "openai" {
		caps.Multimodal = &agui.MultimodalCapabilities{
			Input: &agui.MultimodalInputCapabilities{
				Image: true, Audio: true, Video: true, PDF: true, File: true,
			},
		}
	} else {
		caps.Multimodal = &agui.MultimodalCapabilities{
			Input: &agui.MultimodalInputCapabilities{Image: true},
		}
	}

	// Client-provided tools and HITL interrupts.
	if cfg.ClientTools != nil {
		caps.Tools.ClientProvided = true
		if cfg.ClientTools.Mode == ClientToolModeNextRun {
			caps.HumanInTheLoop = &agui.HumanInTheLoopCapabilities{
				Supported: true, Interrupts: true, Approvals: true,
			}
		}
	}

	// Sub-agents.
	if subs := a.SubAgents(); len(subs) > 0 {
		ma := &agui.MultiAgentCapabilities{Supported: true, Delegation: true}
		for _, sa := range subs {
			ma.Subagents = append(ma.Subagents, agui.SubagentInfo{
				Name: sa.Name(), Description: sa.Description(),
			})
		}
		caps.MultiAgent = ma
	}

	// Everything without a schema category lives under custom so it is
	// never stripped: MESSAGES_SNAPSHOT emission and activity streaming.
	custom := map[string]any{
		"messagesSnapshot": cfg.EmitMessagesSnapshot,
	}
	if cfg.ClientTools != nil && cfg.ClientTools.Mode == ClientToolModeNextRun {
		custom["activities"] = map[string]any{
			"snapshots": true,
			"deltas":    cfg.EmitActivityDeltas,
		}
	}
	caps.Custom = custom

	return caps
}
