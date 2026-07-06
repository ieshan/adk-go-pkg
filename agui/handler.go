package agui

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

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

	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, cfg.MaxBodySize)

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
		session := newSSESession(w, flusher, sseWriter)

		// Start keepalive goroutine if configured.
		if cfg.KeepaliveInterval > 0 {
			keepaliveCtx, cancelKeepalive := context.WithCancel(ctx)
			defer cancelKeepalive()

			go func() {
				ticker := time.NewTicker(cfg.KeepaliveInterval)
				defer ticker.Stop()
				for {
					select {
					case <-keepaliveCtx.Done():
						return
					case <-ticker.C:
						session.WritePing()
					}
				}
			}()
		}

		for ev, err := range agent.Run(ctx, input) {
			if err != nil {
				errEv := events.NewRunErrorEvent(err.Error(), events.WithRunID(input.RunID))
				_ = session.WriteEvent(ctx, errEv)
				if cfg.OnError != nil {
					cfg.OnError(err)
				}
				return
			}
			if writeErr := session.WriteEvent(ctx, ev); writeErr != nil {
				if cfg.OnError != nil {
					cfg.OnError(writeErr)
				}
				return
			}
		}
	})

	// Apply CORS middleware if configured.
	if cfg.CORS != nil {
		handler = CORSMiddleware(cfg.CORS)(handler)
	}

	return handler, nil
}
