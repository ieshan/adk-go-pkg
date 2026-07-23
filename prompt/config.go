package prompt

import (
	"fmt"
)

// TemplateRef is a tagged union for referencing a template by name, inline text,
// or file path.
type TemplateRef struct {
	Name   string `json:"name,omitempty" yaml:"name,omitempty"`
	Inline string `json:"inline,omitempty" yaml:"inline,omitempty"`
	Path   string `json:"path,omitempty" yaml:"path,omitempty"`
}

// IsSet reports whether any reference field is populated.
func (r *TemplateRef) IsSet() bool {
	return r.Name != "" || r.Inline != "" || r.Path != ""
}

// Validate returns an error if zero or more than one field is set.
func (r *TemplateRef) Validate() error {
	count := 0
	if r.Name != "" {
		count++
	}
	if r.Inline != "" {
		count++
	}
	if r.Path != "" {
		count++
	}
	if count == 0 {
		return fmt.Errorf("TemplateRef: exactly one of name, inline, or path must be set")
	}
	if count > 1 {
		return fmt.Errorf("TemplateRef: only one of name, inline, or path may be set")
	}
	return nil
}

// Resolve resolves the template reference.
//
//   - Name resolves from registry.
//   - Inline resolves using the loader's engine.
//   - Path resolves by reading the file through the loader.
func (r *TemplateRef) Resolve(registry *TemplateRegistry, loader *TemplateLoader) (*Template, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}

	switch {
	case r.Name != "":
		if registry == nil {
			return nil, fmt.Errorf("TemplateRef: cannot resolve name %q: no registry", r.Name)
		}
		tmpl, ok := registry.Get(r.Name)
		if !ok {
			return nil, fmt.Errorf("TemplateRef: template %q not found in registry", r.Name)
		}
		return tmpl, nil
	case r.Inline != "":
		if loader == nil {
			return nil, fmt.Errorf("TemplateRef: cannot resolve inline template: no loader")
		}
		return loader.LoadFromString("inline", r.Inline)
	case r.Path != "":
		if loader == nil {
			return nil, fmt.Errorf("TemplateRef: cannot resolve path %q: no loader", r.Path)
		}
		return loader.LoadFromFile(r.Path)
	default:
		return nil, fmt.Errorf("TemplateRef: unresolved")
	}
}
