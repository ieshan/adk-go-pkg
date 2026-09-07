package testutil_test

import (
	"context"
	"testing"

	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/adk/v2/artifact"
	"google.golang.org/genai"
)

func TestFakeArtifactService_SaveAndLoad(t *testing.T) {
	svc := testutil.NewFakeArtifactService()

	// Save
	resp, err := svc.Save(context.Background(), &artifact.SaveRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "sess",
		FileName:  "test.txt",
		Part:      &genai.Part{Text: "hello"},
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if resp.Version != 1 {
		t.Errorf("Save() version = %d, want 1", resp.Version)
	}

	// Load latest
	loadResp, err := svc.Load(context.Background(), &artifact.LoadRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "sess",
		FileName:  "test.txt",
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loadResp.Part.Text != "hello" {
		t.Errorf("Load() text = %q, want %q", loadResp.Part.Text, "hello")
	}
}

func TestFakeArtifactService_Versioning(t *testing.T) {
	svc := testutil.NewFakeArtifactService()
	ctx := context.Background()

	// Save v1
	resp1, err := svc.Save(ctx, &artifact.SaveRequest{
		AppName: "app", UserID: "user", SessionID: "sess",
		FileName: "file.txt", Part: &genai.Part{Text: "v1"},
	})
	if err != nil {
		t.Fatalf("Save v1: %v", err)
	}
	if resp1.Version != 1 {
		t.Errorf("v1 = %d, want 1", resp1.Version)
	}

	// Save v2
	resp2, err := svc.Save(ctx, &artifact.SaveRequest{
		AppName: "app", UserID: "user", SessionID: "sess",
		FileName: "file.txt", Part: &genai.Part{Text: "v2"},
	})
	if err != nil {
		t.Fatalf("Save v2: %v", err)
	}
	if resp2.Version != 2 {
		t.Errorf("v2 = %d, want 2", resp2.Version)
	}

	// Load specific version
	loadResp, err := svc.Load(ctx, &artifact.LoadRequest{
		AppName: "app", UserID: "user", SessionID: "sess",
		FileName: "file.txt", Version: 1,
	})
	if err != nil {
		t.Fatalf("Load(v1) error = %v", err)
	}
	if loadResp.Part.Text != "v1" {
		t.Errorf("Load(v1) text = %q, want %q", loadResp.Part.Text, "v1")
	}

	// Load latest (v2)
	loadResp2, err := svc.Load(ctx, &artifact.LoadRequest{
		AppName: "app", UserID: "user", SessionID: "sess",
		FileName: "file.txt",
	})
	if err != nil {
		t.Fatalf("Load latest: %v", err)
	}
	if loadResp2.Part.Text != "v2" {
		t.Errorf("Load(latest) text = %q, want %q", loadResp2.Part.Text, "v2")
	}

	// Versions
	verResp, err := svc.Versions(ctx, &artifact.VersionsRequest{
		AppName: "app", UserID: "user", SessionID: "sess",
		FileName: "file.txt",
	})
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(verResp.Versions) != 2 {
		t.Errorf("Versions() count = %d, want 2", len(verResp.Versions))
	}
}

func TestFakeArtifactService_List(t *testing.T) {
	svc := testutil.NewFakeArtifactService()
	ctx := context.Background()

	if _, err := svc.Save(ctx, &artifact.SaveRequest{
		AppName: "app", UserID: "user", SessionID: "sess",
		FileName: "a.txt", Part: &genai.Part{Text: "a"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(ctx, &artifact.SaveRequest{
		AppName: "app", UserID: "user", SessionID: "sess",
		FileName: "b.txt", Part: &genai.Part{Text: "b"},
	}); err != nil {
		t.Fatal(err)
	}

	listResp, err := svc.List(ctx, &artifact.ListRequest{
		AppName: "app", UserID: "user", SessionID: "sess",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listResp.FileNames) != 2 {
		t.Errorf("List() count = %d, want 2", len(listResp.FileNames))
	}
	// Should be sorted.
	if listResp.FileNames[0] != "a.txt" || listResp.FileNames[1] != "b.txt" {
		t.Errorf("List() = %v, want [a.txt b.txt]", listResp.FileNames)
	}
}

func TestFakeArtifactService_Delete(t *testing.T) {
	svc := testutil.NewFakeArtifactService()
	ctx := context.Background()

	if _, err := svc.Save(ctx, &artifact.SaveRequest{
		AppName: "app", UserID: "user", SessionID: "sess",
		FileName: "del.txt", Part: &genai.Part{Text: "del"},
	}); err != nil {
		t.Fatal(err)
	}

	err := svc.Delete(ctx, &artifact.DeleteRequest{
		AppName: "app", UserID: "user", SessionID: "sess",
		FileName: "del.txt",
	})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = svc.Load(ctx, &artifact.LoadRequest{
		AppName: "app", UserID: "user", SessionID: "sess",
		FileName: "del.txt",
	})
	if err == nil {
		t.Error("Load() after Delete should fail")
	}
}

func TestFakeArtifactService_Preload(t *testing.T) {
	svc := testutil.NewFakeArtifactService()
	svc.PreloadArtifact("app", "user", "sess", "pre.txt", &genai.Part{Text: "preloaded"})

	loadResp, err := svc.Load(context.Background(), &artifact.LoadRequest{
		AppName: "app", UserID: "user", SessionID: "sess",
		FileName: "pre.txt",
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loadResp.Part.Text != "preloaded" {
		t.Errorf("Load() text = %q, want %q", loadResp.Part.Text, "preloaded")
	}
}

func TestFakeArtifactService_CallTracking(t *testing.T) {
	svc := testutil.NewFakeArtifactService()
	ctx := context.Background()

	if _, err := svc.Save(ctx, &artifact.SaveRequest{
		AppName: "app", UserID: "user", SessionID: "sess",
		FileName: "f.txt", Part: &genai.Part{Text: "x"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Load(ctx, &artifact.LoadRequest{
		AppName: "app", UserID: "user", SessionID: "sess",
		FileName: "f.txt",
	}); err != nil {
		t.Fatal(err)
	}

	if svc.SaveCount() != 1 {
		t.Errorf("SaveCount() = %d, want 1", svc.SaveCount())
	}
	if svc.LoadCount() != 1 {
		t.Errorf("LoadCount() = %d, want 1", svc.LoadCount())
	}
	lastSave := svc.LastSave()
	if lastSave == nil {
		t.Fatal("nil response")
	}
	if lastSave.FileName != "f.txt" {
		t.Errorf("LastSave().FileName = %q, want %q", lastSave.FileName, "f.txt")
	}
}

func TestFakeArtifactService_UserScoped(t *testing.T) {
	svc := testutil.NewFakeArtifactService()
	ctx := context.Background()

	// Save a user-scoped artifact (filename starts with "user:")
	resp, err := svc.Save(ctx, &artifact.SaveRequest{
		AppName: "app", UserID: "user", SessionID: "sess",
		FileName: "user:profile.txt", Part: &genai.Part{Text: "profile"},
	})
	if err != nil {
		t.Fatalf("Save() user-scoped error = %v", err)
	}
	if resp.Version != 1 {
		t.Errorf("Save() user-scoped version = %d, want 1", resp.Version)
	}

	// Load it back
	loadResp, err := svc.Load(ctx, &artifact.LoadRequest{
		AppName: "app", UserID: "user", SessionID: "sess",
		FileName: "user:profile.txt",
	})
	if err != nil {
		t.Fatalf("Load() user-scoped error = %v", err)
	}
	if loadResp.Part.Text != "profile" {
		t.Errorf("Load() user-scoped text = %q, want %q", loadResp.Part.Text, "profile")
	}
}

func TestPreloadUserScopedArtifact(t *testing.T) {
	t.Parallel()
	svc := testutil.NewFakeArtifactService()
	svc.PreloadArtifact("app", "alice", "sess1", "user:report.txt", &genai.Part{Text: "hello"})

	loadResp, err := svc.Load(context.Background(), &artifact.LoadRequest{
		AppName:   "app",
		UserID:    "alice",
		SessionID: "sess1",
		FileName:  "user:report.txt",
	})
	if err != nil {
		t.Fatalf("Load() user-scoped error = %v", err)
	}
	if loadResp.Part.Text != "hello" {
		t.Errorf("Load() user-scoped text = %q, want %q", loadResp.Part.Text, "hello")
	}
}

func TestFakeArtifactService_NotFound(t *testing.T) {
	svc := testutil.NewFakeArtifactService()
	_, err := svc.Load(context.Background(), &artifact.LoadRequest{
		AppName: "app", UserID: "user", SessionID: "sess",
		FileName: "missing.txt",
	})
	if err == nil {
		t.Error("Load() missing file should return error")
	}
}
