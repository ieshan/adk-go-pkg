package prompt

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"text/template"

	"github.com/ieshan/adk-go-pkg/testutil"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/memory"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

// fullContext wraps a FakeReadonlyContext and provides the extra methods needed
// for the extendedContext type assertion used by BuildDataFromReadonlyContext.
type fullContext struct {
	*testutil.FakeReadonlyContext
	art agent.Artifacts
	mem agent.Memory
	ses session.Session
	ag  agent.Agent
}

func (c *fullContext) Artifacts() agent.Artifacts { return c.art }
func (c *fullContext) Memory() agent.Memory       { return c.mem }
func (c *fullContext) Session() session.Session   { return c.ses }
func (c *fullContext) Agent() agent.Agent         { return c.ag }

func newFullContext(t *testing.T) *fullContext {
	t.Helper()
	return &fullContext{
		FakeReadonlyContext: testutil.NewFakeReadonlyContext().
			WithAgentName("test-agent").
			WithAppName("test-app").
			WithUserID("test-user").
			WithSessionID("test-session").
			WithReadonlyState(testutil.NewFakeStateWithData(map[string]any{
				"user_name": "Alice",
				"country":   "Wonderland",
			})),
	}
}

func TestEngineParseExecute(t *testing.T) {
	e := New()

	t.Run("substitution", func(t *testing.T) {
		tmpl, err := e.Parse("sub", "Hello {{.Input.name}}!")
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		got, err := tmpl.Execute(BuildData(map[string]any{"name": "world"}))
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		if got != "Hello world!" {
			t.Errorf("got %q, want %q", got, "Hello world!")
		}
	})

	t.Run("conditional", func(t *testing.T) {
		tmpl := e.MustParse("cond", "{{if .State.Has \"x\"}}yes{{else}}no{{end}}")
		d := BuildData(nil)
		d.State = &StateData{state: testutil.NewFakeStateWithData(map[string]any{"x": 1})}
		got, err := tmpl.Execute(d)
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		if got != "yes" {
			t.Errorf("with key: got %q, want yes", got)
		}

		d.State = &StateData{state: testutil.NewFakeStateWithData(map[string]any{})}
		got, err = tmpl.Execute(d)
		if err != nil {
			t.Fatalf("execute missing: %v", err)
		}
		if got != "no" {
			t.Errorf("without key: got %q, want no", got)
		}
	})

	t.Run("range", func(t *testing.T) {
		tmpl := e.MustParse("range", "{{range $k, $v := .State.All}}{{$k}}={{$v}};{{end}}")
		d := BuildData(nil)
		d.State = &StateData{state: testutil.NewFakeStateWithData(map[string]any{"a": 1, "b": 2})}
		got, err := tmpl.Execute(d)
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		if !strings.Contains(got, "a=1") || !strings.Contains(got, "b=2") {
			t.Errorf("range output missing keys: %q", got)
		}
	})

	t.Run("pipeline", func(t *testing.T) {
		tmpl := e.MustParse("pipe", "{{.Input.text | upper | truncate 5}}")
		got, err := tmpl.Execute(BuildData(map[string]any{"text": "hello world"}))
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		if got != "HE..." {
			t.Errorf("got %q, want %q", got, "HE...")
		}
	})
}

func TestEngineParseError(t *testing.T) {
	_, err := New().Parse("bad", "{{if}}")
	if err == nil {
		t.Error("expected parse error")
	}
}

func TestEngineExecuteError(t *testing.T) {
	tmpl := New().MustParse("bad", "{{.Input.Value | tojson}}")
	_, err := tmpl.Execute(BuildData(map[string]any{"Value": make(chan int)}))
	if err == nil {
		t.Error("expected execute error from json.Marshal")
	}
}

func TestEngineWithFuncs(t *testing.T) {
	e := New(WithFuncs(template.FuncMap{
		"shout": strings.ToUpper,
	}))
	tmpl := e.MustParse("custom", "{{shout .Input.s}}")
	got, err := tmpl.Execute(BuildData(map[string]any{"s": "hello"}))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got != "HELLO" {
		t.Errorf("got %q, want HELLO", got)
	}
}

