package agui

// ReasoningTracker emits REASONING_* events for model thinking visibility.
type ReasoningTracker struct {
	emitter   *EventEmitter
	messageID string
}

// NewReasoningTracker creates a tracker for a reasoning sequence.
func NewReasoningTracker(emitter *EventEmitter, messageID string) *ReasoningTracker {
	return &ReasoningTracker{emitter: emitter, messageID: messageID}
}

// Start emits REASONING_START and REASONING_MESSAGE_START.
func (r *ReasoningTracker) Start(role string) error {
	if err := r.emitter.ReasoningStart(r.messageID); err != nil {
		return err
	}
	return r.emitter.ReasoningMessageStart(r.messageID, role)
}

// Content emits REASONING_MESSAGE_CONTENT.
func (r *ReasoningTracker) Content(delta string) error {
	return r.emitter.ReasoningMessageContent(r.messageID, delta)
}

// End emits REASONING_MESSAGE_END and REASONING_END.
func (r *ReasoningTracker) End() error {
	if err := r.emitter.ReasoningMessageEnd(r.messageID); err != nil {
		return err
	}
	return r.emitter.ReasoningEnd(r.messageID)
}
