package agui

import (
	"context"
	"net/http"
	"sync"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	agsse "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/encoding/sse"
)

// sseSession wraps an http.ResponseWriter and SSEWriter with a mutex,
// enabling concurrent writes from a keepalive goroutine and the event loop.
// It is request-scoped and not safe for reuse across requests.
type sseSession struct {
	mu        sync.Mutex
	w         http.ResponseWriter
	f         http.Flusher
	sseWriter *agsse.SSEWriter
}

func newSSESession(w http.ResponseWriter, f http.Flusher, sw *agsse.SSEWriter) *sseSession {
	return &sseSession{w: w, f: f, sseWriter: sw}
}

func (s *sseSession) WriteEvent(ctx context.Context, ev events.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.sseWriter.WriteEvent(ctx, s.w, ev); err != nil {
		return err
	}
	s.f.Flush()
	return nil
}

func (s *sseSession) WritePing() {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.w.Write([]byte(": ping\n\n"))
	s.f.Flush()
}