func TestBuildData(t *testing.T) {
	d := BuildData(map[string]any{"x": "y"})
	if d.Input["x"] != "y" {
		t.Errorf("Input not set correctly")
	}
	if d.State != nil || d.User != nil || d.Session != nil || d.Agent != nil || d.Memory != nil || d.App != nil {
		t.Error("BuildData should only set Input")
	}
}

func TestBuildDataFromReadonlyContext(t *testing.T) {
	ctx := testutil.NewFakeReadonlyContext().
		WithAgentName("ro-agent").
		WithAppName("ro-app").
		WithUserID("ro-user").
		WithSessionID("ro-session").
		WithReadonlyState(testutil.NewFakeStateWithData(map[string]any{"k": "v"})).
		WithUserContent(genai.NewContentFromText("hello", "user"))

	d := BuildDataFromReadonlyContext(ctx)
	if d.State.Get("k") != "v" {
		t.Errorf("state value missing")
	}
	if d.User.Text != "hello" {
		t.Errorf("user text = %q", d.User.Text)
	}
	if d.Session.ID != "ro-session" || d.Session.AppName != "ro-app" || d.Session.UserID != "ro-user" {
		t.Errorf("session fields wrong: %+v", d.Session)
	}
	if d.Agent.Name != "ro-agent" {
		t.Errorf("agent name = %q", d.Agent.Name)
	}
	if d.App.Name != "ro-app" {
		t.Errorf("app name = %q", d.App.Name)
	}
	if d.Memory != nil {
		t.Error("readonly context without extended methods should not populate Memory")
	}
	if _, err := d.Artifact("missing"); err == nil {
		t.Error("expected artifact error when no artifact source available")
	}
}

func TestBuildDataFromExtendedContext(t *testing.T) {
	ctx := newFullContext(t)

	fakeAgent, err := testutil.NewFakeAgent("agent")
	if err != nil {
		t.Fatalf("fake agent: %v", err)
	}
	fakeAgent.WithDescription("An agent for testing")
	ctx.ag = fakeAgent

	artifactSvc := testutil.NewFakeArtifactService()
	artifactSvc.PreloadArtifact("test-app", "test-user", "test-session", "doc", genai.NewPartFromText("artifact content"))
	ctx.art = testutil.NewFakeArtifacts(artifactSvc, "test-app", "test-user", "test-session")

	memorySvc := testutil.NewFakeMemoryService()
	memorySvc.PreloadMemory("test-user", "test-app", memory.Entry{
		Content: genai.NewContentFromText("past note", "model"),
	})
	ctx.mem = testutil.NewFakeMemory(memorySvc, "test-user", "test-app")

	d := BuildDataFromReadonlyContext(ctx)

	if d.Agent.Description != "An agent for testing" {
		t.Errorf("agent description = %q", d.Agent.Description)
	}

	text, err := d.Artifact("doc")
	if err != nil {
		t.Fatalf("artifact load: %v", err)
	}
	if text != "artifact content" {
		t.Errorf("artifact = %q", text)
	}

	mems, err := d.Memory.Search("note")
	if err != nil {
		t.Fatalf("memory search: %v", err)
	}
	if len(mems) != 1 || mems[0] != "past note" {
		t.Errorf("memory = %v", mems)
	}
}

func TestBuildDataFromInvocationContext(t *testing.T) {
	fakeAgent, err := testutil.NewFakeAgent("inv-agent")
	if err != nil {
		t.Fatalf("fake agent: %v", err)
	}
	fakeAgent.WithDescription("Invocation context agent")

	artifactSvc := testutil.NewFakeArtifactService()
	artifactSvc.PreloadArtifact("test-app", "test-user", "test-session", "report", genai.NewPartFromText("report content"))

	ic := testutil.NewFakeInvocationContext().
		WithAgent(fakeAgent).
		WithArtifacts(testutil.NewFakeArtifacts(artifactSvc, "test-app", "test-user", "test-session")).
		WithUserContent(genai.NewContentFromText("hello", "user"))

	d := BuildDataFromInvocationContext(ic)

	// Session and Agent are populated from InvocationContext methods directly.
	if d.Session.ID != "test-session" {
		t.Errorf("session ID = %q, want %q", d.Session.ID, "test-session")
	}
	if d.Session.AppName != "test-app" {
		t.Errorf("session app = %q, want %q", d.Session.AppName, "test-app")
	}
	if d.Agent.Name != "inv-agent" {
		t.Errorf("agent name = %q, want %q", d.Agent.Name, "inv-agent")
	}
	if d.Agent.Description != "Invocation context agent" {
		t.Errorf("agent description = %q", d.Agent.Description)
	}

	// Artifacts are populated from InvocationContext.Artifacts().
	text, err := d.Artifact("report")
	if err != nil {
		t.Fatalf("artifact load: %v", err)
	}
	if text != "report content" {
		t.Errorf("artifact = %q, want %q", text, "report content")
	}
}

