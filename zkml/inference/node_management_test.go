package inference

import (
	"testing"

	"github.com/aib-protocol/aib/zkml/orchestrator"
)

func TestNodeManagement_RegisterAndUnregister(t *testing.T) {
	scheduler := orchestrator.NewScheduler()

	node := &orchestrator.NodeInfo{
		ID:     "node-1",
		Stake:  1000,
		Active: true,
	}

	if err := scheduler.RegisterNode(node); err != nil {
		t.Fatalf("failed to register node: %v", err)
	}

	stored, ok := scheduler.GetNode("node-1")
	if !ok {
		t.Fatal("expected registered node to be found")
	}
	if stored.ID != "node-1" {
		t.Fatalf("expected node ID node-1, got %s", stored.ID)
	}
	if scheduler.ActiveNodeCount() != 1 {
		t.Fatalf("expected 1 active node, got %d", scheduler.ActiveNodeCount())
	}

	scheduler.UnregisterNode("node-1")
	if _, ok := scheduler.GetNode("node-1"); ok {
		t.Fatal("expected node to be removed after unregister")
	}
	if scheduler.ActiveNodeCount() != 0 {
		t.Fatalf("expected 0 active nodes after unregister, got %d", scheduler.ActiveNodeCount())
	}
}

func TestNodeManagement_HealthCheck(t *testing.T) {
	scheduler := orchestrator.NewScheduler()

	if err := scheduler.RegisterNode(&orchestrator.NodeInfo{ID: "node-a", Active: true}); err != nil {
		t.Fatalf("failed to register node-a: %v", err)
	}
	if err := scheduler.RegisterNode(&orchestrator.NodeInfo{ID: "node-b", Active: true}); err != nil {
		t.Fatalf("failed to register node-b: %v", err)
	}

	if scheduler.ActiveNodeCount() != 2 {
		t.Fatalf("expected 2 active nodes before health update, got %d", scheduler.ActiveNodeCount())
	}

	if err := scheduler.SetNodeActive("node-b", false); err != nil {
		t.Fatalf("failed to mark node-b unhealthy: %v", err)
	}

	if scheduler.ActiveNodeCount() != 1 {
		t.Fatalf("expected 1 active node after node-b unhealthy, got %d", scheduler.ActiveNodeCount())
	}

	nodeB, ok := scheduler.GetNode("node-b")
	if !ok {
		t.Fatal("expected node-b to exist")
	}
	if nodeB.Active {
		t.Fatal("expected node-b to be inactive after health check update")
	}

	if err := scheduler.SetNodeActive("node-b", true); err != nil {
		t.Fatalf("failed to restore node-b health: %v", err)
	}
	if scheduler.ActiveNodeCount() != 2 {
		t.Fatalf("expected 2 active nodes after recovery, got %d", scheduler.ActiveNodeCount())
	}
}

func TestNodeManagement_NodeScoring(t *testing.T) {
	scheduler := orchestrator.NewScheduler()

	nodes := []*orchestrator.NodeInfo{
		{ID: "node-low", Stake: 100, Active: true},
		{ID: "node-mid", Stake: 500, Active: true},
		{ID: "node-high", Stake: 1000, Active: true},
	}

	for _, n := range nodes {
		if err := scheduler.RegisterNode(n); err != nil {
			t.Fatalf("failed to register %s: %v", n.ID, err)
		}
	}

	score := func(n *orchestrator.NodeInfo) float64 {
		if !n.Active {
			return 0
		}
		return n.Stake
	}

	if score(nodes[0]) >= score(nodes[1]) {
		t.Fatalf("expected node-mid score > node-low score, got %.2f <= %.2f", score(nodes[1]), score(nodes[0]))
	}
	if score(nodes[1]) >= score(nodes[2]) {
		t.Fatalf("expected node-high score > node-mid score, got %.2f <= %.2f", score(nodes[2]), score(nodes[1]))
	}

	if err := scheduler.SetNodeActive("node-high", false); err != nil {
		t.Fatalf("failed to deactivate node-high: %v", err)
	}
	storedHigh, ok := scheduler.GetNode("node-high")
	if !ok {
		t.Fatal("expected node-high to exist")
	}
	if got := score(storedHigh); got != 0 {
		t.Fatalf("expected inactive node score 0, got %.2f", got)
	}
}

func TestNodeManagement_NodeSelection(t *testing.T) {
	scheduler := orchestrator.NewScheduler()

	for i, active := range []bool{true, true, true, false} {
		nodeID := "node-" + string(rune('a'+i))
		if err := scheduler.RegisterNode(&orchestrator.NodeInfo{ID: nodeID, Active: active, Stake: 100}); err != nil {
			t.Fatalf("failed to register %s: %v", nodeID, err)
		}
	}

	selected, err := scheduler.SelectNodes(2)
	if err != nil {
		t.Fatalf("expected node selection success, got error: %v", err)
	}
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected nodes, got %d", len(selected))
	}

	seen := make(map[string]bool)
	for _, nodeID := range selected {
		if seen[nodeID] {
			t.Fatalf("selected duplicate node: %s", nodeID)
		}
		seen[nodeID] = true
		node, ok := scheduler.GetNode(nodeID)
		if !ok {
			t.Fatalf("selected node %s not found in scheduler", nodeID)
		}
		if !node.Active {
			t.Fatalf("selected inactive node: %s", nodeID)
		}
	}

	if _, err := scheduler.SelectNodes(4); err == nil {
		t.Fatal("expected selection error when requesting more nodes than active nodes")
	}
}
