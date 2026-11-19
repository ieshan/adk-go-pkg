package agui

import "context"

// clientProtocolVersionKey is the context key for the protocol version the
// client declared on RunAgentInput.
type clientProtocolVersionKey struct{}

// WithClientProtocolVersion returns a context carrying the client's declared
// AG-UI protocol version. Called by Handler; consumers read it via
// ClientProtocolVersion.
func WithClientProtocolVersion(ctx context.Context, version string) context.Context {
	return context.WithValue(ctx, clientProtocolVersionKey{}, version)
}

// ClientProtocolVersion returns the protocol version the client declared on
// RunAgentInput, or ("", false) when absent (a pre-1.0 client).
func ClientProtocolVersion(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(clientProtocolVersionKey{}).(string)
	return v, ok && v != ""
}
