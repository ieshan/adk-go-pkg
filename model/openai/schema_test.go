package openai

import (
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
		t.Fatal("expected non-nil result")
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
		t.Fatalf("properties: expected map[string]any, got %T", got["properties"])
	}
	if _, exists := props["name"]; !exists {
		t.Error("properties: missing 'name'")
	}
	if _, exists := props["age"]; !exists {
		t.Error("properties: missing 'age'")
	}

	nameSchema, ok := props["name"].(map[string]any)
	if !ok {
		t.Fatalf("properties.name: expected map[string]any, got %T", props["name"])
	}
	if nameSchema["type"] != "string" {
		t.Errorf("properties.name.type: got %v, want %q", nameSchema["type"], "string")
	}

	required, ok := got["required"].([]string)
	if !ok {
		t.Fatalf("required: expected []string, got %T", got["required"])
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
		t.Fatalf("anyOf: expected []map[string]any, got %T", got["anyOf"])
	}
	if len(anyOf) != 2 {
		t.Fatalf("anyOf: expected 2 elements, got %d", len(anyOf))
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
		t.Fatalf("items: expected map[string]any, got %T", got["items"])
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
		t.Fatalf("enum: expected []string, got %T", got["enum"])
	}
	if len(enum) != 4 {
		t.Fatalf("enum: expected 4 values, got %d", len(enum))
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
		t.Fatalf("expected anyOf array, got %T: %v", got["anyOf"], got["anyOf"])
	}
	// Should have 3 entries: string, integer, null
	if len(anyOf) != 3 {
		t.Errorf("expected 3 anyOf entries (string, integer, null), got %d: %v", len(anyOf), anyOf)
	}
	// Last entry should be null
	if anyOf[len(anyOf)-1]["type"] != "null" {
		t.Errorf("expected last anyOf entry to be null, got %v", anyOf[len(anyOf)-1])
	}
}

// TestSchemaToJSONSchema_Nil verifies that a nil input returns nil.
func TestSchemaToJSONSchema_Nil(t *testing.T) {
	got := schemaToJSONSchema(nil)
	if got != nil {
		t.Errorf("expected nil for nil input, got %v", got)
	}
}
