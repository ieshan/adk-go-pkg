package agui_test

import (
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// drain collects all events from the channel (non-blocking after emitter is done).
func drain(ch chan events.Event) []events.Event {
	var out []events.Event
	for {
		select {
		case ev := <-ch:
			out = append(out, ev)
		default:
			return out
		}
	}
}
