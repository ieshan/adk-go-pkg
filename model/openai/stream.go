package openai

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

// chatStreamChunk is the top-level object in each SSE data line of an OpenAI
// streaming response.
type chatStreamChunk struct {
	// ID is the unique identifier for the completion, shared across all chunks.
	ID string `json:"id"`
	// Choices holds the streaming delta candidates.
	Choices []chatStreamChoice `json:"choices"`
	// Usage is only populated in the final chunk when stream_options includes
	// usage statistics. It may be nil for intermediate chunks.
	Usage *chatUsage `json:"usage,omitempty"`
}

// chatStreamChoice is one candidate inside a [chatStreamChunk].
type chatStreamChoice struct {
	// Index is the zero-based index of this candidate.
	Index int `json:"index"`
	// Delta carries the incremental content for this chunk.
	Delta chatStreamDelta `json:"delta"`
	// FinishReason, when non-nil, indicates why generation stopped for this
	// candidate. Common values: "stop", "length", "tool_calls".
	FinishReason *string `json:"finish_reason,omitempty"`
}

// chatStreamDelta is the incremental content within a [chatStreamChoice].
type chatStreamDelta struct {
	// Role is the author role; only present in the first chunk ("assistant").
	Role string `json:"role,omitempty"`
	// Content is the incremental text for this chunk.
	Content string `json:"content,omitempty"`
	// ToolCalls contains partial tool-call descriptors. Each element carries an
	// "index" field that identifies which tool call it belongs to.
	ToolCalls []any `json:"tool_calls,omitempty"`
}

const (
	// sseDataPrefix is the prefix that marks payload lines in an SSE stream.
	sseDataPrefix = "data: "
	// sseDone is the sentinel value that signals end-of-stream.
	sseDone = "[DONE]"
)

// accumulatedToolCall holds the data being built up for a single tool call
// whose arguments arrive in fragments across multiple SSE chunks.
type accumulatedToolCall struct {
	// id is the OpenAI tool-call ID (e.g. "call_abc123").
	id string
	// name is the function name.
	name string
	// arguments accumulates the JSON-encoded argument fragments.
	arguments strings.Builder
}

// parseStream reads an OpenAI SSE response body and yields [model.LLMResponse]
// objects for each meaningful event.
//
// Streaming protocol:
//   - Lines beginning with "data: " carry JSON-encoded [chatStreamChunk] payloads.
//   - The sentinel "data: [DONE]" marks the end of the stream; parseStream yields
//     a final response with TurnComplete: true and then stops iteration.
//   - Blank lines and SSE comment lines (starting with ':') are silently skipped.
//   - Text-content deltas are yielded immediately as partial responses
//     (Partial: true).
//   - Tool-call deltas are accumulated in memory across chunks keyed by their
//     "index" field. The assembled tool calls are converted to FunctionCall Parts
//     and emitted in the response that carries a non-nil FinishReason.
//   - If ctx is cancelled before the stream finishes, the iterator yields one
//     final response with Interrupted: true and then stops.
//   - If a data line contains malformed JSON, the iterator yields an error and
//     then stops.
//
// Example usage:
//
//	for resp, err := range parseStream(ctx, httpResp.Body) {
//	    if err != nil {
//	        // handle error
//	        break
//	    }
//	    if resp.TurnComplete {
//	        // stream finished
//	        break
//	    }
//	    // resp.Content may carry incremental text or FunctionCall parts.
//	}
func parseStream(ctx context.Context, body io.Reader) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		scanner := bufio.NewScanner(body)

		// toolCallsByIndex accumulates partial tool-call data across chunks.
		// The map key is the "index" field from the streaming delta.
		toolCallsByIndex := make(map[int]*accumulatedToolCall)
		// toolCallOrder preserves insertion order so the resulting FunctionCall
		// Parts appear in the same sequence as the model emitted them.
		var toolCallOrder []int

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

			// The [DONE] sentinel signals end of stream.
			if payload == sseDone {
				yield(&model.LLMResponse{TurnComplete: true}, nil)
				return
			}

			// Decode the JSON chunk.
			var chunk chatStreamChunk
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				yield(nil, fmt.Errorf("parseStream: decode chunk: %w", err))
				return
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			choice := chunk.Choices[0]
			delta := choice.Delta

			// Accumulate tool-call fragments keyed by their index.
			for _, rawTC := range delta.ToolCalls {
				tcMap, ok := rawTC.(map[string]any)
				if !ok {
					continue
				}
				idxFloat, _ := tcMap["index"].(float64)
				idx := int(idxFloat)

				if _, seen := toolCallsByIndex[idx]; !seen {
					toolCallsByIndex[idx] = &accumulatedToolCall{}
					toolCallOrder = append(toolCallOrder, idx)
				}
				acc := toolCallsByIndex[idx]

				if id, ok := tcMap["id"].(string); ok && id != "" {
					acc.id = id
				}
				fnMap, _ := tcMap["function"].(map[string]any)
				if fnMap != nil {
					if name, ok := fnMap["name"].(string); ok && name != "" {
						acc.name = name
					}
					if args, ok := fnMap["arguments"].(string); ok {
						acc.arguments.WriteString(args)
					}
				}
			}

			// Build a partial response for this chunk.
			resp := &model.LLMResponse{Partial: true}

			// Attach text content when present.
			if delta.Content != "" {
				resp.Content = &genai.Content{
					Role:  "model",
					Parts: []*genai.Part{{Text: delta.Content}},
				}
			}

			// When FinishReason is set, attach any accumulated tool calls and
			// apply usage metadata.
			if choice.FinishReason != nil {
				resp.FinishReason = translateFinishReason(*choice.FinishReason)
				resp.Partial = false

				if len(toolCallsByIndex) > 0 {
					// Convert accumulated tool calls to the map format expected by
					// toolCallsToFunctionCalls.
					tcMaps := make([]map[string]any, 0, len(toolCallOrder))
					for _, idx := range toolCallOrder {
						acc := toolCallsByIndex[idx]
						tcMaps = append(tcMaps, map[string]any{
							"id":   acc.id,
							"type": "function",
							"function": map[string]any{
								"name":      acc.name,
								"arguments": acc.arguments.String(),
							},
						})
					}
					parts := toolCallsToFunctionCalls(tcMaps)
					if len(parts) > 0 {
						if resp.Content == nil {
							resp.Content = &genai.Content{Role: "model"}
						}
						resp.Content.Parts = append(resp.Content.Parts, parts...)
					}
				}

				if chunk.Usage != nil {
					resp.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{
						PromptTokenCount:     chunk.Usage.PromptTokens,
						CandidatesTokenCount: chunk.Usage.CompletionTokens,
						TotalTokenCount:      chunk.Usage.TotalTokens,
					}
				}
			}

			// Yield this partial response; stop if the consumer returns false.
			if !yield(resp, nil) {
				return
			}
		}

		// Handle scanner errors (e.g. unexpected EOF, I/O errors).
		if err := scanner.Err(); err != nil {
			yield(nil, fmt.Errorf("parseStream: scanner: %w", err))
		}
	}
}
