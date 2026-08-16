# AIB1 Bridge Snapshot Tool

AIB1 到 AIB2 桥接迁移快照生成工具。

## 功能

- 从 CSV/JSON 导入账户余额数据
- 生成 Merkle Tree
- 计算各账户的 Merkle Proof
- 导出完整的快照数据供验证使用

## 文件说明

| 文件 | 说明 |
|------|------|
| `aib1_snapshot.json` | 快照配置模板（待填入实际数据） |
| `snapshot_config.json` | 工具运行配置文件 |
| `aib1_accounts_sample.csv` | CSV 格式账户数据示例 |
| `snapshot_tool.go` | 工具源代码 |
| `snapshot_tool` | 编译后的可执行文件 |

## 使用方法

### 1. 准备账户数据

创建 CSV 文件（格式：`address,balance,timestamp,nonce`）：

```csv
0x1234567890123456789012345678901234567890,1000,1735689599,0
0xabcdefabcdefabcdefabcdefabcdefabcdefabcd,5000,1735689599,0
```

或使用 JSON 格式：

```json
[
  {"address": "0x1234...7890", "balance": "1000", "timestamp": 1735689599, "nonce": 0},
  {"address": "0xabcd...efcd", "balance": "5000", "timestamp": 1735689599, "nonce": 0}
]
```

### 2. 生成快照

```bash
# 使用命令行参数
./snapshot_tool \
  -input accounts.csv \
  -output snapshot_result.json \
  -deadline "2027-12-31T23:59:59Z" \
  -id "aib1-snapshot-2025" \
  -v

# 或使用配置文件
./snapshot_tool -config snapshot_config.json -v
```

### 3. 仅验证数据

```bash
./snapshot_tool -input accounts.csv -deadline "2027-12-31T23:59:59Z" -validate
```

## 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-config` | 配置文件路径 | - |
| `-input` | 输入文件路径 | - |
| `-output` | 输出文件路径 | - |
| `-id` | 快照 ID | 自动生成 |
| `-time` | 快照时间戳（RFC3339） | 当前时间 |
| `-deadline` | 认领截止时间（RFC3339） | - |
| `-network` | 网络标识 | aib1-mainnet |
| `-hash` | 哈希算法 | sha256 |
| `-v` | 详细输出 | false |
| `-validate` | 仅验证数据 | false |

## 输出格式

生成的快照包含：

```json
{
  "snapshot_id": "...",
  "snapshot_root": "...",      // Merkle Root (hex)
  "total_accounts": N,
  "total_amount": "...",
  "merkle_tree": [...],         // 完整 Merkle Tree
  "proofs": {                   // 每个地址的证明
    "0x1234...": {
      "leaf_hash": "...",
      "path": ["...", "..."],
      "indices": [0, 0]
    }
  }
}
```

## 地址格式要求

- 40-64 位十六进制字符
- 可选 0x 前缀
- 示例：`0x1234567890123456789012345678901234567890`

## Merkle Tree 结构

- 标准二叉 Merkle Tree
- 奇数节点时复制最后一个节点（Bitcoin 约定）
- SHA-256 哈希算法
- 叶子数据格式：`address:balance:timestamp:nonce`
