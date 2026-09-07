package prompt

import (
	"context"
	"errors"
	"io/fs"
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

func TestEngine_ParseExecute(t *testing.T) {
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

func TestEngine_ParseError(t *testing.T) {
	_, err := New().Parse("bad", "{{if}}")
	if !errors.Is(err, ErrTemplateParse) {
		t.Errorf("got %v, want ErrTemplateParse", err)
	}
}

func TestEngine_ExecuteError(t *testing.T) {
	tmpl := New().MustParse("bad", "{{.Input.Value | tojson}}")
	_, err := tmpl.Execute(BuildData(map[string]any{"Value": make(chan int)}))
	if !errors.Is(err, ErrTemplateExecute) {
		t.Errorf("got %v, want ErrTemplateExecute", err)
	}
}

func TestEngine_WithFuncs(t *testing.T) {
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
		t.Errorf("got Input[x] = %v, want %q", d.Input["x"], "y")
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
		t.Errorf("got state value %v, want %q", d.State.Get("k"), "v")
	}
	if d.User.Text != "hello" {
		t.Errorf("got user text %q, want %q", d.User.Text, "hello")
	}
	if d.Session.ID != "ro-session" || d.Session.AppName != "ro-app" || d.Session.UserID != "ro-user" {
		t.Errorf("got session %+v, want ID=%q AppName=%q UserID=%q", d.Session, "ro-session", "ro-app", "ro-user")
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
		t.Error("got nil error, want artifact error when no artifact source available")
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
		t.Fatal("got nil TemplateData, want non-nil")
	}
	if d.State != nil || d.Session != nil || d.Agent != nil {
		t.Error("got non-nil fields, want nil for nil context")
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
	if _, err := d.Artifact("missing"); !errors.Is(err, ErrNoArtifactSource) {
		t.Errorf("got %v, want ErrNoArtifactSource for missing artifact with no source", err)
	}
	text, err := d.Artifact("missing", true)
	if err != nil {
		t.Errorf("optional artifact should suppress error: %v", err)
	}
	if text != "" {
		t.Errorf("optional missing artifact should be empty")
	}
}

func TestEngine_Functions(t *testing.T) {
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
			name: "truncate n=3 exact fit",
			tmpl: "{{.Input.Text | truncate 3}}",
			data: BuildData(map[string]any{"Text": "hello"}),
			want: "...",
		},
		{
			name: "truncate n=2 no ellipsis room",
			tmpl: "{{.Input.Text | truncate 2}}",
			data: BuildData(map[string]any{"Text": "hello"}),
			want: "he",
		},
		{
			name: "truncate n=1 no ellipsis room",
			tmpl: "{{.Input.Text | truncate 1}}",
			data: BuildData(map[string]any{"Text": "hello"}),
			want: "h",
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
			name: "default empty slice uses fallback",
			tmpl: "{{.Input.Val | default \"fallback\"}}",
			data: BuildData(map[string]any{"Val": []string{}}),
			want: "fallback",
		},
		{
			name: "default empty map uses fallback",
			tmpl: "{{.Input.Val | default \"fallback\"}}",
			data: BuildData(map[string]any{"Val": map[string]int{}}),
			want: "fallback",
		},
		{
			name: "default non-empty slice kept",
			tmpl: "{{.Input.Val | default \"fallback\"}}",
			data: BuildData(map[string]any{"Val": []string{"x"}}),
			want: "[x]",
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
		t.Error("got nil error, want error for invalid template")
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
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatalf("OpenRoot: %v", err)
		}
		t.Cleanup(func() { _ = root.Close() })
		fileLoader := NewLoader(e, root)
		tmpl, err := fileLoader.LoadFromFile("test.tmpl")
		if err != nil {
			t.Fatalf("load file: %v", err)
		}
		if tmpl.Name() != "test.tmpl" {
			t.Errorf("got template name %q, want %q", tmpl.Name(), "test.tmpl")
		}
		got, err := tmpl.Execute(BuildData(map[string]any{"name": "file"}))
		if err != nil || got != "name=file" {
			t.Errorf("got %q err=%v", got, err)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		root, err := os.OpenRoot(t.TempDir())
		if err != nil {
			t.Fatalf("OpenRoot: %v", err)
		}
		t.Cleanup(func() { _ = root.Close() })
		fileLoader := NewLoader(e, root)
		_, err = fileLoader.LoadFromFile("missing.tmpl")
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("got %v, want fs.ErrNotExist for missing file", err)
		}
	})

	t.Run("nil root", func(t *testing.T) {
		_, err := loader.LoadFromFile("test.tmpl")
		if err == nil {
			t.Error("got nil error, want error for nil filesystem")
		}
	})

	t.Run("embed fs", func(t *testing.T) {
		fsys := fstest.MapFS{
			"templates/test.tmpl": &fstest.MapFile{Data: []byte("x={{.Input.x}}")},
		}
		loaderWithFS := NewLoaderFromFS(e, fsys)
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
			t.Error("got nil error, want duplicate registration error")
		}
	})

	t.Run("names", func(t *testing.T) {
		if err := r.Register("b", "b"); err != nil {
			t.Fatal(err)
		}
		if err := r.Register("a", "a"); err != nil {
			t.Fatal(err)
		}
		names := r.Names()
		want := []string{"a", "b", "hello"}
		if len(names) != len(want) || names[0] != "a" || names[1] != "b" || names[2] != "hello" {
			t.Errorf("got names = %v, want %v", names, want)
		}
	})

	t.Run("render", func(t *testing.T) {
		ctx := testutil.NewFakeReadonlyContext().WithReadonlyState(testutil.NewFakeStateWithData(map[string]any{"x": 1}))
		if err := r.Register("state", "{{.State.Get \"x\"}}"); err != nil {
			t.Fatal(err)
		}
		got, err := r.Render("state", ctx)
		if err != nil || got != "1" {
			t.Errorf("got %q err=%v", got, err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		_, err := r.Render("missing", testutil.NewFakeReadonlyContext())
		if err == nil {
			t.Error("got nil error, want error for missing template")
		}
	})

	t.Run("concurrent", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				name := "tmpl" + string(rune('a'+i%26))
				if err := r.Register(name, "{{.Input.i}}"); err != nil {
					// Duplicate registration is expected for collisions; ignore.
					return
				}
				if tmpl, ok := r.Get(name); ok {
					if _, err := tmpl.Execute(BuildData(map[string]any{"i": i})); err != nil {
						t.Errorf("execute template %q: %v", name, err)
						return
					}
				}
			}(i)
		}
		wg.Wait()
	})
}