func TestBuildDataFromInvocationContext_Nil(t *testing.T) {
	d := BuildDataFromInvocationContext(nil)
	if d == nil {
		t.Fatal("expected non-nil TemplateData")
	}
	if d.State != nil || d.Session != nil || d.Agent != nil {
		t.Error("expected nil fields for nil context")
	}
}

func TestStateData(t *testing.T) {
	state := testutil.NewFakeStateWithData(map[string]any{
		"key":  42,
		"name": "Bob",
	})
	sd := &StateData{state: state}

	if sd.Get("missing") != nil {
		t.Errorf("missing key should return nil")
	}
	if sd.Get("key") != 42 {
		t.Errorf("key value wrong")
	}
	if !sd.Has("key") || sd.Has("missing") {
		t.Error("Has logic wrong")
	}

	all := sd.All()
	if len(all) != 2 || all["key"] != 42 {
		t.Errorf("All() snapshot wrong: %v", all)
	}
}

func TestArtifactOptional(t *testing.T) {
	d := &TemplateData{ctx: context.Background()}
	if _, err := d.Artifact("missing"); err == nil {
		t.Error("expected error for missing artifact with no source")
	}
	text, err := d.Artifact("missing", true)
	if err != nil {
		t.Errorf("optional artifact should suppress error: %v", err)
	}
	if text != "" {
		t.Errorf("optional missing artifact should be empty")
	}
}

