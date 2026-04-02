package file

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleMeta returns a fully-populated VersionMetadata for use in tests.
func sampleMeta() *VersionMetadata {
	return &VersionMetadata{
		Version:      3,
		FileName:     "report.pdf",
		MimeType:     "application/pdf",
		CreateTime:   1_700_000_000.123,
		CanonicalURI: "gs://my-bucket/app/user/session/report.pdf/3",
		CustomMetadata: map[string]any{
			"author": "alice",
			"tags":   []any{"finance", "q4"},
		},
	}
}

// TestVersionMetadata_MarshalJSON verifies that VersionMetadata marshals to
// the expected JSON keys and values.
func TestVersionMetadata_MarshalJSON(t *testing.T) {
	meta := sampleMeta()
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	// Decode into a raw map for key-level assertions.
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal into map: %v", err)
	}

	checks := []struct {
		key  string
		want any
	}{
		{"version", float64(3)},
		{"fileName", "report.pdf"},
		{"mimeType", "application/pdf"},
		{"createTime", float64(1_700_000_000.123)},
		{"canonicalUri", "gs://my-bucket/app/user/session/report.pdf/3"},
	}
	for _, c := range checks {
		if got[c.key] != c.want {
			t.Errorf("key %q: got %v (%T), want %v (%T)", c.key, got[c.key], got[c.key], c.want, c.want)
		}
	}

	// Verify customMetadata is present as an object.
	if _, ok := got["customMetadata"]; !ok {
		t.Error("key customMetadata missing from JSON output")
	}
}

