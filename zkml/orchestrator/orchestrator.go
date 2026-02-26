package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/zkml/slashing"
	"github.com/aib-protocol/aib/zkml/verification"
)

// InferenceProvider is the interface that real inference engines must implement
type InferenceProvider interface {
	// Infer runs inference on the given prompt and returns the result
	Infer(ctx context.Context, prompt string) (string, error)
	// ModelID returns the model fingerprint hash
	ModelID() []byte
}

// OrchestratorConfig holds configuration for the orchestrator
type OrchestratorConfig struct {
	MinNodes       int           // Minimum nodes per task (default 5)
	CommitDuration time.Duration // Duration of commit phase
	RevealDuration time.Duration // Duration of reveal phase
	TaskTimeout    time.Duration // Overall task timeout
	AutoSlash      bool          // Automatically slash disagreeing nodes
}

// DefaultOrchestratorConfig returns sensible defaults
func DefaultOrchestratorConfig() *OrchestratorConfig {
	return &OrchestratorConfig{
		MinNodes:       5,
		CommitDuration: 30 * time.Second,
		RevealDuration: 30 * time.Second,
		TaskTimeout:    5 * time.Minute,
		AutoSlash:      true,
	}
}

// Orchestrator coordinates the end-to-end lifecycle of inference tasks.
// It connects verification, slashing, commit-reveal, and scheduling into
// a unified pipeline.
type Orchestrator struct {
	mu sync.RWMutex

	config    *OrchestratorConfig
	scheduler *Scheduler
	verifier  *verification.MajorityVerifier
	commitRev *verification.CommitRevealVerifier
	slashEng  *slashing.SlashEngine
	eventBus  *EventBus

	tasks map[string]*Task // taskID -> Task

	// Metrics
	totalTasks      int
	completedTasks  int
	failedTasks     int
	totalSlashes    int
}

// NewOrchestrator creates a new orchestrator with the given configuration
func NewOrchestrator(config *OrchestratorConfig) *Orchestrator {
	if config == nil {
		config = DefaultOrchestratorConfig()
	}

	verifier := verification.NewMajorityVerifier()
	verifier.SetMinNodes(config.MinNodes)

	return &Orchestrator{
		config:    config,
		scheduler: NewScheduler(),
		verifier:  verifier,
		commitRev: verification.NewCommitRevealVerifier(config.CommitDuration, config.RevealDuration),
		slashEng:  slashing.NewSlashEngine(nil),
		eventBus:  NewEventBus(),
		tasks:     make(map[string]*Task),
	}
}

// Scheduler returns the orchestrator's scheduler for node registration
func (o *Orchestrator) Scheduler() *Scheduler {
	return o.scheduler
}

// EventBus returns the orchestrator's event bus for subscribing to events
func (o *Orchestrator) EventBus() *EventBus {
	return o.eventBus
}

// SlashEngine returns the orchestrator's slash engine
func (o *Orchestrator) SlashEngine() *slashing.SlashEngine {
	return o.slashEng
}

// SubmitTask creates a new inference task and begins the orchestration pipeline
func (o *Orchestrator) SubmitTask(prompt, requesterID string) (*Task, error) {
	if prompt == "" {
		return nil, errors.New("orchestrator: empty prompt")
	}
	if requesterID == "" {
		return nil, errors.New("orchestrator: empty requester ID")
	}

	taskID := generateTaskID()

	task := NewTask(taskID, prompt, requesterID, o.config.MinNodes, o.config.TaskTimeout)

	o.mu.Lock()
	o.tasks[taskID] = task
	o.totalTasks++
	o.mu.Unlock()

	o.eventBus.Publish(&Event{
		Type:      EventTaskCreated,
		TaskID:    taskID,
		Timestamp: time.Now().Unix(),
	})

	// Assign nodes
	if err := o.assignNodes(task); err != nil {
		return task, fmt.Errorf("orchestrator: failed to assign nodes: %w", err)
	}

	return task, nil
}

