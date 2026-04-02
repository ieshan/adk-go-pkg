package agui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ToolMode controls how client tool results are received.
type ToolMode int

const (
	// ToolModeNextRun ends the run after emitting tool call events.
	ToolModeNextRun ToolMode = iota
	// ToolModeInline keeps the SSE connection open while waiting for results.
	ToolModeInline
)

// ToolResultHandler manages pending tool call results for inline mode.
type ToolResultHandler struct {
	mu      sync.Mutex
	pending map[string]chan string
}

// NewToolResultHandler creates a handler for receiving inline tool results.
func NewToolResultHandler() *ToolResultHandler {
	return &ToolResultHandler{pending: make(map[string]chan string)}
}

// Wait blocks until a result is submitted for the given tool call ID.
func (h *ToolResultHandler) Wait(ctx context.Context, toolCallID string, timeout time.Duration) (string, error) {
	ch := make(chan string, 1)
	h.mu.Lock()
	h.pending[toolCallID] = ch
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.pending, toolCallID)
		h.mu.Unlock()
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case result := <-ch:
		return result, nil
	case <-ctx.Done():
		return "", ctx.Err()
	case <-timer.C:
		return "", fmt.Errorf("agui: tool call %s timed out after %v", toolCallID, timeout)
	}
}

// SubmitResult delivers a tool result for a pending tool call.
func (h *ToolResultHandler) SubmitResult(toolCallID, content string) error {
	h.mu.Lock()
	ch, ok := h.pending[toolCallID]
	h.mu.Unlock()
	if !ok {
		return fmt.Errorf("agui: no pending tool call %s", toolCallID)
	}
	ch <- content
	return nil
}

// ToolResultEndpoint returns an http.Handler for POST /tool-result.
// Accepts JSON body: {"toolCallId": "...", "content": "..."}
func ToolResultEndpoint(handler *ToolResultHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ToolCallID string `json:"toolCallId"`
			Content    string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.ToolCallID == "" {
			http.Error(w, "toolCallId required", http.StatusBadRequest)
			return
		}
		if err := handler.SubmitResult(body.ToolCallID, body.Content); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}
