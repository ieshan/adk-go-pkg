package prompt

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"text/template"
)

// defaultFuncs returns the template function map with pipeline-friendly
// signatures. In text/template pipelines, the pipeline value is passed as the
// last argument to the function.
func defaultFuncs() template.FuncMap {
	return template.FuncMap{
		"tojson":    toJSONFunc,
		"fromjson":  fromJSONFunc,
		"truncate":  truncateFunc,
		"indent":    indentFunc,
		"join":      joinFunc,
		"lower":     strings.ToLower,
		"upper":     strings.ToUpper,
		"trim":      strings.TrimSpace,
		"default":   defaultFunc,
		"contains":  containsFunc,
		"hasprefix": hasPrefixFunc,
		"hassuffix": hasSuffixFunc,
	}
}

// toJSONFunc returns the JSON encoding of v as a string.
func toJSONFunc(v any) (string, error) {
	if v == nil {
		return "null", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("tojson: %w", err)
	}
	return string(b), nil
}

// fromJSONFunc parses a JSON string and returns the decoded value.
func fromJSONFunc(s string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, fmt.Errorf("fromjson: %w", err)
	}
	return v, nil
}

// truncateFunc truncates s to at most n runes, appending "..." if truncated.
func truncateFunc(n int, s string) string {
	runes := []rune(s)
	if n <= 0 {
		return ""
	}
	if len(runes) <= n {
		return s
	}
	if n <= 3 {
		return string(runes[:n]) + "..."
	}
	return string(runes[:n-3]) + "..."
}

// indentFunc prefixes every line of s with prefix.
func indentFunc(prefix string, s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

// joinFunc joins elems with sep.
func joinFunc(sep string, elems []string) string {
	return strings.Join(elems, sep)
}

// containsFunc reports whether substr is present in s.
func containsFunc(substr string, s string) bool {
	return strings.Contains(s, substr)
}

// hasPrefixFunc reports whether s starts with prefix.
func hasPrefixFunc(prefix string, s string) bool {
	return strings.HasPrefix(s, prefix)
}

// hasSuffixFunc reports whether s ends with suffix.
func hasSuffixFunc(suffix string, s string) bool {
	return strings.HasSuffix(s, suffix)
}

// defaultFunc returns def when val is considered empty; otherwise it returns val.
func defaultFunc(def any, val any) any {
	if isEmptyValue(val) {
		return def
	}
	return val
}

// isEmptyValue mirrors the zero value checks used by text/template's "default"
// helper: nil, false, 0, and "" are empty; empty maps and slices are also empty.
func isEmptyValue(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Bool:
		return !rv.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() == 0
	case reflect.Complex64, reflect.Complex128:
		return rv.Complex() == 0
	case reflect.String:
		return rv.String() == ""
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
