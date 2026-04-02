package openai

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// chatRequest is the request body for the OpenAI /v1/chat/completions endpoint.
// All fields follow the OpenAI Chat Completions API v1 specification.
type chatRequest struct {
	// Model is the OpenAI model identifier (e.g. "gpt-4o").
	Model string `json:"model"`
	// Messages is the list of conversation turns.
	Messages []chatMessage `json:"messages"`
	// Temperature controls randomness (0–2). Nil means use the model default.
	Temperature *float32 `json:"temperature,omitempty"`
	// TopP is the nucleus-sampling probability mass. Nil means use the model default.
	TopP *float32 `json:"top_p,omitempty"`
	// MaxTokens caps the number of tokens in the generated response.
	MaxTokens *int32 `json:"max_tokens,omitempty"`
	// Stop is a list of sequences that cause generation to halt.
	Stop []string `json:"stop,omitempty"`
	// Stream enables server-sent-event streaming when true.
	Stream bool `json:"stream,omitempty"`
	// Tools is the list of callable functions exposed to the model.
	Tools []any `json:"tools,omitempty"`
	// ToolChoice instructs the model how to select tools (e.g. "auto", "none").
	ToolChoice any `json:"tool_choice,omitempty"`
	// ResponseFormat constrains the output format (e.g. json_object or json_schema).
	ResponseFormat any `json:"response_format,omitempty"`
}

// chatMessage represents a single turn in an OpenAI chat conversation.
// The Content field is polymorphic: it is a plain string for text-only messages
// and a []any (slice of content-part maps) for multimodal messages.
type chatMessage struct {
	// Role identifies the author: "system", "user", "assistant", or "tool".
	Role string `json:"role"`
	// Content holds the message body. Use string for text, []any for multipart.
	Content any `json:"content"`
	// Name is the function name when Role is "tool".
	Name string `json:"name,omitempty"`
	// ToolCalls contains function-call requests emitted by the model.
	ToolCalls []any `json:"tool_calls,omitempty"`
	// ToolCallID links a tool response back to the originating tool call.
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// chatResponse is the response body from the OpenAI /v1/chat/completions endpoint.
type chatResponse struct {
	// ID is the unique identifier for the completion returned by the API.
	ID string `json:"id"`
	// Choices holds the generated completion candidates.
	Choices []chatChoice `json:"choices"`
	// Usage provides token-count statistics for the request.
	Usage *chatUsage `json:"usage,omitempty"`
}

// chatChoice is one generated completion candidate inside a [chatResponse].
type chatChoice struct {
	// Index is the zero-based position of this candidate in the response list.
	Index int `json:"index"`
	// Message is the generated assistant message for this candidate.
	Message chatMessage `json:"message"`
	// FinishReason describes why generation stopped (e.g. "stop", "length").
	FinishReason string `json:"finish_reason"`
}

// chatUsage holds token-count statistics for a completion request.
type chatUsage struct {
	// PromptTokens is the number of tokens consumed by the prompt.
	PromptTokens int32 `json:"prompt_tokens"`
	// CompletionTokens is the number of tokens generated in the completion.
	CompletionTokens int32 `json:"completion_tokens"`
	// TotalTokens is the sum of prompt and completion tokens.
	TotalTokens int32 `json:"total_tokens"`
}

// buildChatRequest converts an ADK [model.LLMRequest] into an OpenAI
// [chatRequest] ready to be sent to the /v1/chat/completions endpoint.
//
// Translation rules:
//   - SystemInstruction from Config is prepended as a "system" role message.
//   - genai role "user" maps to OpenAI "user"; "model" maps to "assistant".
//   - Config.Temperature, TopP, MaxOutputTokens, StopSequences are forwarded.
//   - Config.ResponseSchema → response_format {type:"json_schema", json_schema:{...}}.
//   - Config.ResponseMIMEType "application/json" (without schema) → {type:"json_object"}.
//   - stream is forwarded to chatRequest.Stream.
//
// Returns an error if any Part cannot be translated.
func buildChatRequest(req *model.LLMRequest, modelName string, stream bool) (*chatRequest, error) {
	cr := &chatRequest{
		Model:  modelName,
		Stream: stream,
	}

	// Apply generation config fields when present.
	if req.Config != nil {
		cfg := req.Config
		cr.Temperature = cfg.Temperature
		cr.TopP = cfg.TopP
		if cfg.MaxOutputTokens > 0 {
			cr.MaxTokens = new(cfg.MaxOutputTokens)
		}
		cr.Stop = cfg.StopSequences

		// System instruction becomes the first message.
		if cfg.SystemInstruction != nil {
			sysText := extractText(cfg.SystemInstruction)
			cr.Messages = append(cr.Messages, chatMessage{
				Role:    "system",
				Content: sysText,
			})
		}

		// Response format handling.
		if cfg.ResponseSchema != nil {
			// Structured output via json_schema.
			schemaMap := schemaToJSONSchema(cfg.ResponseSchema)
			cr.ResponseFormat = map[string]any{
				"type": "json_schema",
				"json_schema": map[string]any{
					"name":   "response",
					"strict": true,
					"schema": schemaMap,
				},
			}
		} else if cfg.ResponseMIMEType == "application/json" {
			// Unstructured JSON output.
			cr.ResponseFormat = map[string]any{
				"type": "json_object",
			}
		}

		// Tool/function declarations.
		if len(cfg.Tools) > 0 {
			toolDefs := translateToolDeclarations(cfg.Tools)
			if toolDefs != nil {
				cr.Tools = make([]any, len(toolDefs))
				for i, td := range toolDefs {
					cr.Tools[i] = td
				}
			}
		}

		// Tool choice configuration.
		if cfg.ToolConfig != nil {
			cr.ToolChoice = translateToolConfig(cfg.ToolConfig)
		}
	}

	// Convert conversation contents.
	msgs, err := contentsToMessagesErr(req.Contents)
	if err != nil {
		return nil, err
	}
	cr.Messages = append(cr.Messages, msgs...)

	return cr, nil
}

// extractText returns the concatenated text from all text Parts in a Content.
// Non-text parts are ignored.
// Multiple text-only parts are concatenated directly without separator.
// This preserves the exact text content from each part.
func extractText(c *genai.Content) string {
	if c == nil {
		return ""
	}
	result := ""
	for _, p := range c.Parts {
		result += p.Text
	}
	return result
}

// contentsToMessagesErr converts a slice of [genai.Content] to OpenAI chat
// messages. The genai role "model" is mapped to the OpenAI role "assistant".
// Parts with FunctionResponse are emitted as separate "tool" role messages.
// Multimodal messages (containing InlineData) use the content-part array format.
func contentsToMessagesErr(contents []*genai.Content) ([]chatMessage, error) {
	var msgs []chatMessage
	for _, c := range contents {
		if c == nil {
			continue
		}
		converted, err := contentToMessages(c)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, converted...)
	}
	return msgs, nil
}

