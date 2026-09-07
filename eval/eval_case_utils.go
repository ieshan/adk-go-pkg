package eval

import (
	"google.golang.org/genai"
)

// GetAllToolCalls extracts all tool calls (function calls) from an
// invocation's intermediate data, supporting both legacy and events formats.
func GetAllToolCalls(invocation Invocation) []genai.FunctionCall {
	if invocation.IntermediateData == nil {
		return nil
	}

	// Legacy format.
	if uses := invocation.IntermediateData.GetToolUses(); uses != nil {
		return uses
	}

	// Events format.
	var calls []genai.FunctionCall
	for _, event := range invocation.IntermediateData.GetInvocationEvents() {
		if event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if part.FunctionCall != nil {
				calls = append(calls, *part.FunctionCall)
			}
		}
	}
	return calls
}

// GetAllToolResponses extracts all tool responses (function responses) from
// an invocation's intermediate data, supporting both legacy and events formats.
func GetAllToolResponses(invocation Invocation) []genai.FunctionResponse {
	if invocation.IntermediateData == nil {
		return nil
	}

	// Legacy format.
	if responses := invocation.IntermediateData.GetToolResponses(); responses != nil {
		return responses
	}

	// Events format.
	var responses []genai.FunctionResponse
	for _, event := range invocation.IntermediateData.GetInvocationEvents() {
		if event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if part.FunctionResponse != nil {
				responses = append(responses, *part.FunctionResponse)
			}
		}
	}
	return responses
}

// ToolCallAndResponse pairs a function call with its response.
type ToolCallAndResponse struct {
	Call     *genai.FunctionCall
	Response *genai.FunctionResponse
}

// GetAllToolCallsWithResponses pairs tool calls with their responses from
// an invocation's intermediate data. When function call IDs are available,
// calls and responses are paired by ID (matching ADK-Python). When IDs are
// absent (e.g. legacy format without IDs), calls and responses are paired
// by index.
func GetAllToolCallsWithResponses(invocation Invocation) []ToolCallAndResponse {
	if invocation.IntermediateData == nil {
		return nil
	}

	calls := GetAllToolCalls(invocation)
	if calls == nil {
		return nil
	}
	responses := GetAllToolResponses(invocation)

	// Check if any calls have non-empty IDs — if so, use ID-based pairing.
	hasIDs := false
	for i := range calls {
		if calls[i].ID != "" {
			hasIDs = true
			break
		}
	}

	if hasIDs {
		// Pair by function call ID (matches ADK-Python).
		responseByID := make(map[string]*genai.FunctionResponse, len(responses))
		for i := range responses {
			if responses[i].ID != "" {
				responseByID[responses[i].ID] = &responses[i]
			}
		}
		pairs := make([]ToolCallAndResponse, 0, len(calls))
		for i := range calls {
			pair := ToolCallAndResponse{Call: &calls[i]}
			if calls[i].ID != "" {
				if resp, ok := responseByID[calls[i].ID]; ok {
					pair.Response = resp
				}
			}
			pairs = append(pairs, pair)
		}
		return pairs
	}

	// Fall back to index-based pairing (legacy behavior).
	pairs := make([]ToolCallAndResponse, 0, len(calls))
	for i := range calls {
		pair := ToolCallAndResponse{Call: &calls[i]}
		if i < len(responses) {
			pair.Response = &responses[i]
		}
		pairs = append(pairs, pair)
	}
	return pairs
}
