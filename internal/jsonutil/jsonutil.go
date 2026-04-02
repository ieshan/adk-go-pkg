// Package jsonutil provides shared JSON utility functions for adk-go-pkg.
package jsonutil

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// GenerateID generates a random hex ID of the given byte length.
// For example, GenerateID(16) returns a 32-character hex string.
// Uses crypto/rand for cryptographic randomness.
func GenerateID(byteLen int) string {
	b := make([]byte, byteLen)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
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
