# AIB Protocol Specification

> 在线版: https://aib.one/spec.html (与 aib-node v0.11.42 源码对齐)

SPECIFICATION v1
AIB Protocol 技术规范
一份文档讲透 AIB Protocol —— 代币经济、共识机制、质押规则、API、节点运维与第三方集成。所有项目(MM 做市、网关、钱包、浏览器)以本文档为唯一权威参照。

当前版本: aib-node v0.11.42创世算法: ed25519当前测试网 tip: …状态: Testnet-3 活跃

1. 代币经济学
2. 共识机制 (PoW→PoS)
3. 质押 Staking
4. 地址与签名

签名体系: ed25519(确定性签名,无随机数陷阱,验签快 ~50µs)。AIB 是 PoS 链,每个区块的提议与投票都在签名/验签。

4.1 地址命名方案(权威定义)
- AIB 地址 = ed25519 公钥原样 32 字节(同 Solana/Cardano/Stellar 学派,链上/API canonical = 64 位 hex)
- 显示格式: AIB1… bech32m 大写(BIP-350,6 位校验和防手滑,不可与 EVM 0x 混淆);aib1… 全小写合法;混合大小写非法
- v0.11.42 起转账 API 双格式兼容(AIB1/aib1/64hex 全部接受);EVM 0x 地址直接拒绝,返回明确错误,资金零损失
- 换算: AIB1 ↔ 64hex = 同一 32 字节的两种拼写

| 用途 | AIB1 显示格式 | 64位hex |
|---|---|---|
| faucet | AIB1W4GT7S3DGVJZSDWE4UPGE8WMXWUHHSAYP88VUS9DMM379SW4RGMSZR6VZT | 7550bf42…1d51a37 |
| validator示例 | AIB16F6D7QZJPQYCP46MMV9TWQWGJZ6GU9UNN99LGN94JD0PXWS4RZWSHER5JY | d274df00…a15189d |

4.2 三大地址体系对比
| 维度 | AIB(ed25519 32B) | ETH(0x+20B哈希) | Bitcoin(base58check) |
|---|---|---|---|
| 签名算法 | ed25519 确定性 | ECDSA secp256k1 | ECDSA secp256k1 |
| 随机数坏→私钥泄露 | 数学上不可能 | 历史真实盗币 | 同左 |
| 验签速度 | ~50µs 最快档 | ~200-400µs | ~200-400µs |
| 地址=身份 | 公钥就是地址 | 公钥哈希后20B | 公钥双重哈希 |
| 校验和 | AIB1 6位bech32m | EIP-55半个 | 完整 |
| 前缀辨识 | AIB1 | 0x | 1/3/bc1 |
| PoS高频签名 | 最优 | 可用 | 可用 |

结论: PoS 链签名密度百倍于 PoW → 确定性+吞吐是刚需 → ed25519 裸公钥 + bech32m 显示层兼得安全与可用性。显示层 v0.11.42 上线,链上零改动零分叉。

4.3 bech32m 技术细节
BIP-350: 校验常数 0x2bc830a3,编码时 6 零填充,HRP=aib,5bit 分组,6 字符校验和。早期实现校验和计算有误(缺零填充+用错常数),v0.11.42 修复为标准 BIP-350 并经独立参考实现逐字节交叉验证。


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