func TestFunctions(t *testing.T) {
	e := New()
	tests := []struct {
		name string
		tmpl string
		data *TemplateData
		want string
	}{
		{
			name: "tojson map",
			tmpl: "{{.Input.M | tojson}}",
			data: BuildData(map[string]any{"M": map[string]int{"x": 1}}),
			want: `{"x":1}`,
		},
		{
			name: "fromjson then tojson",
			tmpl: "{{.Input.Json | fromjson | tojson}}",
			data: BuildData(map[string]any{"Json": `{"a":1}`}),
			want: `{"a":1}`,
		},
		{
			name: "truncate long",
			tmpl: "{{.Input.Text | truncate 5}}",
			data: BuildData(map[string]any{"Text": "hello world"}),
			want: "he...",
		},
		{
			name: "truncate short",
			tmpl: "{{.Input.Text | truncate 50}}",
			data: BuildData(map[string]any{"Text": "short"}),
			want: "short",
		},
		{
			name: "indent",
			tmpl: "{{.Input.Text | indent \"  \"}}",
			data: BuildData(map[string]any{"Text": "a\nb"}),
			want: "  a\n  b",
		},
		{
			name: "default nil",
			tmpl: "{{.Input.Val | default \"fallback\"}}",
			data: BuildData(map[string]any{"Val": nil}),
			want: "fallback",
		},
		{
			name: "default non-empty",
			tmpl: "{{.Input.Val | default \"fallback\"}}",
			data: BuildData(map[string]any{"Val": "set"}),
			want: "set",
		},
		{
			name: "join",
			tmpl: "{{.Input.Items | join \", \"}}",
			data: BuildData(map[string]any{"Items": []string{"a", "b"}}),
			want: "a, b",
		},
		{
			name: "contains",
			tmpl: "{{.Input.Text | contains \"lo\"}}",
			data: BuildData(map[string]any{"Text": "hello"}),
			want: "true",
		},
		{
			name: "hasprefix",
			tmpl: "{{.Input.Text | hasprefix \"he\"}}",
			data: BuildData(map[string]any{"Text": "hello"}),
			want: "true",
		},
		{
			name: "hassuffix",
			tmpl: "{{.Input.Text | hassuffix \"lo\"}}",
			data: BuildData(map[string]any{"Text": "hello"}),
			want: "true",
		},
		{
			name: "lower",
			tmpl: "{{.Input.Text | lower}}",
			data: BuildData(map[string]any{"Text": "HELLO"}),
			want: "hello",
		},
		{
			name: "upper",
			tmpl: "{{.Input.Text | upper}}",
			data: BuildData(map[string]any{"Text": "hello"}),
			want: "HELLO",
		},
		{
			name: "trim",
			tmpl: "{{.Input.Text | trim}}",
			data: BuildData(map[string]any{"Text": "  hello  "}),
			want: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := e.Parse("t", tt.tmpl)
			if err != nil {
				t.Fatalf("parse %q: %v", tt.tmpl, err)
			}
			got, err := tmpl.Execute(tt.data)
			if err != nil {
				t.Fatalf("execute %q: %v", tt.tmpl, err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProvider(t *testing.T) {
	tmplText := "Hello {{.State.Get \"user_name\"}}, from {{.Agent.Name}} in {{.App.Name}}."
	provider, err := NewInstructionProvider(tmplText)
	if err != nil {
		t.Fatalf("NewInstructionProvider: %v", err)
	}

	ctx := newFullContext(t)
	got, err := provider(ctx)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	want := "Hello Alice, from test-agent in test-app."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	if _, err := NewInstructionProvider("{{if}"); err == nil {
		t.Error("expected error for invalid template")
	}
}

func TestProviderFromTemplateWithInputFn(t *testing.T) {
	e := New()
	tmpl := e.MustParse("in", "{{.Input.greeting}} {{.State.Get \"user_name\"}}")

	provider := NewInstructionProviderFromTemplate(tmpl, func(ctx agent.ReadonlyContext) map[string]any {
		return map[string]any{"greeting": "Hi"}
	})

	ctx := newFullContext(t)
	got, err := provider(ctx)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if got != "Hi Alice" {
		t.Errorf("got %q, want %q", got, "Hi Alice")
	}
}

func TestLoader(t *testing.T) {
	e := New()
	loader := NewLoader(e, nil)

	t.Run("string", func(t *testing.T) {
		tmpl, err := loader.LoadFromString("inline", "value={{.Input.v}}")
		if err != nil {
			t.Fatalf("load string: %v", err)
		}
		got, err := tmpl.Execute(BuildData(map[string]any{"v": 1}))
		if err != nil || got != "value=1" {
			t.Errorf("got %q err=%v", got, err)
		}
	})

	t.Run("file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.tmpl")
		if err := os.WriteFile(path, []byte("name={{.Input.name}}"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		tmpl, err := loader.LoadFromFile(path)
		if err != nil {
			t.Fatalf("load file: %v", err)
		}
		if tmpl.Name() != "test.tmpl" {
			t.Errorf("template name = %q", tmpl.Name())
		}
		got, err := tmpl.Execute(BuildData(map[string]any{"name": "file"}))
		if err != nil || got != "name=file" {
			t.Errorf("got %q err=%v", got, err)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := loader.LoadFromFile(filepath.Join(t.TempDir(), "missing.tmpl"))
		if err == nil {
			t.Error("expected error for missing file")
		}
	})

	t.Run("embed fs", func(t *testing.T) {
		fsys := fstest.MapFS{
			"templates/test.tmpl": &fstest.MapFile{Data: []byte("x={{.Input.x}}")},
		}
		loaderWithFS := NewLoader(e, fsys)
		tmpl, err := loaderWithFS.LoadFromFile("templates/test.tmpl")
		if err != nil {
			t.Fatalf("load from fs: %v", err)
		}
		got, err := tmpl.Execute(BuildData(map[string]any{"x": 99}))
		if err != nil || got != "x=99" {
			t.Errorf("got %q err=%v", got, err)
		}
	})
}

func TestRegistry(t *testing.T) {
	e := New()
	r := NewRegistry(e)

	t.Run("register get", func(t *testing.T) {
		if err := r.Register("hello", "Hello {{.Input.name}}"); err != nil {
			t.Fatalf("register: %v", err)
		}
		tmpl, ok := r.Get("hello")
		if !ok {
			t.Fatal("template not found")
		}
		got, err := tmpl.Execute(BuildData(map[string]any{"name": "world"}))
		if err != nil || got != "Hello world" {
			t.Errorf("got %q err=%v", got, err)
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		if err := r.Register("hello", "x"); err == nil {
			t.Error("expected duplicate registration error")
		}
	})

	t.Run("names", func(t *testing.T) {
		r.Register("b", "b")
		r.Register("a", "a")
		names := r.Names()
		want := []string{"a", "b", "hello"}
		if len(names) != len(want) || names[0] != "a" || names[1] != "b" || names[2] != "hello" {
			t.Errorf("names = %v", names)
		}
	})

	t.Run("render", func(t *testing.T) {
		ctx := testutil.NewFakeReadonlyContext().WithReadonlyState(testutil.NewFakeStateWithData(map[string]any{"x": 1}))
		r.Register("state", "{{.State.Get \"x\"}}")
		got, err := r.Render("state", ctx)
		if err != nil || got != "1" {
			t.Errorf("got %q err=%v", got, err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		_, err := r.Render("missing", testutil.NewFakeReadonlyContext())
		if err == nil {
			t.Error("expected error for missing template")
		}
	})

	t.Run("concurrent", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				name := "tmpl" + string(rune('a'+i%26))
				r.Register(name, "{{.Input.i}}")
				if tmpl, ok := r.Get(name); ok {
					tmpl.Execute(BuildData(map[string]any{"i": i}))
				}
			}(i)
		}
		wg.Wait()
	})
}

func TestTemplateRef(t *testing.T) {
	e := New()
	r := NewRegistry(e)
	r.Register("named", "{{.Input.v}}")
	loader := NewLoader(e, nil)

	t.Run("validate", func(t *testing.T) {
		valid := []*TemplateRef{
			{Name: "x"},
			{Inline: "x"},
			{Path: "/x"},
		}
		for _, ref := range valid {
			if err := ref.Validate(); err != nil {
				t.Errorf("%v: %v", ref, err)
			}
		}

		invalid := []*TemplateRef{
			{},
			{Name: "x", Inline: "y"},
			{Name: "x", Path: "/y", Inline: "z"},
		}
		for _, ref := range invalid {
			if err := ref.Validate(); err == nil {
				t.Errorf("expected error for %v", ref)
			}
		}
	})

	t.Run("resolve inline", func(t *testing.T) {
		ref := &TemplateRef{Inline: "i={{.Input.i}}"}
		tmpl, err := ref.Resolve(r, loader)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		got, _ := tmpl.Execute(BuildData(map[string]any{"i": 7}))
		if got != "i=7" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("resolve name", func(t *testing.T) {
		ref := &TemplateRef{Name: "named"}
		tmpl, err := ref.Resolve(r, loader)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		got, _ := tmpl.Execute(BuildData(map[string]any{"v": "ok"}))
		if got != "ok" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("resolve path", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "file.tmpl")
		os.WriteFile(path, []byte("p={{.Input.p}}"), 0o644)
		ref := &TemplateRef{Path: path}
		tmpl, err := ref.Resolve(r, loader)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		got, _ := tmpl.Execute(BuildData(map[string]any{"p": "path"}))
		if got != "p=path" {
			t.Errorf("got %q", got)
		}
	})
}

func TestExampleFullFlow(t *testing.T) {
	ctx := newFullContext(t)

	artifactSvc := testutil.NewFakeArtifactService()
	artifactSvc.PreloadArtifact("test-app", "test-user", "test-session", "reference_doc", genai.NewPartFromText("Be concise."))
	ctx.art = testutil.NewFakeArtifacts(artifactSvc, "test-app", "test-user", "test-session")

	tmplText := "{{.Artifact \"reference_doc\"}}\nUser: {{.User.Text}}\nCountry: {{.State.Get \"country\"}}\nLang: {{.Input.lang | default \"en\"}}"
	provider, err := NewInstructionProvider(tmplText)
	if err != nil {
		t.Fatalf("NewInstructionProvider: %v", err)
	}

	providerFn := NewInstructionProviderFromTemplate(New().MustParse("t", tmplText), func(rc agent.ReadonlyContext) map[string]any {
		return map[string]any{"lang": "fr"}
	})

	got, err := provider(ctx)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if !strings.Contains(got, "Be concise.") || !strings.Contains(got, "Wonderland") {
		t.Errorf("full output missing expected text: %q", got)
	}

	got, err = providerFn(ctx)
	if err != nil {
		t.Fatalf("providerFn: %v", err)
	}
	if !strings.Contains(got, "Lang: fr") {
		t.Errorf("input fn output missing: %q", got)
	}
}
