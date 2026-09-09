package openai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"strings"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

// SSE event types for the Responses API.
const (
	evtResponseCreated               = "response.created"
	evtResponseInProgress            = "response.in_progress"
	evtResponseOutputItemAdded       = "response.output_item.added"
	evtResponseOutputTextDelta       = "response.output_text.delta"
	evtResponseReasoningTextDelta    = "response.reasoning_text.delta"
	evtResponseReasoningSummaryDelta = "response.reasoning_summary_text.delta"
	evtResponseFunctionCallArgsDelta = "response.function_call_arguments.delta"
	evtResponseFunctionCallArgsDone  = "response.function_call_arguments.done"
	evtResponseOutputTextDone        = "response.output_text.done"
	evtResponseReasoningTextDone     = "response.reasoning_text.done"
	evtResponseReasoningSummaryDone  = "response.reasoning_summary_text.done"
	evtResponseOutputItemDone        = "response.output_item.done"
	evtResponseCompleted             = "response.completed"
	evtResponseIncomplete            = "response.incomplete"
	evtResponseFailed                = "response.failed"
	evtResponseQueued                = "response.queued"
	evtResponseSteer                 = "response.steer"
	evtError                         = "error"
)

// responsesStreamEvent is the common envelope for all Responses API SSE events.
// Only the Type field is always present; other fields are populated per event type.
type responsesStreamEvent struct {
	Type      string               `json:"type"`
	Response  *responsesResponse   `json:"response,omitempty"`
	Delta     string               `json:"delta,omitempty"`
	ItemID    string               `json:"item_id,omitempty"`
	Item      *responsesOutputItem `json:"item,omitempty"`
	Arguments string               `json:"arguments,omitempty"`
	Message   string               `json:"message,omitempty"`
}

// responsesFailedEvent carries the failure details for response.failed events.
type responsesFailedEvent struct {
	Type     string `json:"type"`
	Response *struct {
		Error *struct {
			Message string `json:"message,omitempty"`
		} `json:"error,omitempty"`
	} `json:"response,omitempty"`
}

