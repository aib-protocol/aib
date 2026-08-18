package economy

import (
	"fmt"
	"math"
	"sync"
	"testing"
)

// ====================== StakeManager tests ======================

func TestStakeManager_NewStakeManager(t *testing.T) {
	sm := NewStakeManager(100.0)
	if sm == nil {
		t.Fatal("NewStakeManager returned nil")
	}
	if sm.minStake != 100.0 {
		t.Fatalf("minimum stake requirement should be 100.0, got: %f", sm.minStake)
	}
	if sm.GetTotalStaked() != 0 {
		t.Fatal("initial total staked should be 0")
	}
}

func TestStakeManager_Stake(t *testing.T) {
	sm := NewStakeManager(100.0)

	// normal stake
	err := sm.Stake("node1", 500.0)
	if err != nil {
		t.Fatalf("stake failed: %v", err)
	}

	// verify stake info
	stake, err := sm.GetStake("node1")
	if err != nil {
		t.Fatalf("get stake info failed: %v", err)
	}
	if stake.Amount != 500.0 {
		t.Fatalf("stake amount should be 500.0, got: %f", stake.Amount)
	}
	if stake.Status != StakeActive {
		t.Fatalf("stake status should be active, got: %s", stake.Status)
	}
}

func TestStakeManager_Stake_Errors(t *testing.T) {
	sm := NewStakeManager(100.0)

	// empty node ID
	if err := sm.Stake("", 500.0); err == nil {
		t.Fatal("empty node ID should return an error")
	}

	// zero amount
	if err := sm.Stake("node1", 0); err == nil {
		t.Fatal("zero amount should return an error")
	}

	// negative amount
	if err := sm.Stake("node1", -100); err == nil {
		t.Fatal("negative amount should return an error")
	}

	// below minimum stake
	if err := sm.Stake("node1", 50); err == nil {
		t.Fatal("below minimum stake should return an error")
	}

	// duplicate stake
	if err := sm.Stake("node1", 500.0); err != nil {
		t.Fatal("first stake should not fail")
	}
	if err := sm.Stake("node1", 500.0); err == nil {
		t.Fatal("duplicate stake should return an error")
	}
}

func TestStakeManager_Unstake(t *testing.T) {
	sm := NewStakeManager(100.0)

	// stake first
	sm.Stake("node1", 500.0)

	// unstake
	err := sm.Unstake("node1")
	if err != nil {
		t.Fatalf("unstake failed: %v", err)
	}

	// verify status becomes locked
	stake, _ := sm.GetStake("node1")
	if stake.Status != StakeLocked {
		t.Fatalf("status should be locked, got: %s", stake.Status)
	}
	if stake.LockedUntil <= 0 {
		t.Fatal("lock time should be greater than 0")
	}

	// verify node is no longer eligible
	if sm.IsEligible("node1") {
		t.Fatal("node should be ineligible after unstaking")
	}
}

func TestStakeManager_Unstake_Errors(t *testing.T) {
	sm := NewStakeManager(100.0)

	// unstaked node
	if err := sm.Unstake("node1"); err == nil {
		t.Fatal("unstaked node should return an error")
	}

	// empty node ID
	if err := sm.Unstake(""); err == nil {
		t.Fatal("empty node ID should return an error")
	}

	// duplicate unstake
	sm.Stake("node1", 500.0)
	sm.Unstake("node1")
	if err := sm.Unstake("node1"); err == nil {
		t.Fatal("duplicate unstake should return an error")
	}
}

func TestStakeManager_Slash(t *testing.T) {
	sm := NewStakeManager(100.0)
	sm.Stake("node1", 1000.0)

	// slash 50%
	slashAmount, err := sm.Slash("node1", 0.5)
	if err != nil {
		t.Fatalf("slash failed: %v", err)
	}
	if slashAmount != 500.0 {
		t.Fatalf("slash amount should be 500.0, got: %f", slashAmount)
	}

	// verify remaining stake
	stake, _ := sm.GetStake("node1")
	if stake.Amount != 500.0 {
		t.Fatalf("remaining stake should be 500.0, got: %f", stake.Amount)
	}
	if stake.SlashTotal != 500.0 {
		t.Fatalf("cumulative slash should be 500.0, got: %f", stake.SlashTotal)
	}
}

