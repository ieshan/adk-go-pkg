package agui_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"iter"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

func TestAgentCapabilities_JSONSerialization(t *testing.T) {
	caps := agui.AgentCapabilities{
		Identity: &agui.IdentityCapabilities{
			Name:        "ADK Assistant",
			Type:        "adk-go",
			Description: "ADK-Go Agent",
		},
		Transport: &agui.TransportCapabilities{
			Streaming: true,
		},
		State: &agui.StateCapabilities{
			Snapshots: true,
			Deltas:    true,
		},
		HumanInTheLoop: &agui.HumanInTheLoopCapabilities{
			Interrupts: true,
		},
	}

	data, err := json.Marshal(caps)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["identity"].(map[string]any)["type"] != "adk-go" {
		t.Errorf("identity mismatch: %s", string(data))
	}
}

func TestHandler_CapabilitiesDiscovery(t *testing.T) {
	caps := &agui.AgentCapabilities{
		Identity: &agui.IdentityCapabilities{Name: "test-agent", Type: "adk-go"},
	}
	handler, err := agui.Handler(agui.Config{
		Agent:        agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] { return nil }),
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
	var got agui.AgentCapabilities
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Identity == nil || got.Identity.Name != "test-agent" {
		t.Errorf("identity = %+v, want name 'test-agent'", got.Identity)
	}
}

func TestHandler_CapabilitiesDiscovery_NotConfigured(t *testing.T) {
	handler, err := agui.Handler(agui.Config{
		Agent: agui.AgentFunc(func(ctx context.Context, input types.RunAgentInput) iter.Seq2[events.Event, error] { return nil }),
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