// assignNodes selects and assigns nodes to a task
func (o *Orchestrator) assignNodes(task *Task) error {
	nodeIDs, err := o.scheduler.SelectNodes(task.MinNodes)
	if err != nil {
		return err
	}

	task.AssignNodes(nodeIDs)
	if err := task.TransitionTo(TaskStateAssigned); err != nil {
		return err
	}

	o.eventBus.Publish(&Event{
		Type:      EventTaskAssigned,
		TaskID:    task.ID,
		Data:      nodeIDs,
		Timestamp: time.Now().Unix(),
	})

	return nil
}

// StartCommitPhase initiates the commit phase for a task
func (o *Orchestrator) StartCommitPhase(taskID string) error {
	task, err := o.getTask(taskID)
	if err != nil {
		return err
	}

	if err := task.TransitionTo(TaskStateCommitPhase); err != nil {
		return fmt.Errorf("orchestrator: %w", err)
	}

	if err := o.commitRev.StartTask(taskID); err != nil {
		return fmt.Errorf("orchestrator: failed to start commit-reveal: %w", err)
	}

	o.eventBus.Publish(&Event{
		Type:      EventCommitPhaseStarted,
		TaskID:    taskID,
		Timestamp: time.Now().Unix(),
	})

	return nil
}

// SubmitCommit allows a node to submit its commit hash
func (o *Orchestrator) SubmitCommit(taskID, nodeID string, commitHash []byte) error {
	task, err := o.getTask(taskID)
	if err != nil {
		return err
	}

	if task.GetState() != TaskStateCommitPhase {
		return fmt.Errorf("orchestrator: task %s is not in commit phase (state: %s)", taskID, task.GetState())
	}

	return o.commitRev.Commit(taskID, nodeID, commitHash)
}

// StartRevealPhase transitions the task to the reveal phase
func (o *Orchestrator) StartRevealPhase(taskID string) error {
	task, err := o.getTask(taskID)
	if err != nil {
		return err
	}

	if err := task.TransitionTo(TaskStateRevealPhase); err != nil {
		return fmt.Errorf("orchestrator: %w", err)
	}

	o.eventBus.Publish(&Event{
		Type:      EventRevealPhaseStarted,
		TaskID:    taskID,
		Timestamp: time.Now().Unix(),
	})

	return nil
}

// SubmitReveal allows a node to reveal its result and nonce
func (o *Orchestrator) SubmitReveal(taskID, nodeID, result string, nonce []byte) error {
	task, err := o.getTask(taskID)
	if err != nil {
		return err
	}

	if task.GetState() != TaskStateRevealPhase {
		return fmt.Errorf("orchestrator: task %s is not in reveal phase (state: %s)", taskID, task.GetState())
	}

	if err := o.commitRev.Reveal(taskID, nodeID, result, nonce); err != nil {
		return err
	}

	task.SetResult(nodeID, result)

	o.eventBus.Publish(&Event{
		Type:      EventResultSubmitted,
		TaskID:    taskID,
		NodeID:    nodeID,
		Timestamp: time.Now().Unix(),
	})

	return nil
}

