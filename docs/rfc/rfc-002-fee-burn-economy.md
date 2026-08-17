# RFC-002: The Fee-Burn Economy — Pure Fee-Funded Staking & Self-Sustaining Consensus

- **Status**: DRAFT — open for public discussion
- **Discussion**: GitHub Issue (linked below once created)
- **Supersedes**: RFC-001 §4 (block producer selection) and §6 (parameters)
- **Version**: 0.1

---

## 中文版（创始人审阅用）

### 一句话

**AIB 链 = AI 推理交易的结算层 + 抽成销毁引擎。没有预挖、没有固定增发，Staking 年化率完全由真实推理流水的抽成决定，市场自动定价。**

### 核心机制

1. 每笔链上结算的推理交易抽成 φ（建议 1%），全部进入当期奖励池
2. Staking 年化：`APR = φ·T / S`（T=当期交易流水，S=总质押，纯浮动，无人为设定）
3. 出块权 = 纯质押加权 VRF（删除交易量权重 — 刷交易量不再获得任何出块优势）
4. 创始人特权：仅一票否决权（veto），无铸造权、无分配权、无升级单方决定权

### 防作弊的数学闭环

| 攻击 | 结果 |
|------|------|
| 自我刷量 | 净损 = φ·v·(1−s) 恒为正 — 严格劣势策略 |
| 出块者塞假交易 | 假交易也要真付款+被抽成，恒亏 |
| 伪造回执合谋 | 伪造成本=真实成本，无套利空间 |
| 51% 买断质押 | 自焚式：链死→抽成池死→质押归零，负期望 |
| 压 APR 赶人再攻击 | 负反馈自动回补（S↓→APR↑→新 staker 进场） |

### 长期自演化（去创始人化路线图）

- 阶段 0（现在）：创始人持 veto，社区 issue/PR 讨论，RFC 治理流程建立
- 阶段 1：veto 范围书面收窄（只能否决"改变代币学/破坏不可增发铁律"的提案）
- 鑉段 2：veto 改为延否决（delayed veto = 延期+仲裁，而非一票杀死），常规升级走链上投票
- 阶段 3：veto 权自动日落（例如质押参与的投票率连续 N 个季度 > 阈值后失效），协议完全自治
- 自适应参数：φ、epoch 等由治理按固定规则公式调整（如 APR 目标带），不许人为拍数字 — "规则修规则，不是人修规则"

### 跨行星 / 百年尺度

协议层不认识"AI"，只认识**带签名的交易、抽成、质押**三样东西。服务形态怎么变（AI→HI→别的），结算逻辑零改动。延迟不敏感设计：
- 出块与结算本地化（行星内），跨行星仅同步 checkpoint（轻量最终性证明）
- 证据窗口按物理时延参数化（火星 ~4-24 分钟单程 → 挑战期按传输时延自动放大）

---

## English Version (canonical)

## RFC-002: The Fee-Burn Economy

### One line

**The AIB chain is a settlement layer for AI-inference trades plus a fee-burn engine. No premine, no fixed emission: the staking APR is set entirely by fees from real inference throughput — the market prices it, not a whitepaper.**

### Mechanism

1. Every on-chain settled inference transaction pays a fee φ (proposed 1%) into the current epoch reward pool.
2. Staking APR: `APR = φ·T / S` (T = epoch transaction volume, S = total stake — floating, market-discovered).
3. Block production: pure stake-weighted VRF. **The transaction-volume weight from RFC-001 §4 is removed** — washing volume grants zero block-selection advantage.
4. Founder privilege: **veto only**. No mint key, no allocation key, no unilateral upgrade power.

### Why cheating is strictly unprofitable

| Attack | Outcome |
|--------|---------|
| Wash trading | Net loss = φ·v·(1−s) > 0 — strictly dominated strategy |
| Proposer stuffing fake txs | Fake txs still require real payment + fee burn — always negative |
| Miner/provider receipt collusion | Forgery cost = real cost; no arbitrage exists |
| Buy 51% of stake | Self-immolating: chain dies → fee pool dies → attacker's own stake → 0 |
| Suppress APR then attack | Negative feedback restores S automatically (S↓ → APR↑ → new stakers enter) |

The security budget is not electricity (PoW) or fixed inflation (PoS) — it is **the real fee cash-flow of the AI economy the chain settles**. Attacks destroy their own funding source.

### Long-term self-evolution (founder-decentralization roadmap)

- **Phase 0 (now)**: founder holds veto; community discussion via issues/PRs; RFC governance process established.
- **Phase 1**: veto scope narrowed in writing — only proposals that alter monetary policy or break the no-premine/no-fixed-emission invariant can be vetoed.
- **Phase 2**: veto becomes *delayed veto* (delay + arbitration instead of kill); routine upgrades move to on-chain voting.
- **Phase 3**: veto sunsets automatically (e.g., after N consecutive quarters of voter participation above threshold). Protocol fully self-governing.
- **Adaptive parameters**: φ, epoch length, etc. adjust only via fixed governance formulas (e.g., an APR target band) — *rules amend rules; humans do not hand-pick numbers*.

### Century / interplanetary scale

The protocol layer does not know what "AI" is. It knows exactly three things: **signed transactions, fees, stake**. Whatever the service becomes (AI → HI → whatever follows), settlement logic needs zero changes.

Latency-tolerant design:

- Block production and settlement are local (per-planet); only checkpoints (light finality proofs) cross the interplanetary gap.
- Challenge windows are parameterized by physical latency (Mars: 4–24 min one-way → windows auto-scaled by measured transit time).

### Open questions for discussion

1. Is φ = 1% right? Should φ be dynamic (fee market like EIP-1559)?
2. Should the reward pool distribute per-epoch (daily) or per-block (continuous)?
3. Veto sunset condition: participation threshold? time-based? both?
4. Should checkpoint cross-links use light-client proofs or a committee of high-stake validators?
5. What belongs in the "untouchable invariant" set (no-premine, fixed supply π×10⁸, fee-only rewards … what else)?

## 9. Dual-Language Policy

English is canonical; Chinese translation maintained for the founder's review.
