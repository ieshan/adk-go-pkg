package prompt_test

import (
	"fmt"

	"github.com/ieshan/adk-go-pkg/prompt"
)

// ExampleTemplate_Execute demonstrates basic template execution with
// New().Parse() and Execute(BuildData(...)).
func ExampleTemplate_Execute() {
	engine := prompt.New()
	tmpl := engine.MustParse("greeting", "Hello, {{.Input.name}}!")

	output, err := tmpl.Execute(prompt.BuildData(map[string]any{
		"name": "World",
	}))
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	fmt.Println(output)
	// Output: Hello, World!
}

// ExampleBuildData demonstrates building template data from a map and rendering
// a template that accesses multiple input keys.
func ExampleBuildData() {
	engine := prompt.New()
	tmpl := engine.MustParse("summary", "{{.Input.greeting}}, {{.Input.subject}}!")

	data := prompt.BuildData(map[string]any{
		"greeting": "Good morning",
		"subject":  "Go developers",
	})

	output, err := tmpl.Execute(data)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	fmt.Println(output)
	// Output: Good morning, Go developers!
}
