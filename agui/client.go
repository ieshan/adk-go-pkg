package agui

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/client/sse"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

// ClientConfig configures a ClientAgent that connects to a remote AG-UI
// endpoint over HTTP/SSE.
type ClientConfig struct {
	// Endpoint is the remote AG-UI SSE URL (required).
	Endpoint string

	// APIKey is an optional bearer token sent in the Authorization header.
	APIKey string

	// AuthHeader overrides the default "Authorization" header name.
	AuthHeader string

	// AuthScheme overrides the default "Bearer" scheme.
	AuthScheme string

	// Headers are additional headers sent with each request.
	Headers map[string]string
}

// ClientAgent is an agui.Agent implementation that streams events from a
// remote AG-UI endpoint. It implements the client side of the AG-UI protocol,
// decoding SSE frames into typed events and propagating context cancellation
// to release HTTP reader goroutines.
type ClientAgent struct {
	cfg    ClientConfig
	client *sse.Client
}

// NewClientAgent creates a ClientAgent for the configured remote AG-UI
// endpoint. The connection is established in Run.
func NewClientAgent(cfg ClientConfig) *ClientAgent {
	sseCfg := sse.Config{
		Endpoint:   cfg.Endpoint,
		APIKey:     cfg.APIKey,
		AuthHeader: cfg.AuthHeader,
		AuthScheme: cfg.AuthScheme,
	}
	c := &ClientAgent{cfg: cfg, client: sse.NewClient(sseCfg)}
	return c
}

// Run implements agui.Agent. It POSTs the RunAgentInput to the remote endpoint,
// streams SSE frames, decodes them into typed AG-UI events, and yields them.
// Context cancellation propagates to the HTTP reader goroutine and stops the
// stream cleanly.
func (c *ClientAgent) Run(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
	return func(yield func(events.Event, error) bool) {
		streamCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		frames, errCh, err := c.client.Stream(sse.StreamOptions{
			Context: streamCtx,
			Payload: input,
			Headers: c.cfg.Headers,
		})
		if err != nil {
			yield(nil, fmt.Errorf("agui client: failed to start stream: %w", err))
			return
		}

		for {
			// Check context first so cancellation takes priority.
			select {
			case <-streamCtx.Done():
				yield(nil, streamCtx.Err())
				return
			default:
			}

			select {
			case frame, ok := <-frames:
				if !ok {
					// Frames channel closed; drain any final error.
					if streamErr, hasErr := <-errCh; hasErr && streamErr != nil {
						yield(nil, fmt.Errorf("agui client: stream error: %w", streamErr))
					}
					return
				}
				ev, decodeErr := decodeFrame(frame.Data)
				if decodeErr != nil {
					yield(nil, fmt.Errorf("agui client: decode error: %w", decodeErr))
					return
				}
				if ev != nil {
					if !yield(ev, nil) {
						return
					}
				}
			case streamErr, ok := <-errCh:
				if !ok {
					// errors channel closed before frames; drain remaining
					// buffered frames before returning.
					for frame := range frames {
						ev, decodeErr := decodeFrame(frame.Data)
						if decodeErr != nil {
							yield(nil, fmt.Errorf("agui client: decode error: %w", decodeErr))
							return
						}
						if ev != nil {
							if !yield(ev, nil) {
								return
							}
						}
					}
					return
				}
				if streamErr != nil {
					yield(nil, fmt.Errorf("agui client: stream error: %w", streamErr))
					return
				}
				// nil error received but channel still open; continue draining frames.
			case <-streamCtx.Done():
				yield(nil, streamCtx.Err())
				return
			}
		}
	}
}

// decodeFrame parses a raw SSE data frame into a typed AG-UI event. It tries
// EventFromJSON for canonical event types, then falls back to a Raw event for
// unknown types.
func decodeFrame(data []byte) (events.Event, error) {
	if len(data) == 0 {
		return nil, nil
	}
	ev, err := events.EventFromJSON(data)
	if err == nil {
		return ev, nil
	}
	// Last resort: return a Raw event wrapping the payload.
	var rawEvent struct {
		Type events.EventType `json:"type"`
	}
	if jsonErr := json.Unmarshal(data, &rawEvent); jsonErr != nil {
		return nil, fmt.Errorf("invalid JSON: %w", jsonErr)
	}
	source := string(rawEvent.Type)
	return events.NewRawEvent(json.RawMessage(data), events.WithSource(source)), nil
}

// Compile-time check that ClientAgent satisfies Agent.
var _ Agent = (*ClientAgent)(nil)
