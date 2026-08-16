package verification

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"sync"
	"time"
)

// CommitRevealVerifier implements a two-phase commit-reveal scheme to prevent
// nodes from peeking at each other's results before submitting their own.
// In the commit phase, nodes submit hash(result + nonce).
// In the reveal phase, nodes reveal their actual result and nonce for verification.
type CommitRevealVerifier struct {
	mu sync.RWMutex

	// commitPhase stores committed hashes: taskID -> nodeID -> commitHash
	commitPhase map[string]map[string][]byte

	// revealPhase stores revealed results: taskID -> nodeID -> actualResult
	revealPhase map[string]map[string]string

	// commitDeadline is the duration of the commit phase from task start
	commitDeadline time.Duration

	// revealDeadline is the duration of the reveal phase from commit deadline
	revealDeadline time.Duration

	// taskTimestamps records when each task was started
	taskTimestamps map[string]int64

	// nowFunc allows overriding time.Now for testing
	nowFunc func() time.Time
}

// NewCommitRevealVerifier creates a new CommitRevealVerifier with the specified
// commit and reveal phase durations.
func NewCommitRevealVerifier(commitDuration, revealDuration time.Duration) *CommitRevealVerifier {
	return &CommitRevealVerifier{
		commitPhase:    make(map[string]map[string][]byte),
		revealPhase:    make(map[string]map[string]string),
		commitDeadline: commitDuration,
		revealDeadline: revealDuration,
		taskTimestamps: make(map[string]int64),
		nowFunc:        time.Now,
	}
}

// StartTask begins a new task, recording its start timestamp.
// If the task already exists, it returns an error.
func (cr *CommitRevealVerifier) StartTask(taskID string) error {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	if taskID == "" {
		return errors.New("commit-reveal: empty task ID")
	}

	if _, exists := cr.taskTimestamps[taskID]; exists {
		return fmt.Errorf("commit-reveal: task %s already exists", taskID)
	}

	cr.taskTimestamps[taskID] = cr.nowFunc().UnixNano()
	cr.commitPhase[taskID] = make(map[string][]byte)
	cr.revealPhase[taskID] = make(map[string]string)

	return nil
}

// Commit allows a node to submit its commit hash during the commit phase.
// The commitHash should be sha256(result + nonce).
func (cr *CommitRevealVerifier) Commit(taskID, nodeID string, commitHash []byte) error {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	if taskID == "" {
		return errors.New("commit-reveal: empty task ID")
	}
	if nodeID == "" {
		return errors.New("commit-reveal: empty node ID")
	}
	if len(commitHash) == 0 {
		return errors.New("commit-reveal: empty commit hash")
	}

	startTime, exists := cr.taskTimestamps[taskID]
	if !exists {
		return fmt.Errorf("commit-reveal: task %s not found", taskID)
	}

	// Check if we are still in the commit phase
	now := cr.nowFunc().UnixNano()
	commitEnd := startTime + cr.commitDeadline.Nanoseconds()
	if now > commitEnd {
		return fmt.Errorf("commit-reveal: commit phase for task %s has ended", taskID)
	}

	// Check for duplicate commit
	if _, alreadyCommitted := cr.commitPhase[taskID][nodeID]; alreadyCommitted {
		return fmt.Errorf("commit-reveal: node %s already committed for task %s", nodeID, taskID)
	}

	// Store the commit hash (make a copy to avoid external mutation)
	hashCopy := make([]byte, len(commitHash))
	copy(hashCopy, commitHash)
	cr.commitPhase[taskID][nodeID] = hashCopy

	return nil
}

