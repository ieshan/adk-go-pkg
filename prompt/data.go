package prompt

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

// ErrNoArtifactSource is returned when an artifact is requested but no
// artifact source is available in the context.
var ErrNoArtifactSource = errors.New("prompt: no artifact source available in context")

// TemplateData is the top-level data object passed to prompt templates.
//
// Fields are exposed as struct fields and nested data objects provide methods
// (rather than function fields) because text/template can call methods directly.
type TemplateData struct {
	State   *StateData
	User    *UserData
	Session *SessionData
	Agent   *AgentData
	Memory  *MemoryData
	App     *AppData
	Input   map[string]any

	// Unexported fields for lazy artifact loading.
	artifacts agent.Artifacts
	ctx       context.Context
}

// extendedContext captures the extra methods available on concrete ADK context
// implementations beyond the public agent.ReadonlyContext interface.
type extendedContext interface {
	agent.ReadonlyContext
	Artifacts() agent.Artifacts
	Memory() agent.Memory
	Session() session.Session
	Agent() agent.Agent
}

// Artifact loads the text content of the named artifact.
//
// The second argument, when true, treats a missing, unloadable, or empty-part
// artifact as an empty string rather than an error.
func (d *TemplateData) Artifact(name string, optional ...bool) (string, error) {
	if d.artifacts == nil {
		if len(optional) > 0 && optional[0] {
			return "", nil
		}
		return "", fmt.Errorf("%w: artifact %q", ErrNoArtifactSource, name)
	}
	resp, err := d.artifacts.Load(d.ctx, name)
	if err != nil {
		if len(optional) > 0 && optional[0] {
			return "", nil
		}
		return "", fmt.Errorf("artifact %q: %w", name, err)
	}
	if resp == nil || resp.Part == nil {
		if len(optional) > 0 && optional[0] {
			return "", nil
		}
		return "", fmt.Errorf("artifact %q: empty response", name)
	}
	return partText(resp.Part), nil
}

// StateData wraps session state for template access.
type StateData struct {
	state session.ReadonlyState
}

// Get returns the state value for key, or nil if the key does not exist.
func (s *StateData) Get(key string) any {
	if s == nil || s.state == nil {
		return nil
	}
	v, err := s.state.Get(key)
	if err != nil && errors.Is(err, session.ErrStateKeyNotExist) {
		return nil
	}
	return v
}

// Has reports whether key exists in state.
func (s *StateData) Has(key string) bool {
	if s == nil || s.state == nil {
		return false
	}
	_, err := s.state.Get(key)
	return err == nil || !errors.Is(err, session.ErrStateKeyNotExist)
}

// All returns a snapshot map of all state entries.
func (s *StateData) All() map[string]any {
	if s == nil || s.state == nil {
		return nil
	}
	all := make(map[string]any)
	for k, v := range s.state.All() {
		all[k] = v
	}
	return all
}

// UserData exposes the content that started the invocation.
type UserData struct {
	Content *genai.Content
	Text    string
}

// SessionData exposes session identifiers.
type SessionData struct {
	ID, AppName, UserID string
}

// AgentData exposes agent metadata.
type AgentData struct {
	Name, Description string
}

// MemoryData exposes memory search.
type MemoryData struct {
	memory agent.Memory
	ctx    context.Context
}

// Search performs a memory search and returns the text of each matching entry.
func (m *MemoryData) Search(query string) ([]string, error) {
	if m == nil || m.memory == nil {
		return nil, fmt.Errorf("memory search: no memory source available")
	}
	resp, err := m.memory.SearchMemory(m.ctx, query)
	if err != nil {
		return nil, fmt.Errorf("memory search %q: %w", query, err)
	}
	var out []string
	for _, e := range resp.Memories {
		out = append(out, contentText(e.Content))
	}
	return out, nil
}

// AppData exposes app metadata.
type AppData struct {
	Name string
}

// BuildData creates a TemplateData that only sets the Input field.
func BuildData(input map[string]any) *TemplateData {
	return &TemplateData{Input: input}
}

// BuildDataFromReadonlyContext builds TemplateData from the public
// ReadonlyContext methods, and uses type assertion to gain access to
// Artifacts, Memory, Session, and Agent metadata when available.
func BuildDataFromReadonlyContext(ctx agent.ReadonlyContext) *TemplateData {
	d := &TemplateData{}
	if ctx == nil {
		return d
	}
	d.ctx = ctx

	d.State = &StateData{state: ctx.ReadonlyState()}
	d.User = &UserData{Content: ctx.UserContent(), Text: contentText(ctx.UserContent())}
	d.Session = &SessionData{ID: ctx.SessionID(), AppName: ctx.AppName(), UserID: ctx.UserID()}
	d.Agent = &AgentData{Name: ctx.AgentName()}
	d.App = &AppData{Name: ctx.AppName()}

	if ext, ok := ctx.(extendedContext); ok {
		d.artifacts = ext.Artifacts()
		d.Memory = &MemoryData{memory: ext.Memory(), ctx: ctx}
		if ext.Session() != nil {
			d.Session.ID = ext.Session().ID()
			d.Session.AppName = ext.Session().AppName()
			d.Session.UserID = ext.Session().UserID()
		}
		if ext.Agent() != nil {
			d.Agent.Name = ext.Agent().Name()
			d.Agent.Description = ext.Agent().Description()
		}
	}

	return d
}

// invocationContext is an extension of agent.InvocationContext with the
// ReadonlyContext methods needed to build TemplateData.
//
// Concrete ADK InvocationContext implementations embed the ReadonlyContext
// methods, so this type assertion succeeds for real contexts.
type invocationReadonlyContext interface {
	agent.InvocationContext
	agent.ReadonlyContext
}

// BuildDataFromInvocationContext builds TemplateData from an InvocationContext.
func BuildDataFromInvocationContext(ctx agent.InvocationContext) *TemplateData {
	d := &TemplateData{}
	if ctx == nil {
		return d
	}
	d.ctx = ctx

	if ext, ok := ctx.(invocationReadonlyContext); ok {
		d.State = &StateData{state: ext.ReadonlyState()}
		d.User = &UserData{Content: ext.UserContent(), Text: contentText(ext.UserContent())}
		d.App = &AppData{Name: ext.AppName()}
	}

	if ctx.Session() != nil {
		d.Session = &SessionData{
			ID:      ctx.Session().ID(),
			AppName: ctx.Session().AppName(),
			UserID:  ctx.Session().UserID(),
		}
	}
	d.artifacts = ctx.Artifacts()
	d.Memory = &MemoryData{memory: ctx.Memory(), ctx: ctx}
	if ctx.Agent() != nil {
		d.Agent = &AgentData{
			Name:        ctx.Agent().Name(),
			Description: ctx.Agent().Description(),
		}
	}

	return d
}

// contentText returns the concatenated text from a genai.Content.
func contentText(c *genai.Content) string {
	if c == nil {
		return ""
	}
	var sb strings.Builder
	for _, p := range c.Parts {
		sb.WriteString(partText(p))
	}
	return sb.String()
}

// partText returns the text of a part.
func partText(p *genai.Part) string {
	if p == nil {
		return ""
	}
	return p.Text
}
