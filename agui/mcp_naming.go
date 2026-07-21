package agui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// SanitizeSegment replaces characters outside [a-zA-Z0-9_-] with underscores
// and trims leading/trailing underscores.
func SanitizeSegment(s string) string {
	s = sanitizeRe.ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}

// MakeUniqueToolName builds a namespaced tool name in the format
// mcp__{sanitizedServerID}__{sanitizedToolName}, truncated to maxToolNameLength.
// If the name collides with an entry in used, a _N suffix is appended until unique.
// The used map is updated with the final name.
func MakeUniqueToolName(serverID, toolName string, used map[string]struct{}) string {
	base := fmt.Sprintf("%s__%s__%s", mcpToolNamePrefix, SanitizeSegment(serverID), SanitizeSegment(toolName))
	if len(base) > maxToolNameLength {
		base = base[:maxToolNameLength]
	}
	name := base
	for i := 2; ; i++ {
		if _, exists := used[name]; !exists {
			used[name] = struct{}{}
			return name
		}
		suffix := fmt.Sprintf("_%d", i)
		name = base
		if len(name)+len(suffix) > maxToolNameLength {
			name = name[:maxToolNameLength-len(suffix)]
		}
		name = name + suffix
	}
}

// GetServerHash returns a deterministic hash of the server config for
// proxied request routing. It uses SHA-256 of the JSON-serialized config
// fields, returning the first 16 hex characters.
func GetServerHash(config MCPClientConfig) string {
	data, err := json.Marshal(struct {
		Type    string            `json:"type"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}{
		Type:    config.Type,
		URL:     config.URL,
		Headers: config.Headers,
	})
	if err != nil {
		return ""
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])[:16]
}
