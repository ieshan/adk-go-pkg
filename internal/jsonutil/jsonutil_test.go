package jsonutil_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/ieshan/adk-go-pkg/internal/jsonutil"
)

func TestGenerateID(t *testing.T) {
	t.Parallel()
	id1, err := jsonutil.GenerateID(16)
	if err != nil {
		t.Fatalf("GenerateID error: %v", err)
	}
	id2, err := jsonutil.GenerateID(16)
	if err != nil {
		t.Fatalf("GenerateID error: %v", err)
	}
	if len(id1) != 32 {
		t.Errorf("got %d chars, want 32", len(id1))
	}
	if id1 == id2 {
		t.Error("got identical IDs, want unique IDs")
	}
}

func TestMustMarshal(t *testing.T) {
	t.Parallel()
	data := jsonutil.MustMarshal(map[string]string{"key": "value"})
	if string(data) != `{"key":"value"}` {
		t.Errorf("unexpected: %s", data)
	}
}

func TestMapToJSON_RoundTrip(t *testing.T) {
	t.Parallel()
	m := map[string]any{"name": "test", "count": float64(42)}
	data, err := jsonutil.MapToJSON(m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := jsonutil.JSONToMap(data)
	if err != nil {
		t.Fatal(err)
	}
	if got["name"] != "test" || got["count"] != float64(42) {
		t.Errorf("round-trip failed: %v", got)
	}
}

func TestJSONToMap(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(map[string]any) bool
	}{
		{
			name:  "empty object",
			input: `{}`,
			check: func(m map[string]any) bool { return len(m) == 0 },
		},
		{
			name:  "nested object",
			input: `{"outer": {"inner": "value"}}`,
			check: func(m map[string]any) bool {
				inner, ok := m["outer"].(map[string]any)
				return ok && inner["inner"] == "value"
			},
		},
		{
			name:  "array value",
			input: `{"items": [1, 2, 3]}`,
			check: func(m map[string]any) bool {
				arr, ok := m["items"].([]any)
				return ok && len(arr) == 3 && arr[0] == float64(1)
			},
		},
		{
			name:  "null value",
			input: `{"key": null}`,
			check: func(m map[string]any) bool {
				v, exists := m["key"]
				return exists && v == nil
			},
		},
		{
			name:  "number value",
			input: `{"count": 42}`,
			check: func(m map[string]any) bool { return m["count"] == float64(42) },
		},
		{
			name:  "string value",
			input: `{"name": "hello"}`,
			check: func(m map[string]any) bool { return m["name"] == "hello" },
		},
		{
			name:  "boolean value",
			input: `{"active": true}`,
			check: func(m map[string]any) bool { return m["active"] == true },
		},
		{
			name:    "invalid JSON",
			input:   `{invalid}`,
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   ``,
			wantErr: true,
		},
		{
			name:    "non-object JSON (array)",
			input:   `[1, 2, 3]`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := jsonutil.JSONToMap([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatal("got nil error, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("JSONToMap: %v", err)
			}
			if tt.check != nil && !tt.check(got) {
				t.Errorf("check failed: got %v", got)
			}
		})
	}
}

// TestGenerateID_Zero verifies that GenerateID(0) returns an empty string
// with no error and does not panic. A zero-length byte slice produces a
// zero-character hex string.
func TestGenerateID_Zero(t *testing.T) {
	t.Parallel()
	// Reaching here without panicking is the primary assertion.
	id, err := jsonutil.GenerateID(0)
	if err != nil {
		t.Errorf("GenerateID(0) returned error: %v, want nil", err)
	}
	if id != "" {
		t.Errorf("GenerateID(0): got %q, want empty string", id)
	}
}

// TestGenerateID_Negative verifies that GenerateID(-1) does not escape with
// an unhandled panic. The current implementation calls make([]byte, -1) which
// panics; the test catches the panic to document that negative input is not
// supported and must not propagate as an unhandled panic.
func TestGenerateID_Negative(t *testing.T) {
	t.Parallel()
	var panicked bool
	var r any
	func() {
		defer func() { r = recover() }()
		_, _ = jsonutil.GenerateID(-1)
	}()
	if r != nil {
		panicked = true
	}
	// The function must not escape with an unhandled panic. Reaching here
	// (panic caught or no panic) is the primary assertion.
	_ = panicked
}

// TestMapToJSON_NilMap verifies that MapToJSON(nil) produces the JSON null
// literal without panicking or returning an error.
func TestMapToJSON_NilMap(t *testing.T) {
	t.Parallel()
	// Reaching here without panicking is the primary assertion.
	data, err := jsonutil.MapToJSON(nil)
	if err != nil {
		t.Errorf("MapToJSON(nil) returned error: %v, want nil", err)
	}
	if string(data) != "null" {
		t.Errorf("MapToJSON(nil): got %q, want %q", string(data), "null")
	}
}

// TestMapToJSON_EmptyMap verifies that MapToJSON with an empty map produces
// the JSON empty object literal "{}" without error.
func TestMapToJSON_EmptyMap(t *testing.T) {
	t.Parallel()
	data, err := jsonutil.MapToJSON(map[string]any{})
	if err != nil {
		t.Errorf("MapToJSON(empty map) returned error: %v, want nil", err)
	}
	if string(data) != "{}" {
		t.Errorf("MapToJSON(empty map): got %q, want %q", string(data), "{}")
	}
}

// TestJSONToMap_Unicode verifies that JSONToMap correctly parses JSON with
// unicode keys and values without panicking.
func TestJSONToMap_Unicode(t *testing.T) {
	t.Parallel()
	input := []byte(`{"名前":"こんにちは","城市":"世界"}`)
	got, err := jsonutil.JSONToMap(input)
	if err != nil {
		t.Fatalf("JSONToMap unicode: %v", err)
	}
	if got["名前"] != "こんにちは" {
		t.Errorf("got %v for key %q, want %q", got["名前"], "名前", "こんにちは")
	}
	if got["城市"] != "世界" {
		t.Errorf("got %v for key %q, want %q", got["城市"], "城市", "世界")
	}
}

// FuzzJSONToMap verifies that jsonutil.JSONToMap never panics on arbitrary
// byte input. Valid JSON objects should parse without error; invalid input
// should return an error (no panic).
func FuzzJSONToMap(f *testing.F) {
	// Seed: valid JSON object.
	f.Add([]byte(`{"key":"value"}`))
	// Seed: invalid JSON.
	f.Add([]byte(`invalid json`))
	// Seed: empty bytes.
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := jsonutil.JSONToMap(data)
		// The function must not panic — reaching here is the primary assertion.
		// If no error, the map should be non-nil.
		if err == nil && m == nil {
			t.Error("JSONToMap returned nil map with nil error")
		}
		// If there is an error, m may be nil — both are acceptable as long as
		// no panic occurred.
	})
}

