package prompt

import (
	"fmt"
	"os"
	"sort"
	"sync"

	"google.golang.org/adk/v2/agent"
)

// TemplateRegistry stores named templates and renders them on demand.
//
// The registry is safe for concurrent use. Templates are parsed once and can be
// executed multiple times.
type TemplateRegistry struct {
	engine    *TemplateEngine
	mu        sync.RWMutex
	templates map[string]*Template
}

// NewRegistry creates an empty TemplateRegistry backed by engine.
func NewRegistry(engine *TemplateEngine) *TemplateRegistry {
	return &TemplateRegistry{
		engine:    engine,
		templates: make(map[string]*Template),
	}
}

// Engine returns the registry's underlying TemplateEngine.
func (r *TemplateRegistry) Engine() *TemplateEngine {
	return r.engine
}

// Register parses and stores a template under name.
// Returns an error if parsing fails or if name is already registered.
func (r *TemplateRegistry) Register(name, text string) error {
	tmpl, err := r.engine.Parse(name, text)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.templates[name]; exists {
		return fmt.Errorf("template %q already registered", name)
	}
	r.templates[name] = tmpl
	return nil
}

// RegisterFile reads and registers a template from path under the provided
// *os.Root, storing it under name. root must not be nil.
func (r *TemplateRegistry) RegisterFile(name string, root *os.Root, path string) error {
	b, err := root.ReadFile(path)
	if err != nil {
		return fmt.Errorf("register template file %q: %w", path, err)
	}
	return r.Register(name, string(b))
}

// Get returns the named template.
func (r *TemplateRegistry) Get(name string) (*Template, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tmpl, ok := r.templates[name]
	return tmpl, ok
}

// Render executes the named template with data built from ctx.
func (r *TemplateRegistry) Render(name string, ctx agent.ReadonlyContext) (string, error) {
	tmpl, ok := r.Get(name)
	if !ok || tmpl == nil {
		return "", fmt.Errorf("template %q not found", name)
	}
	return tmpl.Execute(BuildDataFromReadonlyContext(ctx))
}

// Names returns all registered template names sorted.
func (r *TemplateRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.templates))
	for name := range r.templates {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
