// Package anthropic provides adapters and helpers for integrating ADK-Go
// agents with Anthropic's Messages API and compatible third-party providers.
//
// The primary entry point is [New], which constructs a [model.LLM] that speaks
// the Anthropic Messages API protocol. Any server that exposes an
// Anthropic-compatible endpoint (e.g. LiteLLM, AWS Bedrock, Google Vertex AI,
// corporate gateways) can be targeted by setting [Config.BaseURL].
//
// # Basic usage
//
//	m, err := anthropic.New(anthropic.Config{
//	    Model:  "claude-opus-4",
//	    APIKey: os.Getenv("ANTHROPIC_API_KEY"),
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	req := &model.LLMRequest{
//	    Contents: []*genai.Content{
//	        {Role: "user", Parts: []*genai.Part{{Text: "Hello!"}}},
//	    },
//	}
//
//	for resp, err := range m.GenerateContent(ctx, req, false) {
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    fmt.Println(resp.Content.Parts[0].Text)
//	}
//
// # Streaming
//
// Pass stream=true to [model.LLM.GenerateContent] to enable server-sent-event
// streaming. Each partial delta is yielded with Partial=true; the final
// message_stop event is yielded with TurnComplete=true.
//
//	for resp, err := range m.GenerateContent(ctx, req, true) {
//	    if err != nil { break }
//	    if resp.TurnComplete { break }
//	    if resp.Partial {
//	        fmt.Print(resp.Content.Parts[0].Text)
//	    }
//	}
//
// # Tool use
//
// The adapter automatically translates genai [genai.Tool] declarations into
// Anthropic's input_schema format and converts FunctionCall / FunctionResponse
// parts into tool_use / tool_result blocks.
//
//	req := &model.LLMRequest{
//	    Contents: []*genai.Content{
//	        {Role: "user", Parts: []*genai.Part{{Text: "What's the weather in Paris?"}}},
//	    },
//	    Config: &genai.GenerateContentConfig{
//	        Tools: []*genai.Tool{...},
//	    },
//	}
//
// # Prompt caching
//
// Automatic caching ( Anthropic manages the breakpoint):
//
//	m, _ := anthropic.New(anthropic.Config{
//	    Model:        "claude-opus-4",
//	    APIKey:       os.Getenv("ANTHROPIC_API_KEY"),
//	    CacheControl: map[string]any{"type": "ephemeral"},
//	})
//
// Explicit block-level caching via PartMetadata:
//
//	part := genai.NewPartFromText("Large document context...")
//	part.PartMetadata = map[string]any{"cache_control": map[string]any{"type": "ephemeral"}}
//
// # Structured output
//
//	req := &model.LLMRequest{
//	    Contents: []*genai.Content{
//	        {Role: "user", Parts: []*genai.Part{{Text: "Extract name and age"}}},
//	    },
//	    Config: &genai.GenerateContentConfig{
//	        ResponseSchema: &genai.Schema{
//	            Type: genai.TypeObject,
//	            Properties: map[string]*genai.Schema{
//	                "name": {Type: genai.TypeString},
//	                "age":  {Type: genai.TypeInteger},
//	            },
//	            Required: []string{"name", "age"},
//	        },
//	    },
//	}
//
// # Third-party providers
//
// LiteLLM gateway with Bearer auth:
//
//	m, _ := anthropic.New(anthropic.Config{
//	    Model:      "claude-opus-4",
//	    APIKey:     "gateway-token",
//	    BaseURL:    "https://gateway.example.com",
//	    AuthScheme: "bearer",
//	})
//
// AWS Bedrock:
//
//	m, _ := anthropic.New(anthropic.Config{
//	    Model:      "claude-opus-4",
//	    APIKey:     "bedrock-api-key",
//	    BaseURL:    "https://bedrock-runtime.us-east-1.amazonaws.com",
//	    APIVersion: "bedrock-2023-05-31",
//	})
package anthropic
