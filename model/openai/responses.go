package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"sort"
	"strings"

	"github.com/ieshan/adk-go-pkg/internal/jsonutil"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

// Sentinel errors for the Responses API path. All are prefixed to avoid
// collision with the Chat Completions path errors.
var (
	ErrStopSequencesNotSupported      = errors.New("openai: stop sequences are not supported by the responses api")
	ErrTopKNotSupported               = errors.New("openai: topk is not supported by the responses api")
	ErrMultipleCandidatesNotSupported = errors.New("openai: multiple candidates per request are not supported by the responses api")
	ErrPenaltiesNotSupported          = errors.New("openai: frequency/presence penalties are not supported by the responses api")
	ErrLabelsNotSupported             = errors.New("openai: request labels are not supported by the responses api")
	ErrSafetySettingsNotSupported     = errors.New("openai: gemini safety settings are not supported by the responses api")
	ErrUnsupportedContentPart         = errors.New("openai: unsupported content part for the responses api")
	ErrUnsupportedMIMEType            = errors.New("openai: unsupported mime type for the responses api")
	ErrNoContents                     = errors.New("openai: llm request has no contents to convert")
	ErrNoOutputItems                  = errors.New("openai: response included no output items")
	ErrNoTextOrToolContent            = errors.New("openai: response output did not contain text or tool content")
	ErrUnsupportedOutputItemType      = errors.New("openai: unsupported output item type")
	ErrUnsupportedMessageContentType  = errors.New("openai: unsupported message content type")
	ErrFunctionCallMissingName        = errors.New("openai: function call missing name")
	ErrFunctionResponseMissingCallID  = errors.New("openai: function response missing call id and no pending calls")
	ErrFunctionResponseUnknownCallID  = errors.New("openai: function response references unknown call id")
)

// ErrEmptyJSONSchema is an alias for [jsonutil.ErrEmptyJSONSchema], preserved
// for backward compatibility.
var ErrEmptyJSONSchema = jsonutil.ErrEmptyJSONSchema

// responsesRequest is the request body for the OpenAI /v1/responses endpoint.
type responsesRequest struct {
	Model           string               `json:"model"`
	Input           []responsesInputItem `json:"input"`
	Instructions    string               `json:"instructions,omitempty"`
	Temperature     *float32             `json:"temperature,omitempty"`
	TopP            *float32             `json:"top_p,omitempty"`
	MaxOutputTokens *int32               `json:"max_output_tokens,omitempty"`
	Stream          bool                 `json:"stream,omitempty"`
	Tools           []map[string]any     `json:"tools,omitempty"`
	ToolChoice      any                  `json:"tool_choice,omitempty"`
	Text            *responsesTextConfig `json:"text,omitempty"`
}

// responsesTextConfig holds the text format configuration for structured output.
type responsesTextConfig struct {
	Format any `json:"format"`
}

// responsesInputItem represents a single item in the Responses API input array.
type responsesInputItem struct {
	Type      string                 `json:"type"`
	Role      string                 `json:"role,omitempty"`
	Content   []responsesContentPart `json:"content,omitempty"`
	CallID    string                 `json:"call_id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Arguments string                 `json:"arguments,omitempty"`
	Output    string                 `json:"output,omitempty"`
}

// responsesContentPart is a single content part within a message input item.
type responsesContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// responsesResponse is the response body from the OpenAI /v1/responses endpoint.
type responsesResponse struct {
	ID                string                  `json:"id"`
	Model             string                  `json:"model"`
	Output            []responsesOutputItem   `json:"output"`
	Usage             *responsesUsage         `json:"usage,omitempty"`
	IncompleteDetails *responsesIncompleteDet `json:"incomplete_details,omitempty"`
}

// responsesOutputItem is a single item in the Responses API output array.
type responsesOutputItem struct {
	Type      string                   `json:"type"`
	Role      string                   `json:"role,omitempty"`
	Content   []responsesOutputContent `json:"content,omitempty"`
	CallID    string                   `json:"call_id,omitempty"`
	Name      string                   `json:"name,omitempty"`
	Arguments string                   `json:"arguments,omitempty"`
	ID        string                   `json:"id,omitempty"`
}

// responsesOutputContent is a content part within an output message or reasoning item.
type responsesOutputContent struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Refusal string `json:"refusal,omitempty"`
}

// responsesUsage holds token-count statistics for a Responses API request.
type responsesUsage struct {
	InputTokens         int64                  `json:"input_tokens"`
	OutputTokens        int64                  `json:"output_tokens"`
	TotalTokens         int64                  `json:"total_tokens"`
	InputTokensDetails  *responsesTokenDetails `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *responsesTokenDetails `json:"output_tokens_details,omitempty"`
}

