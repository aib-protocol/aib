# RFC-001: Proof of AI Token Transactions (PoAT)
# AIB 共识机制升级提案 — 从 PoAIW 到 PoAT

- **状态 / Status**: DRAFT (草案 — 等待项目负责人审阅 / awaiting project owner review)
- **版本 / Version**: 0.1
- **日期 / Date**: 2026-08-14
- **审阅人 / Reviewer**: 项目负责人 (中文版为主) / Project owner

---

## 中文版 (供项目内部审阅)

### 1. 动机:为什么放弃 PoAIW?

当前 PoAIW (Proof of AI Work) 存在根本性缺陷:

1. **"假算力"问题**: 传统 PoW 式的挖矿逻辑套用在 AI 上,导致矿工只是重复
   "死算" — 用固定输入跑模型,并没有产生真实的经济价值。链无法区分
   "有意义的推理" 和 "为了出块而刷的推理"。
2. **无摩擦刷分**: 矿工可以自己给自己发推理请求 (self-dealing),零成本刷高
   自己的算力积分,从而垄断出块权。
3. **Staking 奖励凭空产生**: 目前 staking 奖励来自预分配池,与网络安全
   并无实际绑定。

### 2. 新机制: PoAT (Proof of AI Token Transactions)

**核心思想: 挖矿 = 真实的 AI token 交易。**

AI miner 不再"死算",而是接入任意 AI 推理服务,通过**消费 AI token**
(即真实的推理请求) 来参与挖矿:

```
矿工 ──推理请求(消耗token)──► AI 提供方 (云端API / 本地模型 / 任意LLM网关)
  ▲                              │
  │                              ▼
  └─────推理回执(证明)────── 推理结果
```

- **AI 提供方可以是任何东西**: OpenAI 兼容云 API、本地 Ollama/vLLM、
  自建模型服务。协议不关心底层模型,只关心"真实发生了 token 消费"。
- **每一笔 AI 交易都要上链**: 交易里包含推理请求摘要、token 用量、
  提供方签名回执。

### 3. 防欺诈: 交易抽成 (Fee Rotation)

纯交易量可以刷,所以**每一笔 AI 交易链上强制抽成**:

```
每笔 AI 交易: amount × (1 - fee_rate)
                             │
                             ▼
                    AIB Staking 奖励池
```

- **抽成进入 Staking 奖励池** — Staking 奖励不再凭空产生,而是来自真实的
  AI 经济活动。这就是 Staking 收益的**唯一来源**。
- **自己刷自己 = 有真实损耗**: 矿工自我交易 (self-dealing) 也要被抽成,
  刷 100 块损失 fee_rate × 100。刷分的经济成本 = 抽成率,这使得刷分只在
  "预期出块收益 > 抽成损耗" 时才划算,而抽成率参数可治理调优。
- 类比: 这个抽成等价于 PoW 里的电费 — 不可伪造的**真实资源消耗**。

### 4. 出块权选择: 交易积分 + VRF 随机性

沿用 PoS 的 proposer 选择框架,但把"质押量"换成"近期交易积分":

1. **积分 (Score)**: 矿工 i 的近期 (滑动窗口 W 个区块) 有效 AI 交易量
   (已扣抽成的净额),按质押量加权:
   ```
   score(i) = stake(i)^α × feeVolume(i, W)^β     (α+β=1, 初始 α=0.3, β=0.7)
   ```
2. **VRF 加权随机**: 每一轮用上一区块的 VRF 种子,按 score 加权随机选
   proposer。即使算力/积分最大者,每轮也只是概率最大,**不可能垄断**:
   - 独占 50% 交易量的矿工,单轮出块概率上限 ~50% (由 α 压制)
   - 小矿工始终有非零概率被随机选中 — 这正是"意外选中"的随机性来源
3. **最长链规则**: 和 PoW 一样,分叉时选"累计交易积分最大"的链
   (而非单纯最长),使交易积分等价于 PoW 的算力。

### 5. 激励结构

| 角色 | 收入 | 约束 |
|------|------|------|
| AI Miner | 出块奖励 + 用户支付的费用 | 必须真实消费 AI token (被抽成) |
| Staker | 抽成池按质押比例分红 | 锁仓,与链安全绑定 |
| AI 提供方 | 收到矿工支付的推理费 | 签名回执,接受链上声誉评分 |

### 6. 需要项目 owner 决策的参数 (开放问题)

| 参数 | 建议初值 | 说明 |
|------|---------|------|
| fee_rate (抽成率) | 3% | 治理可调;决定刷分成本 |
| α / β (质押 vs 交易权重) | 0.3 / 0.7 | β 越大越"交易本位" |
| W (积分窗口) | 100 区块 (~50分钟) | 太长反应慢,太短易操纵 |
| 出块奖励来源 | 减半表 (现有) | 与抽成池并行 |

