package prompt

import (
	"bytes"
	"errors"
	"fmt"
	"text/template"
)

// ErrTemplateParse is returned when parsing a template fails.
var ErrTemplateParse = errors.New("prompt: parse template")

// ErrTemplateExecute is returned when executing a template fails.
var ErrTemplateExecute = errors.New("prompt: execute template")

// TemplateEngine parses and executes text/template prompts.
//
// The engine is immutable after construction: the function map is fixed at
// creation time and cannot be modified afterwards, avoiding the common
// foot-gun of calling Funcs after Parse.
type TemplateEngine struct {
	funcs template.FuncMap
}

// Option configures a TemplateEngine.
type Option func(*TemplateEngine)

// WithFuncs registers custom template functions.
//
// User-provided functions override the defaults. The functions are merged
// into a copy of the default function map, so the original defaults are not
// mutated.
func WithFuncs(funcs template.FuncMap) Option {
	return func(e *TemplateEngine) {
		merged := make(template.FuncMap, len(e.funcs)+len(funcs))
		for k, v := range e.funcs {
			merged[k] = v
		}
		for k, v := range funcs {
			merged[k] = v
		}
		e.funcs = merged
	}
}

// New creates a TemplateEngine with the default function map.
func New(opts ...Option) *TemplateEngine {
	e := &TemplateEngine{funcs: defaultFuncs()}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Funcs returns a copy of the engine's function map.
func (e *TemplateEngine) Funcs() template.FuncMap {
	copy := make(template.FuncMap, len(e.funcs))
	for k, v := range e.funcs {
		copy[k] = v
	}
	return copy
}

// Template is a parsed prompt template.
type Template struct {
	tmpl *template.Template
}

// Name returns the template name.
func (t *Template) Name() string {
	return t.tmpl.Name()
}

// Execute renders the template with the provided data.
func (t *Template) Execute(data *TemplateData) (string, error) {
	var buf bytes.Buffer
	if err := t.tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("%w %q: %w", ErrTemplateExecute, t.tmpl.Name(), err)
	}
	return buf.String(), nil
}

// Parse parses a template string using the engine's function map.
func (e *TemplateEngine) Parse(name, text string) (*Template, error) {
	tmpl, err := template.New(name).Funcs(e.funcs).Parse(text)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", ErrTemplateParse, name, err)
	}
	return &Template{tmpl: tmpl}, nil
}

// MustParse is like Parse but panics on error.
func (e *TemplateEngine) MustParse(name, text string) *Template {
	t, err := e.Parse(name, text)
	if err != nil {
		panic(err)
	}
	return t
}
