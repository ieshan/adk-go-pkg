// Package file provides a filesystem-backed implementation of the ADK-Go
// artifact.Service interface.
//
// Artifacts are stored in a directory hierarchy rooted at Config.RootDir:
//
//	{RootDir}/users/{userID}/sessions/{sessionID}/artifacts/{fileName}/versions/{version}/
//	  ├── {filename}         # the payload (text → .txt, binary → original name)
//	  └── metadata.json      # version metadata (see VersionMetadata)
//
// For user-scoped artifacts (filenames prefixed with "user:") the session
// segment is omitted:
//
//	{RootDir}/users/{userID}/artifacts/{fileName}/versions/{version}/
//
// Version numbering starts at 0. Each Save call assigns the next version as
// max(existing) + 1. A version of 0 in a Load request is treated as "latest".
// Delete removes all versions of an artifact (it ignores the Version field).
//
// All filesystem access is scoped beneath an [os.Root] opened from Config.RootDir,
// providing kernel-level path traversal protection. Callers must call Close to
// release the underlying file descriptor when the service is no longer needed.
//
// # Example
//
//	svc, err := file.New(file.Config{RootDir: "/tmp/artifacts"})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer svc.Close()
//
//	resp, err := svc.Save(ctx, &artifact.SaveRequest{
//	    AppName:   "myapp",
//	    UserID:    "alice",
//	    SessionID: "session-1",
//	    FileName:  "report.txt",
//	    Part:      &genai.Part{Text: "quarterly report"},
//	})
//	// resp.Version == 0
package file

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/adk/v2/artifact"
	"google.golang.org/genai"
)

// userScopedPrefix is the filename prefix that marks a user-scoped artifact
// (available across all sessions for a given app+user).
const userScopedPrefix = "user:"

// Service is the filesystem-backed implementation of artifact.Service.
// All filesystem access is scoped beneath an [os.Root] opened from Config.RootDir.
type Service struct {
	root *os.Root
}

// Config holds the configuration for the file-backed artifact service.
type Config struct {
	// RootDir is the base directory under which all artifact data is stored.
	// The directory will be created if it does not exist.
	RootDir string
}

// New creates an artifact.Service backed by the local filesystem at cfg.RootDir.
//
// The root directory is created (with mode 0750) if it does not already exist.
// An [os.Root] is opened from the root directory and used for all subsequent
// filesystem access, providing kernel-level path traversal protection.
// Callers must call Close to release the underlying file descriptor.
//
// Example:
//
//	svc, err := file.New(file.Config{RootDir: "/var/lib/myapp/artifacts"})
//	if err != nil {
//	    return fmt.Errorf("start artifact service: %w", err)
//	}
//	defer svc.Close()
func New(cfg Config) (*Service, error) {
	if cfg.RootDir == "" {
		return nil, errors.New("file artifact service: RootDir must not be empty")
	}
	if err := os.MkdirAll(cfg.RootDir, 0750); err != nil {
		return nil, fmt.Errorf("file artifact service: create root dir: %w", err)
	}
	root, err := os.OpenRoot(cfg.RootDir)
	if err != nil {
		return nil, fmt.Errorf("file artifact service: open root: %w", err)
	}
	return &Service{root: root}, nil
}

// Close releases the underlying [os.Root] file descriptor.
// It is safe to call multiple times.
func (s *Service) Close() error {
	return s.root.Close()
}

// artifactDir returns the directory (relative to the root) for a given artifact,
// handling user-scoped filenames (prefixed with "user:") by omitting the session
// path segment.
func (s *Service) artifactDir(userID, sessionID, fileName string) string {
	if strings.HasPrefix(fileName, userScopedPrefix) {
		// User-scoped: users/{userID}/artifacts/{fileName}
		return filepath.Join("users", userID, "artifacts", fileName)
	}
	// Session-scoped: users/{userID}/sessions/{sessionID}/artifacts/{fileName}
	return filepath.Join("users", userID, "sessions", sessionID, "artifacts", fileName)
}

// versionsDir returns the versions sub-directory (relative to the root) for an artifact.
func (s *Service) versionsDir(userID, sessionID, fileName string) string {
	return filepath.Join(s.artifactDir(userID, sessionID, fileName), "versions")
}

