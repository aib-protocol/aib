package orchestrator

import (
	"sync"
)

// EventType represents the type of an orchestrator event
type EventType string

const (
	EventTaskCreated          EventType = "task_created"
	EventTaskAssigned         EventType = "task_assigned"
	EventCommitPhaseStarted   EventType = "commit_phase_started"
	EventRevealPhaseStarted   EventType = "reveal_phase_started"
	EventResultSubmitted      EventType = "result_submitted"
	EventVerificationStarted  EventType = "verification_started"
	EventVerificationComplete EventType = "verification_complete"
	EventVerificationFailed   EventType = "verification_failed"
	EventSlashTriggered       EventType = "slash_triggered"
	EventTaskSettled          EventType = "task_settled"
)

// Event represents an orchestrator lifecycle event
type Event struct {
	Type      EventType
	TaskID    string
	NodeID    string // relevant node (if applicable)
	Data      interface{}
	Timestamp int64
}

// EventHandler is a function that handles an event
type EventHandler func(event *Event)

// EventBus provides publish-subscribe event routing between orchestrator components
type EventBus struct {
	mu       sync.RWMutex
	handlers map[EventType][]EventHandler
}

// NewEventBus creates a new event bus
func NewEventBus() *EventBus {
	return &EventBus{
		handlers: make(map[EventType][]EventHandler),
	}
}

// Subscribe registers a handler for a specific event type
func (eb *EventBus) Subscribe(eventType EventType, handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.handlers[eventType] = append(eb.handlers[eventType], handler)
}

// Publish sends an event to all registered handlers
func (eb *EventBus) Publish(event *Event) {
	eb.mu.RLock()
	handlers := make([]EventHandler, len(eb.handlers[event.Type]))
	copy(handlers, eb.handlers[event.Type])
	eb.mu.RUnlock()

	for _, handler := range handlers {
		handler(event)
	}
}

// SubscribeAll registers a handler for all event types
func (eb *EventBus) SubscribeAll(handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	for _, eventType := range []EventType{
		EventTaskCreated,
		EventTaskAssigned,
		EventCommitPhaseStarted,
		EventRevealPhaseStarted,
		EventResultSubmitted,
		EventVerificationStarted,
		EventVerificationComplete,
		EventVerificationFailed,
		EventSlashTriggered,
		EventTaskSettled,
	} {
		eb.handlers[eventType] = append(eb.handlers[eventType], handler)
	}
}