func TestStakeManager_Slash_Full(t *testing.T) {
	sm := NewStakeManager(100.0)
	sm.Stake("node1", 1000.0)

	// slash in full
	slashAmount, err := sm.Slash("node1", 1.0)
	if err != nil {
		t.Fatalf("full slash failed: %v", err)
	}
	if slashAmount != 1000.0 {
		t.Fatalf("slash amount should be 1000.0, got: %f", slashAmount)
	}

	// verify status becomes slashed
	stake, _ := sm.GetStake("node1")
	if stake.Status != StakeSlashed {
		t.Fatalf("status should be slashed, got: %s", stake.Status)
	}
	if stake.Amount != 0 {
		t.Fatalf("stake amount should be 0, got: %f", stake.Amount)
	}

	// ineligible after slashing
	if sm.IsEligible("node1") {
		t.Fatal("ineligible after full slash")
	}
}

func TestStakeManager_Slash_Errors(t *testing.T) {
	sm := NewStakeManager(100.0)

	// unstaked node
	_, err := sm.Slash("node1", 0.5)
	if err == nil {
		t.Fatal("unstaked node should return an error")
	}

	// empty node ID
	_, err = sm.Slash("", 0.5)
	if err == nil {
		t.Fatal("empty node ID should return an error")
	}

	// invalid ratio
	sm.Stake("node1", 1000.0)
	_, err = sm.Slash("node1", 1.5)
	if err == nil {
		t.Fatal("ratio above 1 should return an error")
	}
	_, err = sm.Slash("node1", -0.1)
	if err == nil {
		t.Fatal("negative ratio should return an error")
	}
}

func TestStakeManager_IsEligible(t *testing.T) {
	sm := NewStakeManager(100.0)

	// not staked
	if sm.IsEligible("node1") {
		t.Fatal("unstaked node should not be eligible")
	}

	// empty node ID
	if sm.IsEligible("") {
		t.Fatal("empty node ID should not be eligible")
	}

	// eligible after staking
	sm.Stake("node1", 500.0)
	if !sm.IsEligible("node1") {
		t.Fatal("node should be eligible after staking")
	}

	// ineligible after partial slash drops below minimum stake
	sm.Slash("node1", 0.9) // 50 remains, below minimum stake 100
	if sm.IsEligible("node1") {
		t.Fatal("ineligible when stake is below minimum")
	}
}

func TestStakeManager_GetTotalStaked(t *testing.T) {
	sm := NewStakeManager(100.0)

	sm.Stake("node1", 500.0)
	sm.Stake("node2", 300.0)
	sm.Stake("node3", 200.0)

	total := sm.GetTotalStaked()
	if total != 1000.0 {
		t.Fatalf("total staked should be 1000.0, got: %f", total)
	}

	// excluded from total after unstake
	sm.Unstake("node3")
	total = sm.GetTotalStaked()
	if total != 800.0 {
		t.Fatalf("total after unstake should be 800.0, got: %f", total)
	}
}

func TestStakeManager_Restake(t *testing.T) {
	sm := NewStakeManager(100.0)

	// stake -> unstake -> withdraw -> restake
	sm.Stake("node1", 500.0)
	sm.Unstake("node1")

	// manually set LockedUntil in the past to simulate lock expiry
	sm.mu.Lock()
	sm.stakes["node1"].LockedUntil = 0
	sm.mu.Unlock()

	_, err := sm.Withdraw("node1")
	if err != nil {
		t.Fatalf("withdraw failed: %v", err)
	}

	// restake
	err = sm.Stake("node1", 600.0)
	if err != nil {
		t.Fatalf("restake failed: %v", err)
	}

	stake, _ := sm.GetStake("node1")
	if stake.Amount != 600.0 {
		t.Fatalf("restake amount should be 600.0, got: %f", stake.Amount)
	}
}

