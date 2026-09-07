package anthropic

import (
	"encoding/json"
	"testing"

	"google.golang.org/genai"
)

func ptr[T any](v T) *T { return &v }

// TestSchemaToJSONSchema_String verifies that a simple string schema with
// description maps correctly to JSON Schema type "string".
func TestSchemaToJSONSchema_String(t *testing.T) {
	s := &genai.Schema{
		Type:        genai.TypeString,
		Description: "A user name",
	}
	js := schemaToJSONSchema(s)
	if js == nil {
		t.Fatal("got nil schema map, want non-nil")
	}
	if js["type"] != "string" {
		t.Errorf("type: got %v, want %q", js["type"], "string")
	}
	if js["description"] != "A user name" {
		t.Errorf("description: got %v, want %q", js["description"], "A user name")
	}
}

// TestSchemaToJSONSchema_Object verifies that an object schema with properties
// and required fields produces the correct nested JSON Schema.
func TestSchemaToJSONSchema_Object(t *testing.T) {
	s := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"name": {Type: genai.TypeString},
			"age":  {Type: genai.TypeInteger},
		},
		Required: []string{"name"},
	}
	js := schemaToJSONSchema(s)
	if js == nil {
		t.Fatal("got nil schema map, want non-nil")
	}
	if js["type"] != "object" {
		t.Errorf("type: got %v, want %q", js["type"], "object")
	}
	props, ok := js["properties"].(map[string]any)
	if !ok {
		t.Fatal("got non-map properties, want map")
	}
	if props["name"].(map[string]any)["type"] != "string" {
		t.Errorf("name.type: got %v, want %q", props["name"].(map[string]any)["type"], "string")
	}
	req, ok := js["required"].([]string)
	if !ok || len(req) != 1 || req[0] != "name" {
		t.Errorf("required: got %v, want [name]", js["required"])
	}
}

// TestSchemaToJSONSchema_Nullable verifies that a nullable string produces the
// anyOf pattern with null.
func TestSchemaToJSONSchema_Nullable(t *testing.T) {
	s := &genai.Schema{
		Type:     genai.TypeString,
		Nullable: ptr(true),
	}
	js := schemaToJSONSchema(s)
	if js == nil {
		t.Fatal("got nil schema map, want non-nil")
	}
	anyOf, ok := js["anyOf"].([]map[string]any)
	if !ok || len(anyOf) != 2 {
		t.Fatalf("got %v for anyOf, want 2 elements", js["anyOf"])
	}
	if anyOf[0]["type"] != "string" {
		t.Errorf("anyOf[0].type: got %v, want %q", anyOf[0]["type"], "string")
	}
	if anyOf[1]["type"] != "null" {
		t.Errorf("anyOf[1].type: got %v, want %q", anyOf[1]["type"], "null")
	}
}

// TestSchemaToJSONSchema_Array verifies that an array schema with items
// produces the correct recursive structure.
func TestSchemaToJSONSchema_Array(t *testing.T) {
	s := &genai.Schema{
		Type: genai.TypeArray,
		Items: &genai.Schema{
			Type: genai.TypeString,
		},
	}
	js := schemaToJSONSchema(s)
	if js == nil {
		t.Fatal("got nil schema map, want non-nil")
	}
	if js["type"] != "array" {
		t.Errorf("type: got %v, want %q", js["type"], "array")
	}
	items, ok := js["items"].(map[string]any)
	if !ok {
		t.Fatal("got non-map items, want map")
	}
	if items["type"] != "string" {
		t.Errorf("items.type: got %v, want %q", items["type"], "string")
	}
}

