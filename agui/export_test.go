package agui

import (
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

// This file exports unexported functions and types for use in external test
// files (package agui_test). It is only compiled during testing and does not
// pollute the production binary.

// UIToolInfoForTest exposes uiToolInfo for testing.
type UIToolInfoForTest = uiToolInfo

// ReconstructMessagesForTest exposes reconstructMessages for testing.
func ReconstructMessagesForTest(original []types.Message, evs []events.Event) []types.Message {
	return reconstructMessages(original, evs)
}

// GetOpenToolCallsForTest exposes getOpenToolCalls for testing.
func GetOpenToolCallsForTest(msgs []types.Message) []types.ToolCall {
	return getOpenToolCalls(msgs)
}

// ExtractProxiedRequestForTest exposes extractProxiedRequest for testing.
func ExtractProxiedRequestForTest(input types.RunAgentInput) (*ProxiedMCPRequest, bool) {
	return extractProxiedRequest(input)
}

// GetPendingUIToolCallsForTest exposes getPendingUIToolCalls for testing.
func GetPendingUIToolCallsForTest(messages []types.Message, uiToolMap map[string]UIToolInfoForTest) []types.ToolCall {
	internal := make(map[string]uiToolInfo, len(uiToolMap))
	for k, v := range uiToolMap {
		internal[k] = v
	}
	return getPendingUIToolCalls(messages, internal)
}