// Verify runs majority verification and triggers slashing if needed.
// This is the critical integration point between verification and slashing.
func (o *Orchestrator) Verify(taskID string) (*verification.VerificationResult, error) {
	task, err := o.getTask(taskID)
	if err != nil {
		return nil, err
	}

	if err := task.TransitionTo(TaskStateVerifying); err != nil {
		return nil, fmt.Errorf("orchestrator: %w", err)
	}

	o.eventBus.Publish(&Event{
		Type:      EventVerificationStarted,
		TaskID:    taskID,
		Timestamp: time.Now().Unix(),
	})

	// Run majority verification
	results := task.GetResults()
	vResult, err := o.verifier.Verify(taskID, results)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: verification error: %w", err)
	}

	if vResult.IsValid {
		if err := task.TransitionTo(TaskStateVerified); err != nil {
			return nil, err
		}
		task.mu.Lock()
		task.FinalResult = vResult.MajorityResult
		task.mu.Unlock()

		o.mu.Lock()
		o.completedTasks++
		o.mu.Unlock()

		o.eventBus.Publish(&Event{
			Type:      EventVerificationComplete,
			TaskID:    taskID,
			Data:      vResult,
			Timestamp: time.Now().Unix(),
		})
	} else {
		if err := task.TransitionTo(TaskStateFailed); err != nil {
			return nil, err
		}

		o.mu.Lock()
		o.failedTasks++
		o.mu.Unlock()

		o.eventBus.Publish(&Event{
			Type:      EventVerificationFailed,
			TaskID:    taskID,
			Data:      vResult,
			Timestamp: time.Now().Unix(),
		})
	}

	// Auto-slash disagreeing nodes (both for valid and invalid results)
	if o.config.AutoSlash && len(vResult.Disagreeing) > 0 {
		o.slashDisagreeingNodes(taskID, vResult.Disagreeing)
	}

	return vResult, nil
}

// SettleTask marks a task as settled (final state)
func (o *Orchestrator) SettleTask(taskID string) error {
	task, err := o.getTask(taskID)
	if err != nil {
		return err
	}

	if err := task.TransitionTo(TaskStateSettled); err != nil {
		return fmt.Errorf("orchestrator: %w", err)
	}

	o.eventBus.Publish(&Event{
		Type:      EventTaskSettled,
		TaskID:    taskID,
		Timestamp: time.Now().Unix(),
	})

	return nil
}

// slashDisagreeingNodes creates evidence and submits to slash engine
func (o *Orchestrator) slashDisagreeingNodes(taskID string, disagreeing []string) {
	for _, nodeID := range disagreeing {
		evidence := &slashing.Evidence{
			Type:        slashing.Misbehavior,
			Offender:    []byte(nodeID),
			Reporter:    []byte("orchestrator"),
			Timestamp:   time.Now().Unix(),
			Description: fmt.Sprintf("Disagreed with majority on task %s", taskID),
			ProofData:   []byte(fmt.Sprintf("task:%s:node:%s", taskID, nodeID)),
			TaskID:      taskID,
			Severity:    4, // Low-medium for simple disagreement
		}

		if err := o.slashEng.SubmitEvidence(evidence); err != nil {
			// Log but don't fail the verification
			continue
		}

		o.mu.Lock()
		o.totalSlashes++
		o.mu.Unlock()

		o.eventBus.Publish(&Event{
			Type:      EventSlashTriggered,
			TaskID:    taskID,
			NodeID:    nodeID,
			Data:      evidence,
			Timestamp: time.Now().Unix(),
		})
	}
}

// GetTask returns a task by ID (read-only)
func (o *Orchestrator) GetTask(taskID string) (*Task, error) {
	return o.getTask(taskID)
}

func (o *Orchestrator) getTask(taskID string) (*Task, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	task, ok := o.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("orchestrator: task %s not found", taskID)
	}
	return task, nil
}

// GetMetrics returns orchestrator metrics
func (o *Orchestrator) GetMetrics() *OrchestratorMetrics {
	o.mu.RLock()
	defer o.mu.RUnlock()

	return &OrchestratorMetrics{
		TotalTasks:     o.totalTasks,
		CompletedTasks: o.completedTasks,
		FailedTasks:    o.failedTasks,
		TotalSlashes:   o.totalSlashes,
		ActiveNodes:    o.scheduler.ActiveNodeCount(),
		PendingTasks:   o.totalTasks - o.completedTasks - o.failedTasks,
	}
}

// OrchestratorMetrics holds operational metrics
type OrchestratorMetrics struct {
	TotalTasks     int
	CompletedTasks int
	FailedTasks    int
	TotalSlashes   int
	ActiveNodes    int
	PendingTasks   int
}

// generateTaskID generates a random task ID
func generateTaskID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "task_" + hex.EncodeToString(b)
}