// TestVersionMetadata_MarshalJSON_OmitEmpty verifies that optional fields
// (MimeType, CustomMetadata) are omitted when zero/nil.
func TestVersionMetadata_MarshalJSON_OmitEmpty(t *testing.T) {
	meta := &VersionMetadata{
		Version:      1,
		FileName:     "empty.txt",
		CreateTime:   1_000_000.0,
		CanonicalURI: "gs://bucket/empty.txt/1",
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	s := string(data)
	if strings.Contains(s, "mimeType") {
		t.Errorf("expected mimeType to be omitted, got: %s", s)
	}
	if strings.Contains(s, "customMetadata") {
		t.Errorf("expected customMetadata to be omitted, got: %s", s)
	}
}

// TestVersionMetadata_UnmarshalJSON verifies that JSON bytes unmarshal into a
// VersionMetadata struct with correct field values.
func TestVersionMetadata_UnmarshalJSON(t *testing.T) {
	raw := `{
		"version": 5,
		"fileName": "data.csv",
		"mimeType": "text/csv",
		"createTime": 1234567890.5,
		"canonicalUri": "gs://bucket/data.csv/5",
		"customMetadata": {"owner": "bob"}
	}`

	var meta VersionMetadata
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if meta.Version != 5 {
		t.Errorf("Version: got %d, want 5", meta.Version)
	}
	if meta.FileName != "data.csv" {
		t.Errorf("FileName: got %q, want %q", meta.FileName, "data.csv")
	}
	if meta.MimeType != "text/csv" {
		t.Errorf("MimeType: got %q, want %q", meta.MimeType, "text/csv")
	}
	if meta.CreateTime != 1234567890.5 {
		t.Errorf("CreateTime: got %f, want %f", meta.CreateTime, 1234567890.5)
	}
	if meta.CanonicalURI != "gs://bucket/data.csv/5" {
		t.Errorf("CanonicalURI: got %q, want %q", meta.CanonicalURI, "gs://bucket/data.csv/5")
	}
	if meta.CustomMetadata["owner"] != "bob" {
		t.Errorf("CustomMetadata[owner]: got %v, want %q", meta.CustomMetadata["owner"], "bob")
	}
}

// TestVersionMetadata_RoundTrip marshals a VersionMetadata to JSON and then
// unmarshals it back, verifying the resulting struct is equal to the original.
func TestVersionMetadata_RoundTrip(t *testing.T) {
	original := sampleMeta()

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var restored VersionMetadata
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if restored.Version != original.Version {
		t.Errorf("Version mismatch: got %d, want %d", restored.Version, original.Version)
	}
	if restored.FileName != original.FileName {
		t.Errorf("FileName mismatch: got %q, want %q", restored.FileName, original.FileName)
	}
	if restored.MimeType != original.MimeType {
		t.Errorf("MimeType mismatch: got %q, want %q", restored.MimeType, original.MimeType)
	}
	if restored.CreateTime != original.CreateTime {
		t.Errorf("CreateTime mismatch: got %f, want %f", restored.CreateTime, original.CreateTime)
	}
	if restored.CanonicalURI != original.CanonicalURI {
		t.Errorf("CanonicalURI mismatch: got %q, want %q", restored.CanonicalURI, original.CanonicalURI)
	}
	// Spot-check one custom metadata key.
	if restored.CustomMetadata["author"] != original.CustomMetadata["author"] {
		t.Errorf("CustomMetadata[author] mismatch: got %v, want %v",
			restored.CustomMetadata["author"], original.CustomMetadata["author"])
	}
}

// TestWriteMetadata writes a VersionMetadata to a temp directory via
// writeMetadata and then reads the raw file back to verify its contents.
func TestWriteMetadata(t *testing.T) {
	dir := t.TempDir()
	meta := sampleMeta()

	if err := writeMetadata(dir, meta); err != nil {
		t.Fatalf("writeMetadata: %v", err)
	}

	// Read the raw file and verify the JSON.
	data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatalf("os.ReadFile: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if got["version"] != float64(3) {
		t.Errorf("version: got %v, want 3", got["version"])
	}
	if got["fileName"] != "report.pdf" {
		t.Errorf("fileName: got %v, want report.pdf", got["fileName"])
	}
	if got["canonicalUri"] != "gs://my-bucket/app/user/session/report.pdf/3" {
		t.Errorf("canonicalUri: got %v", got["canonicalUri"])
	}

	// Verify the file is pretty-printed (contains newlines).
	if !strings.Contains(string(data), "\n") {
		t.Error("expected pretty-printed JSON with newlines")
	}
}

// TestReadMetadata writes a metadata.json to a temp directory directly and
// reads it back via readMetadata, verifying all fields are parsed correctly.
func TestReadMetadata(t *testing.T) {
	dir := t.TempDir()
	raw := `{
  "version": 7,
  "fileName": "image.png",
  "mimeType": "image/png",
  "createTime": 9876543210.0,
  "canonicalUri": "gs://bucket/image.png/7",
  "customMetadata": {"label": "cover"}
}`
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(raw), 0644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	meta, err := readMetadata(dir)
	if err != nil {
		t.Fatalf("readMetadata: %v", err)
	}

	if meta.Version != 7 {
		t.Errorf("Version: got %d, want 7", meta.Version)
	}
	if meta.FileName != "image.png" {
		t.Errorf("FileName: got %q, want image.png", meta.FileName)
	}
	if meta.MimeType != "image/png" {
		t.Errorf("MimeType: got %q, want image/png", meta.MimeType)
	}
	if meta.CreateTime != 9876543210.0 {
		t.Errorf("CreateTime: got %f, want 9876543210.0", meta.CreateTime)
	}
	if meta.CanonicalURI != "gs://bucket/image.png/7" {
		t.Errorf("CanonicalURI: got %q", meta.CanonicalURI)
	}
	if meta.CustomMetadata["label"] != "cover" {
		t.Errorf("CustomMetadata[label]: got %v", meta.CustomMetadata["label"])
	}
}

// TestReadMetadata_NotFound verifies that readMetadata returns an error when
// the target directory (or metadata.json within it) does not exist.
func TestReadMetadata_NotFound(t *testing.T) {
	_, err := readMetadata("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected an error for non-existent path, got nil")
	}
	// The error message should mention "read metadata".
	if !strings.Contains(err.Error(), "read metadata") {
		t.Errorf("error message %q does not contain 'read metadata'", err.Error())
	}
}
