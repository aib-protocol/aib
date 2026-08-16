package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aib-protocol/aib/zkml/orchestrator"
)

// MinerStatus 矿工状态信息
type MinerStatus struct {
	NodeID         string        `json:"node_id"`          // 节点 ID
	Running        bool          `json:"running"`          // 是否运行中
	Uptime         time.Duration `json:"uptime"`           // 运行时长
	TasksProcessed int           `json:"tasks_processed"`  // 已处理任务数
	Model          string        `json:"model"`            // 使用的模型
	StartTime      time.Time     `json:"start_time"`       // 启动时间
}

// Miner 矿工节点核心，负责管理推理任务的执行
type Miner struct {
	config     *MinerConfig      // 矿工配置
	orch       *orchestrator.Orchestrator // 编排器实例
	scheduler  *orchestrator.Scheduler    // 调度器
	running    bool              // 运行状态标志
	startedAt  time.Time         // 启动时间
	tasksCount int               // 已处理任务计数
	mu         sync.RWMutex      // 读写锁，保护并发访问
	cancel     context.CancelFunc // 取消函数，用于停止主循环
}

// NewMiner 创建新的矿工实例
func NewMiner(config *MinerConfig) (*Miner, error) {
	if config == nil {
		return nil, fmt.Errorf("miner: 配置不能为 nil")
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("miner: 配置验证失败: %w", err)
	}

	// 创建编排器，使用默认配置
	orchConfig := orchestrator.DefaultOrchestratorConfig()
	orch := orchestrator.NewOrchestrator(orchConfig)

	return &Miner{
		config:    config,
		orch:      orch,
		scheduler: orch.Scheduler(),
		running:   false,
	}, nil
}

// Start 启动矿工节点
func (m *Miner) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("miner: 矿工已在运行中")
	}

	// 注册节点到调度器
	nodeInfo := &orchestrator.NodeInfo{
		ID:      m.config.NodeID,
		ModelID: []byte(m.config.Model), // 使用模型名作为 ModelID
		Stake:   m.config.StakeAmount,
		Active:  true,
	}

	if err := m.scheduler.RegisterNode(nodeInfo); err != nil {
		return fmt.Errorf("miner: 注册节点失败: %w", err)
	}

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.running = true
	m.startedAt = time.Now()

	// 订阅事件，监听任务分配
	m.orch.EventBus().Subscribe(orchestrator.EventTaskAssigned, m.handleTaskAssigned)

	// 启动主循环（后台 goroutine）
	go m.runLoop(ctx)

	return nil
}

// Stop 停止矿工节点
func (m *Miner) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("miner: 矿工未在运行")
	}

	// 取消上下文，停止主循环
	if m.cancel != nil {
		m.cancel()
	}

	// 从调度器注销节点
	m.scheduler.UnregisterNode(m.config.NodeID)

	m.running = false
	return nil
}

// Status 获取矿工当前状态
func (m *Miner) Status() *MinerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var uptime time.Duration
	if m.running {
		uptime = time.Since(m.startedAt)
	}

	return &MinerStatus{
		NodeID:         m.config.NodeID,
		Running:        m.running,
		Uptime:         uptime,
		TasksProcessed: m.tasksCount,
		Model:          m.config.Model,
		StartTime:      m.startedAt,
	}
}

// ProcessTask 处理单个推理任务
func (m *Miner) ProcessTask(task *orchestrator.Task) error {
	if task == nil {
		return fmt.Errorf("miner: 任务不能为 nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("miner: 矿工未运行，无法处理任务")
	}

	// 检查任务是否分配给当前节点
	assignedToMe := false
	for _, nodeID := range task.AssignedTo {
		if nodeID == m.config.NodeID {
			assignedToMe = true
			break
		}
	}

	if !assignedToMe {
		return fmt.Errorf("miner: 任务 %s 未分配给当前节点", task.ID)
	}

	// 模拟推理过程（实际应调用 Ollama API）
	// 这里仅做演示，实际推理由 inference 模块实现
	result := fmt.Sprintf("inference_result_for_%s", task.ID)

	// 设置任务结果
	task.SetResult(m.config.NodeID, result)

	// 增加任务计数
	m.tasksCount++

	return nil
}

// handleTaskAssigned 处理任务分配事件
func (m *Miner) handleTaskAssigned(event *orchestrator.Event) {
	if event == nil {
		return
	}

	// 获取任务详情
	task, err := m.orch.GetTask(event.TaskID)
	if err != nil {
		return
	}

	// 检查是否分配给当前节点
	for _, nodeID := range task.AssignedTo {
		if nodeID == m.config.NodeID {
			// 异步处理任务，避免阻塞事件总线
			go func(t *orchestrator.Task) {
				if err := m.ProcessTask(t); err != nil {
					// 记录错误但不中断运行
					fmt.Printf("处理任务失败: %v\n", err)
				}
			}(task)
			break
		}
	}
}

// runLoop 主循环，监听和处理任务
func (m *Miner) runLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// 上下文取消，退出循环
			return
		case <-ticker.C:
			// 定期检查任务状态（可扩展心跳、健康检查等）
			// 当前保持最小实现
		}
	}
}

// Orchestrator 返回编排器实例（用于测试和外部访问）
func (m *Miner) Orchestrator() *orchestrator.Orchestrator {
	return m.orch
}

// Config 返回矿工配置（只读访问）
func (m *Miner) Config() *MinerConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}