func TestStakeManager_ExportImport(t *testing.T) {
	sm := NewStakeManager(100.0)

	sm.Stake("node1", 500.0)
	sm.Stake("node2", 300.0)
	sm.Slash("node1", 0.1)

	// export
	data, err := sm.Export()
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	// create new manager and import
	sm2 := NewStakeManager(0)
	err = sm2.Import(data)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// verify state after import
	stake1, _ := sm2.GetStake("node1")
	if stake1.Amount != 450.0 {
		t.Fatalf("node1 stake after import should be 450.0, got: %f", stake1.Amount)
	}
	if stake1.SlashTotal != 50.0 {
		t.Fatalf("node1 slash total after import should be 50.0, got: %f", stake1.SlashTotal)
	}

	stake2, _ := sm2.GetStake("node2")
	if stake2.Amount != 300.0 {
		t.Fatalf("node2 stake after import should be 300.0, got: %f", stake2.Amount)
	}

	if sm2.minStake != 100.0 {
		t.Fatalf("minimum stake after import should be 100.0, got: %f", sm2.minStake)
	}
}

// ====================== RewardDistributor tests ======================

func TestRewardDistributor_NewRewardDistributor(t *testing.T) {
	rd := NewRewardDistributor(10.0)
	if rd == nil {
		t.Fatal("NewRewardDistributor returned nil")
	}
	if rd.baseReward != 10.0 {
		t.Fatalf("base reward should be 10.0, got: %f", rd.baseReward)
	}
	if rd.GetTotalDistributed() != 0 {
		t.Fatal("initial total distributed should be 0")
	}
}

func TestRewardDistributor_DistributeTaskReward(t *testing.T) {
	rd := NewRewardDistributor(10.0)

	err := rd.DistributeTaskReward("task1", []string{"node1", "node2"})
	if err != nil {
		t.Fatalf("distribute task reward failed: %v", err)
	}

	// each node should get 5.0 (10.0 / 2)
	balance1 := rd.GetBalance("node1")
	if balance1 != 5.0 {
		t.Fatalf("node1 balance should be 5.0, got: %f", balance1)
	}
	balance2 := rd.GetBalance("node2")
	if balance2 != 5.0 {
		t.Fatalf("node2 balance should be 5.0, got: %f", balance2)
	}
}

func TestRewardDistributor_DistributeTaskReward_Errors(t *testing.T) {
	rd := NewRewardDistributor(10.0)

	// empty task ID
	if err := rd.DistributeTaskReward("", []string{"node1"}); err == nil {
		t.Fatal("empty task ID should return an error")
	}

	// empty node list
	if err := rd.DistributeTaskReward("task1", nil); err == nil {
		t.Fatal("empty node list should return an error")
	}
	if err := rd.DistributeTaskReward("task1", []string{}); err == nil {
		t.Fatal("empty node list should return an error")
	}
}

func TestRewardDistributor_DistributeReporterReward(t *testing.T) {
	rd := NewRewardDistributor(10.0)

	err := rd.DistributeReporterReward("reporter1", 50.0, "task1")
	if err != nil {
		t.Fatalf("distribute reporter reward failed: %v", err)
	}

	balance := rd.GetBalance("reporter1")
	if balance != 50.0 {
		t.Fatalf("reporter balance should be 50.0, got: %f", balance)
	}

	// query history
	history := rd.GetHistory("reporter1")
	if len(history) != 1 {
		t.Fatalf("history record count should be 1, got: %d", len(history))
	}
	if history[0].Type != RewardReporter {
		t.Fatalf("reward type should be reporter, got: %s", history[0].Type)
	}
}

func TestRewardDistributor_DistributeReporterReward_Errors(t *testing.T) {
	rd := NewRewardDistributor(10.0)

	// empty node ID
	if err := rd.DistributeReporterReward("", 50.0, "task1"); err == nil {
		t.Fatal("empty node ID should return an error")
	}

	// non-positive amount
	if err := rd.DistributeReporterReward("reporter1", 0, "task1"); err == nil {
		t.Fatal("zero amount should return an error")
	}
	if err := rd.DistributeReporterReward("reporter1", -10, "task1"); err == nil {
		t.Fatal("negative amount should return an error")
	}
}

