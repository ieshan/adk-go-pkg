// Package jsonutil provides shared JSON utility functions for adk-go-pkg.
package jsonutil

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrEmptyJSONSchema is returned by NormalizeSchema when the input is nil or encodes to null.
var ErrEmptyJSONSchema = errors.New("jsonutil: empty json schema")

// GenerateID generates a random hex ID of the given byte length.
// For example, GenerateID(16) returns a 32-character hex string.
// Uses crypto/rand for cryptographic randomness.
func GenerateID(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// MustMarshal marshals v to JSON, panicking on error.
// Only use in test code or for values known to be safe.
func MustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("jsonutil.MustMarshal: %v", err))
	}
	return data
}

// MapToJSON converts a map to a JSON-encoded byte slice.
func MapToJSON(m map[string]any) ([]byte, error) {
	return json.Marshal(m)
}

// JSONToMap converts JSON bytes to a map.
func JSONToMap(data []byte) (map[string]any, error) {
	var m map[string]any
	err := json.Unmarshal(data, &m)
	return m, err
}

// NormalizeSchema converts an arbitrary schema value (any) into a
// map[string]any suitable for JSON serialization. This handles the
// *jsonschema.Schema type that ADK's functiontool produces, as well as
// pre-built map[string]any schemas and any other JSON-serializable value.
//
// Returns ErrEmptyJSONSchema when schema is nil, typed nil, or marshals to "null".
// Returns the map directly when schema is already map[string]any.
// Otherwise marshals to JSON and unmarshals into map[string]any using
// UseNumber to preserve large integer precision.
func NormalizeSchema(schema any) (map[string]any, error) {
	if schema == nil {
		return nil, ErrEmptyJSONSchema
	}
	if m, ok := schema.(map[string]any); ok {
		return m, nil
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("jsonutil: marshal schema: %w", err)
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, ErrEmptyJSONSchema
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var m map[string]any
	if err := decoder.Decode(&m); err != nil {
		return nil, fmt.Errorf("jsonutil: unmarshal schema: %w", err)
	}
	if m == nil {
		return nil, ErrEmptyJSONSchema
	}
	return m, nil
}
