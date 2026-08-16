package consensus

import (
	"sync"
	"time"
)

// BlockEvent represents an event that triggers block creation
type BlockEvent struct {
	TaskID          string            `json:"task_id"`           // ZKML task ID
	FinalResult    string            `json:"final_result"`    // Final verification result
	IsValid        bool              `json:"is_valid"`        // Verification success
	AgreementRate  float64           `json:"agreement_rate"`  // Node consensus rate
	NodeResults    map[string]string `json:"node_results"`    // Node results
	ConsensusNodes []string          `json:"consensus_nodes"`  // Nodes in consensus
	DisagreeingNodes []string        `json:"disagreeing_nodes"` // Disagreeing nodes
	Metadata       map[string]string `json:"metadata"`        // Additional metadata
	Timestamp      int64             `json:"timestamp"`        // Event timestamp
	BlockHeight    uint64            `json:"block_height"`    // Expected block height
	Priority       EventPriority     `json:"priority"`        // Event priority
}

// EventPriority represents the priority level of an event
type EventPriority int

const (
	PriorityLow    EventPriority = iota
	PriorityNormal
	PriorityHigh
	PriorityUrgent
)

// String returns the string representation of priority
func (p EventPriority) String() string {
	switch p {
	case PriorityLow:
		return "low"
	case PriorityNormal:
		return "normal"
	case PriorityHigh:
		return "high"
	case PriorityUrgent:
		return "urgent"
	default:
		return "unknown"
	}
}

// NewBlockEvent creates a new block event
func NewBlockEvent(
	taskID string,
	finalResult string,
	isValid bool,
	agreementRate float64,
	nodeResults map[string]string,
	consensusNodes []string,
	disagreeingNodes []string,
	metadata map[string]string,
) *BlockEvent {
	return &BlockEvent{
		TaskID:          taskID,
		FinalResult:    finalResult,
		IsValid:        isValid,
		AgreementRate:  agreementRate,
		NodeResults:    nodeResults,
		ConsensusNodes: consensusNodes,
		DisagreeingNodes: disagreeingNodes,
		Metadata:       metadata,
		Timestamp:      time.Now().Unix(),
		Priority:       PriorityNormal, // Default priority
	}
}

// NewBlockEventWithPriority creates a new block event with specified priority
func NewBlockEventWithPriority(
	taskID string,
	finalResult string,
	isValid bool,
	agreementRate float64,
	nodeResults map[string]string,
	consensusNodes []string,
	disagreeingNodes []string,
	metadata map[string]string,
	priority EventPriority,
) *BlockEvent {
	return &BlockEvent{
		TaskID:          taskID,
		FinalResult:    finalResult,
		IsValid:        isValid,
		AgreementRate:  agreementRate,
		NodeResults:    nodeResults,
		ConsensusNodes: consensusNodes,
		DisagreeingNodes: disagreeingNodes,
		Metadata:       metadata,
		Timestamp:      time.Now().Unix(),
		Priority:       priority,
	}
}

// TaskVerifiedHandler is a function type that handles verified tasks
type TaskVerifiedHandler func(event *BlockEvent)

// EventChannel returns a channel for receiving block events
type EventChannel chan *BlockEvent

// NewEventChannel creates a new event channel
func NewEventChannel(bufferSize int) EventChannel {
	if bufferSize <= 0 {
		bufferSize = 10
	}
	return make(chan *BlockEvent, bufferSize)
}

// EventType represents the type of consensus event
type EventType string

const (
	EventTaskVerified    EventType = "task_verified"
	EventBlockProduced   EventType = "block_produced"
	EventBlockVerified   EventType = "block_verified"
	EventChainReorganized EventType = "chain_reorganized"
)

// ConsensusEvent represents a consensus-related event
type ConsensusEvent struct {
	Type      EventType     `json:"type"`       // Event type
	BlockHash []byte        `json:"block_hash"` // Related block hash
	Height    uint64        `json:"height"`     // Block height
	Data      interface{}   `json:"data"`       // Event data
	Timestamp time.Time     `json:"timestamp"`  // Event timestamp
}

// NewConsensusEvent creates a new consensus event
func NewConsensusEvent(eventType EventType, blockHash []byte, height uint64, data interface{}) *ConsensusEvent {
	return &ConsensusEvent{
		Type:      eventType,
		BlockHash: blockHash,
		Height:    height,
		Data:      data,
		Timestamp: time.Now(),
	}
}

// EventFilter defines a filter for events
type EventFilter interface {
	Match(event *BlockEvent) bool
}

// EventFilterFunc is a function-based event filter
type EventFilterFunc func(event *BlockEvent) bool