func TestRewardDistributor_GetHistory(t *testing.T) {
	rd := NewRewardDistributor(10.0)

	// distribute multiple rewards
	rd.DistributeTaskReward("task1", []string{"node1", "node2"})
	rd.DistributeTaskReward("task2", []string{"node1"})
	rd.DistributeReporterReward("node1", 20.0, "task3")

	// node1 should have 3 records
	history := rd.GetHistory("node1")
	if len(history) != 3 {
		t.Fatalf("node1 history count should be 3, got: %d", len(history))
	}

	// node2 should have 1 record
	history = rd.GetHistory("node2")
	if len(history) != 1 {
		t.Fatalf("node2 history count should be 1, got: %d", len(history))
	}

	// nonexistent node should return empty
	history = rd.GetHistory("node999")
	if len(history) != 0 {
		t.Fatalf("nonexistent node history count should be 0, got: %d", len(history))
	}
}

func TestRewardDistributor_GetTotalDistributed(t *testing.T) {
	rd := NewRewardDistributor(10.0)

	rd.DistributeTaskReward("task1", []string{"node1", "node2"}) // 5.0 + 5.0 = 10.0
	rd.DistributeReporterReward("reporter1", 20.0, "task2")      // 20.0

	total := rd.GetTotalDistributed()
	if math.Abs(total-30.0) > 0.001 {
		t.Fatalf("total distributed should be 30.0, got: %f", total)
	}
}

func TestRewardDistributor_PoCUMultiplier(t *testing.T) {
	rd := NewRewardDistributor(10.0)

	// set PoCU multiplier to 2.0
	err := rd.SetPoCUMultiplier(2.0)
	if err != nil {
		t.Fatalf("set PoCU multiplier failed: %v", err)
	}

	rd.DistributeTaskReward("task1", []string{"node1"})

	// reward should be 10.0 * 2.0 = 20.0
	balance := rd.GetBalance("node1")
	if balance != 20.0 {
		t.Fatalf("balance should be 20.0, got: %f", balance)
	}

	// invalid multiplier
	if err := rd.SetPoCUMultiplier(0); err == nil {
		t.Fatal("zero multiplier should return an error")
	}
	if err := rd.SetPoCUMultiplier(-1); err == nil {
		t.Fatal("negative multiplier should return an error")
	}
}

func TestRewardDistributor_ExportImport(t *testing.T) {
	rd := NewRewardDistributor(10.0)
	rd.SetPoCUMultiplier(1.5)
	rd.DistributeTaskReward("task1", []string{"node1", "node2"})
	rd.DistributeReporterReward("node1", 20.0, "task2")

	// export
	data, err := rd.Export()
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	// import into a new instance
	rd2 := NewRewardDistributor(0)
	err = rd2.Import(data)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// verify balance
	if rd2.GetBalance("node1") != rd.GetBalance("node1") {
		t.Fatalf("node1 balance mismatch after import: %.2f vs %.2f",
			rd2.GetBalance("node1"), rd.GetBalance("node1"))
	}
	if rd2.GetBalance("node2") != rd.GetBalance("node2") {
		t.Fatalf("node2 balance mismatch after import: %.2f vs %.2f",
			rd2.GetBalance("node2"), rd.GetBalance("node2"))
	}

	// verify config
	if rd2.baseReward != 10.0 {
		t.Fatalf("base reward after import should be 10.0, got: %f", rd2.baseReward)
	}
	if rd2.pocuMultiplier != 1.5 {
		t.Fatalf("PoCU multiplier after import should be 1.5, got: %f", rd2.pocuMultiplier)
	}

	// verify history records
	history := rd2.GetHistory("node1")
	if len(history) != 2 {
		t.Fatalf("node1 history count after import should be 2, got: %d", len(history))
	}
}

// ====================== Economy integration tests ======================

func TestEconomy_NewEconomy(t *testing.T) {
	eco := NewEconomy(100.0, 10.0)
	if eco == nil {
		t.Fatal("NewEconomy returned nil")
	}
	if eco.Stakes == nil {
		t.Fatal("Stakes should not be nil")
	}
	if eco.Rewards == nil {
		t.Fatal("Rewards should not be nil")
	}
}