// responsesTokenDetails holds detailed token breakdowns.
type responsesTokenDetails struct {
	CachedTokens    int64 `json:"cached_tokens,omitempty"`
	ReasoningTokens int64 `json:"reasoning_tokens,omitempty"`
}

// responsesIncompleteDet describes why a response was incomplete.
type responsesIncompleteDet struct {
	Reason string `json:"reason,omitempty"`
}

// callTracker generates and tracks function call IDs for the Responses API.
// It is per-request (created fresh in buildResponsesRequest), so it is
// concurrency-safe by construction.
type callTracker struct {
	nextID       int
	pendingCalls []string // call IDs that have not yet received a response
}

// newCallID generates a deterministic call ID when FunctionCall.ID is empty.
func (ct *callTracker) newCallID() string {
	ct.nextID++
	return fmt.Sprintf("adk-openai-call-%d", ct.nextID)
}

// registerCall records a pending function call ID.
func (ct *callTracker) registerCall(id string) {
	ct.pendingCalls = append(ct.pendingCalls, id)
}

// resolveResponseCallID matches a FunctionResponse to a pending FunctionCall.
// If the response ID is non-empty, it must match a pending call.
// If the response ID is empty, the oldest pending call is used.
func (ct *callTracker) resolveResponseCallID(id string) (string, error) {
	if id != "" {
		for _, pending := range ct.pendingCalls {
			if pending == id {
				return id, nil
			}
		}
		return "", fmt.Errorf("%w: %s", ErrFunctionResponseUnknownCallID, id)
	}
	if len(ct.pendingCalls) == 0 {
		return "", ErrFunctionResponseMissingCallID
	}
	// Use the oldest pending call (FIFO).
	oldest := ct.pendingCalls[0]
	ct.pendingCalls = ct.pendingCalls[1:]
	return oldest, nil
}

