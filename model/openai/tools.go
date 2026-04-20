package openai

import (
	"encoding/json"

	"google.golang.org/genai"
)

// translateToolDeclarations converts a slice of [genai.Tool] into the OpenAI
// tools array format expected by the /v1/chat/completions endpoint.
//
// Each [genai.FunctionDeclaration] within every Tool produces one entry of the
// form:
//
//	{
//	    "type": "function",
//	    "function": {
//	        "name":        "<declaration name>",
//	        "description": "<declaration description>",
//	        "parameters":  <JSON Schema map>,
//	    },
//	}
//
// Returns nil when tools is nil or contains no function declarations.
func translateToolDeclarations(tools []*genai.Tool) []map[string]any {
	var result []map[string]any

	for _, tool := range tools {
		if tool == nil {
			continue
		}
		for _, decl := range tool.FunctionDeclarations {
			if decl == nil {
				continue
			}
			fn := map[string]any{
				"name": decl.Name,
			}
			if decl.Description != "" {
				fn["description"] = decl.Description
			}
			if params := schemaToJSONSchema(decl.Parameters); params != nil {
				fn["parameters"] = params
			}
			result = append(result, map[string]any{
				"type":     "function",
				"function": fn,
			})
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// translateToolConfig converts a [genai.ToolConfig] to the OpenAI tool_choice
// value.
//
// Mapping:
//   - AUTO → "auto"
//   - ANY  → "required"
//   - NONE → "none"
//   - nil  → nil
//
// An unrecognised or unspecified mode returns nil so the field is omitted from
// the request, letting the API apply its default behaviour.
func translateToolConfig(cfg *genai.ToolConfig) any {
	if cfg == nil {
		return nil
	}
	if cfg.FunctionCallingConfig == nil {
		return nil
	}
	switch cfg.FunctionCallingConfig.Mode {
	case genai.FunctionCallingConfigModeAuto:
		return "auto"
	case genai.FunctionCallingConfigModeAny:
		return "required"
	case genai.FunctionCallingConfigModeNone:
		return "none"
	default:
		return nil
	}
}

// functionCallToToolCall converts a [genai.FunctionCall] Part into the OpenAI
// tool_call map format required by the assistant message's tool_calls array.
//
// Output format:
//
//	{
//	    "id":   "<fc.ID>",
//	    "type": "function",
//	    "function": {
//	        "name":      "<fc.Name>",
//	        "arguments": "<JSON-encoded fc.Args>",
//	    },
//	}
//
// If marshalling fc.Args fails, "arguments" will be set to "{}".
func functionCallToToolCall(fc *genai.FunctionCall) map[string]any {
	argsJSON, err := json.Marshal(fc.Args)
	if err != nil {
		// Fallback to empty object so the result is always valid JSON.
		argsJSON = []byte("{}")
	}
	return map[string]any{
		"id":   fc.ID,
		"type": "function",
		"function": map[string]any{
			"name":      fc.Name,
			"arguments": string(argsJSON),
		},
	}
}

// toolCallsToFunctionCalls converts a slice of OpenAI tool_call maps (as
// returned in the assistant message) back to genai [genai.Part] values, each
// carrying a [genai.FunctionCall].
//
// Each entry in toolCalls must have the shape:
//
//	{
//	    "id":   "<string>",
//	    "type": "function",
//	    "function": {
//	        "name":      "<string>",
//	        "arguments": "<JSON string>",
//	    },
//	}
//
// Entries that cannot be parsed (wrong types, invalid JSON arguments) are
// silently skipped so that partially-valid responses degrade gracefully.
func toolCallsToFunctionCalls(toolCalls []map[string]any) []*genai.Part {
	var parts []*genai.Part

	for _, tc := range toolCalls {
		id, _ := tc["id"].(string)

		fnMap, ok := tc["function"].(map[string]any)
		if !ok {
			continue
		}

		name, _ := fnMap["name"].(string)
		argsStr, _ := fnMap["arguments"].(string)

		var args map[string]any
		if argsStr != "" {
			if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
				// Skip entries with unparseable arguments.
				continue
			}
		}

		parts = append(parts, &genai.Part{
			FunctionCall: &genai.FunctionCall{
				ID:   id,
				Name: name,
				Args: args,
			},
		})
	}

	return parts
}
