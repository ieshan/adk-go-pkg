package file

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// sampleMeta returns a fully-populated VersionMetadata for use in tests.
func sampleMeta() *VersionMetadata {
	return &VersionMetadata{
		Version:      3,
		FileName:     "report.pdf",
		MimeType:     "application/pdf",
		CreateTime:   time.Date(2023, 11, 14, 22, 13, 20, 123000000, time.UTC),
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
		{"createTime", "2023-11-14T22:13:20.123Z"},
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
		CreateTime:   time.Unix(1000000, 0),
		CanonicalURI: "gs://bucket/empty.txt/1",
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal into map: %v", err)
	}
	if _, ok := got["mimeType"]; ok {
		t.Errorf("got %s, want mimeType to be omitted", data)
	}
	if _, ok := got["customMetadata"]; ok {
		t.Errorf("got %s, want customMetadata to be omitted", data)
	}
}

// TestVersionMetadata_UnmarshalJSON verifies that JSON bytes unmarshal into a
// VersionMetadata struct with correct field values.
func TestVersionMetadata_UnmarshalJSON(t *testing.T) {
	raw := `{
		"version": 5,
		"fileName": "data.csv",
		"mimeType": "text/csv",
		"createTime": "2009-02-13T23:31:30.5Z",
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
	if !meta.CreateTime.Equal(time.Date(2009, 2, 13, 23, 31, 30, 500000000, time.UTC)) {
		t.Errorf("CreateTime: got %v, want %v", meta.CreateTime, time.Date(2009, 2, 13, 23, 31, 30, 500000000, time.UTC))
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
	if !restored.CreateTime.Equal(original.CreateTime) {
		t.Errorf("CreateTime mismatch: got %v, want %v", restored.CreateTime, original.CreateTime)
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

// newTestService creates a Service backed by a temp directory and registers
// cleanup to close the root.
func newTestService(t *testing.T) *Service {
	t.Helper()
	svc, err := New(Config{RootDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

// TestWriteMetadata writes a VersionMetadata via (*Service).writeMetadata and
// then reads the raw file back to verify its contents.
func TestWriteMetadata(t *testing.T) {
	svc := newTestService(t)
	meta := sampleMeta()

	if err := svc.writeMetadata(".", meta); err != nil {
		t.Fatalf("writeMetadata: %v", err)
	}

	// Read the raw file back via the service root to verify the JSON.
	data, err := svc.root.ReadFile("metadata.json")
	if err != nil {
		t.Fatalf("read metadata.json: %v", err)
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
}

// TestReadMetadata writes a metadata.json directly to the service root and
// reads it back via (*Service).readMetadata, verifying all fields are parsed
// correctly.
func TestReadMetadata(t *testing.T) {
	svc := newTestService(t)
	raw := `{
  "version": 7,
  "fileName": "image.png",
  "mimeType": "image/png",
  "createTime": "2286-11-20T17:33:30Z",
  "canonicalUri": "gs://bucket/image.png/7",
  "customMetadata": {"label": "cover"}
}`
	if err := svc.root.WriteFile("metadata.json", []byte(raw), 0600); err != nil {
		t.Fatalf("write metadata.json: %v", err)
	}

	meta, err := svc.readMetadata(".")
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
	if !meta.CreateTime.Equal(time.Date(2286, 11, 20, 17, 33, 30, 0, time.UTC)) {
		t.Errorf("CreateTime: got %v, want %v", meta.CreateTime, time.Date(2286, 11, 20, 17, 33, 30, 0, time.UTC))
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
	svc := newTestService(t)

	_, err := svc.readMetadata("nonexistent")
	if err == nil {
		t.Fatal("got nil error, want error for non-existent path")
	}
	// The error should wrap ErrReadMetadata.
	if !errors.Is(err, ErrReadMetadata) {
		t.Errorf("error %v does not wrap ErrReadMetadata", err)
	}
}