func TestEconomy_FullWorkflow(t *testing.T) {
	eco := NewEconomy(100.0, 10.0)

	// 1. node stakes
	eco.Stakes.Stake("node1", 1000.0)
	eco.Stakes.Stake("node2", 500.0)
	eco.Stakes.Stake("node3", 200.0)

	// 2. process task completion (should only reward eligible nodes)
	eligible, err := eco.ProcessTaskCompletion("task1", []string{"node1", "node2", "node3", "node_no_stake"})
	if err != nil {
		t.Fatalf("process task completion failed: %v", err)
	}
	if len(eligible) != 3 {
		t.Fatalf("eligible node count should be 3, got: %d", len(eligible))
	}

	// 3. verify reward distribution
	// 10.0 / 3 = 3.333...
	balance1 := eco.Rewards.GetBalance("node1")
	expected := 10.0 / 3.0
	if math.Abs(balance1-expected) > 0.001 {
		t.Fatalf("node1 balance should be about %.4f, got: %f", expected, balance1)
	}

	// node_no_stake should get no reward
	if eco.Rewards.GetBalance("node_no_stake") != 0 {
		t.Fatal("unstaked node should get no reward")
	}

	// 4. slashing + reporter reward
	slashAmount, reporterReward, err := eco.ProcessSlash("node1", "node2", 0.5, "task2")
	if err != nil {
		t.Fatalf("slash failed: %v", err)
	}
	if slashAmount != 500.0 {
		t.Fatalf("slash amount should be 500.0, got: %f", slashAmount)
	}
	if reporterReward != 100.0 {
		t.Fatalf("reporter reward should be 100.0, got: %f", reporterReward)
	}

	// verify node2 gets the reporter reward
	node2Balance := eco.Rewards.GetBalance("node2")
	// should be task reward (3.33..) + reporter reward (100.0)
	if node2Balance < 103.0 {
		t.Fatalf("node2 balance should exceed 103.0, got: %f", node2Balance)
	}

	// 5. view summary
	summary := eco.GetNodeSummary("node1")
	if summary.StakeAmount != 500.0 {
		t.Fatalf("node1 stake should be 500.0, got: %f", summary.StakeAmount)
	}
	if summary.SlashTotal != 500.0 {
		t.Fatalf("node1 slash total should be 500.0, got: %f", summary.SlashTotal)
	}
}

func TestEconomy_ProcessTaskCompletion_NoEligible(t *testing.T) {
	eco := NewEconomy(100.0, 10.0)

	// no nodes staked
	eligible, err := eco.ProcessTaskCompletion("task1", []string{"node1", "node2"})
	if err != nil {
		t.Fatalf("should not return error: %v", err)
	}
	if len(eligible) != 0 {
		t.Fatalf("should have no eligible nodes, got: %d", len(eligible))
	}
}

func TestEconomy_ExportImport(t *testing.T) {
	eco := NewEconomy(100.0, 10.0)

	eco.Stakes.Stake("node1", 1000.0)
	eco.Rewards.DistributeTaskReward("task1", []string{"node1"})

	// export
	data, err := eco.Export()
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	// import into a new instance
	eco2 := NewEconomy(0, 0)
	err = eco2.Import(data)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// verify state consistency
	stake1, _ := eco2.Stakes.GetStake("node1")
	if stake1.Amount != 1000.0 {
		t.Fatalf("stake after import should be 1000.0, got: %f", stake1.Amount)
	}
	if eco2.Rewards.GetBalance("node1") != eco.Rewards.GetBalance("node1") {
		t.Fatal("balance mismatch after import")
	}
}

// ====================== concurrency tests ======================

func TestStakeManager_Concurrent(t *testing.T) {
	sm := NewStakeManager(100.0)
	var wg sync.WaitGroup

	// 100 concurrent stakes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			nodeID := fmt.Sprintf("node_%d", id)
			sm.Stake(nodeID, float64(100+id))
		}(i)
	}
	wg.Wait()

	// verify all nodes staked successfully
	for i := 0; i < 100; i++ {
		nodeID := fmt.Sprintf("node_%d", i)
		if !sm.IsEligible(nodeID) {
			t.Fatalf("%s should be eligible after concurrent staking", nodeID)
		}
	}

	// concurrent queries
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			nodeID := fmt.Sprintf("node_%d", id)
			sm.GetStake(nodeID)
			sm.IsEligible(nodeID)
			sm.GetTotalStaked()
		}(i)
	}
	wg.Wait()
}

