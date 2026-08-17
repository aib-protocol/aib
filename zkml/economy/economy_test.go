package economy

import (
	"fmt"
	"math"
	"sync"
	"testing"
)

// ====================== StakeManager 测试 ======================

func TestStakeManager_NewStakeManager(t *testing.T) {
	sm := NewStakeManager(100.0)
	if sm == nil {
		t.Fatal("NewStakeManager 返回 nil")
	}
	if sm.minStake != 100.0 {
		t.Fatalf("最低质押要求应为 100.0, 实际: %f", sm.minStake)
	}
	if sm.GetTotalStaked() != 0 {
		t.Fatal("初始总质押量应为 0")
	}
}

func TestStakeManager_Stake(t *testing.T) {
	sm := NewStakeManager(100.0)

	// 正常质押
	err := sm.Stake("node1", 500.0)
	if err != nil {
		t.Fatalf("质押失败: %v", err)
	}

	// 验证质押信息
	stake, err := sm.GetStake("node1")
	if err != nil {
		t.Fatalf("获取质押信息失败: %v", err)
	}
	if stake.Amount != 500.0 {
		t.Fatalf("质押金额应为 500.0, 实际: %f", stake.Amount)
	}
	if stake.Status != StakeActive {
		t.Fatalf("质押状态应为 active, 实际: %s", stake.Status)
	}
}

func TestStakeManager_Stake_Errors(t *testing.T) {
	sm := NewStakeManager(100.0)

	// 空节点ID
	if err := sm.Stake("", 500.0); err == nil {
		t.Fatal("空节点ID应返回错误")
	}

	// 金额为零
	if err := sm.Stake("node1", 0); err == nil {
		t.Fatal("金额为0应返回错误")
	}

	// 负金额
	if err := sm.Stake("node1", -100); err == nil {
		t.Fatal("负金额应返回错误")
	}

	// 低于最低质押
	if err := sm.Stake("node1", 50); err == nil {
		t.Fatal("低于最低质押应返回错误")
	}

	// 重复质押
	if err := sm.Stake("node1", 500.0); err != nil {
		t.Fatal("第一次质押不应失败")
	}
	if err := sm.Stake("node1", 500.0); err == nil {
		t.Fatal("重复质押应返回错误")
	}
}

func TestStakeManager_Unstake(t *testing.T) {
	sm := NewStakeManager(100.0)

	// 先质押
	sm.Stake("node1", 500.0)

	// 解除质押
	err := sm.Unstake("node1")
	if err != nil {
		t.Fatalf("解除质押失败: %v", err)
	}

	// 验证状态变为 locked
	stake, _ := sm.GetStake("node1")
	if stake.Status != StakeLocked {
		t.Fatalf("状态应为 locked, 实际: %s", stake.Status)
	}
	if stake.LockedUntil <= 0 {
		t.Fatal("锁定时间应该大于0")
	}

	// 验证节点不再有资格
	if sm.IsEligible("node1") {
		t.Fatal("解除质押后节点不应有资格")
	}
}

func TestStakeManager_Unstake_Errors(t *testing.T) {
	sm := NewStakeManager(100.0)

	// 未质押的节点
	if err := sm.Unstake("node1"); err == nil {
		t.Fatal("未质押节点应返回错误")
	}

	// 空节点ID
	if err := sm.Unstake(""); err == nil {
		t.Fatal("空节点ID应返回错误")
	}

	// 重复解除质押
	sm.Stake("node1", 500.0)
	sm.Unstake("node1")
	if err := sm.Unstake("node1"); err == nil {
		t.Fatal("重复解除质押应返回错误")
	}
}

func TestStakeManager_Slash(t *testing.T) {
	sm := NewStakeManager(100.0)
	sm.Stake("node1", 1000.0)

	// 罚没 50%
	slashAmount, err := sm.Slash("node1", 0.5)
	if err != nil {
		t.Fatalf("罚没失败: %v", err)
	}
	if slashAmount != 500.0 {
		t.Fatalf("罚没金额应为 500.0, 实际: %f", slashAmount)
	}

	// 验证剩余质押
	stake, _ := sm.GetStake("node1")
	if stake.Amount != 500.0 {
		t.Fatalf("剩余质押应为 500.0, 实际: %f", stake.Amount)
	}
	if stake.SlashTotal != 500.0 {
		t.Fatalf("累计罚没应为 500.0, 实际: %f", stake.SlashTotal)
	}
}

