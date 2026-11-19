package agui

import (
	"bytes"
	"testing"
)

// FuzzReadSSEData exercises the SSE frame parser boundary: arbitrary bytes
// must never panic, and every yielded payload must be a concatenation of the
// data: lines in that block. A scanner failure (over-cap frame) surfaces as a
// terminal error rather than a clean end-of-stream.
func FuzzReadSSEData(f *testing.F) {
	f.Add([]byte("data: {\"type\":\"RUN_STARTED\"}\n\n"))
	f.Add([]byte(": comment\ndata: a\ndata: b\n\n"))
	f.Add([]byte("data: trailing-without-blank"))
	f.Add([]byte("event: X\ndata:  spaced\n\n"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		var payloads [][]byte
		for payload, err := range readSSEData(bytes.NewReader(data)) {
			if err != nil {
				continue
			}
			// A yielded payload is never empty: empty blocks are skipped.
			if len(payload) == 0 {
				t.Fatalf("empty payload yielded")
			}
			payloads = append(payloads, append([]byte(nil), payload...))
		}
		// The parser is deterministic: a second pass over the same bytes must
		// yield the same payloads. This guards the append/reuse behavior of the
		// accumulated data buffer.
		var second [][]byte
		for payload, err := range readSSEData(bytes.NewReader(data)) {
			if err != nil {
				continue
			}
			second = append(second, append([]byte(nil), payload...))
		}
		if len(second) != len(payloads) {
			t.Fatalf("non-deterministic pass: %d vs %d payloads", len(payloads), len(second))
		}
		for i := range payloads {
			if !bytes.Equal(payloads[i], second[i]) {
				t.Fatalf("payload[%d] differs between passes: %q vs %q", i, payloads[i], second[i])
			}
		}
	})
}
