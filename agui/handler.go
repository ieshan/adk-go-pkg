package agui

import (
	"encoding/json"
	"net/http"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	agsse "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/encoding/sse"
)

// Handler returns an http.Handler that serves the AG-UI SSE endpoint.
func Handler(cfg Config) (http.Handler, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	cfg.applyDefaults()

	// Apply middleware chain.
	agent := cfg.Agent
	if len(cfg.Middlewares) > 0 {
		agent = Chain(cfg.Middlewares...)(agent)
	}

	sseWriter := agsse.NewSSEWriter()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var input types.RunAgentInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Set SSE headers.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		ctx := r.Context()

		for ev, err := range agent.Run(ctx, input) {
			if err != nil {
				// Emit RUN_ERROR event.
				errEv := events.NewRunErrorEvent(err.Error())
				_ = sseWriter.WriteEvent(ctx, w, errEv)
				flusher.Flush()
				if cfg.OnError != nil {
					cfg.OnError(err)
				}
				return
			}

			if writeErr := sseWriter.WriteEvent(ctx, w, ev); writeErr != nil {
				if cfg.OnError != nil {
					cfg.OnError(writeErr)
				}
				return
			}
			flusher.Flush()
		}
	}), nil
}
