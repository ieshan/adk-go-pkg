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
	if caps.Tools == nil || !caps.Tools.Supported {
		t.Errorf("Tools = %+v, want Supported=true", caps.Tools)
	}
	if caps.State == nil || !caps.State.Snapshots || !caps.State.Deltas {
		t.Errorf("State = %+v, want snapshots+deltas", caps.State)
	}
	// Custom carries MESSAGES_SNAPSHOT, which has no 1.0 schema category.
	if caps.Custom == nil {
		t.Fatal("Custom = nil, want messagesSnapshot entry")
	}
	if _, ok := caps.Custom["messagesSnapshot"]; !ok {
		t.Errorf("Custom = %+v, want messagesSnapshot key", caps.Custom)
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
	if caps.MultiAgent == nil || !caps.MultiAgent.Supported || !caps.MultiAgent.Delegation {
		t.Fatalf("MultiAgent = %+v, want supported + delegation", caps.MultiAgent)
	}
	if len(caps.MultiAgent.Subagents) != 1 || caps.MultiAgent.Subagents[0].Name != "child" {
		t.Errorf("Subagents = %+v, want one entry named child", caps.MultiAgent.Subagents)
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
	if caps.Tools == nil || !caps.Tools.ClientProvided {
		t.Errorf("Tools.ClientProvided = %v, want true", caps.Tools)
	}
	if caps.HumanInTheLoop == nil {
		t.Fatal("got nil HumanInTheLoop, want non-nil for NextRun mode")
	}
	if !caps.HumanInTheLoop.Interrupts || !caps.HumanInTheLoop.Approvals {
		t.Errorf("HITL = %+v, want interrupts+approvals for NextRun mode", caps.HumanInTheLoop)
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

func TestInferCapabilities_ProviderGating(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		wantImages bool
		wantAudio  bool
	}{
		{name: "unset provider", provider: "", wantImages: true, wantAudio: true},
		{name: "openai", provider: "openai", wantImages: true, wantAudio: true},
		{name: "other provider", provider: "gemini", wantImages: true, wantAudio: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, err := agent.New(agent.Config{
				Name: "provider-agent",
				Run:  func(agent.InvocationContext) iter.Seq2[*session.Event, error] { return nil },
			})
			if err != nil {
				t.Fatalf("agent.New: %v", err)
			}
			caps := aguiadk.InferCapabilities(a, aguiadk.Config{Agent: a, Provider: tc.provider})
			if caps.Multimodal == nil || caps.Multimodal.Input == nil {
				t.Fatal("got nil multimodal input, want non-nil")
			}
			if got := caps.Multimodal.Input.Image; got != tc.wantImages {
				t.Errorf("image = %v, want %v", got, tc.wantImages)
			}
			if got := caps.Multimodal.Input.Audio; got != tc.wantAudio {
				t.Errorf("audio = %v, want %v", got, tc.wantAudio)
			}
		})
	}
}

func TestInferCapabilities_ReasoningAndState(t *testing.T) {
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
	if caps.Reasoning == nil || !caps.Reasoning.Supported || !caps.Reasoning.Streaming || !caps.Reasoning.Encrypted {
		t.Errorf("Reasoning = %+v, want supported+streaming+encrypted", caps.Reasoning)
	}
}
