package aguiadk

import (
	"net/http"
	"time"
)

// Preset builders return a Config with sensible defaults for common AG-UI
// patterns. Each takes a base Config (typically providing Agent and services)
// and overrides only the preset-specific fields, preserving caller-supplied
// values like Agent, SessionService, and ArtifactService.

// AgenticChatPreset returns a Config for a basic agentic chat agent.
// Client tools are accepted in NextRun mode (the client fulfills tool calls
// and starts a new run with results). State snapshots are enabled.
func AgenticChatPreset(base Config) Config {
	base.EmitStateSnapshot = new(true)
	base.SessionTimeout = 20 * time.Minute
	base.ClientTools = &ClientToolConfig{Mode: ClientToolModeNextRun}
	return base
}

// GenerativeUIPreset returns a Config for a generative-UI agent. It is
// currently equivalent to AgenticChatPreset — client tools are accepted in
// NextRun mode and state snapshots are enabled. The preset exists as a
// semantic marker for generative-UI agents; future versions may add
// structured-output or streaming-tool configuration. Use AgenticGenerativeUIPreset
// for agents that drive UI state through tool calls mapped to STATE_DELTA events.
func GenerativeUIPreset(base Config) Config {
	base.EmitStateSnapshot = new(true)
	base.SessionTimeout = 20 * time.Minute
	base.ClientTools = &ClientToolConfig{Mode: ClientToolModeNextRun}
	return base
}

// HumanInTheLoopPreset returns a Config for a HITL agent that routes
// consequential actions through approval interrupts. Client tools use
// NextRun mode so the client can show approval UI and resume with the result.
// When autoApprove is true, ApprovalModeFunc is set to always return true so
// the bridge skips the approval interrupt and the run finishes normally
// (the client executes the tool in a new run), and no RunStore is configured;
// when false, an in-memory RunStore is created for the interrupt/resume cycle.
func HumanInTheLoopPreset(base Config, autoApprove bool) Config {
	base.EmitStateSnapshot = new(true)
	base.SessionTimeout = 30 * time.Minute
	base.ClientTools = &ClientToolConfig{Mode: ClientToolModeNextRun}
	if autoApprove {
		base.ApprovalModeFunc = func(*http.Request) bool { return true }
	} else if base.RunStore == nil {
		base.RunStore = NewRunStore()
	}
	return base
}

// SharedStatePreset returns a Config for collaborative document editing.
// Tool calls are suppressed (no TOOL_CALL_* events) and instead mapped to
// STATE_DELTA events via the provided mapper. This lets tool invocations
// appear as state mutations in the UI.
func SharedStatePreset(base Config, mapper ToolToStateMapper) Config {
	base.EmitStateSnapshot = new(true)
	base.SessionTimeout = 20 * time.Minute
	base.SuppressToolEvents = true
	base.ToolToStateMapper = mapper
	return base
}

// InlineToolsPreset returns a Config for an agent that keeps the SSE
// connection open while waiting for inline tool results from the client.
// The handler must mount the /tool-result endpoint (use Handler() which
// does this automatically when ClientToolModeInline is set).
func InlineToolsPreset(base Config) Config {
	base.EmitStateSnapshot = new(true)
	base.SessionTimeout = 20 * time.Minute
	base.ClientTools = &ClientToolConfig{
		Mode:    ClientToolModeInline,
		Timeout: 5 * time.Minute,
	}
	return base
}

// HandBackPreset returns a Config for a client-tool agent that ends the run
// with a plain RUN_FINISHED (no interrupt outcome) when a client tool is
// invoked. The client receives the tool call and starts a new run with the
// result. MESSAGES_SNAPSHOT is enabled so the client gets the full conversation
// state at hand-back time.
func HandBackPreset(base Config) Config {
	base.EmitStateSnapshot = new(true)
	base.EmitMessagesSnapshot = true
	base.SessionTimeout = 20 * time.Minute
	base.ClientTools = &ClientToolConfig{Mode: ClientToolModeHandBack}
	return base
}

// PredictiveStatePreset returns a Config for an agent that streams ghosted
// state deltas under the /_predictive namespace while generating, then
// commits and clears on completion. Enables state snapshots, activity deltas
// for streaming tool progress, and step events for phase tracking. The agent
// should use agui.PredictiveStateTracker to emit predictive deltas.
func PredictiveStatePreset(base Config) Config {
	base.EmitStateSnapshot = new(true)
	base.EmitActivityDeltas = true
	base.EmitStepEvents = new(true)
	base.SessionTimeout = 20 * time.Minute
	return base
}

// AgenticGenerativeUIPreset returns a Config for a self-contained generative UI
// agent that drives UI state through tool calls mapped to STATE_DELTA events.
// Tool call events are suppressed (no TOOL_CALL_* emitted to the client);
// instead, the provided mapper converts tool invocations into state mutations.
// Step events and messages snapshots are enabled for UI consistency.
func AgenticGenerativeUIPreset(base Config, mapper ToolToStateMapper) Config {
	base.EmitStateSnapshot = new(true)
	base.EmitMessagesSnapshot = true
	base.EmitStepEvents = new(true)
	base.SuppressToolEvents = true
	base.ToolToStateMapper = mapper
	base.SessionTimeout = 20 * time.Minute
	return base
}
