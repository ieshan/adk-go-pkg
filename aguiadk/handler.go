package aguiadk

import (
	"net/http"
	"strings"

	"github.com/ieshan/adk-go-pkg/agui"
)

// Handler creates a complete AG-UI HTTP handler for an ADK-Go agent.
// It combines New (which bridges ADK to AG-UI) with agui.Handler
// (which serves the SSE endpoint), providing a single-call setup.
//
// The returned handler injects the HTTP request into the context via
// WithHTTPRequest so that AppNameFunc and UserIDFunc can access it.
//
// When inline tool mode is enabled (either agui.ToolModeInline or
// ClientToolModeInline), the handler also serves the /tool-result endpoint.
// The tool-result route matches any path ending in "/tool-result", so the
// handler works correctly when mounted at a sub-path (e.g. /api/agent/).
func Handler(cfg Config, agCfg agui.Config) (http.Handler, error) {
	// Pre-populate ToolResultHandler for inline mode so both agui.Handler
	// and the /tool-result route share the same instance.
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

	needToolResultEndpoint := agCfg.ToolMode == agui.ToolModeInline ||
		(cfg.ClientTools != nil && cfg.ClientTools.Mode == ClientToolModeInline)
	toolResultHandler := agCfg.ToolResultHandler

	wrap := func(w http.ResponseWriter, r *http.Request) {
		// Route /tool-result requests to the tool-result endpoint. Matching
		// by suffix (rather than exact path) allows the handler to be mounted
		// at any sub-path (e.g. /api/agent/) without breaking the endpoint.
		if needToolResultEndpoint && r.Method == http.MethodPost &&
			strings.HasSuffix(r.URL.Path, "/tool-result") {
			agui.ToolResultEndpoint(toolResultHandler).ServeHTTP(w, r)
			return
		}
		r = r.WithContext(WithHTTPRequest(r.Context(), r))
		inner.ServeHTTP(w, r)
	}

	return http.HandlerFunc(wrap), nil
}
