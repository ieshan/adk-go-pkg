package agui

import (
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// StepTracker emits STEP_STARTED/STEP_FINISHED events around a function call.
func StepTracker(emitter *EventEmitter, stepName string, fn func() error) error {
	if err := emitter.StepStarted(stepName); err != nil {
		return err
	}
	fnErr := fn()
	if err := emitter.StepFinished(stepName); err != nil {
		return err
	}
	return fnErr
}

// ActivityTracker provides helpers for emitting activity events.
type ActivityTracker struct {
	emitter      *EventEmitter
	messageID    string
	activityType string
}

// NewActivityTracker creates a tracker for a specific activity.
func NewActivityTracker(emitter *EventEmitter, messageID, activityType string) *ActivityTracker {
	return &ActivityTracker{emitter: emitter, messageID: messageID, activityType: activityType}
}

// Snapshot emits an ACTIVITY_SNAPSHOT.
func (a *ActivityTracker) Snapshot(content any, replace *bool) error {
	return a.emitter.ActivitySnapshot(a.messageID, a.activityType, content, replace)
}

// Delta emits an ACTIVITY_DELTA with JSON Patch operations.
func (a *ActivityTracker) Delta(patch []events.JSONPatchOperation) error {
	return a.emitter.ActivityDelta(a.messageID, a.activityType, patch)
}