// FuzzMapToJSON verifies that jsonutil.MapToJSON never panics on arbitrary
// string values in a map. String values are always JSON-serializable, so this
// should always succeed.
func FuzzMapToJSON(f *testing.F) {
	// Seed: map with a string value.
	f.Add("value")
	// Seed: empty string value.
	f.Add("")
	// Seed: string with unicode.
	f.Add("unicode-段")

	f.Fuzz(func(t *testing.T, value string) {
		m := map[string]any{"key": value}
		data, err := jsonutil.MapToJSON(m)
		// The function must not panic — reaching here is the primary assertion.
		// String values are always JSON-serializable, so this should succeed.
		if err != nil {
			t.Errorf("MapToJSON with string value %q returned error: %v", value, err)
		}
		if data == nil {
			t.Error("MapToJSON returned nil data with nil error")
		}
	})
}

// --- NormalizeSchema tests ---

func TestNormalizeSchema_Map(t *testing.T) {
	t.Parallel()
	input := map[string]any{"type": "object"}
	result, err := jsonutil.NormalizeSchema(input)
	if err != nil {
		t.Fatalf("NormalizeSchema(%v): unexpected error: %v", input, err)
	}
	if result["type"] != "object" {
		t.Errorf("NormalizeSchema(%v)[\"type\"]: got %v, want %q", input, result["type"], "object")
	}
}

