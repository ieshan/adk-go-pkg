// Package openai provides adapters and helpers for integrating ADK-Go agents
// with OpenAI-compatible APIs, including schema translation utilities.
package openai

import "google.golang.org/genai"

// typeMapping translates genai.Type constants to JSON Schema type strings.
var typeMapping = map[genai.Type]string{
	genai.TypeString:  "string",
	genai.TypeNumber:  "number",
	genai.TypeInteger: "integer",
	genai.TypeBoolean: "boolean",
	genai.TypeArray:   "array",
	genai.TypeObject:  "object",
}

// schemaToJSONSchema recursively converts a [genai.Schema] into a JSON Schema
// representation as a map suitable for JSON serialisation.
//
// Nullable fields are expressed using the anyOf pattern:
//
//	{"anyOf": [{"type": "<original>"}, {"type": "null"}]}
//
// Example — translating a simple string schema:
//
//	s := &genai.Schema{
//	    Type:        genai.TypeString,
//	    Description: "A user name",
//	}
//	js := schemaToJSONSchema(s)
//	// js == map[string]any{"type": "string", "description": "A user name"}
//
// Example — translating a nullable integer:
//
//	s := &genai.Schema{Type: genai.TypeInteger, Nullable: ptr(true)}
//	js := schemaToJSONSchema(s)
//	// js == map[string]any{
//	//     "anyOf": []map[string]any{
//	//         {"type": "integer"},
//	//         {"type": "null"},
//	//     },
//	// }
//
// Returns nil when s is nil.
func schemaToJSONSchema(s *genai.Schema) map[string]any {
	if s == nil {
		return nil
	}

	out := make(map[string]any)

	// Resolve the JSON Schema type string.
	typeName := typeMapping[s.Type]

	// Handle AnyOf from the source schema (recursive).
	if len(s.AnyOf) > 0 {
		converted := make([]map[string]any, 0, len(s.AnyOf))
		for _, sub := range s.AnyOf {
			if c := schemaToJSONSchema(sub); c != nil {
				converted = append(converted, c)
			}
		}
		out["anyOf"] = converted
	}

	// Apply nullable wrapping using the anyOf pattern.
	if s.Nullable != nil && *s.Nullable {
		if existing, ok := out["anyOf"]; ok {
			existingAnyOf := existing.([]map[string]any)
			out["anyOf"] = append(existingAnyOf, map[string]any{"type": "null"})
		} else if typeName != "" {
			baseSchema := map[string]any{"type": typeName}
			out["anyOf"] = []map[string]any{baseSchema, {"type": "null"}}
		}
	} else if typeName != "" {
		out["type"] = typeName
	}

	// Scalar fields.
	if s.Description != "" {
		out["description"] = s.Description
	}
	if s.Format != "" {
		out["format"] = s.Format
	}
	if s.Pattern != "" {
		out["pattern"] = s.Pattern
	}
	if s.Title != "" {
		out["title"] = s.Title
	}

	// Enum.
	if len(s.Enum) > 0 {
		out["enum"] = s.Enum
	}

	// Required.
	if len(s.Required) > 0 {
		out["required"] = s.Required
	}

	// Default / Example.
	if s.Default != nil {
		out["default"] = s.Default
	}
	if s.Example != nil {
		out["example"] = s.Example
	}

	// Numeric constraints.
	if s.Minimum != nil {
		out["minimum"] = *s.Minimum
	}
	if s.Maximum != nil {
		out["maximum"] = *s.Maximum
	}

	// String length constraints.
	if s.MinLength != nil {
		out["minLength"] = *s.MinLength
	}
	if s.MaxLength != nil {
		out["maxLength"] = *s.MaxLength
	}

	// Array item count constraints.
	if s.MinItems != nil {
		out["minItems"] = *s.MinItems
	}
	if s.MaxItems != nil {
		out["maxItems"] = *s.MaxItems
	}

	// Recursive Items (array element schema).
	if s.Items != nil {
		out["items"] = schemaToJSONSchema(s.Items)
	}

	// Recursive Properties (object property schemas).
	if len(s.Properties) > 0 {
		props := make(map[string]any, len(s.Properties))
		for k, v := range s.Properties {
			props[k] = schemaToJSONSchema(v)
		}
		out["properties"] = props
	}

	return out
}
