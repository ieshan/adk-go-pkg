package aguiadk

import (
	"net/http"

	"github.com/ieshan/adk-go-pkg/agui"
)

// Handler creates a complete AG-UI HTTP handler for an ADK-Go agent.
// It combines New (which bridges ADK to AG-UI) with agui.Handler
// (which serves the SSE endpoint), providing a single-call setup.
//
// The returned handler injects the HTTP request into the context via
// WithHTTPRequest so that AppNameFunc and UserIDFunc can access it.
func Handler(cfg Config, agCfg agui.Config) (http.Handler, error) {
	// Pre-populate ToolResultHandler for inline mode so both agui.Handler
	// and the /tool-result mux share the same instance.
	if agCfg.ToolMode == agui.ToolModeInline && agCfg.ToolResultHandler == nil {
		agCfg.ToolResultHandler = agui.NewToolResultHandler()
	}

	// Wire ClientTools inline mode: share the ToolResultHandler between
	// the bridge and the agui handler so /tool-result submissions reach
	// the waiting tool handlers.
	if cfg.ClientTools != nil && cfg.ClientTools.Mode == ClientToolModeInline {
		if cfg.ClientTools.ResultHandler == nil {
			if agCfg.ToolResultHandler != nil {
				cfg.ClientTools.ResultHandler = agCfg.ToolResultHandler
			} else {
				cfg.ClientTools.ResultHandler = agui.NewToolResultHandler()
				agCfg.ToolResultHandler = cfg.ClientTools.ResultHandler
			}
		}
	}

	bridge, err := New(cfg)
	if err != nil {
		return nil, err
	}
	agCfg.Agent = bridge

	inner, err := agui.Handler(agCfg)
	if err != nil {
		return nil, err
	}

	wrap := func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(WithHTTPRequest(r.Context(), r))
		inner.ServeHTTP(w, r)
	}

	needToolResultEndpoint := agCfg.ToolMode == agui.ToolModeInline ||
		(cfg.ClientTools != nil && cfg.ClientTools.Mode == ClientToolModeInline)
	if needToolResultEndpoint {
		mux := http.NewServeMux()
		mux.HandleFunc("POST /", wrap)
		mux.Handle("POST /tool-result", agui.ToolResultEndpoint(agCfg.ToolResultHandler))
		return mux, nil
	}

	return http.HandlerFunc(wrap), nil
}