func TestStakeManager_Slash_Full(t *testing.T) {
	sm := NewStakeManager(100.0)
	sm.Stake("node1", 1000.0)

	// 全额罚没
	slashAmount, err := sm.Slash("node1", 1.0)
	if err != nil {
		t.Fatalf("全额罚没失败: %v", err)
	}
	if slashAmount != 1000.0 {
		t.Fatalf("罚没金额应为 1000.0, 实际: %f", slashAmount)
	}

	// 验证状态变为 slashed
	stake, _ := sm.GetStake("node1")
	if stake.Status != StakeSlashed {
		t.Fatalf("状态应为 slashed, 实际: %s", stake.Status)
	}
	if stake.Amount != 0 {
		t.Fatalf("质押金额应为 0, 实际: %f", stake.Amount)
	}

	// 被罚没后不再有资格
	if sm.IsEligible("node1") {
		t.Fatal("被全额罚没后不应有资格")
	}
}

func TestStakeManager_Slash_Errors(t *testing.T) {
	sm := NewStakeManager(100.0)

	// 未质押的节点
	_, err := sm.Slash("node1", 0.5)
	if err == nil {
		t.Fatal("未质押节点应返回错误")
	}

	// 空节点ID
	_, err = sm.Slash("", 0.5)
	if err == nil {
		t.Fatal("空节点ID应返回错误")
	}

	// 无效比例
	sm.Stake("node1", 1000.0)
	_, err = sm.Slash("node1", 1.5)
	if err == nil {
		t.Fatal("比例超过1应返回错误")
	}
	_, err = sm.Slash("node1", -0.1)
	if err == nil {
		t.Fatal("负比例应返回错误")
	}
}

func TestStakeManager_IsEligible(t *testing.T) {
	sm := NewStakeManager(100.0)

	// 未质押
	if sm.IsEligible("node1") {
		t.Fatal("未质押节点不应有资格")
	}

	// 空节点ID
	if sm.IsEligible("") {
		t.Fatal("空节点ID不应有资格")
	}

	// 质押后有资格
	sm.Stake("node1", 500.0)
	if !sm.IsEligible("node1") {
		t.Fatal("质押后节点应有资格")
	}

	// 部分罚没导致低于最低质押后无资格
	sm.Slash("node1", 0.9) // 剩余50, 低于最低质押100
	if sm.IsEligible("node1") {
		t.Fatal("质押不足最低要求时不应有资格")
	}
}

func TestStakeManager_GetTotalStaked(t *testing.T) {
	sm := NewStakeManager(100.0)

	sm.Stake("node1", 500.0)
	sm.Stake("node2", 300.0)
	sm.Stake("node3", 200.0)

	total := sm.GetTotalStaked()
	if total != 1000.0 {
		t.Fatalf("总质押量应为 1000.0, 实际: %f", total)
	}

	// 解除质押后不计入总量
	sm.Unstake("node3")
	total = sm.GetTotalStaked()
	if total != 800.0 {
		t.Fatalf("解除质押后总量应为 800.0, 实际: %f", total)
	}
}

func TestStakeManager_Restake(t *testing.T) {
	sm := NewStakeManager(100.0)

	// 质押 -> 解除 -> 提取 -> 重新质押
	sm.Stake("node1", 500.0)
	sm.Unstake("node1")

	// 手动设置 LockedUntil 为过去时间以模拟锁定期结束
	sm.mu.Lock()
	sm.stakes["node1"].LockedUntil = 0
	sm.mu.Unlock()

	_, err := sm.Withdraw("node1")
	if err != nil {
		t.Fatalf("提取失败: %v", err)
	}

	// 重新质押
	err = sm.Stake("node1", 600.0)
	if err != nil {
		t.Fatalf("重新质押失败: %v", err)
	}

	stake, _ := sm.GetStake("node1")
	if stake.Amount != 600.0 {
		t.Fatalf("重新质押金额应为 600.0, 实际: %f", stake.Amount)
	}
}

