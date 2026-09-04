package agui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"iter"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

func TestAgentCapabilities_JSONSerialization(t *testing.T) {
	caps := AgentCapabilities{
		Identity: &IdentityCapabilities{
			Name:        "ADK Assistant",
			Type:        "adk-go",
			Description: "ADK-Go Agent",
		},
		Transport: &TransportCapabilities{
			Streaming: true,
		},
		State: &StateCapabilities{
			Snapshots: true,
			Deltas:    true,
		},
		HumanInTheLoop: &HumanInTheLoopCapabilities{
			Interrupts: true,
		},
	}

	data, err := json.Marshal(caps)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed map[string]any
	_ = json.Unmarshal(data, &parsed)
	if parsed["identity"].(map[string]any)["type"] != "adk-go" {
		t.Errorf("identity mismatch: %s", string(data))
	}
}

func TestHandler_CapabilitiesDiscovery(t *testing.T) {
	caps := &AgentCapabilities{
		Identity: &IdentityCapabilities{Name: "test-agent", Type: "adk-go"},
	}
	handler, err := Handler(Config{
		Agent:        AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] { return nil }),
		Capabilities: caps,
	})
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/capabilities", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var got AgentCapabilities
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Identity == nil || got.Identity.Name != "test-agent" {
		t.Errorf("identity = %+v, want name 'test-agent'", got.Identity)
	}
}

func TestHandler_CapabilitiesDiscovery_NotConfigured(t *testing.T) {
	handler, err := Handler(Config{
		Agent: AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] { return nil }),
	})
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/capabilities", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}