// buildResponsesRequest converts an ADK [model.LLMRequest] into a
// [responsesRequest] ready to be sent to the /v1/responses endpoint.
//
// Returns an error for unsupported config fields (matching the ADK's
// openaimodel behaviour) and for untranslatable content parts.
func buildResponsesRequest(req *model.LLMRequest, modelName string, stream bool) (responsesRequest, error) {
	if req == nil {
		return responsesRequest{}, fmt.Errorf("openai: request is nil")
	}

	rr := responsesRequest{
		Model:  modelName,
		Stream: stream,
	}

	// req.Model overrides the construction-time model name.
	if req.Model != "" {
		rr.Model = req.Model
	}

	// Apply generation config fields when present.
	if req.Config != nil {
		cfg := req.Config

		// Error on unsupported config fields (matching ADK request.go:323-384).
		if len(cfg.StopSequences) > 0 {
			return rr, ErrStopSequencesNotSupported
		}
		if cfg.TopK != nil {
			return rr, ErrTopKNotSupported
		}
		if cfg.CandidateCount > 1 {
			return rr, ErrMultipleCandidatesNotSupported
		}
		if cfg.FrequencyPenalty != nil || cfg.PresencePenalty != nil {
			return rr, ErrPenaltiesNotSupported
		}
		if len(cfg.Labels) > 0 {
			return rr, ErrLabelsNotSupported
		}
		if len(cfg.SafetySettings) > 0 {
			return rr, ErrSafetySettingsNotSupported
		}

		// Forwarded config fields.
		rr.Temperature = cfg.Temperature
		rr.TopP = cfg.TopP
		if cfg.MaxOutputTokens > 0 {
			rr.MaxOutputTokens = &cfg.MaxOutputTokens
		}

		// System instruction → instructions field.
		if cfg.SystemInstruction != nil {
			rr.Instructions = extractText(cfg.SystemInstruction)
		}

		// Structured output via text.format.
		if cfg.ResponseSchema != nil {
			name := cfg.ResponseSchema.Title
			if name == "" {
				name = "response"
			}
			schemaMap := schemaToJSONSchema(cfg.ResponseSchema)
			if schemaMap == nil {
				schemaMap = map[string]any{}
			}
			enforceStrictOpenAISchema(schemaMap)
			rr.Text = &responsesTextConfig{
				Format: map[string]any{
					"type":   "json_schema",
					"name":   name,
					"strict": true,
					"schema": schemaMap,
				},
			}
		} else if cfg.ResponseJsonSchema != nil {
			schemaMap, err := jsonutil.NormalizeSchema(cfg.ResponseJsonSchema)
			if err != nil {
				return rr, err
			}
			if schemaMap == nil {
				schemaMap = map[string]any{}
			}
			enforceStrictOpenAISchema(schemaMap)
			name := "response"
			if title, ok := schemaMap["title"].(string); ok && title != "" {
				name = title
			}
			rr.Text = &responsesTextConfig{
				Format: map[string]any{
					"type":   "json_schema",
					"name":   name,
					"strict": true,
					"schema": schemaMap,
				},
			}
		} else if cfg.ResponseMIMEType == "application/json" {
			rr.Text = &responsesTextConfig{
				Format: map[string]any{"type": "json_object"},
			}
		} else if cfg.ResponseMIMEType != "" && cfg.ResponseMIMEType != "text/plain" {
			return rr, fmt.Errorf("%w: %s", ErrUnsupportedMIMEType, cfg.ResponseMIMEType)
		}

		// Tool declarations (flat format).
		if len(cfg.Tools) > 0 {
			tools, err := translateToolDeclarationsResponses(cfg.Tools)
			if err != nil {
				return rr, err
			}
			rr.Tools = tools
		}

		// Tool choice.
		if cfg.ToolConfig != nil {
			rr.ToolChoice = translateToolConfigResponses(cfg.ToolConfig)
		}
	}

	// Convert conversation contents to input items.
	if len(req.Contents) == 0 {
		return rr, ErrNoContents
	}

	ct := &callTracker{}
	input, err := contentsToInputItems(req.Contents, ct)
	if err != nil {
		return rr, err
	}
	rr.Input = input

	return rr, nil
}

// contentsToInputItems converts a slice of [genai.Content] to Responses API
// input items, following the ADK's convertContents (request.go:86-160).
func contentsToInputItems(contents []*genai.Content, ct *callTracker) ([]responsesInputItem, error) {
	var items []responsesInputItem
	for _, c := range contents {
		if c == nil {
			continue
		}
		converted, err := contentToInputItems(c, ct)
		if err != nil {
			return nil, err
		}
		items = append(items, converted...)
	}
	return items, nil
}

// contentToInputItems converts a single [genai.Content] to one or more Responses
// API input items.
func contentToInputItems(c *genai.Content, ct *callTracker) ([]responsesInputItem, error) {
	role := openAIRole(c.Role)
	contentType := "input_text"
	if role == "assistant" {
		contentType = "output_text"
	}

	var items []responsesInputItem
	var textParts []*genai.Part

	flushText := func() error {
		if len(textParts) == 0 {
			return nil
		}
		var sb strings.Builder
		for _, p := range textParts {
			sb.WriteString(p.Text)
		}
		text := sb.String()
		textParts = nil
		if strings.TrimSpace(text) == "" {
			return nil
		}
		items = append(items, responsesInputItem{
			Type:    "message",
			Role:    role,
			Content: []responsesContentPart{{Type: contentType, Text: text}},
		})
		return nil
	}

	for _, p := range c.Parts {
		switch {
		case p.FunctionCall != nil:
			if err := flushText(); err != nil {
				return nil, err
			}
			fc := p.FunctionCall
			if fc.Name == "" {
				return nil, ErrFunctionCallMissingName
			}
			callID := fc.ID
			if callID == "" {
				callID = ct.newCallID()
			}
			ct.registerCall(callID)
			argsJSON, err := json.Marshal(fc.Args)
			if err != nil {
				argsJSON = []byte("{}")
			}
			items = append(items, responsesInputItem{
				Type:      "function_call",
				CallID:    callID,
				Name:      fc.Name,
				Arguments: string(argsJSON),
			})
		case p.FunctionResponse != nil:
			if err := flushText(); err != nil {
				return nil, err
			}
			fr := p.FunctionResponse
			callID, err := ct.resolveResponseCallID(fr.ID)
			if err != nil {
				return nil, err
			}
			output, err := json.Marshal(fr.Response)
			if err != nil {
				output = []byte("null")
			}
			items = append(items, responsesInputItem{
				Type:   "function_call_output",
				CallID: callID,
				Output: string(output),
			})
		case p.InlineData != nil || p.FileData != nil:
			return nil, ErrUnsupportedContentPart
		default:
			// Text part (including Thought parts — preserved as text).
			// Skip empty/whitespace-only text parts (match ADK newMessage).
			if p.Text != "" && strings.TrimSpace(p.Text) != "" {
				textParts = append(textParts, p)
			}
		}
	}

	if err := flushText(); err != nil {
		return nil, err
	}

	return items, nil
}

