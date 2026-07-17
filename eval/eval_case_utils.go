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
// an invocation's intermediate data.
func GetAllToolCallsWithResponses(invocation Invocation) []ToolCallAndResponse {
	if invocation.IntermediateData == nil {
		return nil
	}

	// Legacy format: pair by index.
	uses := invocation.IntermediateData.GetToolUses()
	responses := invocation.IntermediateData.GetToolResponses()
	if uses != nil {
		var pairs []ToolCallAndResponse
		for i, call := range uses {
			pair := ToolCallAndResponse{Call: &call}
			if i < len(responses) {
				pair.Response = &responses[i]
			}
			pairs = append(pairs, pair)
		}
		return pairs
	}

	// Events format.
	var pairs []ToolCallAndResponse
	for _, event := range invocation.IntermediateData.GetInvocationEvents() {
		if event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if part.FunctionCall != nil {
				pairs = append(pairs, ToolCallAndResponse{Call: part.FunctionCall})
			}
			if part.FunctionResponse != nil {
				pairs = append(pairs, ToolCallAndResponse{Response: part.FunctionResponse})
			}
		}
	}
	return pairs
}
