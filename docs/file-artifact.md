# File Artifact Service

Package `artifact/file` provides a filesystem-backed implementation of the
ADK-Go `artifact.Service` interface.

## Overview

Artifacts are stored as versioned files on disk with JSON metadata sidecars.
Each `Save` call creates a new version; `Load` retrieves any version (or the
latest by default). The service supports both session-scoped and user-scoped
artifacts.

### Storage Layout

Session-scoped artifacts:

```
{RootDir}/users/{userID}/sessions/{sessionID}/artifacts/{fileName}/versions/{version}/
  +-- {filename}.txt       # text payload (or original name for binary)
  +-- metadata.json        # version metadata
```

User-scoped artifacts (filenames prefixed with `user:`):

```
{RootDir}/users/{userID}/artifacts/{fileName}/versions/{version}/
  +-- {filename}.txt
  +-- metadata.json
```

Version numbering starts at 0 and increments by 1 on each `Save`.

## API Reference

### Config

```go
type Config struct {
    // Base directory for all artifact data. Created if it does not exist.
    RootDir string
}
```

### New

```go
func New(cfg Config) (artifact.Service, error)
```

Creates an `artifact.Service` backed by the local filesystem. Returns an error
when `RootDir` is empty or cannot be created.

### VersionMetadata

Each version directory contains a `metadata.json` with:

```go
type VersionMetadata struct {
    Version        int64          `json:"version"`
    FileName       string         `json:"fileName"`
    MimeType       string         `json:"mimeType,omitempty"`
    CreateTime     float64        `json:"createTime"`
    CanonicalURI   string         `json:"canonicalUri"`
    CustomMetadata map[string]any `json:"customMetadata,omitempty"`
}
```

## Examples

### Save an Artifact

```go
svc, err := file.New(file.Config{RootDir: "/tmp/artifacts"})
if err != nil {
    log.Fatal(err)
}

resp, err := svc.Save(ctx, &artifact.SaveRequest{
    AppName:   "myapp",
    UserID:    "alice",
    SessionID: "session-1",
    FileName:  "report.txt",
    Part:      &genai.Part{Text: "quarterly report content"},
})
if err != nil {
    log.Fatal(err)
}
fmt.Println("saved version:", resp.Version) // 0
```

### Save a Binary Artifact

```go
imageData, _ := os.ReadFile("photo.png")

resp, err := svc.Save(ctx, &artifact.SaveRequest{
    AppName:   "myapp",
    UserID:    "alice",
    SessionID: "session-1",
    FileName:  "photo.png",
    Part: &genai.Part{
        InlineData: &genai.Blob{
            Data:     imageData,
            MIMEType: "image/png",
        },
    },
})
```

### Load an Artifact

> **Note:** `Version: 0` in Load and Delete always resolves to "latest".
> The first saved artifact is version 0. When only one version exists, both "latest"
> and "specific version 0" resolve to the same content.

```go
// Load latest version (Version: 0 means latest)
loadResp, err := svc.Load(ctx, &artifact.LoadRequest{
    AppName:   "myapp",
    UserID:    "alice",
    SessionID: "session-1",
    FileName:  "report.txt",
    Version:   0,
})
if err != nil {
    log.Fatal(err)
}
fmt.Println(loadResp.Part.Text)

// Load a specific version
loadResp, err = svc.Load(ctx, &artifact.LoadRequest{
    AppName:   "myapp",
    UserID:    "alice",
    SessionID: "session-1",
    FileName:  "report.txt",
    Version:   2,
})
```

### List Artifacts

```go
listResp, err := svc.List(ctx, &artifact.ListRequest{
    AppName:   "myapp",
    UserID:    "alice",
    SessionID: "session-1",
})
if err != nil {
    log.Fatal(err)
}

for _, name := range listResp.FileNames {
    fmt.Println(name)
}
```

This returns both session-scoped and user-scoped artifact names, sorted
alphabetically.

### List Versions

```go
versResp, err := svc.Versions(ctx, &artifact.VersionsRequest{
    AppName:   "myapp",
    UserID:    "alice",
    SessionID: "session-1",
    FileName:  "report.txt",
})
if err != nil {
    log.Fatal(err)
}

for _, v := range versResp.Versions {
    fmt.Println("version:", v)
}
```

### User-Scoped Artifacts

Prefix the filename with `user:` to store artifacts outside any session:

```go
resp, err := svc.Save(ctx, &artifact.SaveRequest{
    AppName:   "myapp",
    UserID:    "alice",
    SessionID: "session-1",
    FileName:  "user:preferences",
    Part:      &genai.Part{Text: `{"theme":"dark"}`},
})
```

User-scoped artifacts are accessible from any session for the same user.