// openAIRole translates a genai content role to the corresponding OpenAI role.
func openAIRole(genaiRole string) string {
	if genaiRole == "model" {
		return "assistant"
	}
	return genaiRole
}

// contentToMessages converts a single [genai.Content] to one or more OpenAI
// chat messages. A single genai.Content can yield multiple messages because
// FunctionResponse Parts each become their own "tool" role message.
func contentToMessages(c *genai.Content) ([]chatMessage, error) {
	role := openAIRole(c.Role)

	// Separate FunctionResponse parts from other parts; each FunctionResponse
	// becomes its own "tool" message, as required by the OpenAI API.
	var toolMsgs []chatMessage
	var otherParts []*genai.Part
	var toolCallParts []*genai.Part

	for _, p := range c.Parts {
		// Thought parts (p.Thought == true) fall through to regular text handling.
		// The OpenAI API has no standard reasoning input format, so we preserve
		// the content as-is — thought text is included as normal text in the message.
		switch {
		case p.FunctionResponse != nil:
			responseJSON, err := json.Marshal(p.FunctionResponse.Response)
			if err != nil {
				return nil, fmt.Errorf("marshal function response: %w", err)
			}
			toolMsgs = append(toolMsgs, chatMessage{
				Role:       "tool",
				Content:    string(responseJSON),
				Name:       p.FunctionResponse.Name,
				ToolCallID: p.FunctionResponse.ID,
			})
		case p.FunctionCall != nil:
			toolCallParts = append(toolCallParts, p)
		default:
			otherParts = append(otherParts, p)
		}
	}

	var msgs []chatMessage

	// Build the main message for non-function-response parts.
	if len(otherParts) > 0 || len(toolCallParts) > 0 {
		msg := chatMessage{Role: role}

		if len(otherParts) > 0 {
			content, err := partsToContent(otherParts)
			if err != nil {
				return nil, err
			}
			msg.Content = content
		}

		if len(toolCallParts) > 0 {
			toolCalls, err := functionCallsToToolCalls(toolCallParts)
			if err != nil {
				return nil, err
			}
			msg.ToolCalls = toolCalls
		}

		msgs = append(msgs, msg)
	}

	msgs = append(msgs, toolMsgs...)
	return msgs, nil
}

