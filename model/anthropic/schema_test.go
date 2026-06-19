package anthropic

import (
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
		t.Fatal("expected non-nil schema map")
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
		t.Fatal("expected non-nil schema map")
	}
	if js["type"] != "object" {
		t.Errorf("type: got %v, want %q", js["type"], "object")
	}
	props, ok := js["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
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
		t.Fatal("expected non-nil schema map")
	}
	anyOf, ok := js["anyOf"].([]map[string]any)
	if !ok || len(anyOf) != 2 {
		t.Fatalf("expected anyOf with 2 elements, got %v", js["anyOf"])
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
		t.Fatal("expected non-nil schema map")
	}
	if js["type"] != "array" {
		t.Errorf("type: got %v, want %q", js["type"], "array")
	}
	items, ok := js["items"].(map[string]any)
	if !ok {
		t.Fatal("expected items map")
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
		t.Fatal("expected non-nil schema map")
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
