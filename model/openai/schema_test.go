package openai

import (
	"encoding/json"
	"testing"

	"google.golang.org/genai"
)

// TestSchemaToJSONSchema_String verifies that a simple string schema with a
// description is translated correctly.
func TestSchemaToJSONSchema_String(t *testing.T) {
	s := &genai.Schema{
		Type:        genai.TypeString,
		Description: "A user name",
	}

	got := schemaToJSONSchema(s)

	if got == nil {
		t.Fatal("got nil result, want non-nil")
	}
	if got["type"] != "string" {
		t.Errorf("type: got %v, want %q", got["type"], "string")
	}
	if got["description"] != "A user name" {
		t.Errorf("description: got %v, want %q", got["description"], "A user name")
	}
}

// TestSchemaToJSONSchema_Object verifies that an object schema with properties
// and required fields is translated correctly.
func TestSchemaToJSONSchema_Object(t *testing.T) {
	s := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"name": {Type: genai.TypeString},
			"age":  {Type: genai.TypeInteger},
		},
		Required: []string{"name"},
	}

	got := schemaToJSONSchema(s)

	if got["type"] != "object" {
		t.Errorf("type: got %v, want %q", got["type"], "object")
	}

	props, ok := got["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties: got %T, want map[string]any", got["properties"])
	}
	if _, exists := props["name"]; !exists {
		t.Error("properties: missing 'name'")
	}
	if _, exists := props["age"]; !exists {
		t.Error("properties: missing 'age'")
	}

	nameSchema, ok := props["name"].(map[string]any)
	if !ok {
		t.Fatalf("properties.name: got %T, want map[string]any", props["name"])
	}
	if nameSchema["type"] != "string" {
		t.Errorf("properties.name.type: got %v, want %q", nameSchema["type"], "string")
	}

	required, ok := got["required"].([]string)
	if !ok {
		t.Fatalf("required: got %T, want []string", got["required"])
	}
	if len(required) != 1 || required[0] != "name" {
		t.Errorf("required: got %v, want [name]", required)
	}
}

// TestSchemaToJSONSchema_Nullable verifies that a nullable schema produces the
// anyOf pattern with two elements: the original type and {type: "null"}.
func TestSchemaToJSONSchema_Nullable(t *testing.T) {
	s := &genai.Schema{
		Type:     genai.TypeString,
		Nullable: new(true),
	}

	got := schemaToJSONSchema(s)

	if _, hasType := got["type"]; hasType {
		t.Error("nullable schema should not have a top-level 'type' field")
	}

	anyOf, ok := got["anyOf"].([]map[string]any)
	if !ok {
		t.Fatalf("anyOf: got %T, want []map[string]any", got["anyOf"])
	}
	if len(anyOf) != 2 {
		t.Fatalf("anyOf: got %d elements, want 2", len(anyOf))
	}

	// First element should be the original type.
	if anyOf[0]["type"] != "string" {
		t.Errorf("anyOf[0].type: got %v, want %q", anyOf[0]["type"], "string")
	}
	// Second element should be {type: "null"}.
	if anyOf[1]["type"] != "null" {
		t.Errorf("anyOf[1].type: got %v, want %q", anyOf[1]["type"], "null")
	}
}

// TestSchemaToJSONSchema_Array verifies that an array schema with an items
// sub-schema is translated correctly.
func TestSchemaToJSONSchema_Array(t *testing.T) {
	s := &genai.Schema{
		Type:  genai.TypeArray,
		Items: &genai.Schema{Type: genai.TypeNumber},
	}

	got := schemaToJSONSchema(s)

	if got["type"] != "array" {
		t.Errorf("type: got %v, want %q", got["type"], "array")
	}

	items, ok := got["items"].(map[string]any)
	if !ok {
		t.Fatalf("items: got %T, want map[string]any", got["items"])
	}
	if items["type"] != "number" {
		t.Errorf("items.type: got %v, want %q", items["type"], "number")
	}
}