// partsToContent converts a slice of non-function-response [genai.Part] to an
// OpenAI content value. Returns a plain string when all parts are text-only;
// returns a []any content-part array for multimodal content.
func partsToContent(parts []*genai.Part) (any, error) {
	// Check for multimodal content.
	isMultimodal := false
	for _, p := range parts {
		if p.InlineData != nil || p.FileData != nil {
			isMultimodal = true
			break
		}
	}

	if !isMultimodal && len(parts) == 1 {
		// Simple text-only, single part — use a plain string.
		return parts[0].Text, nil
	}

	if !isMultimodal {
		// Multiple text-only parts are concatenated directly without separator.
		// This preserves the exact text content from each part.
		combined := ""
		for _, p := range parts {
			combined += p.Text
		}
		return combined, nil
	}

	// Multimodal: build content-part array.
	var contentParts []any
	for _, p := range parts {
		switch {
		case p.Text != "":
			contentParts = append(contentParts, map[string]any{
				"type": "text",
				"text": p.Text,
			})
		case p.InlineData != nil:
			encoded := base64.StdEncoding.EncodeToString(p.InlineData.Data)
			dataURL := fmt.Sprintf("data:%s;base64,%s", p.InlineData.MIMEType, encoded)
			contentParts = append(contentParts, map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url": dataURL,
				},
			})
		case p.FileData != nil:
			contentParts = append(contentParts, map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url": p.FileData.FileURI,
				},
			})
		}
	}
	return contentParts, nil
}

// functionCallsToToolCalls converts FunctionCall Parts into OpenAI tool_calls
// entries as required by the Chat Completions API.
//
// Each Part's FunctionCall is converted via [functionCallToToolCall].
//
// Note: error return is reserved for future validation; currently always nil.
func functionCallsToToolCalls(parts []*genai.Part) ([]any, error) {
	var toolCalls []any
	for _, p := range parts {
		if p.FunctionCall == nil {
			continue
		}
		toolCalls = append(toolCalls, functionCallToToolCall(p.FunctionCall))
	}
	return toolCalls, nil
}

// translateResponse converts an OpenAI [chatResponse] into an ADK
// [model.LLMResponse], using only the first choice when present.
//
// Translation rules:
//   - assistant role → genai Content with role "model".
//   - tool_calls in the choice message → FunctionCall Parts.
//   - finish_reason is translated via [translateFinishReason].
//   - Usage statistics are mapped to [genai.GenerateContentResponseUsageMetadata].
func translateResponse(resp *chatResponse) *model.LLMResponse {
	llmResp := &model.LLMResponse{}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		llmResp.FinishReason = translateFinishReason(choice.FinishReason)
		llmResp.Content = messageToContent(choice.Message)
	}

	if resp.Usage != nil {
		llmResp.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     resp.Usage.PromptTokens,
			CandidatesTokenCount: resp.Usage.CompletionTokens,
			TotalTokenCount:      resp.Usage.TotalTokens,
		}
	}

	return llmResp
}

// messageToContent converts an OpenAI [chatMessage] back to a [genai.Content].
//
// Conversion rules:
//   - "assistant" role → "model".
//   - "user" role → "user".
//   - String content → single text Part.
//   - tool_calls → FunctionCall Parts (args parsed from JSON string).
//
// Note: Response-direction thought detection is not yet implemented.
// OpenAI has no standard reasoning response format. When a standard emerges
// (e.g., a "reasoning_content" field), this function should set Thought: true
// on the corresponding genai.Part.
func messageToContent(msg chatMessage) *genai.Content {
	role := msg.Role
	if role == "assistant" {
		role = "model"
	}

	content := &genai.Content{Role: role}

	// Text content.
	if textStr, ok := msg.Content.(string); ok && textStr != "" {
		content.Parts = append(content.Parts, &genai.Part{Text: textStr})
	}

	// Tool calls → FunctionCall Parts.
	for _, tc := range msg.ToolCalls {
		tcMap, ok := tc.(map[string]any)
		if !ok {
			continue
		}
		id, _ := tcMap["id"].(string)
		fnMap, _ := tcMap["function"].(map[string]any)
		if fnMap == nil {
			continue
		}
		name, _ := fnMap["name"].(string)
		argsStr, _ := fnMap["arguments"].(string)

		var args map[string]any
		if argsStr != "" {
			_ = json.Unmarshal([]byte(argsStr), &args)
		}

		content.Parts = append(content.Parts, &genai.Part{
			FunctionCall: &genai.FunctionCall{
				ID:   id,
				Name: name,
				Args: args,
			},
		})
	}

	return content
}

// translateFinishReason maps an OpenAI finish_reason string to the
// corresponding [genai.FinishReason] constant.
//
// Mappings:
//   - "stop"           → [genai.FinishReasonStop]
//   - "length"         → [genai.FinishReasonMaxTokens]
//   - "tool_calls"     → [genai.FinishReasonStop]
//   - "content_filter" → [genai.FinishReasonSafety]
//   - ""               → [genai.FinishReasonUnspecified]
//   - anything else    → [genai.FinishReasonOther]
func translateFinishReason(reason string) genai.FinishReason {
	switch reason {
	case "stop":
		return genai.FinishReasonStop
	case "length":
		return genai.FinishReasonMaxTokens
	case "tool_calls":
		return genai.FinishReasonStop
	case "content_filter":
		return genai.FinishReasonSafety
	case "":
		return genai.FinishReasonUnspecified
	default:
		return genai.FinishReasonOther
	}
}
