package orchestrator

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sync"
)

// NodeInfo represents a registered inference node
type NodeInfo struct {
	ID        string
	PublicKey []byte
	ModelID   []byte
	Stake     float64
	Active    bool
}

// Scheduler manages task assignment and node selection
type Scheduler struct {
	mu    sync.RWMutex
	nodes map[string]*NodeInfo
}

// NewScheduler creates a new task scheduler
func NewScheduler() *Scheduler {
	return &Scheduler{
		nodes: make(map[string]*NodeInfo),
	}
}

// RegisterNode registers a node for task assignment
func (s *Scheduler) RegisterNode(node *NodeInfo) error {
	if node == nil {
		return errors.New("scheduler: nil node")
	}
	if node.ID == "" {
		return errors.New("scheduler: empty node ID")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.nodes[node.ID] = node
	return nil
}

// UnregisterNode removes a node from task assignment
func (s *Scheduler) UnregisterNode(nodeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.nodes, nodeID)
}

// SelectNodes randomly selects n active nodes for a task
// Uses cryptographic randomness to prevent prediction attacks
func (s *Scheduler) SelectNodes(n int) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Collect active nodes
	active := make([]string, 0)
	for id, node := range s.nodes {
		if node.Active {
			active = append(active, id)
		}
	}

	if len(active) < n {
		return nil, fmt.Errorf("scheduler: insufficient active nodes (have %d, need %d)", len(active), n)
	}

	// Fisher-Yates shuffle with crypto/rand
	selected := make([]string, len(active))
	copy(selected, active)

	for i := len(selected) - 1; i > 0; i-- {
		j, err := cryptoRandInt(i + 1)
		if err != nil {
			return nil, fmt.Errorf("scheduler: random selection failed: %w", err)
		}
		selected[i], selected[j] = selected[j], selected[i]
	}

	return selected[:n], nil
}

// GetNode returns info about a specific node
func (s *Scheduler) GetNode(nodeID string) (*NodeInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	node, ok := s.nodes[nodeID]
	return node, ok
}

// ActiveNodeCount returns the number of active nodes
func (s *Scheduler) ActiveNodeCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, node := range s.nodes {
		if node.Active {
			count++
		}
	}
	return count
}

// SetNodeActive sets a node's active status
func (s *Scheduler) SetNodeActive(nodeID string, active bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.nodes[nodeID]
	if !ok {
		return fmt.Errorf("scheduler: node %s not found", nodeID)
	}
	node.Active = active
	return nil
}

// cryptoRandInt returns a cryptographically random integer in [0, max)
func cryptoRandInt(max int) (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}