// TestSchemaToJSONSchema_Constraints verifies that string-level constraint
// fields (minLength, maxLength, pattern) are included in the output.
func TestSchemaToJSONSchema_Constraints(t *testing.T) {
	s := &genai.Schema{
		Type:      genai.TypeString,
		MinLength: new(int64(3)),
		MaxLength: new(int64(50)),
		Pattern:   `^[a-z]+$`,
	}

	got := schemaToJSONSchema(s)

	if got["minLength"] != int64(3) {
		t.Errorf("minLength: got %v (%T), want 3", got["minLength"], got["minLength"])
	}
	if got["maxLength"] != int64(50) {
		t.Errorf("maxLength: got %v (%T), want 50", got["maxLength"], got["maxLength"])
	}
	if got["pattern"] != `^[a-z]+$` {
		t.Errorf("pattern: got %v, want %q", got["pattern"], `^[a-z]+$`)
	}
}

// TestSchemaToJSONSchema_Enum verifies that enum values are included in the
// translated output.
func TestSchemaToJSONSchema_Enum(t *testing.T) {
	s := &genai.Schema{
		Type: genai.TypeString,
		Enum: []string{"north", "south", "east", "west"},
	}

	got := schemaToJSONSchema(s)

	enum, ok := got["enum"].([]string)
	if !ok {
		t.Fatalf("enum: got %T, want []string", got["enum"])
	}
	if len(enum) != 4 {
		t.Fatalf("enum: got %d values, want 4", len(enum))
	}
	if enum[0] != "north" || enum[3] != "west" {
		t.Errorf("enum: unexpected values %v", enum)
	}
}

// TestSchemaToJSONSchema_NullableWithAnyOf verifies that when a schema has both
// AnyOf and Nullable, the null type is appended to the existing anyOf entries.
func TestSchemaToJSONSchema_NullableWithAnyOf(t *testing.T) {
	s := &genai.Schema{
		Nullable: new(true),
		AnyOf: []*genai.Schema{
			{Type: genai.TypeString},
			{Type: genai.TypeInteger},
		},
	}
	got := schemaToJSONSchema(s)
	anyOf, ok := got["anyOf"].([]map[string]any)
	if !ok {
		t.Fatalf("got %T: %v for anyOf, want array", got["anyOf"], got["anyOf"])
	}
	// Should have 3 entries: string, integer, null
	if len(anyOf) != 3 {
		t.Errorf("got %d anyOf entries: %v, want 3 (string, integer, null)", len(anyOf), anyOf)
	}
	// Last entry should be null
	if anyOf[len(anyOf)-1]["type"] != "null" {
		t.Errorf("got %v for last anyOf entry, want null", anyOf[len(anyOf)-1])
	}
}

// TestSchemaToJSONSchema_Nil verifies that a nil input returns nil.
func TestSchemaToJSONSchema_Nil(t *testing.T) {
	got := schemaToJSONSchema(nil)
	if got != nil {
		t.Errorf("got %v for nil input, want nil", got)
	}
}

// TestSchemaToJSONSchema_Empty verifies that an empty &genai.Schema{} produces
// a non-nil empty map without panicking. With no type, no nullable, and no
// other fields set, the output map should contain no entries.
func TestSchemaToJSONSchema_Empty(t *testing.T) {
	// Reaching here without panicking is the primary assertion.
	got := schemaToJSONSchema(&genai.Schema{})
	if got == nil {
		t.Fatal("got nil schema map, want non-nil empty map")
	}
	if len(got) != 0 {
		t.Errorf("got %d entries for empty schema, want 0: %v", len(got), got)
	}
}

// TestSchemaToJSONSchema_Unicode verifies that a schema with unicode in its
// description preserves the unicode content through conversion.
func TestSchemaToJSONSchema_Unicode(t *testing.T) {
	s := &genai.Schema{
		Type:        genai.TypeString,
		Description: "ユーザー名 — 用户名",
	}
	got := schemaToJSONSchema(s)
	if got == nil {
		t.Fatal("got nil schema map, want non-nil")
	}
	if got["type"] != "string" {
		t.Errorf("type: got %v, want %q", got["type"], "string")
	}
	if got["description"] != "ユーザー名 — 用户名" {
		t.Errorf("description: got %v, want %q", got["description"], "ユーザー名 — 用户名")
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
