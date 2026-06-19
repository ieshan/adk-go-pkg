package anthropic

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// streamEvent is the top-level object in each SSE data line of an Anthropic
// streaming response.
type streamEvent struct {
	Type         string `json:"type"`
	Message      any    `json:"message,omitempty"`
	Index        int    `json:"index,omitempty"`
	ContentBlock any    `json:"content_block,omitempty"`
	Delta        any    `json:"delta,omitempty"`
	Usage        any    `json:"usage,omitempty"`
}

const sseDataPrefix = "data: "

// accumulatedBlock holds the data being built up for a single content block
// whose content arrives in fragments across multiple SSE events.
type accumulatedBlock struct {
	blockType string
	id        string
	name      string
	signature string
	text      strings.Builder
	thinking  strings.Builder
	jsonInput strings.Builder
}

// parseStream reads an Anthropic SSE response body and yields
// [model.LLMResponse] objects for each meaningful event.
//
// Streaming protocol:
//   - Lines beginning with "event: " carry the event type.
//   - Lines beginning with "data: " carry JSON-encoded [streamEvent] payloads.
//   - Blank lines separate event groups.
//   - No [DONE] sentinel — stream ends at message_stop.
//
// State machine:
//  1. message_start — initialize message metadata.
//  2. content_block_start — create accumulator for block at index.
//  3. content_block_delta — append delta to current block accumulator;
//     yield Partial=true response for text deltas.
//  4. content_block_stop — finalize block; if tool_use, parse accumulated JSON.
//  5. message_delta — capture stop_reason and usage.
//  6. message_stop — yield final response with TurnComplete=true, all
//     accumulated parts, FinishReason, UsageMetadata.
//
// If ctx is cancelled before the stream finishes, the iterator yields one final
// response with Interrupted: true and then stops.
//
// If a data line contains malformed JSON, the iterator yields an error and
// then stops.
func parseStream(ctx context.Context, body io.Reader) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		scanner := bufio.NewScanner(body)
		blocksByIndex := make(map[int]*accumulatedBlock)
		var blockOrder []int

		var finishReason string
		var usage *genai.GenerateContentResponseUsageMetadata
		var customMetadata map[string]any

		for scanner.Scan() {
			// Check for context cancellation before processing each line.
			if ctx.Err() != nil {
				yield(&model.LLMResponse{Interrupted: true}, nil)
				return
			}

			line := scanner.Text()

			// Skip blank lines and SSE comment lines.
			if line == "" || strings.HasPrefix(line, ":") {
				continue
			}

			// Only process lines that carry a data payload.
			if !strings.HasPrefix(line, sseDataPrefix) {
				continue
			}

			payload := strings.TrimPrefix(line, sseDataPrefix)

			// Decode the JSON event.
			var event streamEvent
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				yield(nil, fmt.Errorf("parseStream: decode event: %w", err))
				return
			}

			switch event.Type {
			case "message_start":
				// Message metadata initialization; nothing to yield yet.

			case "content_block_start":
				blockMap, ok := event.ContentBlock.(map[string]any)
				if !ok {
					continue
				}
				blockType, _ := blockMap["type"].(string)
				acc := &accumulatedBlock{blockType: blockType}
				if id, ok := blockMap["id"].(string); ok {
					acc.id = id
				}
				if name, ok := blockMap["name"].(string); ok {
					acc.name = name
				}
				if sig, ok := blockMap["signature"].(string); ok {
					acc.signature = sig
				}
				// Do NOT pre-populate jsonInput from content_block_start's input
				// field; input_json_delta streams the complete JSON from scratch.
				blocksByIndex[event.Index] = acc
				blockOrder = append(blockOrder, event.Index)

			case "content_block_delta":
				acc, ok := blocksByIndex[event.Index]
				if !ok {
					continue
				}
				deltaMap, ok := event.Delta.(map[string]any)
				if !ok {
					continue
				}

				switch deltaMap["type"] {
				case "text_delta":
					if text, ok := deltaMap["text"].(string); ok {
						acc.text.WriteString(text)
						yield(&model.LLMResponse{
							Partial: true,
							Content: &genai.Content{
								Role:  "model",
								Parts: []*genai.Part{{Text: text}},
							},
						}, nil)
					}
				case "thinking_delta":
					if text, ok := deltaMap["thinking"].(string); ok {
						acc.thinking.WriteString(text)
					}
				case "input_json_delta":
					if partial, ok := deltaMap["partial_json"].(string); ok {
						acc.jsonInput.WriteString(partial)
					}
				}

			case "content_block_stop":
				acc, ok := blocksByIndex[event.Index]
				if !ok {
					continue
				}
				// Finalize tool_use block by parsing accumulated JSON.
				if acc.blockType == "tool_use" && acc.jsonInput.Len() > 0 {
					var args map[string]any
					if err := json.Unmarshal([]byte(acc.jsonInput.String()), &args); err != nil {
						// Preserve nil args so downstream can handle it.
						args = nil
					}
					acc.jsonInput.Reset()
					// Store parsed args as JSON string for later assembly.
					b, _ := json.Marshal(args)
					acc.jsonInput.Write(b)
				}

			case "message_delta":
				deltaMap, ok := event.Delta.(map[string]any)
				if ok {
					if sr, ok := deltaMap["stop_reason"].(string); ok {
						finishReason = sr
					}
				}
				if event.Usage != nil {
					usage = extractUsageFromAny(event.Usage)
					customMetadata = extractCacheMetadataFromAny(event.Usage)
				}

			case "message_stop":
				// Assemble final response with all accumulated blocks.
				resp := &model.LLMResponse{
					TurnComplete: true,
					FinishReason: translateFinishReason(finishReason),
				}
				if usage != nil {
					resp.UsageMetadata = usage
				}
				if customMetadata != nil {
					resp.CustomMetadata = customMetadata
				}

				content := &genai.Content{Role: "model"}
				for _, idx := range blockOrder {
					acc := blocksByIndex[idx]
					switch acc.blockType {
					case "text":
						if acc.text.Len() > 0 {
							content.Parts = append(content.Parts, &genai.Part{
								Text: acc.text.String(),
							})
						}
					case "tool_use":
						var args map[string]any
						if acc.jsonInput.Len() > 0 {
							_ = json.Unmarshal([]byte(acc.jsonInput.String()), &args)
						}
						content.Parts = append(content.Parts, &genai.Part{
							FunctionCall: &genai.FunctionCall{
								ID:   acc.id,
								Name: acc.name,
								Args: args,
							},
						})
					case "thinking":
						content.Parts = append(content.Parts, &genai.Part{
							Text:             acc.thinking.String(),
							Thought:          true,
							ThoughtSignature: []byte(acc.signature),
						})
					}
				}
				if len(content.Parts) > 0 {
					resp.Content = content
				}
				yield(resp, nil)
				return
			}
		}

		// Handle scanner errors (e.g. unexpected EOF, I/O errors).
		if err := scanner.Err(); err != nil {
			yield(nil, fmt.Errorf("parseStream: scanner: %w", err))
		}
	}
}

