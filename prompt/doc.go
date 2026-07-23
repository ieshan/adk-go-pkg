// Package prompt provides text/template-based prompt rendering for ADK-Go.
//
// It exposes a TemplateEngine that parses Go text/template strings and
// executes them against a TemplateData object populated from agent context,
// session state, artifacts, memory, and structured input.
//
// # Basic usage
//
//	engine := prompt.New()
//	tmpl, err := engine.Parse("greeting", "Hello {{.State.Get \"user_name\"}}!")
//	if err != nil { ... }
//
//	data := prompt.BuildData(map[string]any{"language": "en"})
//	rendered, err := tmpl.Execute(data)
//
// # Integration points
//
//   - llmagent.InstructionProvider via prompt.NewInstructionProvider.
//   - config.Registry template registry for declarative agent configs.
//   - planner.PlanReActConfig / planner.ThinkingConfig custom system prompts.
//   - eval/simulation user simulator prompts.
package prompt
