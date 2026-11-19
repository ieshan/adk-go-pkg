package agui

import "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"

// AgentCapabilities describes what an agent supports, matching the 1.0
// AgentCapabilities schema. Every category is optional; integration-specific
// declarations that have no schema category belong in Custom — 1.0 clients
// strip unknown top-level keys.
type AgentCapabilities struct {
	Identity       *IdentityCapabilities       `json:"identity,omitempty"`
	Transport      *TransportCapabilities      `json:"transport,omitempty"`
	Tools          *ToolsCapabilities          `json:"tools,omitempty"`
	Output         *OutputCapabilities         `json:"output,omitempty"`
	State          *StateCapabilities          `json:"state,omitempty"`
	MultiAgent     *MultiAgentCapabilities     `json:"multiAgent,omitempty"`
	Reasoning      *ReasoningCapabilities      `json:"reasoning,omitempty"`
	Multimodal     *MultimodalCapabilities     `json:"multimodal,omitempty"`
	Execution      *ExecutionCapabilities      `json:"execution,omitempty"`
	HumanInTheLoop *HumanInTheLoopCapabilities `json:"humanInTheLoop,omitempty"`
	Custom         map[string]any              `json:"custom,omitempty"`
}

// IdentityCapabilities describes the agent's identity.
type IdentityCapabilities struct {
	Name             string         `json:"name,omitempty"`
	Type             string         `json:"type,omitempty"`
	Description      string         `json:"description,omitempty"`
	Version          string         `json:"version,omitempty"`
	Provider         string         `json:"provider,omitempty"`
	DocumentationURL string         `json:"documentationUrl,omitempty"`
	Metadata         types.Metadata `json:"metadata,omitempty"`
}

// TransportCapabilities describes the transport-level features supported.
type TransportCapabilities struct {
	Streaming         bool `json:"streaming,omitempty"`
	Websocket         bool `json:"websocket,omitempty"`
	HTTPBinary        bool `json:"httpBinary,omitempty"`
	PushNotifications bool `json:"pushNotifications,omitempty"`
	Resumable         bool `json:"resumable,omitempty"`
}

// ToolsCapabilities describes tool support. Items lists the agent's own
// tools (full JSON Schema definitions); ClientProvided marks acceptance of
// client tools passed via RunAgentInput.tools.
type ToolsCapabilities struct {
	Supported      bool         `json:"supported"`
	Items          []types.Tool `json:"items,omitempty"`
	ParallelCalls  bool         `json:"parallelCalls,omitempty"`
	ClientProvided bool         `json:"clientProvided,omitempty"`
}

// OutputCapabilities describes output format support.
type OutputCapabilities struct {
	StructuredOutput   bool     `json:"structuredOutput,omitempty"`
	SupportedMimeTypes []string `json:"supportedMimeTypes,omitempty"`
}

// StateCapabilities describes state management support.
type StateCapabilities struct {
	Snapshots       bool `json:"snapshots,omitempty"`
	Deltas          bool `json:"deltas,omitempty"`
	Memory          bool `json:"memory,omitempty"`
	PersistentState bool `json:"persistentState,omitempty"`
}

// MultiAgentCapabilities describes multi-agent coordination support.
type MultiAgentCapabilities struct {
	Supported  bool           `json:"supported"`
	Delegation bool           `json:"delegation,omitempty"`
	Handoffs   bool           `json:"handoffs,omitempty"`
	Subagents  []SubagentInfo `json:"subagents,omitempty"`
}

// SubagentInfo names a subagent for agent-selection UIs.
type SubagentInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ReasoningCapabilities describes reasoning/thinking support.
type ReasoningCapabilities struct {
	Supported bool `json:"supported"`
	Streaming bool `json:"streaming,omitempty"`
	Encrypted bool `json:"encrypted,omitempty"`
}

// MultimodalCapabilities describes multimodal input/output support.
type MultimodalCapabilities struct {
	Input  *MultimodalInputCapabilities  `json:"input,omitempty"`
	Output *MultimodalOutputCapabilities `json:"output,omitempty"`
}

// MultimodalInputCapabilities describes accepted input media.
type MultimodalInputCapabilities struct {
	Image bool `json:"image,omitempty"`
	Audio bool `json:"audio,omitempty"`
	Video bool `json:"video,omitempty"`
	PDF   bool `json:"pdf,omitempty"`
	File  bool `json:"file,omitempty"`
}

// MultimodalOutputCapabilities describes output media the agent can produce.
type MultimodalOutputCapabilities struct {
	Image bool `json:"image,omitempty"`
	Audio bool `json:"audio,omitempty"`
}

// ExecutionCapabilities describes code execution support.
type ExecutionCapabilities struct {
	CodeExecution    bool `json:"codeExecution,omitempty"`
	Sandboxed        bool `json:"sandboxed,omitempty"`
	MaxIterations    int  `json:"maxIterations,omitempty"`
	MaxExecutionTime int  `json:"maxExecutionTime,omitempty"`
}

// HumanInTheLoopCapabilities describes HITL/interrupt support.
type HumanInTheLoopCapabilities struct {
	Supported        bool `json:"supported"`
	Approvals        bool `json:"approvals,omitempty"`
	Interventions    bool `json:"interventions,omitempty"`
	Feedback         bool `json:"feedback,omitempty"`
	Interrupts       bool `json:"interrupts,omitempty"`
	ApproveWithEdits bool `json:"approveWithEdits,omitempty"`
}
