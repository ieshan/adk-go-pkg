package eval

import (
	"fmt"
	"strings"
)

// ValidatePathSegment validates that a string is safe to use as a path
// segment. It rejects empty strings, null bytes, path separators, and
// directory traversal sequences.
func ValidatePathSegment(value, fieldName string) error {
	if value == "" {
		return fmt.Errorf("%s cannot be empty", fieldName)
	}
	if strings.ContainsRune(value, 0) {
		return fmt.Errorf("%s contains null byte", fieldName)
	}
	if strings.Contains(value, "/") {
		return fmt.Errorf("%s contains path separator '/'", fieldName)
	}
	if strings.Contains(value, "\\") {
		return fmt.Errorf("%s contains path separator '\\'", fieldName)
	}
	if value == "." || value == ".." {
		return fmt.Errorf("%s cannot be '.' or '..'", fieldName)
	}
	return nil
}