func TestStakeManager_ExportImport(t *testing.T) {
	sm := NewStakeManager(100.0)

	sm.Stake("node1", 500.0)
	sm.Stake("node2", 300.0)
	sm.Slash("node1", 0.1)

	// 导出
	data, err := sm.Export()
	if err != nil {
		t.Fatalf("导出失败: %v", err)
	}

	// 创建新管理器并导入
	sm2 := NewStakeManager(0)
	err = sm2.Import(data)
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}

	// 验证导入后的状态
	stake1, _ := sm2.GetStake("node1")
	if stake1.Amount != 450.0 {
		t.Fatalf("导入后 node1 质押应为 450.0, 实际: %f", stake1.Amount)
	}
	if stake1.SlashTotal != 50.0 {
		t.Fatalf("导入后 node1 罚没总额应为 50.0, 实际: %f", stake1.SlashTotal)
	}

	stake2, _ := sm2.GetStake("node2")
	if stake2.Amount != 300.0 {
		t.Fatalf("导入后 node2 质押应为 300.0, 实际: %f", stake2.Amount)
	}

	if sm2.minStake != 100.0 {
		t.Fatalf("导入后最低质押应为 100.0, 实际: %f", sm2.minStake)
	}
}

// ====================== RewardDistributor 测试 ======================

func TestRewardDistributor_NewRewardDistributor(t *testing.T) {
	rd := NewRewardDistributor(10.0)
	if rd == nil {
		t.Fatal("NewRewardDistributor 返回 nil")
	}
	if rd.baseReward != 10.0 {
		t.Fatalf("基础奖励应为 10.0, 实际: %f", rd.baseReward)
	}
	if rd.GetTotalDistributed() != 0 {
		t.Fatal("初始总分发量应为 0")
	}
}

func TestRewardDistributor_DistributeTaskReward(t *testing.T) {
	rd := NewRewardDistributor(10.0)

	err := rd.DistributeTaskReward("task1", []string{"node1", "node2"})
	if err != nil {
		t.Fatalf("分发任务奖励失败: %v", err)
	}

	// 每个节点应得 5.0 (10.0 / 2)
	balance1 := rd.GetBalance("node1")
	if balance1 != 5.0 {
		t.Fatalf("node1 余额应为 5.0, 实际: %f", balance1)
	}
	balance2 := rd.GetBalance("node2")
	if balance2 != 5.0 {
		t.Fatalf("node2 余额应为 5.0, 实际: %f", balance2)
	}
}

func TestRewardDistributor_DistributeTaskReward_Errors(t *testing.T) {
	rd := NewRewardDistributor(10.0)

	// 空任务ID
	if err := rd.DistributeTaskReward("", []string{"node1"}); err == nil {
		t.Fatal("空任务ID应返回错误")
	}

	// 空节点列表
	if err := rd.DistributeTaskReward("task1", nil); err == nil {
		t.Fatal("空节点列表应返回错误")
	}
	if err := rd.DistributeTaskReward("task1", []string{}); err == nil {
		t.Fatal("空节点列表应返回错误")
	}
}

func TestRewardDistributor_DistributeReporterReward(t *testing.T) {
	rd := NewRewardDistributor(10.0)

	err := rd.DistributeReporterReward("reporter1", 50.0, "task1")
	if err != nil {
		t.Fatalf("分发举报奖励失败: %v", err)
	}

	balance := rd.GetBalance("reporter1")
	if balance != 50.0 {
		t.Fatalf("举报者余额应为 50.0, 实际: %f", balance)
	}

	// 查询历史
	history := rd.GetHistory("reporter1")
	if len(history) != 1 {
		t.Fatalf("历史记录数应为 1, 实际: %d", len(history))
	}
	if history[0].Type != RewardReporter {
		t.Fatalf("奖励类型应为 reporter, 实际: %s", history[0].Type)
	}
}

func TestRewardDistributor_DistributeReporterReward_Errors(t *testing.T) {
	rd := NewRewardDistributor(10.0)

	// 空节点ID
	if err := rd.DistributeReporterReward("", 50.0, "task1"); err == nil {
		t.Fatal("空节点ID应返回错误")
	}

	// 非正金额
	if err := rd.DistributeReporterReward("reporter1", 0, "task1"); err == nil {
		t.Fatal("金额为0应返回错误")
	}
	if err := rd.DistributeReporterReward("reporter1", -10, "task1"); err == nil {
		t.Fatal("负金额应返回错误")
	}
}