### 7. 迁移路径 (测试网)

1. **阶段1**: 定义 `AITransaction` 交易类型 + 回执验证 (provider 签名)
2. **阶段2**: 抽成逻辑 + staking 奖励池改造成抽成驱动
3. **阶段3**: proposer 选择切换到 score+VRF
4. **阶段4**: 测试网重置 genesis,公测 + 参数校准
5. **阶段5**: 文档/白皮书更新,主网决策

### 8. 安全考量 (待深入分析)

- **回执伪造**: 提供方与矿工合谋伪造"假交易"?→ 抽成照样收,伪造成本
  = 抽成额,经济上闭环;再加提供方声誉质押可进一步约束
- **积分窗口操纵**: 突击充值刷窗口?→ 窗口 W + 指数衰减缓解
- **长程攻击**: 沿用现有 PoS 的 finality 工具 (checkpoint)

---

## English Version (for publication)

# RFC-001: Proof of AI Token Transactions (PoAT)

- **Status**: DRAFT
- **Version**: 0.1

## 1. Motivation

The current PoAIW (Proof of AI Work) consensus has fundamental flaws:

1. **Fake computation**: Miners can replay trivial inference jobs just to mine
   blocks. The chain cannot distinguish meaningful inference from waste.
2. **Frictionless self-dealing**: A miner can send inference requests to
   itself at zero cost, inflating its measured "AI work" and monopolizing
   block production.
3. **Unanchored staking rewards**: Staking rewards currently come from a
   pre-allocated pool with no tie to real economic activity.

## 2. Proposal: PoAT (Proof of AI Token Transactions)

**Mining = real AI token consumption.**

Miners connect to *any* inference backend — cloud APIs, local models
(Ollama/vLLM), or custom gateways — and mine by **spending tokens on real
inference requests**:

```
Miner ──inference request (spends tokens)──► AI Provider (any backend)
  ▲                                            │
  │                                            ▼
  └────────────signed receipt + result─────────┘
```

Every AI transaction is recorded on-chain with the request digest, token
usage, and a provider-signed receipt.

## 3. Anti-Fraud: Mandatory Fee Rotation

Every AI transaction pays an on-chain fee into the staking reward pool:

```
each AI tx: amount × (1 - fee_rate) → remainder goes to staking pool
```

- Staking rewards are funded **exclusively** by real AI economic activity —
  no longer minted from thin air.
- **Self-dealing has real cost**: a miner trading with itself still pays the
  fee. Inflation attack cost = fee_rate × volume, making grinding profitable
  only when expected block reward exceeds the burn — a tunable economic
  equilibrium, exactly like electricity in PoW.

## 4. Block Producer Selection: Score + VRF

1. **Score**: each miner's net fee volume over a sliding window (W blocks),
   weighted by stake:
   `score(i) = stake(i)^α × feeVolume(i)^β  (α=0.3, β=0.7 initially)`
2. **Weighted VRF lottery**: per-round proposer chosen by score-weighted
   VRF using the previous block's seed. Even the largest player is capped
   probabilistically — small miners always have a non-zero chance of being
   selected (the "unexpected election" property).
3. **Heaviest-chain rule**: forks resolve by cumulative transaction score,
   making transaction score the PoW-hashrate equivalent.

## 5. Incentives

| Role | Income | Constraint |
|------|--------|-----------|
| AI Miner | block reward + user fees | must really spend tokens (fee burn) |
| Staker | pro-rata share of fee pool | bonded stake secures the chain |
| AI Provider | inference fees from miners | signed receipts, on-chain reputation |

## 6. Open Parameters (governance)

| Parameter | Initial | Note |
|-----------|---------|------|
| fee_rate | 3% | sets the cost of grinding |
| α / β | 0.3 / 0.7 | stake vs transaction weight |
| W (score window) | 100 blocks | trade-off: responsiveness vs manipulation |

## 7. Migration (testnet)

1. `AITransaction` type + provider receipt verification
2. Fee rotation + staking pool rewrite
3. Proposer selection → score + VRF
4. Testnet genesis reset, public beta, parameter calibration
5. Whitepaper / docs update, mainnet decision

## 8. Security Considerations

- **Receipt forgery** (miner-provider collusion): fees still apply, so
  forgery is economically equivalent to self-dealing; provider staking and
  reputation further constrain it.
- **Window manipulation** (burst volume): sliding window + exponential decay.
- **Long-range attacks**: reuse existing PoS checkpoint tooling.

## 9. Dual-Language Policy

- English is the canonical language for code, docs, and community discussion.
- Chinese translations are maintained for the project's internal review and
  for Chinese-speaking users; the English version prevails on conflict.