// translateToolDeclarationsResponses converts a slice of [genai.Tool] into the
// flat tool format expected by the /v1/responses endpoint.
//
// Each function declaration produces one entry:
//
//	{"type":"function","name":"...","description":"...","parameters":{...}}
//
// Non-function tools (retrieval, google search, etc.) return an error.
func translateToolDeclarationsResponses(tools []*genai.Tool) ([]map[string]any, error) {
	var result []map[string]any
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		// Non-function tools are not supported.
		if tool.Retrieval != nil || tool.GoogleSearch != nil || tool.FileSearch != nil ||
			tool.GoogleMaps != nil || tool.CodeExecution != nil || tool.ComputerUse != nil ||
			tool.EnterpriseWebSearch != nil {
			return nil, fmt.Errorf("openai: non-function tool type is not supported by the responses api")
		}
		for _, decl := range tool.FunctionDeclarations {
			if decl == nil {
				continue
			}
			fn := map[string]any{
				"type": "function",
				"name": decl.Name,
			}
			if decl.Description != "" {
				fn["description"] = decl.Description
			}
			params, err := resolveToolParameters(decl)
			if err != nil {
				return nil, fmt.Errorf("openai: tool %q: %w", decl.Name, err)
			}
			fn["parameters"] = params
			result = append(result, fn)
		}
	}
	return result, nil
}

// translateToolConfigResponses converts a [genai.ToolConfig] to the Responses
// API tool_choice value.
//
// Mapping:
//   - AUTO, no allowed names → omit (nil — let API decide)
//   - AUTO, with allowed names → {type:"allowed_tools", mode:"auto", tools:[...]}
//   - NONE → "none"
//   - ANY, no allowed names → "required"
//   - ANY, with allowed names → {type:"allowed_tools", mode:"required", tools:[...]}
func translateToolConfigResponses(cfg *genai.ToolConfig) any {
	if cfg == nil || cfg.FunctionCallingConfig == nil {
		return nil
	}
	fcc := cfg.FunctionCallingConfig
	allowed := fcc.AllowedFunctionNames
	switch fcc.Mode {
	case genai.FunctionCallingConfigModeAuto:
		if len(allowed) == 0 {
			return nil
		}
		return buildAllowedTools("auto", allowed)
	case genai.FunctionCallingConfigModeAny:
		if len(allowed) == 0 {
			return "required"
		}
		return buildAllowedTools("required", allowed)
	case genai.FunctionCallingConfigModeNone:
		return "none"
	default:
		return nil
	}
}

// buildAllowedTools constructs the allowed_tools tool_choice object.
func buildAllowedTools(mode string, names []string) map[string]any {
	tools := make([]map[string]any, 0, len(names))
	for _, name := range names {
		tools = append(tools, map[string]any{"type": "function", "name": name})
	}
	return map[string]any{
		"type":  "allowed_tools",
		"mode":  mode,
		"tools": tools,
	}
}

