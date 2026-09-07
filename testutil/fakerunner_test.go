package testutil_test

import (
	"context"
	"testing"

	"google.golang.org/adk/v2/artifact"
	"google.golang.org/adk/v2/memory"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"

	"github.com/ieshan/adk-go-pkg/testutil"
)

func TestRunnerBuilder_NoAgent(t *testing.T) {
	_, err := testutil.NewRunnerBuilder().Build()
	if err == nil {
		t.Error("Build() without agent should return error")
	}
}

func TestRunnerBuilder_BuildWithFakes(t *testing.T) {
	ag := testutil.MustNewFakeAgent("test-agent")
	r, fakes, err := testutil.NewRunnerBuilder().WithAgent(ag).BuildWithFakes()
	if err != nil {
		t.Fatalf("BuildWithFakes() error = %v", err)
	}
	if r == nil {
		t.Error("runner should not be nil")
	}
	if fakes.SessionService == nil {
		t.Error("fakes.SessionService should not be nil")
	}
	if fakes.ArtifactService == nil {
		t.Error("fakes.ArtifactService should not be nil")
	}
	if fakes.MemoryService == nil {
		t.Error("fakes.MemoryService should not be nil")
	}
}

func TestRunnerBuilder_CustomServices(t *testing.T) {
	ag := testutil.MustNewFakeAgent("test-agent")
	customSession := testutil.NewFakeSessionService()
	customArtifact := testutil.NewFakeArtifactService()
	customMemory := testutil.NewFakeMemoryService()

	r, fakes, err := testutil.NewRunnerBuilder().
		WithAgent(ag).
		WithSessionService(customSession).
		WithArtifactService(customArtifact).
		WithMemoryService(customMemory).
		BuildWithFakes()
	if err != nil {
		t.Fatalf("BuildWithFakes() error = %v", err)
	}
	_ = r

	// Custom fakes should be accessible.
	if fakes.SessionService != customSession {
		t.Error("fakes.SessionService should be the custom instance")
	}
	if fakes.ArtifactService != customArtifact {
		t.Error("fakes.ArtifactService should be the custom instance")
	}
	if fakes.MemoryService != customMemory {
		t.Error("fakes.MemoryService should be the custom instance")
	}
}

func TestRunnerBuilder_NonFakeServices(t *testing.T) {
	ag := testutil.MustNewFakeAgent("test-agent")

	// Use a non-FakeSessionService (the real in-memory one).
	realSessionSvc := session.InMemoryService()
	r, fakes, err := testutil.NewRunnerBuilder().
		WithAgent(ag).
		WithSessionService(realSessionSvc).
		BuildWithFakes()
	if err != nil {
		t.Fatalf("BuildWithFakes() error = %v", err)
	}
	_ = r
	if fakes.SessionService != nil {
		t.Error("fakes.SessionService should be nil for non-fake service")
	}
}

func TestRunnerBuilder_WithAppName(t *testing.T) {
	ag := testutil.MustNewFakeAgent("test-agent")
	r, _, err := testutil.NewRunnerBuilder().
		WithAppName("custom-app").
		WithAgent(ag).
		BuildWithFakes()
	if err != nil {
		t.Fatalf("BuildWithFakes() error = %v", err)
	}
	_ = r
}

func TestRunnerBuilder_BuildWithFakesError(t *testing.T) {
	_, _, err := testutil.NewRunnerBuilder().BuildWithFakes()
	if err == nil {
		t.Error("BuildWithFakes() without agent should return error")
	}
}

func TestFakeArtifacts_Delegation(t *testing.T) {
	svc := testutil.NewFakeArtifactService()
	ctx := context.Background()

	// Preload via service directly.
	svc.PreloadArtifact("app", "user", "sess", "test.txt", &genai.Part{Text: "data"})

	art := testutil.NewFakeArtifacts(svc, "app", "user", "sess")
	resp, err := art.Load(ctx, "test.txt")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if resp.Part.Text != "data" {
		t.Errorf("Load() text = %q, want %q", resp.Part.Text, "data")
	}

	// Save via Artifacts interface.
	saveResp, err := art.Save(ctx, "new.txt", &genai.Part{Text: "new"})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if saveResp.Version != 1 {
		t.Errorf("Save() version = %d, want 1", saveResp.Version)
	}

	// List
	listResp, err := art.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listResp.FileNames) != 2 {
		t.Errorf("List() count = %d, want 2", len(listResp.FileNames))
	}

	// LoadVersion
	lvResp, err := art.LoadVersion(ctx, "test.txt", 1)
	if err != nil {
		t.Fatalf("LoadVersion() error = %v", err)
	}
	if lvResp.Part.Text != "data" {
		t.Errorf("LoadVersion() text = %q, want %q", lvResp.Part.Text, "data")
	}
}

func TestFakeMemory_Delegation(t *testing.T) {
	svc := testutil.NewFakeMemoryService()
	svc.PreloadMemory("user1", "app1", testutil.NewMemoryEntry("e1", "hello", "model"))

	mem := testutil.NewFakeMemory(svc, "user1", "app1")
	resp, err := mem.SearchMemory(context.Background(), "hello")
	if err != nil {
		t.Fatalf("SearchMemory() error = %v", err)
	}
	if len(resp.Memories) != 1 {
		t.Errorf("SearchMemory() count = %d, want 1", len(resp.Memories))
	}

	// AddSessionToMemory
	sess := testutil.NewFakeSession().WithAppName("app1").WithUserID("user1")
	err = mem.AddSessionToMemory(context.Background(), sess)
	if err != nil {
		t.Fatalf("AddSessionToMemory() error = %v", err)
	}
}

// Verify interface compliance at compile time for non-fake services.
var _ session.Service = session.InMemoryService()
var _ artifact.Service = artifact.InMemoryService()
var _ memory.Service = memory.InMemoryService()
