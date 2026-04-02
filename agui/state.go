package agui

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// StateManager tracks shared application state and supports
// snapshot/delta operations using RFC 6902 JSON Patch.
type StateManager struct {
	mu    sync.RWMutex
	state map[string]any
}

// NewStateManager creates a state manager with the given initial state.
// The initial value must be JSON-serializable to a map (object). Arrays and
// scalar values are not supported — the state is always stored as map[string]any.
// Returns an error if the initial state cannot be JSON-serialized.
func NewStateManager(initial any) (*StateManager, error) {
	sm := &StateManager{}
	if initial != nil {
		data, err := json.Marshal(initial)
		if err != nil {
			return nil, fmt.Errorf("agui: invalid initial state: %w", err)
		}
		if err := json.Unmarshal(data, &sm.state); err != nil {
			return nil, fmt.Errorf("agui: invalid initial state: %w", err)
		}
	}
	if sm.state == nil {
		sm.state = make(map[string]any)
	}
	return sm, nil
}

// Snapshot returns a deep copy of the current state.
func (s *StateManager) Snapshot() any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := json.Marshal(s.state)
	if err != nil {
		return nil
	}
	var deepCopy map[string]any
	if err := json.Unmarshal(data, &deepCopy); err != nil {
		return nil
	}
	return deepCopy
}

// Set replaces the entire state.
func (s *StateManager) Set(state any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("agui: invalid state: %w", err)
	}
	s.state = make(map[string]any)
	if err := json.Unmarshal(data, &s.state); err != nil {
		return fmt.Errorf("agui: invalid state: %w", err)
	}
	return nil
}

// Apply applies RFC 6902 JSON Patch operations to the current state.
func (s *StateManager) Apply(patch []events.JSONPatchOperation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, op := range patch {
		switch op.Op {
		case "add":
			if err := setPath(s.state, op.Path, op.Value); err != nil {
				return fmt.Errorf("add %s: %w", op.Path, err)
			}
		case "remove":
			if err := removePath(s.state, op.Path); err != nil {
				return fmt.Errorf("remove %s: %w", op.Path, err)
			}
		case "replace":
			if err := setPath(s.state, op.Path, op.Value); err != nil {
				return fmt.Errorf("replace %s: %w", op.Path, err)
			}
		case "move":
			val, err := getPath(s.state, op.From)
			if err != nil {
				return fmt.Errorf("move from %s: %w", op.From, err)
			}
			if err = removePath(s.state, op.From); err != nil {
				return fmt.Errorf("move from %s: %w", op.From, err)
			}
			if err = setPath(s.state, op.Path, val); err != nil {
				return fmt.Errorf("move to %s: %w", op.Path, err)
			}
		case "copy":
			val, err := getPath(s.state, op.From)
			if err != nil {
				return fmt.Errorf("copy from %s: %w", op.From, err)
			}
			// Deep copy the value
			data, _ := json.Marshal(val)
			var copied any
			if err = json.Unmarshal(data, &copied); err != nil {
				return fmt.Errorf("copy from %s: %w", op.From, err)
			}
			if err = setPath(s.state, op.Path, copied); err != nil {
				return fmt.Errorf("copy to %s: %w", op.Path, err)
			}
		case "test":
			val, err := getPath(s.state, op.Path)
			if err != nil {
				return fmt.Errorf("test %s: %w", op.Path, err)
			}
			expected, _ := json.Marshal(op.Value)
			actual, _ := json.Marshal(val)
			if string(expected) != string(actual) {
				return fmt.Errorf("test %s: value mismatch", op.Path)
			}
		default:
			return fmt.Errorf("unknown operation: %s", op.Op)
		}
	}
	return nil
}

// Diff computes JSON Patch operations to transform current state to newState.
func (s *StateManager) Diff(newState any) ([]events.JSONPatchOperation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var target map[string]any
	data, _ := json.Marshal(newState)
	if err := json.Unmarshal(data, &target); err != nil {
		return nil, err
	}
	return diffMaps("", s.state, target), nil
}

// parsePath splits a JSON Pointer path into segments.
// "/a/b/c" -> ["a", "b", "c"], "" -> []
func parsePath(path string) []string {
	if path == "" {
		return nil
	}
	// RFC 6901: "/" targets the empty-string key at root
	return strings.Split(strings.TrimPrefix(path, "/"), "/")
}

// getPath retrieves a value at a JSON Pointer path.
func getPath(obj map[string]any, path string) (any, error) {
	segments := parsePath(path)
	var current any = obj
	for _, seg := range segments {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("not an object at %s", seg)
		}
		current, ok = m[seg]
		if !ok {
			return nil, fmt.Errorf("key %s not found", seg)
		}
	}
	return current, nil
}

// setPath sets a value at a JSON Pointer path, creating intermediate maps.
func setPath(obj map[string]any, path string, value any) error {
	segments := parsePath(path)
	if len(segments) == 0 {
		return fmt.Errorf("empty path")
	}
	current := obj
	for _, seg := range segments[:len(segments)-1] {
		next, ok := current[seg]
		if !ok {
			next = make(map[string]any)
			current[seg] = next
		}
		current, ok = next.(map[string]any)
		if !ok {
			return fmt.Errorf("not an object at %s", seg)
		}
	}
	current[segments[len(segments)-1]] = value
	return nil
}

// removePath removes a value at a JSON Pointer path.
func removePath(obj map[string]any, path string) error {
	segments := parsePath(path)
	if len(segments) == 0 {
		return fmt.Errorf("empty path")
	}
	current := obj
	for _, seg := range segments[:len(segments)-1] {
		next, ok := current[seg].(map[string]any)
		if !ok {
			return fmt.Errorf("not an object at %s", seg)
		}
		current = next
	}
	delete(current, segments[len(segments)-1])
	return nil
}

// diffMaps computes patch operations between two maps.
func diffMaps(prefix string, old, new map[string]any) []events.JSONPatchOperation {
	var ops []events.JSONPatchOperation
	for k, nv := range new {
		path := prefix + "/" + k
		ov, exists := old[k]
		if !exists {
			ops = append(ops, events.JSONPatchOperation{Op: "add", Path: path, Value: nv})
			continue
		}
		om, omOk := ov.(map[string]any)
		nm, nmOk := nv.(map[string]any)
		if omOk && nmOk {
			ops = append(ops, diffMaps(path, om, nm)...)
		} else {
			oj, _ := json.Marshal(ov)
			nj, _ := json.Marshal(nv)
			if string(oj) != string(nj) {
				ops = append(ops, events.JSONPatchOperation{Op: "replace", Path: path, Value: nv})
			}
		}
	}
	for k := range old {
		if _, exists := new[k]; !exists {
			ops = append(ops, events.JSONPatchOperation{Op: "remove", Path: prefix + "/" + k})
		}
	}
	return ops
}
