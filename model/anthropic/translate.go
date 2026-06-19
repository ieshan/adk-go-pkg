package anthropic

import (
	"encoding/base64"
	"fmt"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// messageRequest is the request body for the Anthropic /v1/messages endpoint.
type messageRequest struct {
	Model         string           `json:"model"`
	MaxTokens     int              `json:"max_tokens"`
	Messages      []message        `json:"messages"`
	System        any              `json:"system,omitempty"`
	Tools         []map[string]any `json:"tools,omitempty"`
	ToolChoice    any              `json:"tool_choice,omitempty"`
	StopSequences []string         `json:"stop_sequences,omitempty"`
	Stream        bool             `json:"stream,omitempty"`
	Thinking      *thinkingConfig  `json:"thinking,omitempty"`
	OutputConfig  *outputConfig    `json:"output_config,omitempty"`
	CacheControl  map[string]any   `json:"cache_control,omitempty"`
}

// message represents a single turn in an Anthropic conversation.
type message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// messageResponse is the non-streaming response body from Anthropic /v1/messages.
type messageResponse struct {
	ID           string           `json:"id"`
	Type         string           `json:"type"`
	Role         string           `json:"role"`
	Content      []map[string]any `json:"content"`
	Model        string           `json:"model"`
	StopReason   string           `json:"stop_reason"`
	StopSequence *string          `json:"stop_sequence,omitempty"`
	Usage        *anthropicUsage  `json:"usage,omitempty"`
}

// anthropicUsage holds token-count statistics for an Anthropic request.
type anthropicUsage struct {
	InputTokens              int32 `json:"input_tokens"`
	OutputTokens             int32 `json:"output_tokens"`
	CacheCreationInputTokens int32 `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int32 `json:"cache_read_input_tokens,omitempty"`
}

// thinkingConfig maps to Anthropic's thinking request field.
type thinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int32  `json:"budget_tokens,omitempty"`
}

// outputConfig maps to Anthropic's output_config request field for structured
// outputs.
type outputConfig struct {
	Format     string         `json:"format"`
	JSONSchema map[string]any `json:"json_schema,omitempty"`
}

// buildMessageRequest converts an ADK [model.LLMRequest] into an Anthropic
// [messageRequest] ready to be sent to the /v1/messages endpoint.
func buildMessageRequest(req *model.LLMRequest, modelName string, stream bool, cacheControl map[string]any) (*messageRequest, error) {
	mr := &messageRequest{
		Model:  modelName,
		Stream: stream,
	}

	if req.Config != nil {
		cfg := req.Config

		// max_tokens is required by Anthropic; default to 4096 if zero.
		if cfg.MaxOutputTokens > 0 {
			mr.MaxTokens = int(cfg.MaxOutputTokens)
		} else {
			mr.MaxTokens = 4096
		}

		mr.StopSequences = cfg.StopSequences

		// System instruction goes to top-level system field.
		if cfg.SystemInstruction != nil {
			sysText := extractSystemText(cfg.SystemInstruction)
			if sysText != "" {
				mr.System = sysText
			}
		}

		// Response schema / structured output.
		if cfg.ResponseSchema != nil {
			mr.OutputConfig = &outputConfig{
				Format:     "json",
				JSONSchema: schemaToJSONSchema(cfg.ResponseSchema),
			}
		} else if cfg.ResponseJsonSchema != nil {
			if m, ok := cfg.ResponseJsonSchema.(map[string]any); ok {
				mr.OutputConfig = &outputConfig{
					Format:     "json",
					JSONSchema: m,
				}
			}
		} else if cfg.ResponseMIMEType == "application/json" {
			mr.OutputConfig = &outputConfig{Format: "json"}
		}

		// Tools.
		if len(cfg.Tools) > 0 {
			mr.Tools = translateToolDeclarations(cfg.Tools)
		}

		// Tool choice.
		if cfg.ToolConfig != nil {
			mr.ToolChoice = translateToolConfig(cfg.ToolConfig)
		}

		// Thinking config.
		if cfg.ThinkingConfig != nil {
			thinking := &thinkingConfig{}
			if cfg.ThinkingConfig.IncludeThoughts {
				thinking.Type = "enabled"
			} else {
				thinking.Type = "adaptive"
			}
			if cfg.ThinkingConfig.ThinkingBudget != nil {
				thinking.BudgetTokens = *cfg.ThinkingConfig.ThinkingBudget
			}
			mr.Thinking = thinking
		}
	} else {
		mr.MaxTokens = 4096
	}

	// Top-level automatic caching.
	if cacheControl != nil {
		mr.CacheControl = cacheControl
	}

	// Convert conversation contents.
	msgs, err := contentsToMessages(req.Contents)
	if err != nil {
		return nil, err
	}
	mr.Messages = msgs

	return mr, nil
}

// contentsToMessages converts a slice of [genai.Content] to Anthropic messages.
// The genai role "model" is mapped to "assistant". Each genai.Content can
// yield one or more Anthropic messages because FunctionResponse parts each
// become separate tool_result blocks.
func contentsToMessages(contents []*genai.Content) ([]message, error) {
	var msgs []message
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

// contentToMessages converts a single [genai.Content] to one or more Anthropic
// messages.
func contentToMessages(c *genai.Content) ([]message, error) {
	role := c.Role
	if role == "model" {
		role = "assistant"
	}

	var toolResultBlocks []map[string]any
	var otherParts []*genai.Part
	var toolCallParts []*genai.Part

	for _, p := range c.Parts {
		switch {
		case p.FunctionResponse != nil:
			blocks, err := contentBlocksFromFunctionResponse(p.FunctionResponse)
			if err != nil {
				return nil, fmt.Errorf("function response: %w", err)
			}
			toolResultBlocks = append(toolResultBlocks, blocks...)
		case p.FunctionCall != nil:
			toolCallParts = append(toolCallParts, p)
		default:
			otherParts = append(otherParts, p)
		}
	}

	var msgs []message

	// Build the main message for non-function-response parts.
	if len(otherParts) > 0 || len(toolCallParts) > 0 {
		msg := message{Role: role}
		blocks, err := partsToContentBlocks(otherParts, toolCallParts)
		if err != nil {
			return nil, err
		}
		msg.Content = blocks
		msgs = append(msgs, msg)
	}

	// tool_result blocks go into user messages.
	if len(toolResultBlocks) > 0 {
		msgs = append(msgs, message{
			Role:    "user",
			Content: toolResultBlocks,
		})
	}

	return msgs, nil
}

// partsToContentBlocks converts genai Parts into Anthropic content blocks.
// It handles text, images (inline and URL), tool_use, and thinking parts.
func partsToContentBlocks(parts []*genai.Part, toolCallParts []*genai.Part) ([]map[string]any, error) {
	var blocks []map[string]any

	for _, p := range parts {
		if p.Thought {
			block := map[string]any{
				"type":      "thinking",
				"thinking":  p.Text,
				"signature": string(p.ThoughtSignature),
			}
			// Thinking blocks are not cacheable; skip cache_control silently.
			blocks = append(blocks, block)
			continue
		}

		if p.Text != "" {
			block := map[string]any{
				"type": "text",
				"text": p.Text,
			}
			// Check for explicit cache control on this part.
			if cc := extractCacheControl(p); cc != nil {
				block["cache_control"] = cc
			}
			blocks = append(blocks, block)
			continue
		}

		if p.InlineData != nil {
			block := map[string]any{
				"type": "image",
				"source": map[string]any{
					"type":       "base64",
					"media_type": p.InlineData.MIMEType,
					"data":       base64.StdEncoding.EncodeToString(p.InlineData.Data),
				},
			}
			if cc := extractCacheControl(p); cc != nil {
				block["cache_control"] = cc
			}
			blocks = append(blocks, block)
			continue
		}

		if p.FileData != nil {
			if !isHTTPURL(p.FileData.FileURI) {
				return nil, fmt.Errorf("FileData.FileURI %q is not an HTTP/HTTPS URL; Anthropic adapter only supports URL images", p.FileData.FileURI)
			}
			block := map[string]any{
				"type": "image",
				"source": map[string]any{
					"type":       "url",
					"url":        p.FileData.FileURI,
					"media_type": p.FileData.MIMEType,
				},
			}
			if cc := extractCacheControl(p); cc != nil {
				block["cache_control"] = cc
			}
			blocks = append(blocks, block)
			continue
		}
	}

	// Add tool_use blocks from toolCallParts.
	for _, p := range toolCallParts {
		if p.FunctionCall != nil {
			blocks = append(blocks, functionCallToToolUse(p.FunctionCall))
		}
	}

	return blocks, nil
}

// extractCacheControl extracts a cache_control map from a Part's PartMetadata.
// Returns nil if the part has no cache_control metadata or if the block type
// is non-cacheable (tool_use, tool_result, thinking).
func extractCacheControl(p *genai.Part) map[string]any {
	if p.PartMetadata == nil {
		return nil
	}
	cc, ok := p.PartMetadata["cache_control"].(map[string]any)
	if !ok {
		return nil
	}
	return cc
}

// translateResponse converts an Anthropic [messageResponse] into an ADK
// [model.LLMResponse].
func translateResponse(resp *messageResponse) *model.LLMResponse {
	llmResp := &model.LLMResponse{
		FinishReason: translateFinishReason(resp.StopReason),
	}

	content := &genai.Content{Role: "model"}
	for _, block := range resp.Content {
		blockType, _ := block["type"].(string)
		switch blockType {
		case "text":
			if text, ok := block["text"].(string); ok {
				content.Parts = append(content.Parts, &genai.Part{Text: text})
			}
		case "tool_use":
			content.Parts = append(content.Parts, &genai.Part{
				FunctionCall: toolUseToFunctionCall(block),
			})
		case "thinking":
			thinking, _ := block["thinking"].(string)
			sig, _ := block["signature"].(string)
			content.Parts = append(content.Parts, &genai.Part{
				Text:             thinking,
				Thought:          true,
				ThoughtSignature: []byte(sig),
			})
		}
	}
	if len(content.Parts) > 0 {
		llmResp.Content = content
	}

	if resp.Usage != nil {
		llmResp.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:        resp.Usage.InputTokens,
			CandidatesTokenCount:    resp.Usage.OutputTokens,
			TotalTokenCount:         resp.Usage.InputTokens + resp.Usage.OutputTokens,
			CachedContentTokenCount: resp.Usage.CacheReadInputTokens,
		}
		if resp.Usage.CacheCreationInputTokens > 0 || resp.Usage.CacheReadInputTokens > 0 {
			llmResp.CustomMetadata = map[string]any{
				"cache_creation_input_tokens": resp.Usage.CacheCreationInputTokens,
				"cache_read_input_tokens":     resp.Usage.CacheReadInputTokens,
			}
		}
	}

	return llmResp
}

// translateFinishReason maps an Anthropic stop_reason string to the
// corresponding [genai.FinishReason] constant.
//
// Mappings:
//   - "end_turn"      → [genai.FinishReasonStop]
//   - "max_tokens"    → [genai.FinishReasonMaxTokens]
//   - "stop_sequence" → [genai.FinishReasonStop]
//   - "tool_use"      → [genai.FinishReasonStop]
//   - "refusal"       → [genai.FinishReasonSafety]
//   - ""              → [genai.FinishReasonUnspecified]
//   - anything else   → [genai.FinishReasonOther]
func translateFinishReason(reason string) genai.FinishReason {
	switch reason {
	case "end_turn":
		return genai.FinishReasonStop
	case "max_tokens":
		return genai.FinishReasonMaxTokens
	case "stop_sequence":
		return genai.FinishReasonStop
	case "tool_use":
		return genai.FinishReasonStop
	case "refusal":
		return genai.FinishReasonSafety
	case "":
		return genai.FinishReasonUnspecified
	default:
		return genai.FinishReasonOther
	}
}