// extractUsageFromAny extracts usage metadata from the usage field of a
// message_delta event.
func extractUsageFromAny(usageAny any) *genai.GenerateContentResponseUsageMetadata {
	usageMap, ok := usageAny.(map[string]any)
	if !ok {
		return nil
	}
	um := &genai.GenerateContentResponseUsageMetadata{}
	if v, ok := usageMap["input_tokens"].(float64); ok {
		um.PromptTokenCount = int32(v)
	}
	if v, ok := usageMap["output_tokens"].(float64); ok {
		um.CandidatesTokenCount = int32(v)
	}
	um.TotalTokenCount = um.PromptTokenCount + um.CandidatesTokenCount
	if v, ok := usageMap["cache_read_input_tokens"].(float64); ok {
		um.CachedContentTokenCount = int32(v)
	}
	return um
}

// extractCacheMetadataFromAny extracts Anthropic-specific cache usage from the
// usage object and returns it as CustomMetadata fields.
func extractCacheMetadataFromAny(usageAny any) map[string]any {
	usageMap, ok := usageAny.(map[string]any)
	if !ok {
		return nil
	}
	meta := make(map[string]any)
	if v, ok := usageMap["cache_creation_input_tokens"].(float64); ok {
		meta["cache_creation_input_tokens"] = int32(v)
	}
	if v, ok := usageMap["cache_read_input_tokens"].(float64); ok {
		meta["cache_read_input_tokens"] = int32(v)
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}
