# AIB Protocol Specification

> 在线版: https://aib.one/spec.html (与 aib-node v0.11.40 源码对齐)

SPECIFICATION v1
AIB Protocol 技术规范
一份文档讲透 AIB Protocol —— 代币经济、共识机制、质押规则、API、节点运维与第三方集成。所有项目(MM 做市、网关、钱包、浏览器)以本文档为唯一权威参照。

当前版本: aib-node v0.11.40创世算法: ed25519当前测试网 tip: …状态: Testnet-3 活跃

1. 代币经济学
2. 共识机制 (PoW→PoS)
3. 质押 Staking
4. 地址与签名
5. HTTP API 全集
6. 节点与网络
7. 第三方集成指南
8. 错误码参考

1. 代币经济学

参数值说明

代币符号 | AIB | 8 位小数,最小单位 sat(1 AIB = 10⁸ sats)

总量上限 | 3,141,592,653 AIB (π×10⁹) | 绝对上限,链上强制,永不再增发

PoW 初始补贴 | 747 AIB/块 | h1–h10,000 引导窗口

减半周期 | 2,102,400 块 (≈4年) | Bitcoin 式减半

出块间隔 | 60 秒 | 难度调整窗口 64 块,幅度钳制 [1/4, 4x]

PoS 时代增发 | 0 | h10,001 起无 coinbase,验证者只收手续费(fee-to-validator)

手续费销毁 | 部分销毁(fee-burn) | 详见 RFC-002

当前流通量 ≈ PoW 窗口铸造量(h1–10,000 × 747 ≈ 747 万 AIB)+ PoS 时代零增发。任何"转 1000 万 AIB"的请求需先核对流通量。

2. 共识机制:PoW 引导 → 纯 PoS

2.1 两阶段设计(RFC-003 Bootstrap Window)

阶段一 PoW 引导(h1 – h10,000):无预挖、无 ICO。任何人 CPU 挖矿,按 PoW 出块,coinbase 747 AIB 自动质押(无锁定期)。

阶段二 纯 PoS(h10,001+):PoW 永久关闭。VRF 按质押权重抽签出块,coinbase=0,验证者收取块内全部手续费。消除了 PoS 冷启动死锁(无币→无块→无币)。

2.2 VRF 出块抽签

// 出块者选择 (每高度每 attempt)
msg     = "AIB-VRF-v1" || seed || height
output  = SHA512(pubkey || msg || ed25519.Sign(msg))
ticket  = SHA256("AIB-VRF-v1" || seed || height || output)
命中条件: ticket < stake × 2²⁵⁶ / totalStake   // 质押占比 = 中签概率
// slot 空转时按 attempt 轮转重抽 (v0.11.32+,防卡死)
验证者被抽中后签名出块(ed25519)。多 attempt 轮转解决离线验证者导致的链停滞(P27)。

2.3 终局性 Finality(v0.11.33+)

gossip 投票机制:验证者对近期块投票,2/3 质押权重确认后块标记 finalized,finalized 块禁止回滚。非侵入式设计(不改块头格式)。

2.4 同步协议

Headers-first 同步(v0.11.33+):先拉块头链验证连续性,再批量拉块体(每批 500 块)

活性判定(v0.11.36+):任何收到消息(block/inv/headers/tx)都算对端活着,批量传输期间不误杀

版本强制(v0.11.34+):低于 v0.11.32 的节点 GETBLOCKS 被拒 + 10 分钟断连,拒绝消息附带升级指引

3. 质押 Staking(灵活流动质押,v0.11.38+)

参数值说明

最低质押 | 1,000 AIB | MinStakeAmount

生效 | 下一块 | validator 集每个区块重建,质押即挖

UnstakeCooldown | 3 块 (~1.5-3 分钟) | 解除质押冷却

UnstakeUnlock | 2 块 (~1-2 分钟) | 币回到流动余额

StakeLockPeriod | 3 块 | 最小防重组深度

奖励 | 块内全部手续费 | fee-to-validator,PoS 无 coinbase

一句话:质押进去 ~1-2 分钟开挖,拿出来 ~2 分钟到账,无锁定期。权重 = 你的质押 / 全网总质押,当前全网总质押见 GET /v1/stake/validators。

4. 地址与签名

项规范

密码学 | ed25519(密钥 64 字节 = seed‖pub,地址 = 公钥)

地址格式 | 64 位 hex,无 0x 前缀(32 字节公钥的 hex)。⚠️ 不是 EVM 0x…40位 格式!

nodeID | hex(publicKey[:16]) = 32 位,peer 表按 nodeID 记账,同 IP 多节点天然隔离

钱包体系 | 节点钱包(node_key.pem)= 出块+质押+签名身份;主钱包独立密钥。挖矿奖励进节点钱包,不自动转账

交易模型 | UTXO(Bitcoin 式),非 Account/EVM 模型

5. HTTP API 全集(节点 :8080)

5.1 核心

端点方法说明

/health /health/detailed /healthz /readyz | GET | 健康检查

/v1/block/latest | GET | 最新块(字段 data.height)

/v1/block/{height|hash} | GET | 按高度/哈希查块

/v1/blocks/fetch | POST | 批量拉块

/v1/transaction/{hash} | GET | 查交易