func TestTemplateRef(t *testing.T) {
	e := New()
	r := NewRegistry(e)
	if err := r.Register("named", "{{.Input.v}}"); err != nil {
		t.Fatal(err)
	}
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
			if err := ref.Validate(); !errors.Is(err, ErrTemplateRefInvalid) {
				t.Errorf("got %v for %v, want ErrTemplateRefInvalid", err, ref)
			}
		}
	})

	t.Run("resolve inline", func(t *testing.T) {
		ref := &TemplateRef{Inline: "i={{.Input.i}}"}
		tmpl, err := ref.Resolve(r, loader)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		got, err := tmpl.Execute(BuildData(map[string]any{"i": 7}))
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
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
		got, err := tmpl.Execute(BuildData(map[string]any{"v": "ok"}))
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		if got != "ok" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("resolve path", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "file.tmpl")
		if err := os.WriteFile(path, []byte("p={{.Input.p}}"), 0o644); err != nil {
			t.Fatal(err)
		}
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatalf("OpenRoot: %v", err)
		}
		t.Cleanup(func() { _ = root.Close() })
		fileLoader := NewLoader(e, root)
		ref := &TemplateRef{Path: "file.tmpl"}
		tmpl, err := ref.Resolve(r, fileLoader)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		got, err := tmpl.Execute(BuildData(map[string]any{"p": "path"}))
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		if got != "p=path" {
			t.Errorf("got %q", got)
		}
	})
}