func TestRewardDistributor_GetHistory(t *testing.T) {
	rd := NewRewardDistributor(10.0)

	// 分发多个奖励
	rd.DistributeTaskReward("task1", []string{"node1", "node2"})
	rd.DistributeTaskReward("task2", []string{"node1"})
	rd.DistributeReporterReward("node1", 20.0, "task3")

	// node1 应有 3 条记录
	history := rd.GetHistory("node1")
	if len(history) != 3 {
		t.Fatalf("node1 历史记录数应为 3, 实际: %d", len(history))
	}

	// node2 应有 1 条记录
	history = rd.GetHistory("node2")
	if len(history) != 1 {
		t.Fatalf("node2 历史记录数应为 1, 实际: %d", len(history))
	}

	// 不存在的节点应返回空
	history = rd.GetHistory("node999")
	if len(history) != 0 {
		t.Fatalf("不存在节点历史记录数应为 0, 实际: %d", len(history))
	}
}

func TestRewardDistributor_GetTotalDistributed(t *testing.T) {
	rd := NewRewardDistributor(10.0)

	rd.DistributeTaskReward("task1", []string{"node1", "node2"}) // 5.0 + 5.0 = 10.0
	rd.DistributeReporterReward("reporter1", 20.0, "task2")      // 20.0

	total := rd.GetTotalDistributed()
	if math.Abs(total-30.0) > 0.001 {
		t.Fatalf("总分发量应为 30.0, 实际: %f", total)
	}
}

func TestRewardDistributor_PoCUMultiplier(t *testing.T) {
	rd := NewRewardDistributor(10.0)

	// 设置 PoCU 乘数为 2.0
	err := rd.SetPoCUMultiplier(2.0)
	if err != nil {
		t.Fatalf("设置 PoCU 乘数失败: %v", err)
	}

	rd.DistributeTaskReward("task1", []string{"node1"})

	// 奖励应为 10.0 * 2.0 = 20.0
	balance := rd.GetBalance("node1")
	if balance != 20.0 {
		t.Fatalf("余额应为 20.0, 实际: %f", balance)
	}

	// 无效乘数
	if err := rd.SetPoCUMultiplier(0); err == nil {
		t.Fatal("乘数为0应返回错误")
	}
	if err := rd.SetPoCUMultiplier(-1); err == nil {
		t.Fatal("负乘数应返回错误")
	}
}

func TestRewardDistributor_ExportImport(t *testing.T) {
	rd := NewRewardDistributor(10.0)
	rd.SetPoCUMultiplier(1.5)
	rd.DistributeTaskReward("task1", []string{"node1", "node2"})
	rd.DistributeReporterReward("node1", 20.0, "task2")

	// 导出
	data, err := rd.Export()
	if err != nil {
		t.Fatalf("导出失败: %v", err)
	}

	// 导入到新实例
	rd2 := NewRewardDistributor(0)
	err = rd2.Import(data)
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}

	// 验证余额
	if rd2.GetBalance("node1") != rd.GetBalance("node1") {
		t.Fatalf("导入后 node1 余额不一致: %.2f vs %.2f",
			rd2.GetBalance("node1"), rd.GetBalance("node1"))
	}
	if rd2.GetBalance("node2") != rd.GetBalance("node2") {
		t.Fatalf("导入后 node2 余额不一致: %.2f vs %.2f",
			rd2.GetBalance("node2"), rd.GetBalance("node2"))
	}

	// 验证配置
	if rd2.baseReward != 10.0 {
		t.Fatalf("导入后基础奖励应为 10.0, 实际: %f", rd2.baseReward)
	}
	if rd2.pocuMultiplier != 1.5 {
		t.Fatalf("导入后 PoCU 乘数应为 1.5, 实际: %f", rd2.pocuMultiplier)
	}

	// 验证历史记录
	history := rd2.GetHistory("node1")
	if len(history) != 2 {
		t.Fatalf("导入后 node1 历史记录数应为 2, 实际: %d", len(history))
	}
}

// ====================== Economy 集成测试 ======================

