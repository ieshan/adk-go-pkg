# Plan: ToolToStateMapper Suppress Boolean Refactor

## Goal
Change `ToolToStateMapper` to return a boolean indicating whether the tool call should be suppressed, enabling per-tool-call control over suppression.

## Changes

### 1. `bridge.go` — Type signature change
- **Line 136-139**: Change `ToolToStateMapper` from `func(toolName string, args map[string]any) []events.JSONPatchOperation` to `func(toolName string, args map[string]any) ([]events.JSONPatchOperation, bool)`
- The `bool` return: `true` = suppress tool call (emit state delta), `false` = emit normal tool events

### 2. `bridge.go` — Doc comments (lines 78-90)
- Update `SuppressToolEvents` comment: mapper now returns `(ops, suppress)` instead of using nil to signal fallback
- Update `ToolToStateMapper` comment: document the bool return value

### 3. `bridge.go` — `emitFunctionCall` (lines 887-894)
- Change from `if ops := t.toolToStateMapper(fc.Name, fc.Args); ops != nil` to `if ops, suppress := t.toolToStateMapper(fc.Name, fc.Args); suppress`
- When `suppress=true`: emit STATE_DELTA (if ops non-nil) and return
- When `suppress=false`: fall through to normal emission (START/ARGS/END)
- Suppressed calls return before populating `toolCallIDs`, so `emitFunctionResponse` won't find them

### 4. `bridge.go` — `emitFunctionResponse` (lines 1020-1024)
- Remove the blanket `if t.suppressTools { return }` check
- Suppressed tool calls naturally won't be found in `toolCallIDs` (returned early in emitFunctionCall), so they'll hit the "no matching tool call, skip" path
- Non-suppressed tool calls (mapper returned false) will have entries in `toolCallIDs` and get their TOOL_CALL_RESULT

### 5. `bridge_test.go` — Update test mappers (lines 1644-1651, 1718-1720)
- `TestBridge_SuppressedToolMode`: mapper returns `([]ops, true)` for "update_doc", `(nil, false)` for others
- `TestBridge_SuppressedToolModeMapperReturnsNil`: mapper returns `(nil, false)` — still falls through to normal emission

### 6. `presets_test.go` — Update test mappers (lines 77-81, 139-143)
- `TestPresets_SharedState`: mapper returns `([]ops, true)`
- `TestPresets_AgenticGenerativeUI`: mapper returns `([]ops, true)`
- Both tests also need to update the call site to handle the new return signature (line 92, 160)

### 7. Run tests
- `go test ./aguiadk/...` to verify all changes