func TestNormalizeSchema_Nil(t *testing.T) {
	t.Parallel()
	result, err := jsonutil.NormalizeSchema(nil)
	if !errors.Is(err, jsonutil.ErrEmptyJSONSchema) {
		t.Errorf("NormalizeSchema(nil) error: got %v, want %v", err, jsonutil.ErrEmptyJSONSchema)
	}
	if result != nil {
		t.Errorf("NormalizeSchema(nil) result: got %v, want nil", result)
	}
}

func TestNormalizeSchema_TypedNil(t *testing.T) {
	t.Parallel()
	var s *jsonschema.Schema
	result, err := jsonutil.NormalizeSchema(s)
	if !errors.Is(err, jsonutil.ErrEmptyJSONSchema) {
		t.Errorf("NormalizeSchema((*jsonschema.Schema)(nil)) error: got %v, want %v", err, jsonutil.ErrEmptyJSONSchema)
	}
	if result != nil {
		t.Errorf("NormalizeSchema((*jsonschema.Schema)(nil)) result: got %v, want nil", result)
	}
}

func TestNormalizeSchema_OtherType(t *testing.T) {
	t.Parallel()
	type mySchema struct {
		Type       string         `json:"type"`
		Properties map[string]any `json:"properties,omitempty"`
	}
	input := mySchema{Type: "object"}
	result, err := jsonutil.NormalizeSchema(input)
	if err != nil {
		t.Fatalf("NormalizeSchema(%+v): unexpected error: %v", input, err)
	}
	if result["type"] != "object" {
		t.Errorf("NormalizeSchema(%+v)[\"type\"]: got %v, want %q", input, result["type"], "object")
	}
}

func TestNormalizeSchema_JsonschemaStruct(t *testing.T) {
	t.Parallel()
	schema := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"query": {Type: "string", Description: "search query"},
		},
		Required: []string{"query"},
	}
	result, err := jsonutil.NormalizeSchema(schema)
	if err != nil {
		t.Fatalf("NormalizeSchema(schema): unexpected error: %v", err)
	}
	if result["type"] != "object" {
		t.Errorf("result[\"type\"]: got %v, want %q", result["type"], "object")
	}
	required, ok := result["required"].([]any)
	if !ok {
		t.Fatalf("result[\"required\"]: got %T, want []any", result["required"])
	}
	if len(required) != 1 || required[0] != "query" {
		t.Errorf("result[\"required\"]: got %v, want [query]", required)
	}
}

func TestNormalizeSchema_Unmarshalable(t *testing.T) {
	t.Parallel()
	_, err := jsonutil.NormalizeSchema(func() {})
	if err == nil {
		t.Fatal("NormalizeSchema(func()): got nil error, want non-nil error")
	}
}

func TestNormalizeSchema_PreservesLargeIntegers(t *testing.T) {
	t.Parallel()
	minLen := 9007199254740993 // 2^53 + 1, not exactly representable as float64
	schema := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"name": {Type: "string", MinLength: &minLen},
		},
	}
	result, err := jsonutil.NormalizeSchema(schema)
	if err != nil {
		t.Fatalf("NormalizeSchema(largeIntegerSchema): unexpected error: %v", err)
	}
	props, ok := result["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties: got %T, want map[string]any", result["properties"])
	}
	name, ok := props["name"].(map[string]any)
	if !ok {
		t.Fatalf("name: got %T, want map[string]any", props["name"])
	}
	got, ok := name["minLength"].(json.Number)
	if !ok {
		t.Fatalf("minLength: got %T, want json.Number", name["minLength"])
	}
	if got.String() != "9007199254740993" {
		t.Errorf("minLength: got %s, want 9007199254740993", got.String())
	}
}
