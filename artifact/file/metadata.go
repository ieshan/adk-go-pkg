package file

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"
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

	// CreateTime is the timestamp at which this version was created.
	CreateTime time.Time `json:"createTime"`

	// CanonicalURI is the fully qualified storage URI that uniquely identifies
	// this version (e.g. "file://appName/userID/sessionID/fileName/3").
	CanonicalURI string `json:"canonicalUri"`

	// CustomMetadata is reserved for caller-supplied key-value pairs. The
	// current Service does not populate this field; it is always nil for
	// artifacts created via Save. It is omitted from JSON when nil.
	CustomMetadata map[string]any `json:"customMetadata,omitempty"`
}

// writeMetadata serialises meta as indented JSON and writes it to
// filepath.Join(dir, "metadata.json") beneath the service's [os.Root],
// creating or truncating the file as needed. The file is written with mode 0600.
//
// Returns a wrapped error on marshal failure or I/O error.
func (s *Service) writeMetadata(dir string, meta *VersionMetadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	return s.root.WriteFile(filepath.Join(dir, "metadata.json"), data, 0600)
}

// readMetadata reads and parses the metadata.json file located inside dir
// (relative to the service's [os.Root]).
//
// It returns a pointer to the populated VersionMetadata on success, or a
// wrapped error if the file cannot be read or the JSON is malformed.
func (s *Service) readMetadata(dir string) (*VersionMetadata, error) {
	data, err := s.root.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	var meta VersionMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}
	return &meta, nil
}
