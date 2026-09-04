package aguiadk

import (
	"iter"
	"testing"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/session"
)

func TestInferCapabilities_BasicAgent(t *testing.T) {
	a, err := agent.New(agent.Config{
		Name:        "my-agent",
		Description: "A test agent",
		Run:         func(agent.InvocationContext) iter.Seq2[*session.Event, error] { return nil },
	})
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}

	caps := InferCapabilities(a, Config{Agent: a})
	if caps == nil {
		t.Fatal("expected non-nil capabilities")
	}
	if caps.Identity == nil || caps.Identity.Name != "my-agent" {
		t.Errorf("identity = %+v, want name 'my-agent'", caps.Identity)
	}
	if caps.Identity.Type != "adk-go" {
		t.Errorf("type = %q, want adk-go", caps.Identity.Type)
	}
	if caps.Transport == nil || !caps.Transport.Streaming {
		t.Error("expected streaming transport")
	}
	if caps.Tools == nil || !caps.Tools.ServerTools {
		t.Error("expected server tools support")
	}
}

func TestInferCapabilities_WithSubAgents(t *testing.T) {
	child, err := agent.New(agent.Config{
		Name:        "child",
		Description: "child agent",
		Run:         func(agent.InvocationContext) iter.Seq2[*session.Event, error] { return nil },
	})
	if err != nil {
		t.Fatalf("agent.New child: %v", err)
	}
	parent, err := agent.New(agent.Config{
		Name:        "parent",
		Description: "parent agent",
		SubAgents:   []agent.Agent{child},
		Run:         func(agent.InvocationContext) iter.Seq2[*session.Event, error] { return nil },
	})
	if err != nil {
		t.Fatalf("agent.New parent: %v", err)
	}

	caps := InferCapabilities(parent, Config{Agent: parent})
	if caps == nil {
		t.Fatal("nil capabilities")
	}
	if caps.Activities == nil {
		t.Error("expected activity capabilities for agent with sub-agents")
	}
}

func TestInferCapabilities_NilAgent(t *testing.T) {
	if caps := InferCapabilities(nil, Config{}); caps != nil {
		t.Fatal("expected nil for nil agent")
	}
}
