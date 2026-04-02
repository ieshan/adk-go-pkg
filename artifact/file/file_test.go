package file_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/ieshan/adk-go-pkg/artifact/file"
	"google.golang.org/adk/artifact"
	"google.golang.org/genai"
)

// --- helpers ---

func newService(t *testing.T) artifact.Service {
	t.Helper()
	svc, err := file.New(file.Config{RootDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

const (
	testApp     = "myapp"
	testUser    = "user1"
	testSession = "session1"
)

func saveText(t *testing.T, svc artifact.Service, fileName, text string) *artifact.SaveResponse {
	t.Helper()
	resp, err := svc.Save(context.Background(), &artifact.SaveRequest{
		AppName:   testApp,
		UserID:    testUser,
		SessionID: testSession,
		FileName:  fileName,
		Part:      &genai.Part{Text: text},
	})
	if err != nil {
		t.Fatalf("Save(%q): %v", fileName, err)
	}
	return resp
}

// --- tests ---

// TestSave_NewArtifact verifies that saving a text artifact for the first time
// returns version 0 and writes a content file to disk.
func TestSave_NewArtifact(t *testing.T) {
	rootDir := t.TempDir()
	svc, err := file.New(file.Config{RootDir: rootDir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := svc.Save(context.Background(), &artifact.SaveRequest{
		AppName:   testApp,
		UserID:    testUser,
		SessionID: testSession,
		FileName:  "hello.txt",
		Part:      &genai.Part{Text: "hello world"},
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if resp.Version != 0 {
		t.Errorf("Version: got %d, want 0", resp.Version)
	}

	// Verify the version directory exists on disk.
	versionDir := filepath.Join(rootDir, "users", testUser, "sessions", testSession,
		"artifacts", "hello.txt", "versions", fmt.Sprintf("%d", resp.Version))
	if _, err := os.Stat(versionDir); err != nil {
		t.Errorf("version dir not found: %v", err)
	}
	// Verify metadata.json was written.
	if _, err := os.Stat(filepath.Join(versionDir, "metadata.json")); err != nil {
		t.Errorf("metadata.json not found: %v", err)
	}
}

// TestSave_IncrementVersion verifies that saving an artifact twice yields
// versions 0 and 1.
func TestSave_IncrementVersion(t *testing.T) {
	svc := newService(t)

	r0 := saveText(t, svc, "notes.txt", "v0 content")
	if r0.Version != 0 {
		t.Errorf("first save: got version %d, want 0", r0.Version)
	}

	r1 := saveText(t, svc, "notes.txt", "v1 content")
	if r1.Version != 1 {
		t.Errorf("second save: got version %d, want 1", r1.Version)
	}
}

// TestSave_BinaryContent verifies that saving a Part with InlineData writes
// the raw bytes to the version directory on disk.
func TestSave_BinaryContent(t *testing.T) {
	rootDir := t.TempDir()
	svc, err := file.New(file.Config{RootDir: rootDir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	data := []byte{0x89, 0x50, 0x4e, 0x47} // PNG magic bytes
	resp, err := svc.Save(context.Background(), &artifact.SaveRequest{
		AppName:   testApp,
		UserID:    testUser,
		SessionID: testSession,
		FileName:  "image.png",
		Part: &genai.Part{InlineData: &genai.Blob{
			Data:     data,
			MIMEType: "image/png",
		}},
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if resp.Version != 0 {
		t.Errorf("Version: got %d, want 0", resp.Version)
	}

	versionDir := filepath.Join(rootDir, "users", testUser, "sessions", testSession,
		"artifacts", "image.png", "versions", fmt.Sprintf("%d", resp.Version))
	entries, err := os.ReadDir(versionDir)
	if err != nil {
		t.Fatalf("ReadDir version dir: %v", err)
	}

	var foundContent bool
	for _, e := range entries {
		if e.Name() == "metadata.json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(versionDir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile content: %v", err)
		}
		if string(raw) != string(data) {
			t.Errorf("content mismatch: got %v, want %v", raw, data)
		}
		foundContent = true
	}
	if !foundContent {
		t.Error("no content file found in version directory")
	}
}

// TestLoad_LatestVersion saves 3 versions and verifies that loading without
// specifying a version (Version == 0) returns the latest content.
func TestLoad_LatestVersion(t *testing.T) {
	svc := newService(t)

	saveText(t, svc, "doc.txt", "version A")
	saveText(t, svc, "doc.txt", "version B")
	saveText(t, svc, "doc.txt", "version C")

	resp, err := svc.Load(context.Background(), &artifact.LoadRequest{
		AppName:   testApp,
		UserID:    testUser,
		SessionID: testSession,
		FileName:  "doc.txt",
		// Version == 0 means "latest"
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if resp.Part == nil {
		t.Fatal("Load returned nil Part")
	}
	if resp.Part.Text != "version C" {
		t.Errorf("got %q, want %q", resp.Part.Text, "version C")
	}
}

// TestLoad_SpecificVersion saves 3 versions and verifies that requesting
// version 1 returns the content from that exact version.
func TestLoad_SpecificVersion(t *testing.T) {
	svc := newService(t)

	saveText(t, svc, "doc.txt", "version 0 text")
	saveText(t, svc, "doc.txt", "version 1 text")
	saveText(t, svc, "doc.txt", "version 2 text")

	resp, err := svc.Load(context.Background(), &artifact.LoadRequest{
		AppName:   testApp,
		UserID:    testUser,
		SessionID: testSession,
		FileName:  "doc.txt",
		Version:   1,
	})
	if err != nil {
		t.Fatalf("Load(version=1): %v", err)
	}
	if resp.Part.Text != "version 1 text" {
		t.Errorf("got %q, want %q", resp.Part.Text, "version 1 text")
	}
}

// TestLoad_NotFound verifies that loading a non-existent artifact returns an
// error wrapping fs.ErrNotExist.
func TestLoad_NotFound(t *testing.T) {
	svc := newService(t)

	_, err := svc.Load(context.Background(), &artifact.LoadRequest{
		AppName:   testApp,
		UserID:    testUser,
		SessionID: testSession,
		FileName:  "nonexistent.txt",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected fs.ErrNotExist, got: %v", err)
	}
}

// TestDelete saves an artifact then deletes it, verifying the artifact
// directory is removed from disk.
func TestDelete(t *testing.T) {
	rootDir := t.TempDir()
	svc, err := file.New(file.Config{RootDir: rootDir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	saveText(t, svc, "todel.txt", "to be deleted")

	if err := svc.Delete(context.Background(), &artifact.DeleteRequest{
		AppName:   testApp,
		UserID:    testUser,
		SessionID: testSession,
		FileName:  "todel.txt",
	}); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	artifactDir := filepath.Join(rootDir, "users", testUser, "sessions", testSession,
		"artifacts", "todel.txt")
	if _, statErr := os.Stat(artifactDir); !os.IsNotExist(statErr) {
		t.Error("artifact dir still exists after Delete")
	}
}

// TestList_SessionScoped saves two artifacts in a session and verifies List
// returns both filenames sorted alphabetically.
func TestList_SessionScoped(t *testing.T) {
	svc := newService(t)

	saveText(t, svc, "bravo.txt", "b")
	saveText(t, svc, "alpha.txt", "a")

	resp, err := svc.List(context.Background(), &artifact.ListRequest{
		AppName:   testApp,
		UserID:    testUser,
		SessionID: testSession,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := []string{"alpha.txt", "bravo.txt"}
	got := make([]string, len(resp.FileNames))
	copy(got, resp.FileNames)
	sort.Strings(got)

	if len(got) != len(want) {
		t.Fatalf("FileNames: got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("FileNames[%d]: got %q, want %q", i, got[i], w)
		}
	}
}

// TestList_UserScoped saves one user-scoped artifact (filename prefixed with
// "user:") and one session-scoped artifact, then verifies List returns both.
func TestList_UserScoped(t *testing.T) {
	svc := newService(t)

	saveText(t, svc, "session-only.txt", "session content")
	saveText(t, svc, "user:shared.txt", "user-scoped content")

	resp, err := svc.List(context.Background(), &artifact.ListRequest{
		AppName:   testApp,
		UserID:    testUser,
		SessionID: testSession,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := []string{"session-only.txt", "user:shared.txt"}
	got := make([]string, len(resp.FileNames))
	copy(got, resp.FileNames)
	sort.Strings(got)

	if len(got) != len(want) {
		t.Fatalf("FileNames: got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("FileNames[%d]: got %q, want %q", i, got[i], w)
		}
	}
}

// TestVersions saves 3 versions and verifies Versions returns [0, 1, 2] in
// ascending sorted order.
func TestVersions(t *testing.T) {
	svc := newService(t)

	saveText(t, svc, "versioned.txt", "v0")
	saveText(t, svc, "versioned.txt", "v1")
	saveText(t, svc, "versioned.txt", "v2")

	resp, err := svc.Versions(context.Background(), &artifact.VersionsRequest{
		AppName:   testApp,
		UserID:    testUser,
		SessionID: testSession,
		FileName:  "versioned.txt",
	})
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}

	want := []int64{0, 1, 2}
	if len(resp.Versions) != len(want) {
		t.Fatalf("Versions: got %v, want %v", resp.Versions, want)
	}
	for i, w := range want {
		if resp.Versions[i] != w {
			t.Errorf("Versions[%d]: got %d, want %d", i, resp.Versions[i], w)
		}
	}
}

// TestSave_PathTraversal verifies that a filename containing ".." is rejected.
func TestSave_PathTraversal(t *testing.T) {
	svc := newService(t)

	_, err := svc.Save(context.Background(), &artifact.SaveRequest{
		AppName:   testApp,
		UserID:    testUser,
		SessionID: testSession,
		FileName:  "../secret",
		Part:      &genai.Part{Text: "should not be written"},
	})
	if err == nil {
		t.Fatal("expected error for path traversal filename, got nil")
	}
}

// TestSave_AbsolutePath verifies that an absolute filename is rejected.
func TestSave_AbsolutePath(t *testing.T) {
	svc := newService(t)

	_, err := svc.Save(context.Background(), &artifact.SaveRequest{
		AppName:   testApp,
		UserID:    testUser,
		SessionID: testSession,
		FileName:  "/etc/passwd",
		Part:      &genai.Part{Text: "should not be written"},
	})
	if err == nil {
		t.Fatal("expected error for absolute path filename, got nil")
	}
}
