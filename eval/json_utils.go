package eval

import (
	"encoding/json"
	"fmt"
	"os"

	"google.golang.org/genai"
)

// LoadEvalSetFromFile loads an eval set from a JSON file, handling both
// new and old formats.
func LoadEvalSetFromFile(path string) (*EvalSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, NewNotFoundError("eval set file", path)
		}
		return nil, fmt.Errorf("failed to read eval set file: %w", err)
	}
	return UnmarshalEvalSet(data)
}

// UnmarshalEvalSet unmarshals an eval set from JSON, handling both new
// and old formats.
func UnmarshalEvalSet(data []byte) (*EvalSet, error) {
	// Try new format first.
	var evalSet EvalSet
	if err := json.Unmarshal(data, &evalSet); err == nil && evalSet.EvalSetID != "" {
		return &evalSet, nil
	}

	// Try old format: array of objects with query/reference/expected_tool_use.
	converted, err := ConvertEvalSetToNewSchema(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse eval set (tried both new and old format): %w", err)
	}
	return converted, nil
}

// ConvertEvalSetToNewSchema migrates old JSON format to new EvalSet schema.
// Old format: array of objects with `query`, `reference`, `expected_tool_use` fields.
func ConvertEvalSetToNewSchema(data []byte) (*EvalSet, error) {
	var oldCases []map[string]json.RawMessage
	if err := json.Unmarshal(data, &oldCases); err != nil {
		return nil, fmt.Errorf("failed to parse old format: %w", err)
	}

	evalSet := &EvalSet{
		EvalCases: []EvalCase{},
	}

	for i, oldCase := range oldCases {
		evalCase := EvalCase{
			EvalID: fmt.Sprintf("eval_case_%d", i),
		}

		if queryRaw, ok := oldCase[Query]; ok {
			var query string
			if err := json.Unmarshal(queryRaw, &query); err == nil && query != "" {
				evalCase.Conversation = append(evalCase.Conversation, Invocation{
					UserContent: textToContent(query),
				})
			}
		}

		if refRaw, ok := oldCase[Reference]; ok {
			var reference string
			if err := json.Unmarshal(refRaw, &reference); err == nil && reference != "" {
				if len(evalCase.Conversation) > 0 {
					evalCase.Conversation[0].FinalResponse = textToContent(reference)
				}
			}
		}

		if toolRaw, ok := oldCase[ExpectedToolUse]; ok {
			var toolUses []map[string]any
			if err := json.Unmarshal(toolRaw, &toolUses); err == nil && len(toolUses) > 0 {
				if len(evalCase.Conversation) > 0 {
					evalCase.Conversation[0].IntermediateData = &LegacyIntermediateData{
						ToolUses: convertToFunctionCalls(toolUses),
					}
				}
			}
		}

		if len(evalCase.Conversation) > 0 {
			evalSet.EvalCases = append(evalSet.EvalCases, evalCase)
		}
	}

	return evalSet, nil
}

// MarshalEvalSet serializes an eval set to JSON with indentation.
func MarshalEvalSet(evalSet *EvalSet) ([]byte, error) {
	return json.MarshalIndent(evalSet, "", "  ")
}

// textToContent creates a genai.Content from a text string with "user" role.
func textToContent(text string) *genai.Content {
	return &genai.Content{
		Parts: []*genai.Part{{Text: text}},
		Role:  "user",
	}
}

// convertToFunctionCalls converts old format tool use maps to FunctionCalls.
func convertToFunctionCalls(toolUses []map[string]any) []genai.FunctionCall {
	var calls []genai.FunctionCall
	for _, tu := range toolUses {
		call := genai.FunctionCall{}
		if name, ok := tu[ToolName].(string); ok {
			call.Name = name
		}
		if input, ok := tu[ToolInput].(map[string]any); ok {
			call.Args = input
		}
		calls = append(calls, call)
	}
	return calls
}
