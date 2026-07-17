package eval

// RubricContent represents the content of a rubric.
type RubricContent struct {
	// TextProperty is the property being evaluated.
	// Example: "The agent's response is grammatically correct."
	TextProperty string `json:"textProperty,omitempty"`
}

// Rubric represents a single rubric used for evaluation.
type Rubric struct {
	// RubricID is the unique identifier for the rubric.
	RubricID string `json:"rubricId"`

	// RubricContent is the actual testable criterion for the rubric.
	RubricContent RubricContent `json:"rubricContent"`

	// Description provides details on how to interpret the rubric assessment.
	Description string `json:"description,omitempty"`

	// Type is an optional designator for the rubric type.
	// Examples: "TOOL_USE_QUALITY", "FINAL_RESPONSE_QUALITY",
	// "INSTRUCTION_ADHERENCE".
	Type string `json:"type,omitempty"`
}

// RubricScore is the score obtained after applying a rubric to the agent's
// response.
type RubricScore struct {
	// RubricID is the id of the rubric that was assessed.
	RubricID string `json:"rubricId"`

	// Rationale is the reasoning for the score.
	Rationale string `json:"rationale,omitempty"`

	// Score is the score obtained after assessing the rubric.
	// Optional, as assessment might not have happened.
	Score *float64 `json:"score,omitempty"`
}