// TestSchemaToJSONSchema_Complex verifies full feature coverage: enum, format,
// pattern, minimum, maximum, minLength, maxLength.
func TestSchemaToJSONSchema_Complex(t *testing.T) {
	s := &genai.Schema{
		Type:        genai.TypeString,
		Description: "A formatted string",
		Format:      "email",
		Pattern:     "^.*@.*$",
		Enum:        []string{"a", "b"},
		Minimum:     ptr(float64(1)),
		Maximum:     ptr(float64(100)),
		MinLength:   ptr(int64(5)),
		MaxLength:   ptr(int64(50)),
	}
	js := schemaToJSONSchema(s)
	if js == nil {
		t.Fatal("got nil schema map, want non-nil")
	}
	if js["type"] != "string" {
		t.Errorf("type: got %v, want %q", js["type"], "string")
	}
	if js["format"] != "email" {
		t.Errorf("format: got %v, want %q", js["format"], "email")
	}
	if js["pattern"] != "^.*@.*$" {
		t.Errorf("pattern: got %v, want %q", js["pattern"], "^.*@.*$")
	}
	enum, ok := js["enum"].([]string)
	if !ok || len(enum) != 2 {
		t.Errorf("enum: got %v, want 2 elements", js["enum"])
	}
	if js["minimum"] != float64(1) {
		t.Errorf("minimum: got %v, want 1", js["minimum"])
	}
	if js["maximum"] != float64(100) {
		t.Errorf("maximum: got %v, want 100", js["maximum"])
	}
	if js["minLength"] != int64(5) {
		t.Errorf("minLength: got %v, want 5", js["minLength"])
	}
	if js["maxLength"] != int64(50) {
		t.Errorf("maxLength: got %v, want 50", js["maxLength"])
	}
}

// TestSchemaToJSONSchema_Nil verifies that schemaToJSONSchema(nil) returns nil
// without panicking.
func TestSchemaToJSONSchema_Nil(t *testing.T) {
	// Reaching here without panicking is the primary assertion.
	js := schemaToJSONSchema(nil)
	if js != nil {
		t.Errorf("got %v for nil input, want nil", js)
	}
}

// TestSchemaToJSONSchema_Empty verifies that an empty &genai.Schema{} produces
// a non-nil empty map without panicking. With no type, no nullable, and no
// other fields set, the output map should contain no entries.
func TestSchemaToJSONSchema_Empty(t *testing.T) {
	// Reaching here without panicking is the primary assertion.
	js := schemaToJSONSchema(&genai.Schema{})
	if js == nil {
		t.Fatal("got nil schema map, want non-nil empty map")
	}
	if len(js) != 0 {
		t.Errorf("got %d entries for empty schema, want 0: %v", len(js), js)
	}
}

// TestSchemaToJSONSchema_Unicode verifies that a schema with unicode in its
// description preserves the unicode content through conversion.
func TestSchemaToJSONSchema_Unicode(t *testing.T) {
	s := &genai.Schema{
		Type:        genai.TypeString,
		Description: "ユーザー名 — 用户名",
	}
	js := schemaToJSONSchema(s)
	if js == nil {
		t.Fatal("got nil schema map, want non-nil")
	}
	if js["type"] != "string" {
		t.Errorf("type: got %v, want %q", js["type"], "string")
	}
	if js["description"] != "ユーザー名 — 用户名" {
		t.Errorf("description: got %v, want %q", js["description"], "ユーザー名 — 用户名")
	}
}

// FuzzSchemaToJSONSchema verifies that schemaToJSONSchema never panics on
// arbitrary JSON input. Valid schema JSON should parse and translate without
// panicking; invalid input is skipped (no panic).
func FuzzSchemaToJSONSchema(f *testing.F) {
	// Seed: valid schema JSON.
	f.Add([]byte(`{"type":"string","description":"A name"}`))
	// Seed: malformed JSON.
	f.Add([]byte(`invalid json`))
	// Seed: empty object.
	f.Add([]byte(`{}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var s genai.Schema
		if err := json.Unmarshal(data, &s); err != nil {
			// Skip unmarshal failures — expected for random bytes.
			return
		}
		js := schemaToJSONSchema(&s)
		// The function must not panic — reaching here is the primary assertion.
		_ = js
	})
}