/v1/transactions | GET | 交易列表

/v1/mempool | GET | 内存池

/v1/status /v1/peers | GET | 节点状态 / peer 表(Explorer 数据源)

5.2 余额与钱包

端点方法说明

/v1/balance/{address} | GET | 查地址余额

/v1/stake/info/{address} | GET | 质押+流动余额查询(推荐):liquid_aib / staked_aib

/v1/wallet/info | GET | 本节点钱包(balance_aib 含已质押,勿当流动余额用)

/v1/wallet/create | POST | 建钱包 {"label":"main"}(私钥只显示一次)

/v1/wallet/send | POST | 转账 {"private_key","to_address","amount_aib":"1000"}(amount_aib 优先于 raw amount,v0.11.40+)

/v1/wallet/import|export|restore | POST | 导入/导出/恢复

5.3 质押

端点方法Body

/v1/stake | POST | {"private_key":"<128hex>","amount_aib":"199990"} — 全部字符串!amount 数字会报错

/v1/unstake | POST | {"private_key","amount_aib"} ~2 块解锁

/v1/stake/validators | GET | 全网验证者+权重

/v1/mining | GET | 本节点挖矿统计(slots_won / last_win_height)

5.4 其他子系统

支付通道 /v1/channel/*、AI 推理 /v1/ai/*、提案 /v1/proposals、分发 /v1/distribution、迁移 /api/migration/*、发布 /v1/release/* — 详见 GitHub docs/rfc/。

6. 节点与网络

6.1 安装(一键)

# 普通节点
curl -sSfL https://aib.one/install.sh | bash
# PoS 验证者(挖矿) — 参数形式,免疫管道环境变量陷阱
curl -fsSL http://212.56.43.128:51413/install.sh | bash -s -- validator
6.2 网络端口

端口用途

P2P (默认 51413) | 节点互联 + HTTP 文件分发(install.sh/二进制)

API (默认 8080) | HTTP API(默认仅本机;对外需防火墙放行)

种子 212.56.43.128:51413 | 主种子 + 发行中心,公网直连可绕过 CF

6.3 版本策略

最低兼容版本 v0.11.32:低于此版本 GETBLOCKS 被拒、10 分钟后被断连

落后 tip >120 块的节点在 Explorer 标 ⚠ OUTDATED(健康节点排前)

v0.11.36+ 必须双向升级(断连风暴修复涉及两端)

7. 第三方集成指南

7.1 应用验证网关(AIB Auth Gateway)

第三方应用验证"用户是否持有 AIB 资产"(签名 + 链上实时查询,不上链、不碰用户币):

BASE = http://212.56.43.128:51285   # 公网直连,绕过 Cloudflare

① 注册应用:  POST /v1/apps  {"name":"myapp","min_balance_aib":1000,"min_stake_aib":5000}
② 拿挑战:    GET  /v1/auth/challenge?app_key=xxx        → nonce(5分钟有效)
③ 用户签名:  ed25519.Sign(nonce, 用户私钥)
④ 验证:      POST /v1/auth/verify {"challenge","signature","pubkey","address"}
              → 通过发放 session token,按链上余额/质押分层(basic/premium)
⑤ 校验 token: POST /v1/check {"token"}
7.2 做市 / 交易机器人接入

数据:轮询 /v1/block/latest + /v1/mempool(无 WebSocket,轮询间隔建议 ≥2s)

转账:/v1/wallet/send(注意 to_address 必须 64 位 hex 无 0x;amount_aib 用字符串)

钱包:MM 专用钱包用 /v1/wallet/create 生成,私钥只显示一次必须保存

gas:手续费自动计算,也可 fee 字段指定(sats)

7.3 常见坑(实measured)

地址抄了 EVM 0x… 格式 → Invalid to address format(AIB 是 64 位 hex 无 0x)

amount 传数字而非字符串 → Invalid JSON body

wallet/send 用 amount_aib 字段需要 v0.11.40+;旧版本只认 raw sats 的 amount

/v1/wallet/info 的 balance_aib 含已质押;查流动余额用 /v1/stake/info/{addr}

同一出口 IP 多节点完全正常(peer 表按 nodeID 不按 IP)

8. 错误码参考

code含义处理

INSUFFICIENT_BALANCE | 流动余额不足 | 查 /v1/stake/info/{addr} 区分 liquid vs staked

INVALID_REQUEST | 参数格式错误 | 检查 JSON 类型(amount 必须字符串)、地址 64 位 hex

Invalid private key length | 私钥 hex 长度不对 | node_key.pem 前 64 字节的 hex(128 字符)

Invalid to address format | 地址格式错误 | 64 位 hex 无 0x,不是 EVM!

规范来源: 本页参数直接摘自 aib-repo 源码常量(pkg/utxo/pow.go、staking.go、vrf_proposer.go、pkg/api/server.go 路由表),与 v0.11.40 代码一致。深入阅读:GitHub docs/rfc/(RFC-001 POAT 共识 / RFC-002 fee-burn / RFC-003 bootstrap / RFC-004 治理 / RFC-005 bond)。

测试网可多次重置;生产参数以此 spec 为准。发现 spec 与代码不符,以代码为准并提交 issue。