func TestEconomy_NewEconomy(t *testing.T) {
	eco := NewEconomy(100.0, 10.0)
	if eco == nil {
		t.Fatal("NewEconomy 返回 nil")
	}
	if eco.Stakes == nil {
		t.Fatal("Stakes 不应为 nil")
	}
	if eco.Rewards == nil {
		t.Fatal("Rewards 不应为 nil")
	}
}

func TestEconomy_FullWorkflow(t *testing.T) {
	eco := NewEconomy(100.0, 10.0)

	// 1. 节点质押
	eco.Stakes.Stake("node1", 1000.0)
	eco.Stakes.Stake("node2", 500.0)
	eco.Stakes.Stake("node3", 200.0)

	// 2. 处理任务完成（应只奖励有资格的节点）
	eligible, err := eco.ProcessTaskCompletion("task1", []string{"node1", "node2", "node3", "node_no_stake"})
	if err != nil {
		t.Fatalf("处理任务完成失败: %v", err)
	}
	if len(eligible) != 3 {
		t.Fatalf("有资格节点数应为 3, 实际: %d", len(eligible))
	}

	// 3. 验证奖励分发
	// 10.0 / 3 = 3.333...
	balance1 := eco.Rewards.GetBalance("node1")
	expected := 10.0 / 3.0
	if math.Abs(balance1-expected) > 0.001 {
		t.Fatalf("node1 余额应约为 %.4f, 实际: %f", expected, balance1)
	}

	// node_no_stake 不应获得奖励
	if eco.Rewards.GetBalance("node_no_stake") != 0 {
		t.Fatal("未质押节点不应获得奖励")
	}

	// 4. 罚没 + 举报奖励
	slashAmount, reporterReward, err := eco.ProcessSlash("node1", "node2", 0.5, "task2")
	if err != nil {
		t.Fatalf("罚没失败: %v", err)
	}
	if slashAmount != 500.0 {
		t.Fatalf("罚没金额应为 500.0, 实际: %f", slashAmount)
	}
	if reporterReward != 100.0 {
		t.Fatalf("举报奖励应为 100.0, 实际: %f", reporterReward)
	}

	// 验证 node2 获得举报奖励
	node2Balance := eco.Rewards.GetBalance("node2")
	// 应该是 任务奖励(3.33..) + 举报奖励(100.0)
	if node2Balance < 103.0 {
		t.Fatalf("node2 余额应大于 103.0, 实际: %f", node2Balance)
	}

	// 5. 查看摘要
	summary := eco.GetNodeSummary("node1")
	if summary.StakeAmount != 500.0 {
		t.Fatalf("node1 质押应为 500.0, 实际: %f", summary.StakeAmount)
	}
	if summary.SlashTotal != 500.0 {
		t.Fatalf("node1 罚没总额应为 500.0, 实际: %f", summary.SlashTotal)
	}
}

func TestEconomy_ProcessTaskCompletion_NoEligible(t *testing.T) {
	eco := NewEconomy(100.0, 10.0)

	// 没有节点质押
	eligible, err := eco.ProcessTaskCompletion("task1", []string{"node1", "node2"})
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if len(eligible) != 0 {
		t.Fatalf("应无有资格节点, 实际: %d", len(eligible))
	}
}

func TestEconomy_ExportImport(t *testing.T) {
	eco := NewEconomy(100.0, 10.0)

	eco.Stakes.Stake("node1", 1000.0)
	eco.Rewards.DistributeTaskReward("task1", []string{"node1"})

	// 导出
	data, err := eco.Export()
	if err != nil {
		t.Fatalf("导出失败: %v", err)
	}

	// 导入到新实例
	eco2 := NewEconomy(0, 0)
	err = eco2.Import(data)
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}

	// 验证状态一致
	stake1, _ := eco2.Stakes.GetStake("node1")
	if stake1.Amount != 1000.0 {
		t.Fatalf("导入后质押应为 1000.0, 实际: %f", stake1.Amount)
	}
	if eco2.Rewards.GetBalance("node1") != eco.Rewards.GetBalance("node1") {
		t.Fatal("导入后余额不一致")
	}
}

// ====================== 并发测试 ======================

