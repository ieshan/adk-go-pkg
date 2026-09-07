package agui_test

import (
	"strings"
	"testing"

	"github.com/ieshan/adk-go-pkg/agui"
)

func TestSanitizeSegment(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "myserver", "myserver"},
		{"with spaces", "my server", "my_server"},
		{"with dots", "server.example.com", "server_example_com"},
		{"with slashes", "a/b/c", "a_b_c"},
		{"leading underscore", "_server", "server"},
		{"trailing underscore", "server_", "server"},
		{"leading/trailing underscore", "_server_", "server"},
		{"empty", "", ""},
		{"only special chars", "!!!", ""},
		{"unicode", "sérver", "s_rver"},
		{"already clean", "my-server_1", "my-server_1"},
		{"single dot", ".", ""},
		{"very long segment", strings.Repeat("a", 100), strings.Repeat("a", 100)},
		{"long segment with invalid chars", "a.b!" + strings.Repeat("a", 60) + "!b.", "a_b_" + strings.Repeat("a", 60) + "_b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := agui.SanitizeSegment(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeSegment(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMakeUniqueToolName(t *testing.T) {
	t.Parallel()
	t.Run("basic namespacing", func(t *testing.T) {
		t.Parallel()
		used := make(map[string]struct{})
		got := agui.MakeUniqueToolName("myserver", "search", used)
		want := "mcp__myserver__search"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if _, ok := used[got]; !ok {
			t.Error("name not added to used map")
		}
	})

	t.Run("empty serverID", func(t *testing.T) {
		t.Parallel()
		used := make(map[string]struct{})
		got := agui.MakeUniqueToolName("", "search", used)
		want := "mcp____search"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("truncation", func(t *testing.T) {
		t.Parallel()
		used := make(map[string]struct{})
		longServer := strings.Repeat("a", 100)
		longTool := strings.Repeat("b", 100)
		got := agui.MakeUniqueToolName(longServer, longTool, used)
		if len(got) > 64 {
			t.Errorf("name length %d exceeds max 64", len(got))
		}
	})

	t.Run("dedup with suffix", func(t *testing.T) {
		t.Parallel()
		used := make(map[string]struct{})
		first := agui.MakeUniqueToolName("srv", "tool", used)
		second := agui.MakeUniqueToolName("srv", "tool", used)
		if first == second {
			t.Errorf("got %q twice, want different names", first)
		}
		wantSecond := "mcp__srv__tool_2"
		if second != wantSecond {
			t.Errorf("second = %q, want %q", second, wantSecond)
		}
	})

	t.Run("dedup with suffix third", func(t *testing.T) {
		t.Parallel()
		used := make(map[string]struct{})
		agui.MakeUniqueToolName("srv", "tool", used)
		agui.MakeUniqueToolName("srv", "tool", used)
		third := agui.MakeUniqueToolName("srv", "tool", used)
		wantThird := "mcp__srv__tool_3"
		if third != wantThird {
			t.Errorf("third = %q, want %q", third, wantThird)
		}
	})

	t.Run("dedup truncation preserves suffix", func(t *testing.T) {
		t.Parallel()
		used := make(map[string]struct{})
		longServer := strings.Repeat("a", 40)
		longTool := strings.Repeat("b", 40)
		first := agui.MakeUniqueToolName(longServer, longTool, used)
		second := agui.MakeUniqueToolName(longServer, longTool, used)
		if len(second) > 64 {
			t.Errorf("second name length %d exceeds max 64", len(second))
		}
		if first == second {
			t.Errorf("got %q twice, want different names", first)
		}
	})

	t.Run("empty tool name", func(t *testing.T) {
		used := make(map[string]struct{})
		got := agui.MakeUniqueToolName("myserver", "", used)
		want := "mcp__myserver__"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("both empty", func(t *testing.T) {
		used := make(map[string]struct{})
		got := agui.MakeUniqueToolName("", "", used)
		want := "mcp____"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("names with special characters", func(t *testing.T) {
		used := make(map[string]struct{})
		got := agui.MakeUniqueToolName("my.server/v2", "search.tool!", used)
		want := "mcp__my_server_v2__search_tool"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestGetServerHash(t *testing.T) {
	t.Parallel()
	cfg := agui.MCPClientConfig{
		Type:     "http",
		URL:      "https://example.com/mcp",
		Headers:  map[string]string{"Authorization": "Bearer token"},
		ServerID: "srv1",
	}

	t.Run("deterministic", func(t *testing.T) {
		t.Parallel()
		h1 := agui.GetServerHash(cfg)
		h2 := agui.GetServerHash(cfg)
		if h1 != h2 {
			t.Errorf("hash not deterministic: %q vs %q", h1, h2)
		}
	})

	t.Run("different config different hash", func(t *testing.T) {
		t.Parallel()
		cfg2 := cfg
		cfg2.URL = "https://other.com/mcp"
		h1 := agui.GetServerHash(cfg)
		h2 := agui.GetServerHash(cfg2)
		if h1 == h2 {
			t.Errorf("different configs produced same hash: %q", h1)
		}
	})

	t.Run("serverID not in hash", func(t *testing.T) {
		t.Parallel()
		cfg2 := cfg
		cfg2.ServerID = "different"
		h1 := agui.GetServerHash(cfg)
		h2 := agui.GetServerHash(cfg2)
		if h1 != h2 {
			t.Errorf("serverID should not affect hash: %q vs %q", h1, h2)
		}
	})

	t.Run("hash length", func(t *testing.T) {
		t.Parallel()
		h := agui.GetServerHash(cfg)
		if len(h) != 16 {
			t.Errorf("hash length = %d, want 16", len(h))
		}
	})
}

// FuzzSanitizeSegment verifies that SanitizeSegment never panics and always
// returns a string containing only [a-zA-Z0-9_-] characters (after trimming
// leading/trailing underscores).
func FuzzSanitizeSegment(f *testing.F) {
	f.Add("valid.segment")
	f.Add("")
	f.Add("unicode-段")

	f.Fuzz(func(t *testing.T, input string) {
		got := agui.SanitizeSegment(input)
		// The result must never contain characters outside [a-zA-Z0-9_-].
		for _, r := range got {
			if (r < 'a' || r > 'z') &&
				(r < 'A' || r > 'Z') &&
				(r < '0' || r > '9') &&
				r != '_' && r != '-' {
				t.Errorf("SanitizeSegment(%q) = %q contains invalid rune %q", input, got, r)
			}
		}
		// Result must not have leading or trailing underscores.
		if len(got) > 0 && (got[0] == '_' || got[len(got)-1] == '_') {
			t.Errorf("SanitizeSegment(%q) = %q has leading/trailing underscore", input, got)
		}
	})
}

// FuzzMakeUniqueToolName verifies that MakeUniqueToolName never panics and
// always returns a non-empty name within the max length constraint.
func FuzzMakeUniqueToolName(f *testing.F) {
	f.Add("server1", "tool1")
	f.Add("", "")
	f.Add("server", "tool")

	f.Fuzz(func(t *testing.T, serverID, toolName string) {
		used := make(map[string]struct{})
		got := agui.MakeUniqueToolName(serverID, toolName, used)
		if got == "" {
			t.Errorf("MakeUniqueToolName(%q, %q) returned empty name", serverID, toolName)
		}
		if len(got) > 64 {
			t.Errorf("MakeUniqueToolName(%q, %q) = %q exceeds max length 64", serverID, toolName, got)
		}
		// The returned name must be recorded in the used map.
		if _, ok := used[got]; !ok {
			t.Errorf("MakeUniqueToolName(%q, %q) = %q not added to used map", serverID, toolName, got)
		}
	})
}