// Match implements EventFilter interface
func (f EventFilterFunc) Match(event *BlockEvent) bool {
	return f(event)
}

// Common event filters
var (
	// FilterValidEvents only matches valid events
	FilterValidEvents = EventFilterFunc(func(event *BlockEvent) bool {
		return event.IsValid
	})

	// FilterInvalidEvents only matches invalid events
	FilterInvalidEvents = EventFilterFunc(func(event *BlockEvent) bool {
		return !event.IsValid
	})

	// FilterHighAgreement only matches events with high agreement rate (>= 0.8)
	FilterHighAgreement = EventFilterFunc(func(event *BlockEvent) bool {
		return event.AgreementRate >= 0.8
	})

	// FilterLowAgreement only matches events with low agreement rate (< 0.5)
	FilterLowAgreement = EventFilterFunc(func(event *BlockEvent) bool {
		return event.AgreementRate < 0.5
	})

	// FilterUrgentPriority only matches urgent events
	FilterUrgentPriority = EventFilterFunc(func(event *BlockEvent) bool {
		return event.Priority == PriorityUrgent
	})

	// FilterHighOrUrgent only matches high or urgent events
	FilterHighOrUrgent = EventFilterFunc(func(event *BlockEvent) bool {
		return event.Priority >= PriorityHigh
	})
)

// CompositeFilter combines multiple filters with AND logic
type CompositeFilter struct {
	filters []EventFilter
}

// NewCompositeFilter creates a composite filter with AND logic
func NewCompositeFilter(filters ...EventFilter) *CompositeFilter {
	return &CompositeFilter{filters: filters}
}

// Match implements EventFilter interface
func (cf *CompositeFilter) Match(event *BlockEvent) bool {
	for _, f := range cf.filters {
		if !f.Match(event) {
			return false
		}
	}
	return true
}

// OrFilter combines multiple filters with OR logic
type OrFilter struct {
	filters []EventFilter
}

// NewOrFilter creates a composite filter with OR logic
func NewOrFilter(filters ...EventFilter) *OrFilter {
	return &OrFilter{filters: filters}
}

// Match implements EventFilter interface
func (of *OrFilter) Match(event *BlockEvent) bool {
	for _, f := range of.filters {
		if f.Match(event) {
			return true
		}
	}
	return false
}

// NotFilter inverts a filter
type NotFilter struct {
	filter EventFilter
}

// NewNotFilter creates an inverted filter
func NewNotFilter(filter EventFilter) *NotFilter {
	return &NotFilter{filter: filter}
}

// Match implements EventFilter interface
func (nf *NotFilter) Match(event *BlockEvent) bool {
	return !nf.filter.Match(event)
}

// PriorityEventQueue is a priority queue for events
type PriorityEventQueue struct {
	items  []*priorityItem
	mu     sync.RWMutex
	cond   *sync.Cond
	closed bool
}

type priorityItem struct {
	event    *BlockEvent
	priority EventPriority
	seq      uint64 // Sequence number for FIFO ordering within same priority
}

// NewPriorityEventQueue creates a new priority event queue
func NewPriorityEventQueue() *PriorityEventQueue {
	q := &PriorityEventQueue{
		items: make([]*priorityItem, 0),
	}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// Push adds an event to the queue
func (q *PriorityEventQueue) Push(event *BlockEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return
	}

	item := &priorityItem{
		event:    event,
		priority: event.Priority,
		seq:      uint64(len(q.items)), // Simple sequence
	}

	// Insert in priority order (higher priority first)
	q.items = q.insertOrdered(q.items, item)
	q.cond.Signal()
}

// Pop removes and returns the highest priority event
func (q *PriorityEventQueue) Pop() *BlockEvent {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.items) == 0 && !q.closed {
		q.cond.Wait()
	}

	if q.closed {
		return nil
	}

	if len(q.items) == 0 {
		return nil
	}

	item := q.items[0]
	q.items = q.items[1:]
	return item.event
}

// TryPop tries to pop without blocking
func (q *PriorityEventQueue) TryPop() *BlockEvent {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 || q.closed {
		return nil
	}

	item := q.items[0]
	q.items = q.items[1:]
	return item.event
}

// Peek returns the highest priority event without removing it
func (q *PriorityEventQueue) Peek() *BlockEvent {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if len(q.items) == 0 {
		return nil
	}
	return q.items[0].event
}

// Len returns the number of events in the queue
func (q *PriorityEventQueue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.items)
}

// Close closes the queue
func (q *PriorityEventQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closed = true
	q.cond.Broadcast()
}