// translateResponsesResponse converts a [responsesResponse] into an ADK
// [model.LLMResponse], following the ADK's convertResponse and convertOutputItems.
func translateResponsesResponse(resp *responsesResponse) (*model.LLMResponse, error) {
	if len(resp.Output) == 0 {
		return nil, ErrNoOutputItems
	}

	llmResp := &model.LLMResponse{
		Content: &genai.Content{Role: "model"},
	}

	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			for _, c := range item.Content {
				switch c.Type {
				case "output_text":
					llmResp.Content.Parts = append(llmResp.Content.Parts, &genai.Part{Text: c.Text})
				case "refusal":
					llmResp.Content.Parts = append(llmResp.Content.Parts, &genai.Part{Text: c.Refusal})
				default:
					return nil, fmt.Errorf("%w: %s", ErrUnsupportedMessageContentType, c.Type)
				}
			}
		case "function_call":
			var args map[string]any
			if item.Arguments != "" {
				if err := json.Unmarshal([]byte(item.Arguments), &args); err != nil {
					return nil, fmt.Errorf("openai: parse function call arguments: %w", err)
				}
			} else {
				args = map[string]any{}
			}
			llmResp.Content.Parts = append(llmResp.Content.Parts, &genai.Part{
				FunctionCall: &genai.FunctionCall{
					ID:   item.CallID,
					Name: item.Name,
					Args: args,
				},
			})
		case "reasoning":
			for _, c := range item.Content {
				if c.Text != "" {
					llmResp.Content.Parts = append(llmResp.Content.Parts, &genai.Part{
						Text:    c.Text,
						Thought: true,
					})
				}
			}
		default:
			return nil, fmt.Errorf("%w: %s", ErrUnsupportedOutputItemType, item.Type)
		}
	}

	if len(llmResp.Content.Parts) == 0 {
		return nil, ErrNoTextOrToolContent
	}

	llmResp.FinishReason = responsesFinishReason(resp.IncompleteDetails)

	if resp.Usage != nil {
		llmResp.UsageMetadata = translateResponsesUsage(resp.Usage)
	}

	// Attach metadata.
	if llmResp.CustomMetadata == nil {
		llmResp.CustomMetadata = make(map[string]any)
	}
	if resp.ID != "" {
		llmResp.CustomMetadata["openai_response_id"] = resp.ID
	}
	if resp.Model != "" {
		llmResp.CustomMetadata["openai_model"] = resp.Model
	}

	return llmResp, nil
}

// responsesFinishReason maps the Responses API incomplete_details reason to a
// genai FinishReason.
func responsesFinishReason(incomplete *responsesIncompleteDet) genai.FinishReason {
	if incomplete == nil || incomplete.Reason == "" {
		return genai.FinishReasonStop
	}
	switch incomplete.Reason {
	case "max_output_tokens":
		return genai.FinishReasonMaxTokens
	case "content_filter":
		return genai.FinishReasonSafety
	default:
		return genai.FinishReasonOther
	}
}

// translateResponsesUsage maps Responses API usage to genai usage metadata.
func translateResponsesUsage(u *responsesUsage) *genai.GenerateContentResponseUsageMetadata {
	meta := &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:     safeInt32(u.InputTokens),
		CandidatesTokenCount: safeInt32(u.OutputTokens),
		TotalTokenCount:      safeInt32(u.TotalTokens),
	}
	if u.InputTokensDetails != nil {
		meta.CachedContentTokenCount = safeInt32(u.InputTokensDetails.CachedTokens)
	}
	if u.OutputTokensDetails != nil {
		meta.ThoughtsTokenCount = safeInt32(u.OutputTokensDetails.ReasoningTokens)
	}
	return meta
}

// safeInt32 clamps an int64 to the int32 range to prevent overflow.
func safeInt32(n int64) int32 {
	if n > int64(^uint32(0)>>1) {
		return int32(^uint32(0) >> 1)
	}
	if n < int64(-int32(^uint32(0)>>1)-1) {
		return -int32(^uint32(0)>>1) - 1
	}
	return int32(n)
}

// enforceStrictOpenAISchema recursively modifies a JSON Schema map to comply
// with OpenAI's strict mode requirements:
//   - Sets additionalProperties:false on all object types.
//   - Ensures all properties are listed in the required array (sorted).
//   - Handles $ref (strips other keys), $defs, anyOf/oneOf/allOf, items.
//
// Ported from the ADK's enforceStrictOpenAISchema (request.go:463-525).
func enforceStrictOpenAISchema(schema map[string]any) {
	enforceStrictOpenAISchemaRecursive(schema)
}

