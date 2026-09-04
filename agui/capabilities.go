package agui

// AgentCapabilities describes what an agent supports, matching the AG-UI
// AgentCapabilitiesSchema. All sub-capabilities are optional pointers so an
// agent can advertise only the capabilities it supports.
type AgentCapabilities struct {
	Identity       *IdentityCapabilities       `json:"identity,omitempty"`
	Transport      *TransportCapabilities      `json:"transport,omitempty"`
	State          *StateCapabilities          `json:"state,omitempty"`
	Messages       *MessageCapabilities        `json:"messages,omitempty"`
	Tools          *ToolCapabilities           `json:"tools,omitempty"`
	HumanInTheLoop *HumanInTheLoopCapabilities `json:"humanInTheLoop,omitempty"`
	Activities     *ActivityCapabilities       `json:"activities,omitempty"`
	Reasoning      *ReasoningCapabilities      `json:"reasoning,omitempty"`
}

// IdentityCapabilities describes the agent's identity.
type IdentityCapabilities struct {
	Name        string `json:"name,omitempty"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
}

// TransportCapabilities describes the transport-level features supported.
type TransportCapabilities struct {
	Streaming bool `json:"streaming"`
	Binary    bool `json:"binary,omitempty"`
	Protobuf  bool `json:"protobuf,omitempty"`
}

// StateCapabilities describes state management support.
type StateCapabilities struct {
	Snapshots bool `json:"snapshots"`
	Deltas    bool `json:"deltas"`
}

// MessageCapabilities describes message-level features.
type MessageCapabilities struct {
	Snapshots     bool `json:"snapshots"`
	StreamingText bool `json:"streamingText"`
}

// ToolCapabilities describes tool support, including client/server
// disambiguation.
type ToolCapabilities struct {
	Supported   bool `json:"supported"`
	ClientTools bool `json:"clientTools,omitempty"`
	ServerTools bool `json:"serverTools,omitempty"`
	Streaming   bool `json:"streaming,omitempty"`
}

// HumanInTheLoopCapabilities describes HITL/interrupt support.
type HumanInTheLoopCapabilities struct {
	Interrupts bool `json:"interrupts"`
}

// ActivityCapabilities describes activity tracking support.
type ActivityCapabilities struct {
	Snapshots bool `json:"snapshots,omitempty"`
	Deltas    bool `json:"deltas,omitempty"`
}

// ReasoningCapabilities describes reasoning/thinking support.
type ReasoningCapabilities struct {
	Supported bool `json:"supported"`
	Streaming bool `json:"streaming,omitempty"`
	Encrypted bool `json:"encrypted,omitempty"`
}
