package prompt

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// TemplateLoader loads templates from strings, files, or an embed.FS.
type TemplateLoader struct {
	engine *TemplateEngine
	fsys   fs.FS // nil means use os.ReadFile
}

// NewLoader creates a TemplateLoader.
//
// If fsys is nil, LoadFromFile reads from the host filesystem with os.ReadFile.
// If fsys is non-nil, LoadFromFile reads from fsys.
func NewLoader(engine *TemplateEngine, fsys fs.FS) *TemplateLoader {
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

// LoadFromFile reads and parses a template from path.
//
// The template name is set to filepath.Base(path).
func (l *TemplateLoader) LoadFromFile(path string) (*Template, error) {
	var b []byte
	var err error
	if l.fsys != nil {
		b, err = fs.ReadFile(l.fsys, path)
	} else {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("load template file %q: %w", path, err)
	}
	name := filepath.Base(path)
	return l.engine.Parse(name, string(b))
}
