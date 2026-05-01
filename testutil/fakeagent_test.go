package testutil

import (
	"context"
	"iter"
	"testing"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

func TestFakeAgent_Name(t *testing.T) {
	f := NewFakeAgent("my-agent")
	if f.Name() != "my-agent" {
		t.Errorf("Name() = %q, want %q", f.Name(), "my-agent")
	}
}

func TestFakeAgent_SubAgents(t *testing.T) {
	child := NewFakeAgent("child")
	parent := NewFakeAgent("parent").WithSubAgents(child)

	subs := parent.SubAgents()
	if len(subs) != 1 || subs[0].Name() != "child" {
		t.Errorf("SubAgents() = %v, want [child]", subs)
	}

	found := parent.FindSubAgent("child")
	if found == nil || found.Name() != "child" {
		t.Error("FindSubAgent(child) should find child")
	}

	notFound := parent.FindSubAgent("nonexistent")
	if notFound != nil {
		t.Error("FindSubAgent(nonexistent) should return nil")
	}
}

func TestFakeAgent_RunTracking(t *testing.T) {
	f := NewFakeAgent("tracker")
	ic := NewFakeInvocationContext()

	events, err := CollectEvents(f.Run(ic))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	// Default run yields no events.
	if len(events) != 0 {
		t.Errorf("default Run yielded %d events, want 0", len(events))
	}
	_ = events

	if f.CallCount() != 1 {
		t.Errorf("CallCount() = %d, want 1", f.CallCount())
	}
}

func TestFakeAgent_WithRunFunc(t *testing.T) {
	f := NewFakeAgent("custom").
		WithRunFunc(func(ic agent.InvocationContext) iter.Seq2[*session.Event, error] {
			return func(yield func(*session.Event, error) bool) {
				yield(NewTextEvent("custom", "hello"), nil)
			}
		})

	ic := NewFakeInvocationContext()
	events, err := CollectEvents(f.Run(ic))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Author != "custom" {
		t.Errorf("event author = %q, want %q", events[0].Author, "custom")
	}
}

func TestFakeAgent_Reset(t *testing.T) {
	f := NewFakeAgent("reset")
	ic := NewFakeInvocationContext()
	_, _ = CollectEvents(f.Run(ic))
	_, _ = CollectEvents(f.Run(ic))

	if f.CallCount() != 2 {
		t.Errorf("CallCount() = %d, want 2", f.CallCount())
	}

	f.Reset()
	if f.CallCount() != 0 {
		t.Errorf("after Reset, CallCount() = %d, want 0", f.CallCount())
	}
}

func TestFakeInvocationContext(t *testing.T) {
	sess := NewFakeSession().WithID("s-42")
	ic := NewFakeInvocationContext().
		WithSession(sess).
		WithInvocationID("inv-1").
		WithBranch("parent.child").
		WithUserContent(genai.NewContentFromText("hi", genai.RoleUser))

	if ic.Session().ID() != "s-42" {
		t.Errorf("Session().ID() = %q, want %q", ic.Session().ID(), "s-42")
	}
	if ic.InvocationID() != "inv-1" {
		t.Errorf("InvocationID() = %q, want %q", ic.InvocationID(), "inv-1")
	}
	if ic.Branch() != "parent.child" {
		t.Errorf("Branch() = %q, want %q", ic.Branch(), "parent.child")
	}
	if ic.UserContent().Parts[0].Text != "hi" {
		t.Errorf("UserContent() text = %q, want %q", ic.UserContent().Parts[0].Text, "hi")
	}

	// EndInvocation
	if ic.Ended() {
		t.Error("Ended() should be false initially")
	}
	ic.EndInvocation()
	if !ic.Ended() {
		t.Error("Ended() should be true after EndInvocation")
	}

	// WithContext
	newCtx := context.Background()
	ic2 := ic.WithContext(newCtx)
	if ic2 == nil {
		t.Error("WithContext() returned nil")
	}
}

func TestFakeCallbackContext(t *testing.T) {
	cb := NewFakeCallbackContext().
		WithAgentName("my-agent").
		WithUserID("u-1").
		WithAppName("app-1").
		WithSessionID("s-1")

	if cb.AgentName() != "my-agent" {
		t.Errorf("AgentName() = %q, want %q", cb.AgentName(), "my-agent")
	}
	if cb.UserID() != "u-1" {
		t.Errorf("UserID() = %q, want %q", cb.UserID(), "u-1")
	}
	if cb.AppName() != "app-1" {
		t.Errorf("AppName() = %q, want %q", cb.AppName(), "app-1")
	}
	if cb.SessionID() != "s-1" {
		t.Errorf("SessionID() = %q, want %q", cb.SessionID(), "s-1")
	}

	// State
	cb.State().Set("key", "val")
	v, err := cb.State().Get("key")
	if err != nil || v != "val" {
		t.Errorf("State Get(key) = %v, %v; want val, nil", v, err)
	}
}

func TestFakeReadonlyContext(t *testing.T) {
	rc := NewFakeReadonlyContext().
		WithAgentName("ro-agent").
		WithUserID("u-2").
		WithAppName("app-2")

	if rc.AgentName() != "ro-agent" {
		t.Errorf("AgentName() = %q, want %q", rc.AgentName(), "ro-agent")
	}
	if rc.UserID() != "u-2" {
		t.Errorf("UserID() = %q, want %q", rc.UserID(), "u-2")
	}
}
