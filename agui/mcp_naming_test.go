package agui_test

import (
	"strings"
	"testing"

	"github.com/ieshan/adk-go-pkg/agui"
)

func TestSanitizeSegment(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agui.SanitizeSegment(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeSegment(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMakeUniqueToolName(t *testing.T) {
	t.Run("basic namespacing", func(t *testing.T) {
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
		used := make(map[string]struct{})
		got := agui.MakeUniqueToolName("", "search", used)
		want := "mcp____search"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("truncation", func(t *testing.T) {
		used := make(map[string]struct{})
		longServer := strings.Repeat("a", 100)
		longTool := strings.Repeat("b", 100)
		got := agui.MakeUniqueToolName(longServer, longTool, used)
		if len(got) > 64 {
			t.Errorf("name length %d exceeds max 64", len(got))
		}
	})

	t.Run("dedup with suffix", func(t *testing.T) {
		used := make(map[string]struct{})
		first := agui.MakeUniqueToolName("srv", "tool", used)
		second := agui.MakeUniqueToolName("srv", "tool", used)
		if first == second {
			t.Errorf("expected different names, got %q twice", first)
		}
		wantSecond := "mcp__srv__tool_2"
		if second != wantSecond {
			t.Errorf("second = %q, want %q", second, wantSecond)
		}
	})

	t.Run("dedup with suffix third", func(t *testing.T) {
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
		used := make(map[string]struct{})
		longServer := strings.Repeat("a", 40)
		longTool := strings.Repeat("b", 40)
		first := agui.MakeUniqueToolName(longServer, longTool, used)
		second := agui.MakeUniqueToolName(longServer, longTool, used)
		if len(second) > 64 {
			t.Errorf("second name length %d exceeds max 64", len(second))
		}
		if first == second {
			t.Errorf("expected different names, got %q twice", first)
		}
	})
}

func TestGetServerHash(t *testing.T) {
	cfg := agui.MCPClientConfig{
		Type:     "http",
		URL:      "https://example.com/mcp",
		Headers:  map[string]string{"Authorization": "Bearer token"},
		ServerID: "srv1",
	}

	t.Run("deterministic", func(t *testing.T) {
		h1 := agui.GetServerHash(cfg)
		h2 := agui.GetServerHash(cfg)
		if h1 != h2 {
			t.Errorf("hash not deterministic: %q vs %q", h1, h2)
		}
	})

	t.Run("different config different hash", func(t *testing.T) {
		cfg2 := cfg
		cfg2.URL = "https://other.com/mcp"
		h1 := agui.GetServerHash(cfg)
		h2 := agui.GetServerHash(cfg2)
		if h1 == h2 {
			t.Errorf("different configs produced same hash: %q", h1)
		}
	})

	t.Run("serverID not in hash", func(t *testing.T) {
		cfg2 := cfg
		cfg2.ServerID = "different"
		h1 := agui.GetServerHash(cfg)
		h2 := agui.GetServerHash(cfg2)
		if h1 != h2 {
			t.Errorf("serverID should not affect hash: %q vs %q", h1, h2)
		}
	})

	t.Run("hash length", func(t *testing.T) {
		h := agui.GetServerHash(cfg)
		if len(h) != 16 {
			t.Errorf("hash length = %d, want 16", len(h))
		}
	})
}
