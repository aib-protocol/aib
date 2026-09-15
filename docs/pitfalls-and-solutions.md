# AIB Protocol 踩坑档案与解决方案 (Pitfalls & Solutions Reference)

> 本文档供所有 agent 与后续开发者使用。每个坑都经过实测验证,含根因、修复 commit、验证方法。
> 最后更新: 2026-09-15 (chain h47850+, v0.11.30)

## 目录

1. [共识/出块](#共识出块)
2. [交易/手续费/UTXO](#交易手续费utxo)
3. [Mempool](#mempool)
4. [P2P/同步](#p2p同步)
5. [API 层](#api-层)
6. [应用层(资产引擎/DEX)](#应用层)
7. [运维](#运维)

---

## 共识/出块

### P1. fresh-sync proposer mismatch 死锁
- **症状**: 新节点同步卡死在固定高度(h6308/8247),永远追不上
- **根因**: 同步路径不重建 validator set、不恢复共识高度 → 每个节点 VRF 选出不同的 winner → 死锁
- **修复**: commit `97e6abc` — sync 路径按已应用块重建 validator set + 恢复共识高度;深历史块(timestamp>1h)跳过 proposer sortition(签名+PoW+连通性仍验证)
- **效果**: 新节点 25k 块 ~2min 同步完成

### P2. 时间漂移检查拒绝过去的块 → 永久落后
- **症状**: 节点落后 5-10min 后永远追不上(1块/30s爬行被 30s 出块抵消)
- **根因**: 时间戳漂移检查拒绝了"过去方向"的近尖端历史块
- **修复**: commit `03434b6` — 只检查未来方向(Bitcoin 语义)。实测 182 节点曾因此落后 17 块数小时

### P3. 手续费凭空销毁 — staker 零收益 (CRITICAL)
- **症状**: 出块者余额永不变,质押无任何收益,验证节点零激励
- **根因**: tx 的 fee(inputs>outputs 差值)没有任何人认领 — 不进 coinbase、不进 epoch 结算(`epochFees.add` 从未被调用,死代码)
- **修复**: commit `ca71a62` — produceBlock 计算本块全部 tx 的 fee 总和,加进 coinbase 付给 proposer 钱包
- **兼容性**: 验证路径本就允许 PoW era 后存在 coinbase(是"无要求"不是"禁止"),旧节点也接受 → 软分叉安全
- **验证**: 发 fee=3000 的 tx,另一节点出块打包(h47835),coinbase 记录 fee

### P4. 测试网小容量参数 ≠ 正式经济模型
- **事实**: `PoWEraBlocks=1000`(注释自认 "testnet fast era"),奖励 31.415/块,半小时挖完 31,415 AIB
- **正式设计**: RFC-003 bootstrap window K=10,000 块 × 1 AIB/块(等权 VRF 抽签,auto-stake),60s 块;总量上限 π×10⁸;之后 fee-burn 模式零通胀
- **坑**: 不要把测试网参数当成经济模型结论

### P5. V2 奖励代码是死代码
- `CreateCoinbaseV2`(50 AIB 拆 30 质押+20 推理)从未被调用,只是规范草稿的代码化。真实路径是 `CreateCoinbaseTransaction`。读代码时勿被误导

## 交易/手续费/UTXO

### P6. wallet/send 默认 fee 低于最低费率 → tx 永久卡死 (CRITICAL)
- **症状**: API 返回成功,tx 进 mempool,但**永不打包**、也永不报错。USDC 锚定假成功 12 小时才被发现
- **根因**: 默认 `feePerByte=1 × 估算200B = 200 sats`,但实际 tx 244B → 有效费率 0.82 sat/B < 共识最低 `BaseFeePerByte=10 sat/B`。mempool 接受了(无费率下限检查)但出块路径…实际上能打包(无过滤),真正卡死原因见 P7
- **修复**: commit `ca71a62` — 默认改为 10 sat/B × 300B 保守估算;调用方指定低于一半下限的 fee 自动抬到下限
- **教训**: "API 返回 success" ≠ "交易会上链"。**必须轮询 confirmations>0 才算成功**

### P7. 锚定 verify 端点被 mempool 欺骗
- **症状**: `/v1/verify?tx=` 对 mempool 里未上链的 tx 也返回 FOUND
- **根因**: verify 查询的是"是否见过此 tx"而非"是否在块里"
- **正确验证方法**: `/v1/transaction/<hash>` 检查 `confirmations > 0`
- **后果实例**: USDC 锚定 tx dc30cefa 卡 mempool 12h,账本记为成功;后来另一 tx 花掉同一 UTXO 上链,锚定彻底作废 → **引擎 SQLite 账本与链不一致**

### P8. memo 字段被静默丢弃
- `/v1/wallet/send` 接受 `memo` 但从不序列化进 tx。链上没有 memo 载荷
- **绕行**: 用**递增 amount 做 nonce**(amount=1,2,3…锚定第 N 个事件),每个锚定 tx 唯一可识别

### P9. 无 nonce → 相同 in/out 产生字节级相同的 tx
- tx hash = double-SHA256(serialize),无 nonce 字段。完全相同的输入输出发两次 = double-spend 检测直接拒绝(即使意图是两笔合法交易)
- **绕行**: amount±1 区分

### P10. 单 UTXO 钱包连续锚定必须等块
- 引擎钱包每次锚定花掉唯一 UTXO,产出新 UTXO 要等进块(~35s)才能再花。连续调用会 double-spend 被拒
- **解决**: 引擎内置 wait-for-block + 重试循环(~8×35s 上限)

### P11. /v1/utxo 显示 index bug
- 返回 0 余额/空列表,但 UTXO 实际存在且可花(wallet/send 的 UTXO 选择不受影响)。非阻塞,勿据此判断余额(用 /v1/balance)

## Mempool

### P12. 已上链 tx 的 mempool 条目不清理(部分路径)
- **症状**: dc30cefa 已被替代(其输入 UTXO 被另一 tx 花掉上链),但 mempool 仍持有它数小时,且新 tx 引用同 UTXO 时报 "double spend detected in mempool"
- **现状**: 自己产的块有 RemoveConfirmed(cmd/aib-node/main.go:1193, chain.go:258);冲突 tx(被更优 tx 顶替)的清理缺失
- **待修**: mempool 需要在收到新块时清理"输入已被花费"的冲突条目(Bitcoin 的 removeConflicts 语义)

## P2P/同步

### P13. tx gossip 链路(已验证正常,勿重复排查)
- 链路: API `wallet/send` → `txBroadcaster`(main.go:471 接线)→ `BroadcastTx`(peer_manager.go:404, MsgTx)→ 对端 `onTx` 回调入 mempool → `relayTx` 二跳
- 实测: tx 提交到节点 A,节点 B 出块打包 ✓。**曾误判为"广播不工作",真凶是 P6 低 fee**

## API 层

### P14. tx 查询的 height/confirmations 字段不可靠
- `/v1/transaction/<hash>`: confirmations 可信(>0=已上链);`height` 字段经常为 0(未填)。判断上链只看 confirmations

### P15. 节点 API 端口因部署参数而异
- 种子: 31999(127.0.0.1) | node3: 8081(127.0.0.1) | 远程节点各不同。P2P 口≠API 口。查 `systemctl cat aib-*` 的 ExecStart

## 应用层

### P16. Go embed + HTML 反引号冲突
- Go raw string 里嵌 HTML/JS 模板字符串(backtick)会语法错误。**解决**: HTML 独立成 `web/index.html` 用 `go:embed`

### P17. lightweight-charts v5 API 变更
- `addCandlestickSeries()` 已废弃 → `chart.addSeries(LightweightCharts.CandlestickSeries, {...})`。unpkg 默认最新版是 5.x,旧教程全部失效

### P18. Go mutex 不可重入
- handler 持 `mu.Lock()` 后调用内部也 `mu.Lock()` 的函数 = 自死锁,HTTP 请求永久挂起(asset-engine 曾因此 issue 请求 280s 超时)。锁要分层:导出锁 vs 内部锁分开

### P19. 浮点仓位除零 panic 可击穿整个进程
- dex-engine 净额平仓后 Qty=0,反向单触发 `margin/qty` 除零 → panic → 进程崩溃重启循环。**所有除法前检查分母**;http recover 不保护进程级崩溃

### P20. 已成交订单残留 book
- maker 单全额成交后需标记 filled 并从 book 删除,否则 depth 聚合出现幽灵档位。成交、部分成交、净额平仓三条路径都要验证订单状态迁移

## 运维

### P21. UFW 端口范围语法陷阱
- nf_tables 后端下 `ufw allow 51200:51240/tcp` 的 iptables-restore 可能失败自封。**用逗号分隔逐个端口**;改前备份 `/etc/ufw/user.rules` 到 /tmp;enable 后立即验证 SSH

### P22. 节点升级 = 共识层变更需要全网络滚动升级
- 出块逻辑改动(fee-to-validator 等)虽软分叉兼容(旧验证节点接受新块),但只有升级后的节点**出块时才会支付 fee**。混合网络期间收益不对称 — 升级要快、全

### P23. Subagent 600s 硬超时
- 复杂前端/重构任务几乎必超时(v1×3 + v2×2 全灭)。策略: 子代理只做窄范围机械任务;复杂工作直接做;子代理的部分产出要人工验收(曾留下 P18 死锁)

---

## 快速诊断手册

| 症状 | 首查 |
|------|------|
| tx 提交成功但不上链 | `/v1/transaction/<hash>` confirmations;mempool fee 是否 <10sat/B×size |
| 新节点同步卡死 | P1/P2;看 journalctl proposer mismatch |
| 出块者不赚钱 | 块里是否有 coinbase fee(P3);节点是否升级 ca71a62+ |
| wallet/send 报 double spend in mempool | mempool 僵尸 tx(P12);换 amount 或等块 |
| 余额显示 0 但 UTXO 应该存在 | P11;用 /v1/balance |