// validateFileName checks that name is not an absolute path and does not
// contain ".." traversal that would escape the base directory.
// This is a defence-in-depth guard; the ADK-Go Validate methods also check
// for path separators, but callers may skip Validate. The [os.Root] provides
// an additional kernel-level boundary.
func validateFileName(name string) error {
	if filepath.IsAbs(name) {
		return fmt.Errorf("invalid filename %q: absolute paths are not allowed", name)
	}
	// filepath.Rel will add ".." components if name escapes the base.
	rel, err := filepath.Rel(".", name)
	if err != nil {
		return fmt.Errorf("invalid filename %q: %w", name, err)
	}
	if strings.HasPrefix(rel, "..") {
		return fmt.Errorf("invalid filename %q: path traversal is not allowed", name)
	}
	return nil
}

// validatePathSegment checks that a user ID or session ID is safe to use as a
// path component. It rejects empty strings, absolute paths, ".." traversal,
// and path separators. This is a defence-in-depth guard; the ADK-Go Validate
// methods also check for empty fields, but callers may skip Validate. The
// [os.Root] provides an additional kernel-level boundary, but does not prevent
// logical cross-user/session access within the root (e.g. "alice/../bob").
func validatePathSegment(name, kind string) error {
	if name == "" {
		return fmt.Errorf("invalid %s: must not be empty", kind)
	}
	if filepath.IsAbs(name) {
		return fmt.Errorf("invalid %s %q: absolute paths are not allowed", kind, name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("invalid %s %q: path traversal is not allowed", kind, name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid %s %q: path separators are not allowed", kind, name)
	}
	return nil
}

// listVersions returns all version numbers present in the versions/ directory,
// sorted ascending.  An empty slice (no error) is returned when the directory
// does not exist yet.
func (s *Service) listVersions(versDir string) ([]int64, error) {
	entries, err := fs.ReadDir(s.root.FS(), versDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read versions dir: %w", err)
	}

	var versions []int64
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		v, err := strconv.ParseInt(e.Name(), 10, 64)
		if err != nil {
			continue // skip non-numeric entries
		}
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
	return versions, nil
}

// latestVersion returns the maximum version number stored for an artifact, and
// a boolean indicating whether any versions exist.
func (s *Service) latestVersion(versDir string) (int64, bool, error) {
	versions, err := s.listVersions(versDir)
	if err != nil {
		return 0, false, err
	}
	if len(versions) == 0 {
		return 0, false, nil
	}
	return versions[len(versions)-1], true, nil
}

// Save implements artifact.Service.
//
// Each call stores the artifact payload plus a metadata.json sidecar in a new
// versioned subdirectory.  The first call for a given artifact creates version
// 0; subsequent calls increment the version by 1.
//
// Text content (Part.Text != "") is written to a ".txt" file.  Binary content
// (Part.InlineData != nil) is written to a file named after the artifact
// FileName.
func (s *Service) Save(_ context.Context, req *artifact.SaveRequest) (*artifact.SaveResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}
	if err := validatePathSegment(req.UserID, "user_id"); err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}
	if err := validatePathSegment(req.SessionID, "session_id"); err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}
	if err := validateFileName(req.FileName); err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}

	versDir := s.versionsDir(req.UserID, req.SessionID, req.FileName)

	versions, err := s.listVersions(versDir)
	if err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}

	var nextVersion int64
	if len(versions) > 0 {
		nextVersion = versions[len(versions)-1] + 1
	}

	vDir := filepath.Join(versDir, strconv.FormatInt(nextVersion, 10))
	if err := s.root.MkdirAll(vDir, 0750); err != nil {
		return nil, fmt.Errorf("save: create version dir: %w", err)
	}

	// Determine content filename and payload.
	var (
		contentName string
		payload     []byte
		mimeType    string
	)
	if req.Part.Text != "" {
		contentName = req.FileName + ".txt"
		payload = []byte(req.Part.Text)
		mimeType = "text/plain"
	} else {
		blob := req.Part.InlineData
		contentName = req.FileName
		payload = blob.Data
		mimeType = blob.MIMEType
	}

	if err := s.root.WriteFile(filepath.Join(vDir, contentName), payload, 0600); err != nil {
		return nil, fmt.Errorf("save: write content: %w", err)
	}

	meta := &VersionMetadata{
		Version:      nextVersion,
		FileName:     req.FileName,
		MimeType:     mimeType,
		CreateTime:   time.Now(),
		CanonicalURI: canonicalURI(req.AppName, req.UserID, req.SessionID, req.FileName, nextVersion),
	}
	if err := s.writeMetadata(vDir, meta); err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}

	return &artifact.SaveResponse{Version: nextVersion}, nil
}

