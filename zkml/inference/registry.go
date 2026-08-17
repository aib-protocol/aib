package inference

import (
	"sync"
)

// ModelRegistry 模型注册表

// 维护所有已注册模型的信息、权重、性能指标

// 无需权限即可注册新模型（permissionless）
// 权重通过治理决定
type ModelRegistry struct {
	mu            sync.RWMutex
	models        map[string]*ModelInfo        // modelID -> ModelInfo
	performance   map[string]*ModelPerformance // modelID -> 性能数据
	votingWeights map[string]float64           // modelID -> 投票中权重
}

// NewModelRegistry 创建新模型注册表
func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		models:        make(map[string]*ModelInfo),
		performance:   make(map[string]*ModelPerformance),
		votingWeights: make(map[string]float64),
	}
}

// RegisterModel 注册新模型

// 任何人都可以注册（permissionless）
// 初始权重为 1.0（基准）
func (r *ModelRegistry) RegisterModel(info *ModelInfo) error {
	if info == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	info.Weight = 1.0 // 初始权重
	if info.Performance == nil {
		info.Performance = &ModelPerformance{}
	}

	r.models[info.ModelID] = info
	r.performance[info.ModelID] = info.Performance
	r.votingWeights[info.ModelID] = 1.0 // 初始投票权重

	return nil
}

// GetModelInfo 获取模型信息
func (r *ModelRegistry) GetModelInfo(modelID string) *ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.models[modelID]
}

// GetPerformance 获取模型性能数据
func (r *ModelRegistry) GetPerformance(modelID []byte) *ModelPerformance {
	if modelID == nil {
		return nil
	}

	modelIDStr := string(modelID)
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.performance[modelIDStr]
}

// UpdatePerformance 更新模型性能数据

// 节点定期提交性能数据
func (r *ModelRegistry) UpdatePerformance(modelID string, perf *ModelPerformance) error {
	if perf == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.models[modelID] == nil {
		return nil // 模型未注册
	}

	r.performance[modelID] = perf
	r.models[modelID].Performance = perf

	return nil
}

// ListModels 列出所有已注册模型
func (r *ModelRegistry) ListModels() []*ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ModelInfo, 0, len(r.models))
	for _, info := range r.models {
		result = append(result, info)
	}

	return result
}

// GetWeight 获取模型当前权重
func (r *ModelRegistry) GetWeight(modelID string) float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if info, ok := r.models[modelID]; ok {
		return info.Weight
	}
	return 1.0 // 默认权重
}

// SetWeight 设置模型权重

// 只能通过治理提案调用
func (r *ModelRegistry) SetWeight(modelID string, weight float64) error {
	if weight <= 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.models[modelID] == nil {
		return nil
	}

	r.models[modelID].Weight = weight
	r.votingWeights[modelID] = weight // 同步更新投票权重

	return nil
}

// ProposalManager 治理提案管理器

type ProposalManager struct {
	mu        sync.RWMutex
	proposals map[string]*GovernanceProposal
	votes     map[string][]*GovernanceVote // proposalID -> votes
	registry  *ModelRegistry
}

// NewProposalManager 创建提案管理器
func NewProposalManager(registry *ModelRegistry) *ProposalManager {
	return &ProposalManager{
		proposals: make(map[string]*GovernanceProposal),
		votes:     make(map[string][]*GovernanceVote),
		registry:  registry,
	}
}

// SubmitProposal 提交治理提案

// 任何人都可以提交，基于真实数据
func (pm *ProposalManager) SubmitProposal(proposal *GovernanceProposal) error {
	if proposal == nil {
		return nil
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	proposal.Status = ProposalStatusActive
	pm.proposals[proposal.ID] = proposal
	pm.votes[proposal.ID] = make([]*GovernanceVote, 0)

	return nil
}

// Vote 对提案投票

// 需要持有 AIB token 才能投票
func (pm *ProposalManager) Vote(proposalID, voter string, vote bool) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	proposal := pm.proposals[proposalID]
	if proposal == nil {
		return nil // 提案不存在
	}

	if proposal.Status != ProposalStatusActive {
		return nil // 提案不在投票期
	}

	// 添加投票
	governanceVote := &GovernanceVote{
		ProposalID: proposalID,
		Voter:      voter,
		Vote:       vote,
		Timestamp:  proposal.Deadline,
	}

	pm.votes[proposalID] = append(pm.votes[proposalID], governanceVote)

	return nil
}

// GetProposal 获取提案信息
func (pm *ProposalManager) GetProposal(proposalID string) *GovernanceProposal {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return pm.proposals[proposalID]
}

// FinalizeProposal 完成提案

// 检查投票结果，自动执行
func (pm *ProposalManager) FinalizeProposal(proposalID string) (*GovernanceProposal, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	proposal := pm.proposals[proposalID]
	if proposal == nil {
		return nil, nil
	}

	votes := pm.votes[proposalID]

	// 统计投票
	yesCount := 0
	totalCount := 0

	for _, v := range votes {
		totalCount++
		if v.Vote {
			yesCount++
		}
	}

	// 检查阈值（67% 多数）
	if totalCount > 0 && float64(yesCount)/float64(totalCount) >= 0.67 {
		proposal.Status = ProposalStatusPassed

		// 执行提案
		switch proposal.Type {
		case ProposalTypeWeightAdjustment:
			modelID, _ := proposal.Evidence["model_id"].(string)
			weight, _ := proposal.Evidence["proposed_weight"].(float64)
			pm.registry.SetWeight(modelID, weight)
		}

		return proposal, nil
	}

	proposal.Status = ProposalStatusRejected
	return proposal, nil
}

// ListActiveProposals 列出活跃提案
func (pm *ProposalManager) ListActiveProposals() []*GovernanceProposal {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make([]*GovernanceProposal, 0)
	for _, p := range pm.proposals {
		if p.Status == ProposalStatusActive {
			result = append(result, p)
		}
	}

	return result
}
