package eval

// Constants for old (legacy) JSON eval set format field names.
// These are used during migration from the old format to the new
// Pydantic-schema-compatible format.
const (
	// Query is the field name for the user query in old format.
	Query = "query"

	// ExpectedToolUse is the field name for expected tool calls in old format.
	ExpectedToolUse = "expected_tool_use"

	// Response is the field name for the agent response in old format.
	Response = "response"

	// Reference is the field name for the reference/golden response in old format.
	Reference = "reference"

	// ToolName is the field name for tool name within expected_tool_use entries.
	ToolName = "tool_name"

	// ToolInput is the field name for tool input within expected_tool_use entries.
	ToolInput = "tool_input"

	// MockToolOutput is the field name for mock tool output in old format.
	MockToolOutput = "mock_tool_output"

	// ExpectedIntermediateAgentResponses is the field name for expected
	// intermediate agent responses in old format.
	ExpectedIntermediateAgentResponses = "expected_intermediate_agent_responses"

	// InitialSession is the field name for initial session data in old format.
	InitialSession = "initial_session"

	// Name is the field name for the eval case name in old format.
	Name = "name"

	// Data is the field name for the eval case data array in old format.
	Data = "data"
)