// insertOrdered inserts an item in the correct priority order
func (q *PriorityEventQueue) insertOrdered(items []*priorityItem, item *priorityItem) []*priorityItem {
	// Find the right position (higher priority = lower index value, earlier in queue)
	i := 0
	for i < len(items) && items[i].priority > item.priority {
		i++
	}

	// For same priority, append to end (FIFO)
	for i < len(items) && items[i].priority == item.priority {
		i++
	}

	// Insert at position i
	if i == len(items) {
		return append(items, item)
	}

	items = append(items, nil)
	copy(items[i+1:], items[i:])
	items[i] = item
	return items
}

// EventBuffer provides persistence and recovery for events
type EventBuffer struct {
	mu          sync.RWMutex
	events      []*BlockEvent
	maxSize     int
	persistPath string
	persisted   bool
}

// NewEventBuffer creates a new event buffer
func NewEventBuffer(maxSize int) *EventBuffer {
	return &EventBuffer{
		events:  make([]*BlockEvent, 0, maxSize),
		maxSize: maxSize,
	}
}

// Add adds an event to the buffer
func (eb *EventBuffer) Add(event *BlockEvent) error {
	if event == nil {
		return nil
	}

	eb.mu.Lock()
	defer eb.mu.Unlock()

	// Create a copy to avoid reference issues
	eventCopy := &BlockEvent{
		TaskID:           event.TaskID,
		FinalResult:      event.FinalResult,
		IsValid:          event.IsValid,
		AgreementRate:    event.AgreementRate,
		NodeResults:      copyStringMap(event.NodeResults),
		ConsensusNodes:   copyStringSlice(event.ConsensusNodes),
		DisagreeingNodes: copyStringSlice(event.DisagreeingNodes),
		Metadata:         copyStringMap(event.Metadata),
		Timestamp:        event.Timestamp,
		BlockHeight:      event.BlockHeight,
		Priority:         event.Priority,
	}

	eb.events = append(eb.events, eventCopy)

	// Maintain max size
	if len(eb.events) > eb.maxSize {
		eb.events = eb.events[1:]
	}

	return nil
}

// GetEvents returns all events in the buffer
func (eb *EventBuffer) GetEvents() []*BlockEvent {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	result := make([]*BlockEvent, len(eb.events))
	copy(result, eb.events)
	return result
}

// GetFilteredEvents returns events matching the filter
func (eb *EventBuffer) GetFilteredEvents(filter EventFilter) []*BlockEvent {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	result := make([]*BlockEvent, 0)
	for _, event := range eb.events {
		if filter == nil || filter.Match(event) {
			result = append(result, event)
		}
	}
	return result
}

// Clear clears all events from the buffer
func (eb *EventBuffer) Clear() {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.events = make([]*BlockEvent, 0, eb.maxSize)
}

// Len returns the number of events in the buffer
func (eb *EventBuffer) Len() int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	return len(eb.events)
}

// GetEventByID finds an event by TaskID
func (eb *EventBuffer) GetEventByID(taskID string) *BlockEvent {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	for _, event := range eb.events {
		if event.TaskID == taskID {
			return event
		}
	}
	return nil
}

// GetEventsByPriority returns events with the specified priority
func (eb *EventBuffer) GetEventsByPriority(priority EventPriority) []*BlockEvent {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	result := make([]*BlockEvent, 0)
	for _, event := range eb.events {
		if event.Priority == priority {
			result = append(result, event)
		}
	}
	return result
}

// GetEventsInRange returns events within a time range
func (eb *EventBuffer) GetEventsInRange(start, end int64) []*BlockEvent {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	result := make([]*BlockEvent, 0)
	for _, event := range eb.events {
		if event.Timestamp >= start && event.Timestamp <= end {
			result = append(result, event)
		}
	}
	return result
}

// RemoveEvents removes events from the buffer that match the filter
func (eb *EventBuffer) RemoveEvents(filter EventFilter) int {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	newEvents := make([]*BlockEvent, 0)
	removed := 0

	for _, event := range eb.events {
		if filter == nil || !filter.Match(event) {
			newEvents = append(newEvents, event)
		} else {
			removed++
		}
	}

	eb.events = newEvents
	return removed
}

// Recover restores events from another buffer
func (eb *EventBuffer) Recover(source *EventBuffer) int {
	if source == nil {
		return 0
	}

	source.mu.RLock()
	events := source.GetEvents()
	source.mu.RUnlock()

	count := 0
	for _, event := range events {
		if eb.Add(event) == nil {
			count++
		}
	}
	return count
}

// Helper functions for copying
func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

func copyStringSlice(s []string) []string {
	if s == nil {
		return nil
	}
	result := make([]string, len(s))
	copy(result, s)
	return result
}