func TestRewardDistributor_Concurrent(t *testing.T) {
	rd := NewRewardDistributor(10.0)
	var wg sync.WaitGroup

	// 100 concurrent reward distributions
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			taskID := fmt.Sprintf("task_%d", id)
			nodeID := fmt.Sprintf("node_%d", id%10)
			rd.DistributeTaskReward(taskID, []string{nodeID})
		}(i)
	}
	wg.Wait()

	// verify total distributed
	total := rd.GetTotalDistributed()
	expected := 10.0 * 100 // 100 tasks, 10.0 each
	if math.Abs(total-expected) > 0.001 {
		t.Fatalf("total distributed should be %.0f, got: %f", expected, total)
	}
}

func TestEconomy_Concurrent(t *testing.T) {
	eco := NewEconomy(100.0, 10.0)

	// stake first
	for i := 0; i < 10; i++ {
		nodeID := fmt.Sprintf("node_%d", i)
		eco.Stakes.Stake(nodeID, 1000.0)
	}

	var wg sync.WaitGroup
	// concurrent task completion and queries
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			taskID := fmt.Sprintf("task_%d", id)
			nodeIDs := []string{fmt.Sprintf("node_%d", id%10)}
			eco.ProcessTaskCompletion(taskID, nodeIDs)
		}(i)
	}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			nodeID := fmt.Sprintf("node_%d", id%10)
			eco.GetNodeSummary(nodeID)
		}(i)
	}
	wg.Wait()

	// pass if no panic
}

// ====================== edge-case tests ======================

func TestStakeManager_SlashBelowMinStake(t *testing.T) {
	sm := NewStakeManager(100.0)
	sm.Stake("node1", 150.0)

	// after 50% slash, 75.0 remains, below minimum stake 100.0
	sm.Slash("node1", 0.5)
	if sm.IsEligible("node1") {
		t.Fatal("ineligible when below minimum stake after slashing")
	}

	// stake info should still be queryable
	stake, err := sm.GetStake("node1")
	if err != nil {
		t.Fatalf("get stake failed: %v", err)
	}
	if stake.Amount != 75.0 {
		t.Fatalf("stake amount should be 75.0, got: %f", stake.Amount)
	}
}

func TestStakeManager_MultipleSlashes(t *testing.T) {
	sm := NewStakeManager(100.0)
	sm.Stake("node1", 1000.0)

	// consecutive slashes
	sm.Slash("node1", 0.1) // slash 100, 900 remains
	sm.Slash("node1", 0.1) // slash 90, 810 remains
	sm.Slash("node1", 0.1) // slash 81, 729 remains

	stake, _ := sm.GetStake("node1")
	if math.Abs(stake.Amount-729.0) > 0.001 {
		t.Fatalf("stake after consecutive slashes should be about 729.0, got: %f", stake.Amount)
	}
	expectedSlashTotal := 1000.0 - 729.0
	if math.Abs(stake.SlashTotal-expectedSlashTotal) > 0.001 {
		t.Fatalf("cumulative slash should be about %.1f, got: %f", expectedSlashTotal, stake.SlashTotal)
	}
}

func TestRewardDistributor_SingleNodeMultipleTasks(t *testing.T) {
	rd := NewRewardDistributor(10.0)

	// one node participates in multiple tasks
	for i := 0; i < 5; i++ {
		rd.DistributeTaskReward(fmt.Sprintf("task_%d", i), []string{"node1"})
	}

	// balance should be 50.0
	balance := rd.GetBalance("node1")
	if balance != 50.0 {
		t.Fatalf("balance should be 50.0, got: %f", balance)
	}

	// history should have 5 records
	history := rd.GetHistory("node1")
	if len(history) != 5 {
		t.Fatalf("history record count should be 5, got: %d", len(history))
	}
}

func TestEconomy_SlashWithNoReporter(t *testing.T) {
	eco := NewEconomy(100.0, 10.0)
	eco.Stakes.Stake("node1", 1000.0)

	// slashing without a reporter
	slashAmount, reporterReward, err := eco.ProcessSlash("node1", "", 0.5, "task1")
	if err != nil {
		t.Fatalf("slash without reporter failed: %v", err)
	}
	if slashAmount != 500.0 {
		t.Fatalf("slash amount should be 500.0, got: %f", slashAmount)
	}
	if reporterReward != 0 {
		t.Fatalf("reporter reward should be 0 with no reporter, got: %f", reporterReward)
	}
}
