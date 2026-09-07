package aguiadk_test

import (
	"iter"
	"testing"

	"github.com/ieshan/adk-go-pkg/aguiadk"
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

	caps := aguiadk.InferCapabilities(a, aguiadk.Config{Agent: a})
	if caps == nil {
		t.Fatal("got nil capabilities, want non-nil")
	}
	if caps.Identity == nil || caps.Identity.Name != "my-agent" {
		t.Errorf("identity = %+v, want name 'my-agent'", caps.Identity)
	}
	if caps.Identity.Type != "adk-go" {
		t.Errorf("type = %q, want adk-go", caps.Identity.Type)
	}
	if caps.Transport == nil || !caps.Transport.Streaming {
		t.Errorf("Transport = %+v, want streaming", caps.Transport)
	}
	if caps.Tools == nil || !caps.Tools.ServerTools {
		t.Errorf("Tools = %+v, want ServerTools=true", caps.Tools)
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

	caps := aguiadk.InferCapabilities(parent, aguiadk.Config{Agent: parent})
	if caps == nil {
		t.Fatal("nil capabilities")
	}
	if caps.Activities == nil {
		t.Error("got nil Activities, want non-nil for agent with sub-agents")
	}
}

func TestInferCapabilities_NilAgent(t *testing.T) {
	if caps := aguiadk.InferCapabilities(nil, aguiadk.Config{}); caps != nil {
		t.Fatal("got non-nil capabilities, want nil for nil agent")
	}
}

func TestInferCapabilities_NextRunHITL(t *testing.T) {
	t.Parallel()
	a, err := agent.New(agent.Config{
		Name:        "hitl-agent",
		Description: "A HITL test agent",
		Run:         func(agent.InvocationContext) iter.Seq2[*session.Event, error] { return nil },
	})
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}

	caps := aguiadk.InferCapabilities(a, aguiadk.Config{
		Agent:       a,
		ClientTools: &aguiadk.ClientToolConfig{Mode: aguiadk.ClientToolModeNextRun},
	})
	if caps == nil {
		t.Fatal("got nil capabilities, want non-nil")
	}
	if caps.HumanInTheLoop == nil {
		t.Fatal("got nil HumanInTheLoop, want non-nil for NextRun mode")
	}
	if !caps.HumanInTheLoop.Interrupts {
		t.Errorf("Interrupts = %v, want true for NextRun mode", caps.HumanInTheLoop.Interrupts)
	}
}

func TestInferCapabilities_HandBackNoHITL(t *testing.T) {
	t.Parallel()
	a, err := agent.New(agent.Config{
		Name:        "handback-agent",
		Description: "A handback test agent",
		Run:         func(agent.InvocationContext) iter.Seq2[*session.Event, error] { return nil },
	})
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}

	caps := aguiadk.InferCapabilities(a, aguiadk.Config{
		Agent:       a,
		ClientTools: &aguiadk.ClientToolConfig{Mode: aguiadk.ClientToolModeHandBack},
	})
	if caps == nil {
		t.Fatal("got nil capabilities, want non-nil")
	}
	if caps.HumanInTheLoop != nil {
		t.Errorf("got %+v, want nil HumanInTheLoop for HandBack mode", caps.HumanInTheLoop)
	}
}

func TestInferCapabilities_StreamingAndEncrypted(t *testing.T) {
	t.Parallel()
	a, err := agent.New(agent.Config{
		Name:        "streaming-agent",
		Description: "A streaming test agent",
		Run:         func(agent.InvocationContext) iter.Seq2[*session.Event, error] { return nil },
	})
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}

	caps := aguiadk.InferCapabilities(a, aguiadk.Config{Agent: a})
	if caps == nil {
		t.Fatal("got nil capabilities, want non-nil")
	}
	if caps.Tools == nil || !caps.Tools.Streaming {
		t.Errorf("Tools.Streaming = %v, want true", caps.Tools.Streaming)
	}
	if caps.Reasoning == nil || !caps.Reasoning.Encrypted {
		t.Errorf("Reasoning.Encrypted = %v, want true", caps.Reasoning.Encrypted)
	}
}
