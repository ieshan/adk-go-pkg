package agui

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	jsonpatch "github.com/evanphx/json-patch/v5"
)

// StateManager tracks shared application state and supports
// snapshot/delta operations using RFC 6902 JSON Patch.
//
// StateManager serves as the project's "DocState" — it provides the same
// Apply/Snapshot semantics as the example server's docstate.go, plus Diff
// and Set. There is no separate DocState type; creating one would be a
// redundant subset of StateManager.
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
	if err = json.Unmarshal(data, &deepCopy); err != nil {
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

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("agui: marshal patch: %w", err)
	}

	decodedPatch, err := jsonpatch.DecodePatch(patchBytes)
	if err != nil {
		return fmt.Errorf("agui: decode patch: %w", err)
	}

	stateBytes, err := json.Marshal(s.state)
	if err != nil {
		return fmt.Errorf("agui: marshal state: %w", err)
	}

	applied, err := decodedPatch.Apply(stateBytes)
	if err != nil {
		return fmt.Errorf("agui: apply patch: %w", err)
	}

	s.state = make(map[string]any)
	if err := json.Unmarshal(applied, &s.state); err != nil {
		return fmt.Errorf("agui: unmarshal patched state: %w", err)
	}
	return nil
}

// Diff computes JSON Patch operations to transform current state to newState.
func (s *StateManager) Diff(newState any) ([]events.JSONPatchOperation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var target map[string]any
	data, err := json.Marshal(newState)
	if err != nil {
		return nil, fmt.Errorf("marshal new state: %w", err)
	}
	if err := json.Unmarshal(data, &target); err != nil {
		return nil, err
	}
	return diffMaps("", s.state, target), nil
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
			oj, oErr := json.Marshal(ov)
			nj, nErr := json.Marshal(nv)
			if oErr != nil || nErr != nil {
				ops = append(ops, events.JSONPatchOperation{Op: "replace", Path: path, Value: nv})
				continue
			}
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
