package agui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"
	"time"

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
	cfg        ClientConfig
	httpClient *http.Client
}

// NewClientAgent creates a ClientAgent for the configured remote AG-UI
// endpoint. The connection is established in Run.
func NewClientAgent(cfg ClientConfig) *ClientAgent {
	return &ClientAgent{cfg: cfg, httpClient: newClientHTTPClient()}
}

// newClientHTTPClient builds the HTTP client used to stream SSE frames.
//
// The transport is cloned from http.DefaultTransport with two settings
// preserved from the old SDK client's transport: DisableCompression (the
// default transport advertises gzip, and a compressing intermediary can
// buffer/delay SSE frames) and a 30s ResponseHeaderTimeout (without it a
// black-holed endpoint hangs until ctx death). Client.Timeout is left at 0 —
// cancellation is ctx-driven.
func newClientHTTPClient() *http.Client {
	transport := http.DefaultTransport
	if t, ok := transport.(*http.Transport); ok {
		cloned := t.Clone()
		cloned.DisableCompression = true
		cloned.ResponseHeaderTimeout = 30 * time.Second
		transport = cloned
	}
	return &http.Client{Transport: transport}
}

// versionedInput wraps RunAgentInput to declare the client's protocol
// version on the wire — the negotiation pair to the producer's declaration
// on RUN_STARTED.
type versionedInput struct {
	types.RunAgentInput
}

// MarshalJSON serializes the input and injects protocolVersion.
func (v versionedInput) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(v.RunAgentInput)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err = json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	vb, err := json.Marshal(ProtocolVersion)
	if err != nil {
		return nil, err
	}
	m["protocolVersion"] = vb
	return json.Marshal(m)
}

// Run implements agui.Agent. It POSTs the RunAgentInput to the remote endpoint,
// streams SSE frames, decodes them into typed AG-UI events, and yields them.
// Context cancellation propagates to the HTTP request and stops the stream
// cleanly.
func (c *ClientAgent) Run(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] {
	return func(yield func(events.Event, error) bool) {
		body, err := json.Marshal(versionedInput{input})
		if err != nil {
			yield(nil, fmt.Errorf("agui client: marshal input: %w", err))
			return
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(body))
		if err != nil {
			yield(nil, fmt.Errorf("agui client: create request: %w", err))
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		if c.cfg.APIKey != "" {
			// Match the SDK client's semantics exactly: the scheme applies
			// only to the Authorization header; a custom AuthHeader (e.g.
			// X-API-Key) carries the raw key with no scheme prefix.
			header := c.cfg.AuthHeader
			if header == "" {
				header = "Authorization"
			}
			if header == "Authorization" {
				scheme := c.cfg.AuthScheme
				if scheme == "" {
					scheme = "Bearer"
				}
				req.Header.Set(header, scheme+" "+c.cfg.APIKey)
			} else {
				req.Header.Set(header, c.cfg.APIKey)
			}
		}
		for k, v := range c.cfg.Headers {
			req.Header.Set(k, v)
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			yield(nil, fmt.Errorf("agui client: request failed: %w", err))
			return
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
			yield(nil, fmt.Errorf("agui client: unexpected status %d: %s", resp.StatusCode, string(b)))
			return
		}
		// Preserve the SDK's content-type check: a 200 that is not an SSE
		// stream (e.g. a proxy error page) must not be fed to the frame
		// reader, where it would end silently or decode as garbage.
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
			yield(nil, fmt.Errorf("agui client: unexpected content-type: %s", ct))
			return
		}
		for data, serr := range readSSEData(resp.Body) {
			if serr != nil {
				yield(nil, serr)
				return
			}
			ev, derr := decodeFrame(data)
			if derr != nil {
				yield(nil, fmt.Errorf("agui client: decode error: %w", derr))
				return
			}
			if ev != nil {
				if !yield(ev, nil) {
					return
				}
			}
		}
	}
}

// readSSEData yields the accumulated data payload of each SSE event block.
// Per the SSE spec, consecutive data: lines within one block concatenate
// with "\n"; comment lines (leading ':') and event:/id:/retry: fields are
// ignored — decodeFrame only consumes the data payload. A scanner failure
// (truncated stream, over-cap frame) is yielded as a terminal error so a
// truncated run is never mistaken for a clean end-of-stream. A cancelled
// request context aborts the underlying read and surfaces as that error.
func readSSEData(r io.Reader) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64<<10), 4<<20) // 4 MiB frame cap
		var data []byte
		for sc.Scan() {
			line := sc.Bytes()
			if len(line) == 0 { // blank line: end of event block
				if len(data) > 0 {
					if !yield(data, nil) {
						return
					}
					data = nil
				}
				continue
			}
			if line[0] == ':' || !bytes.HasPrefix(line, []byte("data:")) {
				continue
			}
			payload := line[len("data:"):]
			if len(payload) > 0 && payload[0] == ' ' {
				payload = payload[1:] // strip single leading space per spec
			}
			if len(data) > 0 {
				data = append(data, '\n')
			}
			data = append(data, payload...)
		}
		if err := sc.Err(); err != nil {
			yield(nil, fmt.Errorf("agui client: read stream: %w", err))
			return
		}
		if len(data) > 0 { // trailing block without a blank line
			yield(data, nil)
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
