// Copyright 2025 ieshan
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package testutil

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"

	"google.golang.org/adk/artifact"
	"google.golang.org/genai"
)

// Compile-time interface check.
var _ artifact.Service = (*FakeArtifactService)(nil)

// artifactKey uniquely identifies an artifact by app, user, session, and
// filename.
type artifactKey struct {
	AppName, UserID, SessionID, FileName string
}

// FakeArtifactService implements artifact.Service for testing.
// It stores artifacts in-memory with version tracking and records all calls.
//
// Thread-safe.
type FakeArtifactService struct {
	mu        sync.RWMutex
	artifacts map[artifactKey][]*genai.Part // key -> ordered versions (index = version-1)
	saves     []*artifact.SaveRequest
	loads     []*artifact.LoadRequest
}

// NewFakeArtifactService creates a FakeArtifactService.
func NewFakeArtifactService() *FakeArtifactService {
	return &FakeArtifactService{
		artifacts: make(map[artifactKey][]*genai.Part),
	}
}

// PreloadArtifact adds an artifact for test setup (bypasses Save).
func (f *FakeArtifactService) PreloadArtifact(appName, userID, sessionID, filename string, part *genai.Part) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := artifactKey{AppName: appName, UserID: userID, SessionID: sessionID, FileName: filename}
	f.artifacts[key] = append(f.artifacts[key], part)
}

// Save implements artifact.Service.
func (f *FakeArtifactService) Save(ctx context.Context, req *artifact.SaveRequest) (*artifact.SaveResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.saves = append(f.saves, req)

	sessionID := req.SessionID
	if strings.HasPrefix(req.FileName, "user:") {
		sessionID = "user"
	}

	key := artifactKey{AppName: req.AppName, UserID: req.UserID, SessionID: sessionID, FileName: req.FileName}
	versions := f.artifacts[key]
	nextVersion := int64(len(versions) + 1)
	f.artifacts[key] = append(versions, req.Part)

	return &artifact.SaveResponse{Version: nextVersion}, nil
}

// Load implements artifact.Service.
func (f *FakeArtifactService) Load(ctx context.Context, req *artifact.LoadRequest) (*artifact.LoadResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.loads = append(f.loads, req)

	sessionID := req.SessionID
	if strings.HasPrefix(req.FileName, "user:") {
		sessionID = "user"
	}

	key := artifactKey{AppName: req.AppName, UserID: req.UserID, SessionID: sessionID, FileName: req.FileName}
	versions, ok := f.artifacts[key]
	if !ok || len(versions) == 0 {
		return nil, fmt.Errorf("artifact not found: %w", fs.ErrNotExist)
	}

	if req.Version > 0 {
		idx := int(req.Version) - 1
		if idx < 0 || idx >= len(versions) {
			return nil, fmt.Errorf("artifact version not found: %w", fs.ErrNotExist)
		}
		return &artifact.LoadResponse{Part: versions[idx]}, nil
	}

	// Return the latest version.
	return &artifact.LoadResponse{Part: versions[len(versions)-1]}, nil
}

// Delete implements artifact.Service.
func (f *FakeArtifactService) Delete(ctx context.Context, req *artifact.DeleteRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("request validation failed: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	sessionID := req.SessionID
	if strings.HasPrefix(req.FileName, "user:") {
		sessionID = "user"
	}

	key := artifactKey{AppName: req.AppName, UserID: req.UserID, SessionID: sessionID, FileName: req.FileName}

	if req.Version != 0 {
		idx := int(req.Version) - 1
		if versions, ok := f.artifacts[key]; ok && idx >= 0 && idx < len(versions) {
			f.artifacts[key] = append(versions[:idx], versions[idx+1:]...)
		}
		return nil
	}

	delete(f.artifacts, key)
	return nil
}

// List implements artifact.Service.
func (f *FakeArtifactService) List(ctx context.Context, req *artifact.ListRequest) (*artifact.ListResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	files := make(map[string]bool)
	for key, versions := range f.artifacts {
		if key.AppName == req.AppName && key.UserID == req.UserID && key.SessionID == req.SessionID && len(versions) > 0 {
			files[key.FileName] = true
		}
	}

	// Also include user-scoped artifacts.
	userKey := "user"
	for key, versions := range f.artifacts {
		if key.AppName == req.AppName && key.UserID == req.UserID && key.SessionID == userKey && len(versions) > 0 {
			files[key.FileName] = true
		}
	}

	filenames := make([]string, 0, len(files))
	for name := range files {
		filenames = append(filenames, name)
	}
	sort.Strings(filenames)

	return &artifact.ListResponse{FileNames: filenames}, nil
}

// Versions implements artifact.Service.
func (f *FakeArtifactService) Versions(ctx context.Context, req *artifact.VersionsRequest) (*artifact.VersionsResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	sessionID := req.SessionID
	if strings.HasPrefix(req.FileName, "user:") {
		sessionID = "user"
	}

	key := artifactKey{AppName: req.AppName, UserID: req.UserID, SessionID: sessionID, FileName: req.FileName}
	versions, ok := f.artifacts[key]
	if !ok || len(versions) == 0 {
		return nil, fmt.Errorf("artifact not found: %w", fs.ErrNotExist)
	}

	versionNums := make([]int64, len(versions))
	for i := range versions {
		versionNums[i] = int64(i + 1)
	}

	return &artifact.VersionsResponse{Versions: versionNums}, nil
}

// GetArtifactVersion implements artifact.Service.
func (f *FakeArtifactService) GetArtifactVersion(ctx context.Context, req *artifact.GetArtifactVersionRequest) (*artifact.GetArtifactVersionResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	sessionID := req.SessionID
	if strings.HasPrefix(req.FileName, "user:") {
		sessionID = "user"
	}

	key := artifactKey{AppName: req.AppName, UserID: req.UserID, SessionID: sessionID, FileName: req.FileName}
	versions, ok := f.artifacts[key]
	if !ok || len(versions) == 0 {
		return nil, fmt.Errorf("artifact not found: %w", fs.ErrNotExist)
	}

	var version int64
	var part *genai.Part
	if req.Version > 0 {
		idx := int(req.Version) - 1
		if idx < 0 || idx >= len(versions) {
			return nil, fmt.Errorf("artifact version not found: %w", fs.ErrNotExist)
		}
		version = req.Version
		part = versions[idx]
	} else {
		version = int64(len(versions))
		part = versions[len(versions)-1]
	}

	mimeType := "text/plain"
	if part != nil && part.InlineData != nil {
		mimeType = part.InlineData.MIMEType
	}

	return &artifact.GetArtifactVersionResponse{
		ArtifactVersion: &artifact.ArtifactVersion{
			Version:  version,
			MimeType: mimeType,
		},
	}, nil
}

// SaveCount returns the number of Save calls.
func (f *FakeArtifactService) SaveCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.saves)
}

// LoadCount returns the number of Load calls.
func (f *FakeArtifactService) LoadCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.loads)
}

// LastSave returns the most recent SaveRequest, or nil.
func (f *FakeArtifactService) LastSave() *artifact.SaveRequest {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if len(f.saves) == 0 {
		return nil
	}
	return f.saves[len(f.saves)-1]
}