func TestStakeManager_Concurrent(t *testing.T) {
	sm := NewStakeManager(100.0)
	var wg sync.WaitGroup

	// 100 个并发质押
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			nodeID := fmt.Sprintf("node_%d", id)
			sm.Stake(nodeID, float64(100+id))
		}(i)
	}
	wg.Wait()

	// 验证所有节点都质押成功
	for i := 0; i < 100; i++ {
		nodeID := fmt.Sprintf("node_%d", i)
		if !sm.IsEligible(nodeID) {
			t.Fatalf("并发质押后 %s 应有资格", nodeID)
		}
	}

	// 并发查询
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

	// 100 个并发奖励分发
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

	// 验证总分发量
	total := rd.GetTotalDistributed()
	expected := 10.0 * 100 // 100个任务，每个10.0
	if math.Abs(total-expected) > 0.001 {
		t.Fatalf("总分发量应为 %.0f, 实际: %f", expected, total)
	}
}

func TestEconomy_Concurrent(t *testing.T) {
	eco := NewEconomy(100.0, 10.0)

	// 先质押
	for i := 0; i < 10; i++ {
		nodeID := fmt.Sprintf("node_%d", i)
		eco.Stakes.Stake(nodeID, 1000.0)
	}

	var wg sync.WaitGroup
	// 并发处理任务完成和查询
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

	// 验证没有 panic 即为通过
}

// ====================== 边界条件测试 ======================

func TestStakeManager_SlashBelowMinStake(t *testing.T) {
	sm := NewStakeManager(100.0)
	sm.Stake("node1", 150.0)

	// 罚没 50% 后剩余 75.0，低于最低质押 100.0
	sm.Slash("node1", 0.5)
	if sm.IsEligible("node1") {
		t.Fatal("罚没后低于最低质押时不应有资格")
	}

	// 仍然可以查询质押信息
	stake, err := sm.GetStake("node1")
	if err != nil {
		t.Fatalf("获取质押失败: %v", err)
	}
	if stake.Amount != 75.0 {
		t.Fatalf("质押金额应为 75.0, 实际: %f", stake.Amount)
	}
}

func TestStakeManager_MultipleSlashes(t *testing.T) {
	sm := NewStakeManager(100.0)
	sm.Stake("node1", 1000.0)

	// 连续罚没
	sm.Slash("node1", 0.1) // 罚 100, 剩 900
	sm.Slash("node1", 0.1) // 罚 90, 剩 810
	sm.Slash("node1", 0.1) // 罚 81, 剩 729

	stake, _ := sm.GetStake("node1")
	if math.Abs(stake.Amount-729.0) > 0.001 {
		t.Fatalf("连续罚没后质押应约为 729.0, 实际: %f", stake.Amount)
	}
	expectedSlashTotal := 1000.0 - 729.0
	if math.Abs(stake.SlashTotal-expectedSlashTotal) > 0.001 {
		t.Fatalf("累计罚没应约为 %.1f, 实际: %f", expectedSlashTotal, stake.SlashTotal)
	}
}

func TestRewardDistributor_SingleNodeMultipleTasks(t *testing.T) {
	rd := NewRewardDistributor(10.0)

	// 一个节点参与多个任务
	for i := 0; i < 5; i++ {
		rd.DistributeTaskReward(fmt.Sprintf("task_%d", i), []string{"node1"})
	}

	// 余额应为 50.0
	balance := rd.GetBalance("node1")
	if balance != 50.0 {
		t.Fatalf("余额应为 50.0, 实际: %f", balance)
	}

	// 历史记录应为 5 条
	history := rd.GetHistory("node1")
	if len(history) != 5 {
		t.Fatalf("历史记录数应为 5, 实际: %d", len(history))
	}
}

func TestEconomy_SlashWithNoReporter(t *testing.T) {
	eco := NewEconomy(100.0, 10.0)
	eco.Stakes.Stake("node1", 1000.0)

	// 无举报者的罚没
	slashAmount, reporterReward, err := eco.ProcessSlash("node1", "", 0.5, "task1")
	if err != nil {
		t.Fatalf("无举报者罚没失败: %v", err)
	}
	if slashAmount != 500.0 {
		t.Fatalf("罚没金额应为 500.0, 实际: %f", slashAmount)
	}
	if reporterReward != 0 {
		t.Fatalf("无举报者时奖励应为 0, 实际: %f", reporterReward)
	}
}
