package orchestrator

import (
	"fmt"
	"sync"
	"time"
)

// TaskState represents the lifecycle state of a task
type TaskState string

const (
	TaskStateCreated     TaskState = "created"
	TaskStateAssigned    TaskState = "assigned"
	TaskStateCommitPhase TaskState = "commit_phase"
	TaskStateRevealPhase TaskState = "reveal_phase"
	TaskStateVerifying   TaskState = "verifying"
	TaskStateVerified    TaskState = "verified"
	TaskStateFailed      TaskState = "failed"
	TaskStateSettled     TaskState = "settled"
)

// validTransitions defines which state transitions are allowed
var validTransitions = map[TaskState][]TaskState{
	TaskStateCreated:     {TaskStateAssigned},
	TaskStateAssigned:    {TaskStateCommitPhase},
	TaskStateCommitPhase: {TaskStateRevealPhase},
	TaskStateRevealPhase: {TaskStateVerifying},
	TaskStateVerifying:   {TaskStateVerified, TaskStateFailed},
	TaskStateVerified:    {TaskStateSettled},
	TaskStateFailed:      {TaskStateSettled},
}

// Task represents an inference task to be processed by the network
type Task struct {
	mu sync.RWMutex

	ID          string            // Unique task ID
	Prompt      string            // Input prompt for inference
	RequesterID string            // ID of the entity requesting inference
	State       TaskState         // Current lifecycle state
	CreatedAt   int64             // Unix timestamp of creation
	UpdatedAt   int64             // Unix timestamp of last update
	AssignedTo  []string          // Node IDs assigned to this task
	MinNodes    int               // Minimum nodes required
	Timeout     time.Duration     // Task timeout
	Results     map[string]string // nodeID -> result (populated during reveal)
	FinalResult string            // Consensus result after verification
	Metadata    map[string]string // Arbitrary metadata
}

// NewTask creates a new task in the Created state
func NewTask(id, prompt, requesterID string, minNodes int, timeout time.Duration) *Task {
	now := time.Now().Unix()
	return &Task{
		ID:          id,
		Prompt:      prompt,
		RequesterID: requesterID,
		State:       TaskStateCreated,
		CreatedAt:   now,
		UpdatedAt:   now,
		AssignedTo:  make([]string, 0),
		MinNodes:    minNodes,
		Timeout:     timeout,
		Results:     make(map[string]string),
		Metadata:    make(map[string]string),
	}
}

// TransitionTo moves the task to a new state if the transition is valid
func (t *Task) TransitionTo(newState TaskState) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	allowed, ok := validTransitions[t.State]
	if !ok {
		return fmt.Errorf("task: no transitions from state %s", t.State)
	}

	for _, s := range allowed {
		if s == newState {
			t.State = newState
			t.UpdatedAt = time.Now().Unix()
			return nil
		}
	}

	return fmt.Errorf("task: invalid transition from %s to %s", t.State, newState)
}

// GetState returns the current state (thread-safe)
func (t *Task) GetState() TaskState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.State
}

// SetResult stores a node's result
func (t *Task) SetResult(nodeID, result string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Results[nodeID] = result
	t.UpdatedAt = time.Now().Unix()
}

// GetResults returns a copy of all results
func (t *Task) GetResults() map[string]string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	results := make(map[string]string, len(t.Results))
	for k, v := range t.Results {
		results[k] = v
	}
	return results
}

// IsExpired checks if the task has exceeded its timeout
func (t *Task) IsExpired() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return time.Now().Unix() > t.CreatedAt+int64(t.Timeout.Seconds())
}

// AssignNodes assigns a list of node IDs to this task
func (t *Task) AssignNodes(nodeIDs []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.AssignedTo = make([]string, len(nodeIDs))
	copy(t.AssignedTo, nodeIDs)
	t.UpdatedAt = time.Now().Unix()
}
