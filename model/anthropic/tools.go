package anthropic

import (
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// translateToolDeclarations converts a slice of [genai.Tool] into the Anthropic
// tools array format expected by the /v1/messages endpoint.
//
// Each [genai.FunctionDeclaration] within every Tool produces one entry of the
// form:
//
//	{
//	    "name":        "<declaration name>",
//	    "description": "<declaration description>",
//	    "input_schema": <JSON Schema map>,
//	}
//
// Only FunctionDeclarations are supported; other tool types (Retrieval,
// GoogleSearch, etc.) are silently skipped.
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
			td := map[string]any{
				"name": decl.Name,
			}
			if decl.Description != "" {
				td["description"] = decl.Description
			}
			var schema map[string]any
			if decl.Parameters != nil {
				schema = schemaToJSONSchema(decl.Parameters)
			} else if decl.ParametersJsonSchema != nil {
				// ParametersJsonSchema is `any`; if it's already a map, use it directly.
				if m, ok := decl.ParametersJsonSchema.(map[string]any); ok {
					schema = m
				}
			}
			if schema != nil {
				td["input_schema"] = schema
			}
			result = append(result, td)
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// translateToolConfig converts a [genai.ToolConfig] to the Anthropic tool_choice
// value.
//
// Mapping:
//   - AUTO → {"type": "auto"}
//   - ANY  → {"type": "any"}
//   - NONE → {"type": "none"}
//   - Specific function names → {"type": "tool", "name": "..."} (if single) or
//     {"type": "auto"} with filtered tools (if multiple).
//   - nil  → nil
func translateToolConfig(cfg *genai.ToolConfig) any {
	if cfg == nil {
		return nil
	}
	if cfg.FunctionCallingConfig == nil {
		return nil
	}

	mode := cfg.FunctionCallingConfig.Mode
	names := cfg.FunctionCallingConfig.AllowedFunctionNames

	// If exactly one function name is allowed, force that tool regardless of mode.
	if len(names) == 1 {
		return map[string]any{
			"type": "tool",
			"name": names[0],
		}
	}

	switch mode {
	case genai.FunctionCallingConfigModeAuto:
		return map[string]any{"type": "auto"}
	case genai.FunctionCallingConfigModeAny:
		return map[string]any{"type": "any"}
	case genai.FunctionCallingConfigModeNone:
		return map[string]any{"type": "none"}
	default:
		// Multiple allowed names with unspecified mode falls back to auto.
		if len(names) > 1 {
			return map[string]any{"type": "auto"}
		}
		return nil
	}
}

// functionCallToToolUse converts a [genai.FunctionCall] into the Anthropic
// tool_use block format used in assistant messages.
//
// Output format:
//
//	{
//	    "type": "tool_use",
//	    "id":   "<fc.ID>",
//	    "name": "<fc.Name>",
//	    "input": <fc.Args map>,
//	}
func functionCallToToolUse(fc *genai.FunctionCall) map[string]any {
	return map[string]any{
		"type":  "tool_use",
		"id":    fc.ID,
		"name":  fc.Name,
		"input": fc.Args,
	}
}

// toolUseToFunctionCall converts an Anthropic tool_use content block back to a
// [genai.FunctionCall].
//
// The block must have the shape:
//
//	{
//	    "type":  "tool_use",
//	    "id":    "<string>",
//	    "name":  "<string>",
//	    "input": <JSON object>,
//	}
//
// If the input field cannot be parsed as a JSON object, Args will be nil.
func toolUseToFunctionCall(block map[string]any) *genai.FunctionCall {
	id, _ := block["id"].(string)
	name, _ := block["name"].(string)

	var args map[string]any
	if input, ok := block["input"]; ok {
		switch v := input.(type) {
		case map[string]any:
			args = v
		case string:
			if v != "" {
				if err := json.Unmarshal([]byte(v), &args); err != nil {
					args = nil
				}
			}
		default:
			// Try marshalling and unmarshalling as fallback.
			b, err := json.Marshal(input)
			if err == nil {
				_ = json.Unmarshal(b, &args)
			}
		}
	}

	return &genai.FunctionCall{
		ID:   id,
		Name: name,
		Args: args,
	}
}

// isHTTPURL reports whether s is an HTTP or HTTPS URL.
func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// extractSystemText returns the concatenated text from all text Parts in a
// Content, ignoring non-text parts. Used to build Anthropic's top-level system
// field.
func extractSystemText(c *genai.Content) string {
	if c == nil {
		return ""
	}
	var sb strings.Builder
	for _, p := range c.Parts {
		sb.WriteString(p.Text)
	}
	return sb.String()
}

// contentBlocksFromFunctionResponse converts a genai FunctionResponse into
// Anthropic tool_result content blocks.
//
// When FunctionResponse.Parts is non-empty and contains InlineData, each part
// becomes a content block in the tool_result's content array.
// When FunctionResponse.Response is set, it is serialised to a JSON string.
//
// Anthropic tool_result format:
//
//	{"type": "tool_result", "tool_use_id": "...", "content": "..." | [...]}
func contentBlocksFromFunctionResponse(fr *genai.FunctionResponse) ([]map[string]any, error) {
	if fr.ID == "" {
		return nil, fmt.Errorf("function response has empty tool call ID")
	}

	blocks := []map[string]any{
		{
			"type":        "tool_result",
			"tool_use_id": fr.ID,
		},
	}

	// If Parts with InlineData are present, build array of content blocks.
	if len(fr.Parts) > 0 {
		var contentBlocks []map[string]any
		for _, part := range fr.Parts {
			if part.InlineData != nil {
				// Anthropic supports image blocks inside tool_result content.
				contentBlocks = append(contentBlocks, map[string]any{
					"type": "image",
					"source": map[string]any{
						"type":       "base64",
						"media_type": part.InlineData.MIMEType,
						"data":       part.InlineData.Data,
					},
				})
			}
		}
		if len(contentBlocks) > 0 {
			blocks[0]["content"] = contentBlocks
			return blocks, nil
		}
	}

	// Otherwise serialize Response map to JSON string.
	if fr.Response != nil {
		respJSON, err := json.Marshal(fr.Response)
		if err != nil {
			return nil, fmt.Errorf("marshal function response: %w", err)
		}
		blocks[0]["content"] = string(respJSON)
	}

	return blocks, nil
}
