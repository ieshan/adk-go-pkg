// Package config provides types and utilities for loading, parsing, and
// translating agent configuration files in JSON or YAML format.
package config

import (
	"context"
	"fmt"
	"io"

	"google.golang.org/adk/tool/skilltoolset/skill"
)

// FilteredSource wraps a skill.Source to expose only specific skill names.
// This enables agents to access only a subset of skills from a larger source
// (e.g., only 2 specific skills from a folder containing 20+ skills).
//
// Filtered skills are completely hidden - they do not appear in ListFrontmatters
// and return ErrSkillNotFound if accessed directly. This provides both security
// (agent cannot discover filtered skills) and performance (filtered skills are
// not loaded during preload operations).
//
// The filter is case-sensitive and matches exact skill names only.
// Names not present in the underlying source are silently ignored.
type FilteredSource struct {
	base  skill.Source
	names map[string]bool // Set of allowed skill names for O(1) lookup
}

// NewFilteredSource creates a source that filters another source to only
// expose skills with the given names. If names is empty, returns base unchanged
// (no filtering).
//
// This is used internally by the builder when SkillsetRef.Names is specified.
func NewFilteredSource(base skill.Source, names []string) skill.Source {
	if len(names) == 0 {
		return base
	}

	namesSet := make(map[string]bool, len(names))
	for _, name := range names {
		namesSet[name] = true
	}

	return &FilteredSource{
		base:  base,
		names: namesSet,
	}
}

// ListFrontmatters returns only frontmatters for skills in the allowlist.
func (f *FilteredSource) ListFrontmatters(ctx context.Context) ([]*skill.Frontmatter, error) {
	allFrontmatters, err := f.base.ListFrontmatters(ctx)
	if err != nil {
		return nil, err
	}

	var filtered []*skill.Frontmatter
	for _, fm := range allFrontmatters {
		if f.names[fm.Name] {
			filtered = append(filtered, fm)
		}
	}

	return filtered, nil
}

// LoadFrontmatter returns frontmatter only if skill name is in allowlist.
func (f *FilteredSource) LoadFrontmatter(ctx context.Context, name string) (*skill.Frontmatter, error) {
	if !f.names[name] {
		return nil, fmt.Errorf("%w: %q is not in the allowed skill names list", skill.ErrSkillNotFound, name)
	}
	return f.base.LoadFrontmatter(ctx, name)
}

// LoadInstructions returns instructions only if skill name is in allowlist.
func (f *FilteredSource) LoadInstructions(ctx context.Context, name string) (string, error) {
	if !f.names[name] {
		return "", fmt.Errorf("%w: %q is not in the allowed skill names list", skill.ErrSkillNotFound, name)
	}
	return f.base.LoadInstructions(ctx, name)
}

// ListResources returns resources only if skill name is in allowlist.
func (f *FilteredSource) ListResources(ctx context.Context, name, subpath string) ([]string, error) {
	if !f.names[name] {
		return nil, fmt.Errorf("%w: %q is not in the allowed skill names list", skill.ErrSkillNotFound, name)
	}
	return f.base.ListResources(ctx, name, subpath)
}

// LoadResource returns resource only if skill name is in allowlist.
func (f *FilteredSource) LoadResource(ctx context.Context, name, resourcePath string) (io.ReadCloser, error) {
	if !f.names[name] {
		return nil, fmt.Errorf("%w: %q is not in the allowed skill names list", skill.ErrSkillNotFound, name)
	}
	return f.base.LoadResource(ctx, name, resourcePath)
}
