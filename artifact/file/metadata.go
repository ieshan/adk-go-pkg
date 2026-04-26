package file

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// VersionMetadata is stored as metadata.json alongside each artifact version.
// It captures all non-payload information about a single artifact version and
// is serialised/deserialised with the standard library's encoding/json package.
//
// Fields mirror the metadata tracked by the ADK-Go artifact service.
type VersionMetadata struct {
	// Version is the monotonically increasing version number for this artifact.
	// Version numbers start at 0 and are assigned by the service on each Save.
	Version int64 `json:"version"`

	// FileName is the human-readable name of the artifact as provided by the
	// caller (e.g. "report.pdf").
	FileName string `json:"fileName"`

	// MimeType is the IANA media type of the artifact content (e.g.
	// "application/pdf"). It is optional; the field is omitted from JSON when
	// empty.
	MimeType string `json:"mimeType,omitempty"`

	// CreateTime is the Unix epoch timestamp (seconds with sub-second precision)
	// at which this version was created.
	CreateTime float64 `json:"createTime"`

	// CanonicalURI is the fully qualified storage URI that uniquely identifies
	// this version (e.g. "gs://bucket/appName/userID/sessionID/fileName/3").
	CanonicalURI string `json:"canonicalUri"`

	// CustomMetadata holds arbitrary caller-supplied key-value pairs associated
	// with this version. It is optional; the field is omitted from JSON when nil.
	CustomMetadata map[string]any `json:"customMetadata,omitempty"`
}

// writeMetadata serialises meta as indented JSON and writes it to
// filepath.Join(dir, "metadata.json"), creating or truncating the file as
// needed. The file is written with mode 0644.
//
// Returns a wrapped error on marshal failure or I/O error.
func writeMetadata(dir string, meta *VersionMetadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0644)
}

// readMetadata reads and parses the metadata.json file located inside dir.
//
// It returns a pointer to the populated VersionMetadata on success, or a
// wrapped error if the file cannot be read or the JSON is malformed.
func readMetadata(dir string) (*VersionMetadata, error) {
	data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	var meta VersionMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}
	return &meta, nil
}
