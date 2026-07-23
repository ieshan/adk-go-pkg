package prompt

import (
	"fmt"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
)

// NewInstructionProvider parses templateText and returns an
// llmagent.InstructionProvider that renders it on each invocation.
//
// The returned function populates TemplateData from the agent context, then
// executes the parsed template. Errors are wrapped with the template name.
func NewInstructionProvider(templateText string) (llmagent.InstructionProvider, error) {
	engine := New()
	tmpl, err := engine.Parse("instruction", templateText)
	if err != nil {
		return nil, fmt.Errorf("new instruction provider: %w", err)
	}
	return NewInstructionProviderFromTemplate(tmpl), nil
}

// NewInstructionProviderFromTemplate returns an InstructionProvider backed by
// an already-parsed template.
//
// The optional inputFn, if provided, is called with the agent context and its
// return value is assigned to TemplateData.Input before template execution. This
// supports agents that pass structured input to the template.
func NewInstructionProviderFromTemplate(t *Template, inputFn ...func(agent.ReadonlyContext) map[string]any) llmagent.InstructionProvider {
	return func(ctx agent.ReadonlyContext) (string, error) {
		data := BuildDataFromReadonlyContext(ctx)
		if len(inputFn) > 0 && inputFn[0] != nil {
			data.Input = inputFn[0](ctx)
		}
		rendered, err := t.Execute(data)
		if err != nil {
			return "", fmt.Errorf("render instruction template %q: %w", t.Name(), err)
		}
		return rendered, nil
	}
}