// canonicalURI builds a deterministic identifier for an artifact version.
func canonicalURI(appName, userID, sessionID, fileName string, version int64) string {
	return fmt.Sprintf("file://%s/%s/%s/%s/%d", appName, userID, sessionID, fileName, version)
}

// Load implements artifact.Service.
//
// When req.Version is 0 the latest (highest version number) is returned.
// For any other value the exact version is returned.  An error wrapping
// fs.ErrNotExist is returned when the artifact or the requested version does
// not exist.
func (s *Service) Load(_ context.Context, req *artifact.LoadRequest) (*artifact.LoadResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}
	if err := validatePathSegment(req.UserID, "user_id"); err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}
	if err := validatePathSegment(req.SessionID, "session_id"); err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}
	if err := validateFileName(req.FileName); err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}

	versDir := s.versionsDir(req.UserID, req.SessionID, req.FileName)

	var targetVersion int64
	if req.Version == 0 {
		// 0 means "latest"
		latest, ok, err := s.latestVersion(versDir)
		if err != nil {
			return nil, fmt.Errorf("load: %w", err)
		}
		if !ok {
			return nil, fmt.Errorf("load: artifact %q not found: %w", req.FileName, fs.ErrNotExist)
		}
		targetVersion = latest
	} else {
		targetVersion = req.Version
	}

	vDir := filepath.Join(versDir, strconv.FormatInt(targetVersion, 10))
	meta, err := s.readMetadata(vDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("load: artifact %q version %d not found: %w", req.FileName, targetVersion, fs.ErrNotExist)
		}
		return nil, fmt.Errorf("load: %w", err)
	}

	// Find the content file (everything that is not metadata.json).
	entries, err := fs.ReadDir(s.root.FS(), vDir)
	if err != nil {
		return nil, fmt.Errorf("load: read version dir: %w", err)
	}

	var contentFile string
	for _, e := range entries {
		if e.Name() != "metadata.json" {
			contentFile = filepath.Join(vDir, e.Name())
			break
		}
	}
	if contentFile == "" {
		return nil, fmt.Errorf("load: content file missing for %q version %d", req.FileName, targetVersion)
	}

	data, err := s.root.ReadFile(contentFile)
	if err != nil {
		return nil, fmt.Errorf("load: read content: %w", err)
	}

	var part *genai.Part
	if meta.MimeType == "text/plain" {
		part = &genai.Part{Text: string(data)}
	} else {
		part = &genai.Part{InlineData: &genai.Blob{
			Data:     data,
			MIMEType: meta.MimeType,
		}}
	}

	return &artifact.LoadResponse{Part: part}, nil
}

// Delete implements artifact.Service.
//
// Removes the entire artifact directory (all versions) for the given filename.
// Deleting a non-existent artifact is not an error.
func (s *Service) Delete(_ context.Context, req *artifact.DeleteRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	if err := validatePathSegment(req.UserID, "user_id"); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	if err := validatePathSegment(req.SessionID, "session_id"); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	if err := validateFileName(req.FileName); err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	dir := s.artifactDir(req.UserID, req.SessionID, req.FileName)
	if err := s.root.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete: remove artifact dir: %w", err)
	}
	return nil
}

// List implements artifact.Service.
//
// Returns the sorted filenames of all artifacts stored in the given session,
// including user-scoped artifacts (those whose filename starts with "user:").
// The ListRequest requires a non-empty SessionID even though user-scoped
// artifacts are stored outside the session path; this matches the ADK-Go
// contract.
func (s *Service) List(_ context.Context, req *artifact.ListRequest) (*artifact.ListResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	if err := validatePathSegment(req.UserID, "user_id"); err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	if err := validatePathSegment(req.SessionID, "session_id"); err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}

	names := map[string]struct{}{}

	// Session-scoped artifacts.
	sessionArtifactsDir := filepath.Join("users", req.UserID, "sessions", req.SessionID, "artifacts")
	if err := s.collectArtifactNames(sessionArtifactsDir, names); err != nil {
		return nil, fmt.Errorf("list: session artifacts: %w", err)
	}

	// User-scoped artifacts.
	userArtifactsDir := filepath.Join("users", req.UserID, "artifacts")
	if err := s.collectArtifactNames(userArtifactsDir, names); err != nil {
		return nil, fmt.Errorf("list: user artifacts: %w", err)
	}

	fileNames := make([]string, 0, len(names))
	for n := range names {
		fileNames = append(fileNames, n)
	}
	sort.Strings(fileNames)

	return &artifact.ListResponse{FileNames: fileNames}, nil
}

