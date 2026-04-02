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
	bridge, err := New(cfg)
	if err != nil {
		return nil, err
	}
	agCfg.Agent = bridge
	inner, err := agui.Handler(agCfg)
	if err != nil {
		return nil, err
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(WithHTTPRequest(r.Context(), r))
		inner.ServeHTTP(w, r)
	}), nil
}
