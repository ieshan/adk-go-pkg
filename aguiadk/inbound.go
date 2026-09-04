package aguiadk

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"

	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

// convertInboundMessages processes the full AG-UI input.Messages sequence and
// returns:
//   - currentUserContent: the genai.Content for the latest user message (or a
//     container with tool-response parts when the latest message is a tool
//     result with no preceding user text), suitable for passing to runner.Run.
//   - priorEvents: session.Events representing the conversation history
//     before the current turn, for seeding into a freshly created ADK session
//     so multi-turn context survives.
//
// RoleTool messages are converted to genai.FunctionResponse parts. Tool call
// IDs and names are preserved for correlation. JSON tool content is parsed
// when possible; on parse failure a fallback map {"result": content} is used.
//
// The provider string controls multimodal gating for user content, matching
// userMessageToContent's behavior.
func convertInboundMessages(messages []types.Message, provider string) (currentUserContent *genai.Content, priorEvents []*session.Event, err error) {
	if len(messages) == 0 {
		return &genai.Content{Role: "user", Parts: []*genai.Part{{Text: ""}}}, nil, nil
	}

	ctx := context.Background()
	lastUserIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == types.RoleUser {
			lastUserIdx = i
			break
		}
	}

	// Build prior events for all messages before the current user turn.
	// If there is no user message, all messages become prior events and
	// currentUserContent is built from tool responses (if any) or empty.
	priorEnd := lastUserIdx
	if priorEnd < 0 {
		priorEnd = len(messages)
	}

	for i := 0; i < priorEnd; i++ {
		msg := messages[i]
		ev := session.NewEvent(ctx, "")
		ev.Author = authorForRole(msg.Role)
		ev.LLMResponse = model.LLMResponse{
			Content: messageToGenaiContent(msg, provider),
		}
		priorEvents = append(priorEvents, ev)
	}

	// Build the current user content from the last user message.
	if lastUserIdx >= 0 {
		currentUserContent = userMessageToContent(messages[lastUserIdx], provider)
		return currentUserContent, priorEvents, nil
	}

	// No user message: build a user container from any trailing tool responses.
	currentUserContent = &genai.Content{Role: "user"}
	for i := 0; i < len(messages); i++ {
		if messages[i].Role != types.RoleTool {
			continue
		}
		currentUserContent.Parts = append(currentUserContent.Parts, toolMessageToFunctionResponsePart(messages[i]))
	}
	if len(currentUserContent.Parts) == 0 {
		currentUserContent.Parts = []*genai.Part{{Text: ""}}
	}
	return currentUserContent, priorEvents, nil
}

// authorForRole maps an AG-UI role to an ADK event author.
func authorForRole(role types.Role) string {
	switch role {
	case types.RoleUser:
		return "user"
	case types.RoleAssistant:
		return "model"
	case types.RoleSystem:
		return "system"
	case types.RoleDeveloper:
		return "developer"
	case types.RoleTool:
		return "tool"
	default:
		return string(role)
	}
}

// messageToGenaiContent converts a single AG-UI message to genai.Content for
// session seeding. Text, multimodal, tool calls, and tool responses are all
// preserved so the ADK runner sees the full history.
func messageToGenaiContent(msg types.Message, provider string) *genai.Content {
	role := genaiRoleForMessage(msg.Role)
	content := &genai.Content{Role: role}

	switch msg.Role {
	case types.RoleTool:
		content.Parts = []*genai.Part{toolMessageToFunctionResponsePart(msg)}
		return content

	case types.RoleAssistant:
		// Text content.
		if text, ok := msg.ContentString(); ok && text != "" {
			content.Parts = append(content.Parts, &genai.Part{Text: text})
		}
		// Tool calls.
		for _, tc := range msg.ToolCalls {
			var args map[string]any
			if tc.Function.Arguments != "" {
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
			}
			content.Parts = append(content.Parts, &genai.Part{
				FunctionCall: &genai.FunctionCall{
					ID:   tc.ID,
					Name: tc.Function.Name,
					Args: args,
				},
			})
		}
		return content

	case types.RoleUser:
		if text, ok := msg.ContentString(); ok {
			content.Parts = []*genai.Part{{Text: text}}
			return content
		}
		if contents, ok := msg.ContentInputContents(); ok {
			parts := inputContentsToGenaiParts(contents, provider)
			if len(parts) > 0 {
				content.Parts = parts
			}
		}
		if len(content.Parts) == 0 {
			content.Parts = []*genai.Part{{Text: ""}}
		}
		return content

	default:
		// system, developer, etc.
		if text, ok := msg.ContentString(); ok {
			content.Parts = []*genai.Part{{Text: text}}
			return content
		}
		content.Parts = []*genai.Part{{Text: ""}}
		return content
	}
}

// userMessageToContent converts a user-role AG-UI message to genai.Content
// for the current runner input. Reuses the same multimodal logic as
// messageToGenaiContent so provider gating is preserved.
func userMessageToContent(msg types.Message, provider string) *genai.Content {
	if text, ok := msg.ContentString(); ok {
		return &genai.Content{Role: "user", Parts: []*genai.Part{{Text: text}}}
	}
	if contents, ok := msg.ContentInputContents(); ok {
		parts := inputContentsToGenaiParts(contents, provider)
		if len(parts) > 0 {
			return &genai.Content{Role: "user", Parts: parts}
		}
	}
	return &genai.Content{Role: "user", Parts: []*genai.Part{{Text: ""}}}
}

// toolMessageToFunctionResponsePart converts a RoleTool AG-UI message to a
// genai.FunctionResponse part. The tool content is parsed as JSON when
// possible; on parse failure a fallback map {"result": content} is used.
// ToolCallID and Name are preserved for correlation.
func toolMessageToFunctionResponsePart(msg types.Message) *genai.Part {
	resp := parseToolResponseContent(msg.Content)
	return &genai.Part{
		FunctionResponse: &genai.FunctionResponse{
			ID:       msg.ToolCallID,
			Name:     msg.Name,
			Response: resp,
		},
	}
}

// parseToolResponseContent parses tool message content as JSON. On failure,
// returns a fallback map {"result": content}.
func parseToolResponseContent(content any) map[string]any {
	switch v := content.(type) {
	case nil:
		return map[string]any{"result": ""}
	case string:
		var parsed map[string]any
		if json.Unmarshal([]byte(v), &parsed) == nil {
			return parsed
		}
		return map[string]any{"result": v}
	case map[string]any:
		return v
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return map[string]any{"result": fmt.Sprintf("%v", v)}
		}
		var parsed map[string]any
		if json.Unmarshal(data, &parsed) == nil {
			return parsed
		}
		return map[string]any{"result": string(data)}
	}
}

// genaiRoleForMessage maps an AG-UI role to a genai.Content role string.
func genaiRoleForMessage(role types.Role) string {
	switch role {
	case types.RoleUser:
		return "user"
	case types.RoleAssistant:
		return "model"
	case types.RoleSystem:
		return "system"
	case types.RoleDeveloper:
		return "developer"
	case types.RoleTool:
		return "user" // tool responses go as user-role content in genai
	default:
		return string(role)
	}
}
