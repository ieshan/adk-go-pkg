package agui_test

import (
	"context"
	"encoding/json"
	"iter"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/ieshan/adk-go-pkg/agui"
)

// schemaCapabilityKeys is the set of top-level AgentCapabilities keys defined
// by the AG-UI 1.0 schema. Hardcoded so any drift fails the test.
var schemaCapabilityKeys = []string{
	"identity", "transport", "tools", "output", "state", "multiAgent",
	"reasoning", "multimodal", "execution", "humanInTheLoop", "custom",
}

func TestAgentCapabilities_SchemaKeys(t *testing.T) {
	caps := agui.AgentCapabilities{
		Identity:   &agui.IdentityCapabilities{Name: "ADK Assistant", Type: "adk-go"},
		Transport:  &agui.TransportCapabilities{Streaming: true},
		Tools:      &agui.ToolsCapabilities{Supported: true, ClientProvided: true},
		State:      &agui.StateCapabilities{Snapshots: true, Deltas: true},
		MultiAgent: &agui.MultiAgentCapabilities{Supported: true, Subagents: []agui.SubagentInfo{{Name: "child"}}},
		Reasoning:  &agui.ReasoningCapabilities{Supported: true},
		Multimodal: &agui.MultimodalCapabilities{Input: &agui.MultimodalInputCapabilities{Image: true}},
		HumanInTheLoop: &agui.HumanInTheLoopCapabilities{
			Supported: true, Interrupts: true,
		},
		Custom: map[string]any{"messagesSnapshot": true},
	}

	data, err := json.Marshal(caps)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for key := range parsed {
		if !slices.Contains(schemaCapabilityKeys, key) {
			t.Errorf("top-level key %q is not in the 1.0 schema set %v", key, schemaCapabilityKeys)
		}
	}

	// Spot-check nested field names.
	tools := parsed["tools"].(map[string]any)
	if tools["clientProvided"] != true {
		t.Errorf("tools.clientProvided = %v, want true", tools["clientProvided"])
	}
	transport := parsed["transport"].(map[string]any)
	if _, ok := transport["httpBinary"]; ok {
		t.Errorf("false httpBinary should be omitted, got %v", transport["httpBinary"])
	}
	ma := parsed["multiAgent"].(map[string]any)
	subs, ok := ma["subagents"].([]any)
	if !ok || len(subs) != 1 {
		t.Fatalf("multiAgent.subagents = %v, want one entry", ma["subagents"])
	}
	if subs[0].(map[string]any)["name"] != "child" {
		t.Errorf("multiAgent.subagents[0].name = %v, want child", subs[0])
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