// parseResponsesStream reads an OpenAI Responses API SSE response body and
// yields [model.LLMResponse] values for each meaningful event.
//
// Streaming protocol:
//   - Lines beginning with "data: " carry JSON-encoded event payloads.
//   - There is no [DONE] sentinel (unlike Chat Completions); the stream
//     terminates on response.completed / response.incomplete / response.failed / error.
//   - Text and reasoning deltas are yielded immediately as partial responses
//     (Partial: true).
//   - Function call argument deltas are buffered by item_id; the assembled
//     FunctionCall is emitted on the function_call_arguments.done event.
//   - The terminal event (response.completed / response.incomplete) produces
//     a final response with TurnComplete: true, built from the full response
//     object via [translateResponsesResponse].
//   - If ctx is cancelled before the stream finishes, the iterator yields one
//     final response with Interrupted: true and then stops.
//   - If a data line contains malformed JSON, the iterator yields an error and
//     then stops.
func parseResponsesStream(ctx context.Context, body io.Reader) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		scanner := bufio.NewScanner(body)

		// Track call_id and name mappings from output_item.added events.
		callIDs := make(map[string]string) // item_id → call_id
		names := make(map[string]string)   // item_id → name
		// Buffer function call argument deltas by item_id.
		argBuffers := make(map[string]*strings.Builder)

		var finalResponse *responsesResponse
		sawFinalResponse := false

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

			// Decode the JSON event envelope.
			var evt responsesStreamEvent
			if err := json.Unmarshal([]byte(payload), &evt); err != nil {
				yield(nil, fmt.Errorf("parseResponsesStream: decode event: %w", err))
				return
			}

			switch evt.Type {
			case evtResponseCreated:
				// Capture the response object for metadata + final assembly.
				if evt.Response != nil {
					finalResponse = evt.Response
				}

			case evtResponseInProgress:
				// Informational, skip.

			case evtResponseOutputItemAdded:
				// Track item_id → call_id and item_id → name (only when non-empty).
				if evt.Item != nil {
					if evt.Item.CallID != "" {
						callIDs[evt.ItemID] = evt.Item.CallID
					}
					if evt.Item.Name != "" {
						names[evt.ItemID] = evt.Item.Name
					}
				}

			case evtResponseOutputTextDelta:
				if evt.Delta != "" {
					if !yield(singlePartResponse(&genai.Part{Text: evt.Delta}), nil) {
						return
					}
				}

			case evtResponseReasoningTextDelta, evtResponseReasoningSummaryDelta:
				if evt.Delta != "" {
					if !yield(singlePartResponse(&genai.Part{Text: evt.Delta, Thought: true}), nil) {
						return
					}
				}

			case evtResponseFunctionCallArgsDelta:
				if evt.Delta != "" {
					buf, ok := argBuffers[evt.ItemID]
					if !ok {
						buf = &strings.Builder{}
						argBuffers[evt.ItemID] = buf
					}
					buf.WriteString(evt.Delta)
				}

			case evtResponseFunctionCallArgsDone:
				// Emit the FunctionCall Part as a partial response.
				args := evt.Arguments
				if args == "" {
					if buf, ok := argBuffers[evt.ItemID]; ok {
						args = buf.String()
					}
				}
				var argMap map[string]any
				if args != "" {
					_ = json.Unmarshal([]byte(args), &argMap)
				} else {
					argMap = map[string]any{}
				}
				callID := callIDs[evt.ItemID]
				name := names[evt.ItemID]
				part := &genai.Part{
					FunctionCall: &genai.FunctionCall{
						ID:   callID,
						Name: name,
						Args: argMap,
					},
				}
				if !yield(singlePartResponse(part), nil) {
					return
				}

			case evtResponseOutputTextDone, evtResponseReasoningTextDone,
				evtResponseReasoningSummaryDone, evtResponseOutputItemDone:
				// Informational, skip.

			case evtResponseCompleted, evtResponseIncomplete:
				if evt.Response != nil {
					finalResponse = evt.Response
				}
				sawFinalResponse = true

			case evtResponseFailed:
				// Parse the failed event for the error message.
				var failed responsesFailedEvent
				_ = json.Unmarshal([]byte(payload), &failed)
				msg := "openai: response failed"
				if failed.Response != nil && failed.Response.Error != nil && failed.Response.Error.Message != "" {
					msg = failed.Response.Error.Message
				}
				yield(nil, fmt.Errorf("openai: response failed: %s", msg))
				return

			case evtError:
				msg := "openai stream error"
				if evt.Message != "" {
					msg = evt.Message
				}
				yield(nil, fmt.Errorf("openai: stream error: %s", msg))
				return

			case evtResponseQueued, evtResponseSteer:
				// Background/WebSocket mode — out of scope, skip.

			default:
				// Unknown event type — skip.
			}
		}

		// Handle scanner errors (e.g. unexpected EOF, I/O errors).
		if err := scanner.Err(); err != nil {
			yield(nil, fmt.Errorf("parseResponsesStream: scanner: %w", err))
			return
		}

		// After the stream ends, if we saw a terminal event, build the final
		// response from the terminal response object.
		if sawFinalResponse && finalResponse != nil {
			llmResp, err := translateResponsesResponse(finalResponse)
			if err != nil {
				yield(nil, fmt.Errorf("openai: translate final response: %w", err))
				return
			}
			llmResp.TurnComplete = true
			yield(llmResp, nil)
		}
		// If !sawFinalResponse (stream ended without terminal event, e.g.
		// connection dropped), yield nothing further — the turn is incomplete.
	}
}

// singlePartResponse wraps a single Part as a [model.LLMResponse] with
// Partial=true and no FinishReason. The final response carries the real
// FinishReason from the terminal event.
func singlePartResponse(part *genai.Part) *model.LLMResponse {
	return &model.LLMResponse{
		Partial: true,
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{part},
		},
	}
}