// collectArtifactNames reads the immediate subdirectories of dir (each
// subdirectory is an artifact name) and adds them to names.  If dir does not
// exist the function is a no-op.
func (s *Service) collectArtifactNames(dir string, names map[string]struct{}) error {
	entries, err := fs.ReadDir(s.root.FS(), dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			names[e.Name()] = struct{}{}
		}
	}
	return nil
}

// Versions implements artifact.Service.
//
// Returns all version numbers for the artifact in ascending order.  An error
// wrapping fs.ErrNotExist is returned when no versions exist.
func (s *Service) Versions(_ context.Context, req *artifact.VersionsRequest) (*artifact.VersionsResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("versions: %w", err)
	}
	if err := validatePathSegment(req.UserID, "user_id"); err != nil {
		return nil, fmt.Errorf("versions: %w", err)
	}
	if err := validatePathSegment(req.SessionID, "session_id"); err != nil {
		return nil, fmt.Errorf("versions: %w", err)
	}
	if err := validateFileName(req.FileName); err != nil {
		return nil, fmt.Errorf("versions: %w", err)
	}

	versDir := s.versionsDir(req.UserID, req.SessionID, req.FileName)
	versions, err := s.listVersions(versDir)
	if err != nil {
		return nil, fmt.Errorf("versions: %w", err)
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("versions: artifact %q not found: %w", req.FileName, fs.ErrNotExist)
	}
	return &artifact.VersionsResponse{Versions: versions}, nil
}

// GetArtifactVersion implements artifact.Service.
//
// Returns metadata for a specific artifact version. When req.Version is 0,
// the latest version is returned. An error wrapping fs.ErrNotExist is returned
// when the artifact or requested version does not exist.
func (s *Service) GetArtifactVersion(_ context.Context, req *artifact.GetArtifactVersionRequest) (*artifact.GetArtifactVersionResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("getArtifactVersion: %w", err)
	}
	if err := validatePathSegment(req.UserID, "user_id"); err != nil {
		return nil, fmt.Errorf("getArtifactVersion: %w", err)
	}
	if err := validatePathSegment(req.SessionID, "session_id"); err != nil {
		return nil, fmt.Errorf("getArtifactVersion: %w", err)
	}
	if err := validateFileName(req.FileName); err != nil {
		return nil, fmt.Errorf("getArtifactVersion: %w", err)
	}

	versDir := s.versionsDir(req.UserID, req.SessionID, req.FileName)

	var targetVersion int64
	if req.Version == 0 {
		// 0 means "latest"
		latest, ok, err := s.latestVersion(versDir)
		if err != nil {
			return nil, fmt.Errorf("getArtifactVersion: %w", err)
		}
		if !ok {
			return nil, fmt.Errorf("getArtifactVersion: artifact %q not found: %w", req.FileName, fs.ErrNotExist)
		}
		targetVersion = latest
	} else {
		targetVersion = req.Version
	}

	vDir := filepath.Join(versDir, strconv.FormatInt(targetVersion, 10))
	meta, err := s.readMetadata(vDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("getArtifactVersion: artifact %q version %d not found: %w", req.FileName, targetVersion, fs.ErrNotExist)
		}
		return nil, fmt.Errorf("getArtifactVersion: %w", err)
	}

	return &artifact.GetArtifactVersionResponse{
		ArtifactVersion: &artifact.ArtifactVersion{
			Version:        meta.Version,
			CanonicalURI:   meta.CanonicalURI,
			CustomMetadata: meta.CustomMetadata,
			CreateTime:     meta.CreateTime,
			MimeType:       meta.MimeType,
		},
	}, nil
}

// Ensure Service satisfies the interface at compile time.
var _ artifact.Service = (*Service)(nil)