// TestRegistry_RegisterFile verifies that RegisterFile reads a template from
// a file beneath an os.Root and registers it under the given name.
func TestRegistry_RegisterFile(t *testing.T) {
	e := New()
	r := NewRegistry(e)

	dir := t.TempDir()
	path := filepath.Join(dir, "greeting.tmpl")
	if err := os.WriteFile(path, []byte("Hello {{.Input.name}}"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	if rerr := r.RegisterFile("greeting", root, "greeting.tmpl"); rerr != nil {
		t.Fatalf("RegisterFile: %v", rerr)
	}
	tmpl, ok := r.Get("greeting")
	if !ok || tmpl == nil {
		t.Fatal("template not found after RegisterFile")
	}
	got, err := tmpl.Execute(BuildData(map[string]any{"name": "world"}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got != "Hello world" {
		t.Errorf("got %q, want %q", got, "Hello world")
	}
}

func TestEngine_ExampleFullFlow(t *testing.T) {
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
		t.Errorf("got output %q, want it to contain 'Be concise.' and 'Wonderland'", got)
	}

	got, err = providerFn(ctx)
	if err != nil {
		t.Fatalf("providerFn: %v", err)
	}
	if !strings.Contains(got, "Lang: fr") {
		t.Errorf("input fn output missing: %q", got)
	}
}

// FuzzTemplateParse verifies that New().Parse(name, text) never panics on
// arbitrary template text. Valid templates should parse without error; invalid
// templates should return an error (no panic).
func FuzzTemplateParse(f *testing.F) {
	// Seed: valid template.
	f.Add("{{.Input}}")
	// Seed: invalid template (unclosed action).
	f.Add("{{if}}")
	// Seed: empty string.
	f.Add("")

	f.Fuzz(func(t *testing.T, text string) {
		e := New()
		tmpl, err := e.Parse("fuzz", text)
		// The function must not panic — reaching here is the primary assertion.
		// If parsing succeeded, tmpl must be non-nil.
		if err == nil && tmpl == nil {
			t.Error("Parse returned nil template with nil error")
		}
		// If parsing failed, tmpl should be nil — both outcomes are acceptable
		// as long as no panic occurred.
	})
}

// TestEngine_ExecuteNilTemplateData verifies that Execute with a nil
// *TemplateData does not panic — it must either return an error or produce
// empty output. A template that references fields on nil data is expected to
// error; a template with no field references should produce empty output.
func TestEngine_ExecuteNilTemplateData(t *testing.T) {
	t.Parallel()
	e := New()

	t.Run("field reference errors", func(t *testing.T) {
		t.Parallel()
		tmpl := e.MustParse("nil-data", "Hello {{.Input.name}}!")
		// Reaching here without panicking is the primary assertion.
		got, err := tmpl.Execute(nil)
		if err == nil && got != "" {
			t.Errorf("got %q with nil error, want either error or empty output", got)
		}
	})

	t.Run("no field references produces empty", func(t *testing.T) {
		t.Parallel()
		tmpl := e.MustParse("nil-data-static", "static text")
		got, err := tmpl.Execute(nil)
		if err != nil {
			t.Fatalf("Execute(nil) with static template returned error: %v", err)
		}
		if got != "static text" {
			t.Errorf("got %q, want %q", got, "static text")
		}
	})
}

// TestEngine_ExecuteEmptyTemplate verifies that parsing and executing an empty
// template string produces empty output without error.
func TestEngine_ExecuteEmptyTemplate(t *testing.T) {
	t.Parallel()
	e := New()
	tmpl, err := e.Parse("empty", "")
	if err != nil {
		t.Fatalf("Parse empty template: %v", err)
	}
	got, err := tmpl.Execute(BuildData(nil))
	if err != nil {
		t.Fatalf("Execute empty template: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

// TestEngine_ExecuteUnicode verifies that unicode content in both the template
// text and the input data is preserved through rendering.
func TestEngine_ExecuteUnicode(t *testing.T) {
	t.Parallel()
	e := New()
	tmpl, err := e.Parse("unicode", "Hello, 世界! {{.Input.name}}")
	if err != nil {
		t.Fatalf("Parse unicode template: %v", err)
	}
	got, err := tmpl.Execute(BuildData(map[string]any{"name": "こんにちは"}))
	if err != nil {
		t.Fatalf("Execute unicode template: %v", err)
	}
	want := "Hello, 世界! こんにちは"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// FuzzTemplateExecute verifies that tmpl.Execute(BuildData(...)) never panics
// on arbitrary string data values. The template is fixed and valid; we fuzz
// the data value to ensure execution handles any string gracefully.
func FuzzTemplateExecute(f *testing.F) {
	// Seed: valid string value.
	f.Add("value")
	// Seed: empty string.
	f.Add("")
	// Seed: string with special characters.
	f.Add("{{not a template}}")

	f.Fuzz(func(t *testing.T, value string) {
		e := New()
		tmpl := e.MustParse("fuzz-exec", "{{.Input.key}}")
		_, err := tmpl.Execute(BuildData(map[string]any{"key": value}))
		// The function must not panic — reaching here is the primary assertion.
		// For a valid template with string data, execution should succeed.
		if err != nil {
			t.Errorf("Execute with string value %q returned error: %v", value, err)
		}
	})
}
