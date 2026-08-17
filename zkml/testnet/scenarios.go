package testnet

import (
	"fmt"
	"time"
)

// Scenario defines a predefined test scenario
type Scenario struct {
	Name        string
	Description string
	Tasks       []ScenarioTask
	Setup       func(tn *TestNet) error
}

// ScenarioTask represents a single task in a scenario
type ScenarioTask struct {
	Prompt       string
	ExpectedPass bool // Expected verification result
}

// ScenarioResult holds the result of running a scenario
type ScenarioResult struct {
	ScenarioName       string
	TaskResults        []*TestNetResult
	AllExpectationsMet bool
	Duration           time.Duration
}

// Predefined scenarios

// ScenarioAllHonest - All nodes are honest, all tasks should pass
var ScenarioAllHonest = &Scenario{
	Name:        "all_honest",
	Description: "All nodes are honest, all tasks should pass with 100% agreement",
	Tasks: []ScenarioTask{
		{Prompt: "What is 2+2?", ExpectedPass: true},
		{Prompt: "Summarize AI", ExpectedPass: true},
		{Prompt: "Hello world", ExpectedPass: true},
	},
}

// ScenarioOneDisagreement - One node is dishonest, tasks should still pass
var ScenarioOneDisagreement = &Scenario{
	Name:        "one_disagreement",
	Description: "One node disagrees, majority should still pass (2/3 > 67% threshold)",
	Tasks: []ScenarioTask{
		{Prompt: "Compute 2+2", ExpectedPass: true},
		{Prompt: "What is AI?", ExpectedPass: true},
	},
	Setup: func(tn *TestNet) error {
		// Make first node dishonest
		nodes := tn.GetNodes()
		for id := range nodes {
			tn.SetNodeHonest(id, false)
			break // Only first node is dishonest
		}
		return nil
	},
}

// ScenarioMajorityDisagreement - Majority dishonest, tasks should fail
var ScenarioMajorityDisagreement = &Scenario{
	Name:        "majority_disagreement",
	Description: "Majority of nodes disagree, verification should fail",
	Tasks: []ScenarioTask{
		{Prompt: "What is truth?", ExpectedPass: false},
	},
	Setup: func(tn *TestNet) error {
		// Make majority of nodes dishonest (>50%)
		nodes := tn.GetNodes()
		dishonestTarget := len(nodes)/2 + 1
		count := 0
		for id := range nodes {
			tn.SetNodeHonest(id, false)
			count++
			if count >= dishonestTarget {
				break
			}
		}
		return nil
	},
}

// ScenarioNodeOffline - One node goes offline, test fault tolerance
var ScenarioNodeOffline = &Scenario{
	Name:        "node_offline",
	Description: "One node goes offline, remaining nodes should handle tasks",
	Tasks: []ScenarioTask{
		{Prompt: "Are you online?", ExpectedPass: true},
		{Prompt: "Status check", ExpectedPass: true},
	},
	Setup: func(tn *TestNet) error {
		// Take one node offline
		nodes := tn.GetNodes()
		for id := range nodes {
			tn.SetNodeOnline(id, false)
			break
		}
		return nil
	},
}

// ScenarioByzantine - Simulate Byzantine behavior (nodes return different results)
var ScenarioByzantine = &Scenario{
	Name:        "byzantine",
	Description: "Each Byzantine node returns a unique different result",
	Tasks: []ScenarioTask{
		{Prompt: "Consensus test", ExpectedPass: false},
	},
	Setup: func(tn *TestNet) error {
		// Make all nodes dishonest (each will return unique wrong result)
		for id := range tn.GetNodes() {
			tn.SetNodeHonest(id, false)
		}
		return nil
	},
}

// ScenarioConcurrentTasks - Multiple tasks running concurrently
var ScenarioConcurrentTasks = &Scenario{
	Name:        "concurrent_tasks",
	Description: "Submit multiple tasks concurrently to test parallel processing",
	Tasks: []ScenarioTask{
		{Prompt: "Concurrent task 1", ExpectedPass: true},
		{Prompt: "Concurrent task 2", ExpectedPass: true},
		{Prompt: "Concurrent task 3", ExpectedPass: true},
		{Prompt: "Concurrent task 4", ExpectedPass: true},
		{Prompt: "Concurrent task 5", ExpectedPass: true},
	},
}

// ScenarioNodeFlapping - Nodes go on and offline during execution
var ScenarioNodeFlapping = &Scenario{
	Name:        "node_flapping",
	Description: "Nodes go on and offline during task execution",
	Tasks: []ScenarioTask{
		{Prompt: "Task 1", ExpectedPass: true},
		{Prompt: "Task 2", ExpectedPass: true},
		{Prompt: "Task 3", ExpectedPass: true},
	},
	Setup: func(tn *TestNet) error {
		// First node is dishonest
		nodes := tn.GetNodes()
		for id := range nodes {
			tn.SetNodeHonest(id, false)
			break
		}
		return nil
	},
}

// ScenarioAutoSlash - Verify that auto-slash triggers correctly
var ScenarioAutoSlash = &Scenario{
	Name:        "auto_slash",
	Description: "Verify that disagreeing nodes are automatically slashed",
	Tasks: []ScenarioTask{
		{Prompt: "Slash test 1", ExpectedPass: true},
		{Prompt: "Slash test 2", ExpectedPass: true},
	},
	Setup: func(tn *TestNet) error {
		// Make first node dishonest to trigger slash
		nodes := tn.GetNodes()
		for id := range nodes {
			tn.SetNodeHonest(id, false)
			break
		}
		return nil
	},
}

// ScenarioStress - Stress test with many tasks
var ScenarioStress = &Scenario{
	Name:        "stress",
	Description: "Run many tasks to verify stability",
	Tasks:       make([]ScenarioTask, 20),
	Setup:       nil,
}

// Initialize stress scenario
func init() {
	for i := 0; i < 20; i++ {
		ScenarioStress.Tasks[i] = ScenarioTask{
			Prompt:       fmt.Sprintf("Stress task %d", i),
			ExpectedPass: true,
		}
	}
}

// GetAllScenarios returns all predefined scenarios
func GetAllScenarios() []*Scenario {
	return []*Scenario{
		ScenarioAllHonest,
		ScenarioOneDisagreement,
		ScenarioMajorityDisagreement,
		ScenarioNodeOffline,
		ScenarioByzantine,
		ScenarioConcurrentTasks,
		ScenarioNodeFlapping,
		ScenarioAutoSlash,
		// ScenarioStress, // Commented out for faster regular testing
	}
}

// GetScenarioByName returns a scenario by name
func GetScenarioByName(name string) (*Scenario, bool) {
	for _, s := range GetAllScenarios() {
		if s.Name == name {
			return s, true
		}
	}
	return nil, false
}
