package eval

import (
	"encoding/json"
	"strings"

	"google.golang.org/genai"
)

// Label represents the label extracted from an auto-rater's response.
type Label string

const (
	LabelValid          Label = "valid"
	LabelInvalid        Label = "invalid"
	LabelAlmost         Label = "almost"
	LabelTrue           Label = "true"
	LabelFalse          Label = "false"
	LabelNotFound       Label = "not_found"
	LabelPartiallyValid Label = "partially_valid"
)

// GetTextFromContent extracts text from a genai.Content, joining all text
// parts. Returns empty string if content is nil or has no text parts.
func GetTextFromContent(content *genai.Content) string {
	if content == nil {
		return ""
	}
	var sb strings.Builder
	for _, part := range content.Parts {
		if part.Text != "" {
			sb.WriteString(part.Text)
		}
	}
	return sb.String()
}

// GetTextFromInvocation extracts the final response text from an invocation.
// If includeIntermediateResponsesInFinal is true, intermediate agent responses
// are also included.
func GetTextFromInvocation(invocation Invocation, includeIntermediateResponsesInFinal bool) string {
	var texts []string

	if includeIntermediateResponsesInFinal && invocation.IntermediateData != nil {
		if events := invocation.IntermediateData.GetInvocationEvents(); events != nil {
			for _, event := range events {
				if event.Author != UserAuthor && event.Content != nil {
					texts = append(texts, GetTextFromContent(event.Content))
				}
			}
		}
	}

	if invocation.FinalResponse != nil {
		texts = append(texts, GetTextFromContent(invocation.FinalResponse))
	}

	return strings.Join(texts, "\n")
}

// GetToolCallsAndResponsesAsJSONStr returns a JSON string representation of
// tool calls and their responses from an invocation's intermediate data.
func GetToolCallsAndResponsesAsJSONStr(invocation Invocation) string {
	if invocation.IntermediateData == nil {
		return "[]"
	}

	type toolCallWithResponse struct {
		Call     *genai.FunctionCall     `json:"call,omitempty"`
		Response *genai.FunctionResponse `json:"response,omitempty"`
	}

	var entries []toolCallWithResponse

	// Legacy format: pair tool uses with tool responses by index.
	uses := invocation.IntermediateData.GetToolUses()
	responses := invocation.IntermediateData.GetToolResponses()
	for i, call := range uses {
		entry := toolCallWithResponse{Call: &call}
		if i < len(responses) {
			entry.Response = &responses[i]
		}
		entries = append(entries, entry)
	}

	// Events format: extract function calls and responses from events.
	events := invocation.IntermediateData.GetInvocationEvents()
	for _, event := range events {
		if event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if part.FunctionCall != nil {
				entries = append(entries, toolCallWithResponse{Call: part.FunctionCall})
			}
			if part.FunctionResponse != nil {
				entries = append(entries, toolCallWithResponse{Response: part.FunctionResponse})
			}
		}
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(data)
}

// GetToolDeclarationsAsJSONStr returns a JSON string representation of tool
// declarations from AppDetails.
func GetToolDeclarationsAsJSONStr(appDetails *AppDetails) string {
	if appDetails == nil {
		return "[]"
	}

	var allTools []*genai.Tool
	for _, details := range appDetails.AgentDetails {
		allTools = append(allTools, details.ToolDeclarations...)
	}

	if len(allTools) == 0 {
		return "[]"
	}

	data, err := json.MarshalIndent(allTools, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(data)
}

// GetEvalStatus determines the evaluation status based on a score and
// threshold. If threshold is nil, returns NOT_EVALUATED. If score is nil,
// returns NOT_EVALUATED. If score >= threshold, returns PASSED. Otherwise
// returns FAILED.
func GetEvalStatus(score *float64, threshold *float64) EvalStatus {
	if threshold == nil || score == nil {
		return EvalStatusNotEvaluated
	}
	if *score >= *threshold {
		return EvalStatusPassed
	}
	return EvalStatusFailed
}

// GetAverageRubricScore computes the average score from a list of rubric
// scores, ignoring nil scores. Returns nil if no scores are available.
func GetAverageRubricScore(scores []RubricScore) *float64 {
	var total float64
	count := 0
	for _, s := range scores {
		if s.Score != nil {
			total += *s.Score
			count++
		}
	}
	if count == 0 {
		return nil
	}
	avg := total / float64(count)
	return &avg
}

// AggregateRubricScores is an alias for GetAverageRubricScore. It aggregates
// a list of rubric scores into a single average score, ignoring nil scores.
// Returns nil if no scores are available.
func AggregateRubricScores(scores []RubricScore) *float64 {
	return GetAverageRubricScore(scores)
}

// Float64Ptr returns a pointer to the given float64 value.
func Float64Ptr(v float64) *float64 { return &v }