// enforceStrictOpenAISchemaRecursive recurses into the schema map.
func enforceStrictOpenAISchemaRecursive(node map[string]any) {
	// Handle $ref — strip all other keys.
	if _, hasRef := node["$ref"]; hasRef {
		ref := node["$ref"]
		clear(node)
		node["$ref"] = ref
		return
	}

	// Recurse into $defs.
	if defs, ok := node["$defs"].(map[string]any); ok {
		for _, def := range defs {
			if dm, ok := def.(map[string]any); ok {
				enforceStrictOpenAISchemaRecursive(dm)
			}
		}
	}

	// Recurse into anyOf / oneOf / allOf.
	for _, key := range []string{"anyOf", "oneOf", "allOf"} {
		if arr, ok := node[key].([]any); ok {
			for _, sub := range arr {
				if sm, ok := sub.(map[string]any); ok {
					enforceStrictOpenAISchemaRecursive(sm)
				}
			}
		}
	}

	// Recurse into items.
	if items, ok := node["items"].(map[string]any); ok {
		enforceStrictOpenAISchemaRecursive(items)
	}

	// For object types, set additionalProperties:false and ensure all
	// properties are required.
	if node["type"] == "object" || hasProperties(node) {
		node["additionalProperties"] = false
		if props, ok := node["properties"].(map[string]any); ok {
			for _, prop := range props {
				if pm, ok := prop.(map[string]any); ok {
					enforceStrictOpenAISchemaRecursive(pm)
				}
			}
			required := make([]string, 0, len(props))
			for name := range props {
				required = append(required, name)
			}
			sort.Strings(required)
			node["required"] = required
		}
	}
}

// hasProperties returns true if the node has a non-empty "properties" map.
func hasProperties(node map[string]any) bool {
	props, ok := node["properties"].(map[string]any)
	return ok && len(props) > 0
}

// generateResponses sends the request to the OpenAI Responses endpoint and
// yields [model.LLMResponse] values.
//
// When stream is false, the endpoint is called without streaming; the response
// body is read in full, unmarshalled as a [responsesResponse], and translated
// using [translateResponsesResponse]. A single response is yielded with
// TurnComplete set to true.
//
// When stream is true, the response body is treated as an SSE stream and
// forwarded to [parseResponsesStream]. The caller must consume the full
// iterator or cancel ctx to release resources.
//
// Non-2xx HTTP status codes cause a single error to be yielded and the iterator
// stops.
func (m *openaiModel) generateResponses(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		// 1. Build the responses request.
		respReq, err := buildResponsesRequest(req, m.model, stream)
		if err != nil {
			yield(nil, fmt.Errorf("openai: build request: %w", err))
			return
		}

		// 2. Marshal to JSON.
		body, err := json.Marshal(respReq)
		if err != nil {
			yield(nil, fmt.Errorf("openai: marshal request: %w", err))
			return
		}

		// 3. Create HTTP POST to {baseURL}/responses.
		url := m.baseURL + "/responses"
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			yield(nil, fmt.Errorf("openai: create HTTP request: %w", err))
			return
		}

		// 4. Set standard and custom headers.
		m.setHeaders(httpReq)

		// 5. Execute the request.
		resp, err := m.client.Do(httpReq)
		if err != nil {
			yield(nil, fmt.Errorf("openai: HTTP request: %w", err))
			return
		}
		defer func() { _ = resp.Body.Close() }()

		// 6. Check status code.
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			errBody, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				yield(nil, &HTTPError{StatusCode: resp.StatusCode, Body: fmt.Sprintf("body read failed: %v: %s", readErr, errBody)})
				return
			}
			yield(nil, &HTTPError{StatusCode: resp.StatusCode, Body: string(errBody)})
			return
		}

		// 7. Streaming path — delegate to parseResponsesStream.
		if stream {
			for r, e := range parseResponsesStream(ctx, resp.Body) {
				if !yield(r, e) {
					return
				}
			}
			return
		}

		// 8. Non-streaming path — read body, unmarshal, translate, yield once.
		rawBody, err := io.ReadAll(resp.Body)
		if err != nil {
			yield(nil, fmt.Errorf("openai: read response body: %w", err))
			return
		}

		var respObj responsesResponse
		if err = json.Unmarshal(rawBody, &respObj); err != nil {
			yield(nil, fmt.Errorf("openai: unmarshal response: %w", err))
			return
		}

		llmResp, err := translateResponsesResponse(&respObj)
		if err != nil {
			yield(nil, fmt.Errorf("openai: translate response: %w", err))
			return
		}
		llmResp.TurnComplete = true
		yield(llmResp, nil)
	}
}