// Reveal allows a node to reveal its actual result and nonce during the reveal phase.
// It verifies that sha256(result + nonce) matches the previously committed hash.
func (cr *CommitRevealVerifier) Reveal(taskID, nodeID string, result string, nonce []byte) error {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	if taskID == "" {
		return errors.New("commit-reveal: empty task ID")
	}
	if nodeID == "" {
		return errors.New("commit-reveal: empty node ID")
	}

	startTime, exists := cr.taskTimestamps[taskID]
	if !exists {
		return fmt.Errorf("commit-reveal: task %s not found", taskID)
	}

	// Check timing: reveal phase starts after commit phase ends
	now := cr.nowFunc().UnixNano()
	commitEnd := startTime + cr.commitDeadline.Nanoseconds()
	revealEnd := commitEnd + cr.revealDeadline.Nanoseconds()

	if now < commitEnd {
		return fmt.Errorf("commit-reveal: reveal phase for task %s has not started yet", taskID)
	}
	if now > revealEnd {
		return fmt.Errorf("commit-reveal: reveal phase for task %s has ended", taskID)
	}

	// Check that the node has committed
	committedHash, hasCommitted := cr.commitPhase[taskID][nodeID]
	if !hasCommitted {
		return fmt.Errorf("commit-reveal: node %s has not committed for task %s", nodeID, taskID)
	}

	// Check for duplicate reveal
	if _, alreadyRevealed := cr.revealPhase[taskID][nodeID]; alreadyRevealed {
		return fmt.Errorf("commit-reveal: node %s already revealed for task %s", nodeID, taskID)
	}

	// Compute expected hash: sha256(result + nonce)
	expectedHash := computeCommitHash(result, nonce)

	// Constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare(expectedHash, committedHash) != 1 {
		return fmt.Errorf("commit-reveal: hash mismatch for node %s in task %s", nodeID, taskID)
	}

	// Store the revealed result
	cr.revealPhase[taskID][nodeID] = result

	return nil
}

// GetResults returns all revealed results for a task.
// It only returns results after the reveal phase has ended.
func (cr *CommitRevealVerifier) GetResults(taskID string) (map[string]string, error) {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	if taskID == "" {
		return nil, errors.New("commit-reveal: empty task ID")
	}

	startTime, exists := cr.taskTimestamps[taskID]
	if !exists {
		return nil, fmt.Errorf("commit-reveal: task %s not found", taskID)
	}

	// Check that reveal phase has ended
	now := cr.nowFunc().UnixNano()
	commitEnd := startTime + cr.commitDeadline.Nanoseconds()
	revealEnd := commitEnd + cr.revealDeadline.Nanoseconds()

	if now < revealEnd {
		return nil, fmt.Errorf("commit-reveal: reveal phase for task %s has not ended yet", taskID)
	}

	results := cr.revealPhase[taskID]
	if len(results) == 0 {
		return nil, fmt.Errorf("commit-reveal: no results revealed for task %s", taskID)
	}

	// Return a copy to prevent external mutation
	resultsCopy := make(map[string]string, len(results))
	for k, v := range results {
		resultsCopy[k] = v
	}

	return resultsCopy, nil
}

// IsCommitPhase returns true if the task is currently in the commit phase.
func (cr *CommitRevealVerifier) IsCommitPhase(taskID string) bool {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	startTime, exists := cr.taskTimestamps[taskID]
	if !exists {
		return false
	}

	now := cr.nowFunc().UnixNano()
	commitEnd := startTime + cr.commitDeadline.Nanoseconds()

	return now >= startTime && now <= commitEnd
}

// IsRevealPhase returns true if the task is currently in the reveal phase.
func (cr *CommitRevealVerifier) IsRevealPhase(taskID string) bool {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	startTime, exists := cr.taskTimestamps[taskID]
	if !exists {
		return false
	}

	now := cr.nowFunc().UnixNano()
	commitEnd := startTime + cr.commitDeadline.Nanoseconds()
	revealEnd := commitEnd + cr.revealDeadline.Nanoseconds()

	return now > commitEnd && now <= revealEnd
}

// computeCommitHash computes sha256(result + nonce).
func computeCommitHash(result string, nonce []byte) []byte {
	h := sha256.New()
	h.Write([]byte(result))
	h.Write(nonce)
	return h.Sum(nil)
}

// ComputeCommitHash is the exported version for use by nodes to create their commit hash.
// It computes sha256(result + nonce).
func ComputeCommitHash(result string, nonce []byte) []byte {
	return computeCommitHash(result, nonce)
}
