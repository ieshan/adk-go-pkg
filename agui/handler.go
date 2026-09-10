package agui

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
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
		// Capabilities discovery: GET / or GET /capabilities returns the
		// configured AgentCapabilities as JSON. Matching by suffix allows
		// the handler to be mounted at any sub-path (e.g. /api/agent/).
		if r.Method == http.MethodGet && (r.URL.Path == "/" || strings.HasSuffix(r.URL.Path, "/capabilities")) {
			if cfg.Capabilities == nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(cfg.Capabilities); err != nil {
				http.Error(w, "failed to encode capabilities: "+err.Error(), http.StatusInternalServerError)
			}
			return
		}

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

		// Accept header negotiation: this server only supports SSE
		// (text/event-stream) and JSON. Reject protobuf transport requests.
		if isProtobufAccept(r.Header.Get("Accept")) {
			http.Error(w, "protobuf transport not supported", http.StatusNotAcceptable)
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

		runCtx, cancel := context.WithCancel(r.Context())
		defer cancel()

		session := newSSESession(w, flusher, sseWriter, cancel)

		// Start keepalive goroutine if configured.
		if cfg.KeepaliveInterval > 0 {
			keepaliveCtx, cancelKeepalive := context.WithCancel(runCtx)
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

		for ev, err := range agent.Run(runCtx, input) {
			if err != nil {
				errEv := events.NewRunErrorEvent(err.Error(), events.WithRunID(input.RunID))
				_ = session.WriteEvent(runCtx, errEv)
				if cfg.OnError != nil {
					cfg.OnError(err)
				}
				return
			}
			if writeErr := session.WriteEvent(runCtx, ev); writeErr != nil {
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

// isProtobufAccept reports whether the Accept header requests a protobuf
// transport. The AG-UI protocol registers several protobuf media types
// (application/vnd.ag-ui.event+proto, application/x-protobuf,
// application/protobuf, application/vnd.google.protobuf); this server
// only supports SSE (text/event-stream) and JSON.
func isProtobufAccept(accept string) bool {
	protobufTypes := []string{
		"application/vnd.ag-ui.event+proto",
		"application/x-protobuf",
		"application/protobuf",
		"application/vnd.google.protobuf",
	}
	for _, pt := range protobufTypes {
		if strings.Contains(accept, pt) {
			return true
		}
	}
	return false
}
