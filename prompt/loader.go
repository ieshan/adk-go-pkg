package prompt

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// TemplateLoader loads templates from strings, files beneath an *os.Root,
// or files in an arbitrary fs.FS (embed.FS, fstest.MapFS, etc.).
// Use [NewLoader] with an *os.Root or [NewLoaderFromFS] with an fs.FS.
type TemplateLoader struct {
	engine *TemplateEngine
	fsys   fs.FS // nil means no filesystem available
}

// NewLoader creates a TemplateLoader backed by an [os.Root].
//
// If root is nil, LoadFromFile returns an error (no filesystem available).
// LoadFromString still works regardless of root.
func NewLoader(engine *TemplateEngine, root *os.Root) *TemplateLoader {
	if root == nil {
		return &TemplateLoader{engine: engine}
	}
	return &TemplateLoader{engine: engine, fsys: root.FS()}
}

// NewLoaderFromFS creates a TemplateLoader backed by an arbitrary [fs.FS].
// Use this for embed.FS, fstest.MapFS, or other custom filesystems.
// If fsys is nil, LoadFromFile returns an error (no filesystem available).
// LoadFromString still works regardless of fsys.
func NewLoaderFromFS(engine *TemplateEngine, fsys fs.FS) *TemplateLoader {
	return &TemplateLoader{engine: engine, fsys: fsys}
}

// Engine returns the loader's underlying TemplateEngine.
func (l *TemplateLoader) Engine() *TemplateEngine {
	return l.engine
}

// LoadFromString parses a template from text.
func (l *TemplateLoader) LoadFromString(name, text string) (*Template, error) {
	return l.engine.Parse(name, text)
}

// LoadFromFile reads and parses a template from path beneath the loader's filesystem.
//
// The template name is set to filepath.Base(path).
// Returns an error if no filesystem was configured (nil root passed to
// NewLoader or nil fs.FS passed to NewLoaderFromFS).
func (l *TemplateLoader) LoadFromFile(path string) (*Template, error) {
	if l.fsys == nil {
		return nil, fmt.Errorf("load template file %q: no filesystem available", path)
	}
	b, err := fs.ReadFile(l.fsys, path)
	if err != nil {
		return nil, fmt.Errorf("load template file %q: %w", path, err)
	}
	name := filepath.Base(path)
	return l.engine.Parse(name, string(b))
}
