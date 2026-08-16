# AIB 2.0 部署自动化脚本

本目录包含 AIB 2.0 主网上线和节点升级的自动化脚本。

## 目录结构

```
scripts/deploy/
├── upgrade.sh              # 节点升级脚本
├── mainnet-init.sh         # 主网初始化脚本
├── validate-upgrade.sh     # 升级验证脚本
├── rollback.sh             # 紧急回滚脚本
├── templates/              # 配置模板
│   ├── config.toml         # 节点配置模板
│   └── aib2-mainnet.service # systemd 服务模板
├── backups/                # 备份目录
├── logs/                   # 日志目录
└── README.md              # 本文档
```

## 脚本说明

### 1. upgrade.sh - 节点升级脚本

自动将节点从当前版本升级到新版本。

**用法:**
```bash
./upgrade.sh [选项]
```

**选项:**
- `-v, --version VERSION` - 指定升级版本 (默认: latest)
- `-b, --backup-dir DIR` - 备份目录 (默认: ./backups)
- `-s, --skip-backup` - 跳过备份步骤 (不推荐)
- `-f, --force` - 强制升级，跳过确认
- `-d, --dry-run` - 模拟运行，不执行实际升级
- `-h, --help` - 显示帮助信息

**示例:**
```bash
# 升级到最新版本
./upgrade.sh

# 升级到指定版本
./upgrade.sh -v 2.1.0

# 模拟运行
./upgrade.sh -n

# 使用自定义备份目录
./upgrade.sh -b /custom/backup/path
```

**升级流程:**
1. 检查当前节点状态
2. 备份数据和配置
3. 下载新版本二进制
4. 验证二进制完整性
5. 更新配置文件
6. 停止当前服务
7. 部署新版本
8. 启动服务
9. 验证升级成功

### 2. mainnet-init.sh - 主网初始化脚本

初始化新节点加入主网或测试网。

**用法:**
```bash
./mainnet-init.sh [选项]
```

**选项:**
- `-n, --network NETWORK` - 指定网络: mainnet|testnet (默认: mainnet)
- `-i, --node-id NODE_ID` - 指定节点 ID (可选，自动生成)
- `-p, --port PORT` - 指定 API 端口 (默认: 51200)
- `-m, --moniker NODE_NAME` - 指定节点名称 (必需)
- `-s, --stake AMOUNT` - 质押金额 (默认: 1000000)
- `-d, --data-dir DIR` - 数据目录
- `-f, --force` - 强制初始化（覆盖现有数据）
- `-h, --help` - 显示帮助信息

**示例:**
```bash
# 初始化主网验证者节点
./mainnet-init.sh -m "my-validator" -s 1000000

# 初始化测试网节点
./mainnet-init.sh -n testnet -m "test-validator"

# 指定自定义端口
./mainnet-init.sh -p 51201 -m "secondary-node"
```

### 3. validate-upgrade.sh - 升级验证脚本

验证升级后的节点状态和功能。

**用法:**
```bash
./validate-upgrade.sh [选项]
```

**选项:**
- `-c, --check CHECKS` - 指定检查项: all|version|consensus|api|p2p|sync (默认: all)
- `-s, --service SERVICE` - 指定服务名 (默认: aib2-mainnet)
- `-p, --port PORT` - API 端口 (默认: 51200)
- `-t, --timeout SECONDS` - 超时时间 (默认: 30)
- `-v, --verbose` - 详细输出
- `-h, --help` - 显示帮助信息

**检查项:**
- `version` - 验证节点版本
- `consensus` - 验证共识状态
- `api` - 验证 API 可用性
- `p2p` - 验证 P2P 网络连接
- `sync` - 验证同步状态
- `chain` - 验证链上活动

**示例:**
```bash
# 验证所有项目
./validate-upgrade.sh

# 仅验证版本
./validate-upgrade.sh -c version

# 验证多个项目
./validate-upgrade.sh -c version,consensus,api

# 详细输出
./validate-upgrade.sh -v
```

### 4. rollback.sh - 紧急回滚脚本

在升级失败时回滚到之前的版本。

**用法:**
```bash
./rollback.sh [选项]
```

**选项:**
- `-b, --backup BACKUP_PATH` - 指定备份路径
- `-l, --list` - 列出可用备份
- `-s, --service SERVICE` - 服务名 (默认: aib2-mainnet)
- `-f, --force` - 强制回滚，跳过确认
- `-h, --help` - 显示帮助信息

**示例:**
```bash
# 列出可用备份
./rollback.sh -l

# 回滚到指定备份
./rollback.sh -b /path/to/backup

# 强制回滚
./rollback.sh -b backup_path -f

# 回滚到最近的备份
./rollback.sh -b latest
```

## 使用流程

### 首次部署

1. 初始化节点:
```bash
./mainnet-init.sh -m "my-validator" -s 1000000
```

2. 验证节点状态:
```bash
./validate-upgrade.sh -c all -v
```

### 节点升级

1. 模拟升级:
```bash
./upgrade.sh -d
```

2. 执行升级:
```bash
./upgrade.sh -v 2.1.0
```

3. 验证升级:
```bash
./validate-upgrade.sh -c all -v
```

### 紧急回滚

1. 列出备份:
```bash
./rollback.sh -l
```

2. 执行回滚:
```bash
./rollback.sh -b latest -f
```

3. 验证回滚:
```bash
./validate-upgrade.sh -c all
```

## 权限要求

所有脚本需要设置为可执行:
```bash
chmod +x scripts/deploy/*.sh
```

## 依赖要求

- Bash 4.0+
- systemctl (systemd)
- curl
- jq
- sha256sum
- openssl

## 注意事项

1. **备份**: 所有操作都会自动备份，建议定期检查备份目录
2. **测试**: 在生产环境前，先在测试环境验证
3. **监控**: 升级后持续监控节点状态
4. **日志**: 检查日志以排查问题
5. **回滚**: 如遇问题，及时使用回滚脚本

## 日志位置

- 节点日志: `./logs/mainnet.log`
- 错误日志: `./logs/mainnet.error.log`
- Systemd 日志: `journalctl -u aib2-mainnet -f`

## 常见问题

### Q: 升级后节点无法启动
A: 检查日志，确认版本兼容性，必要时使用回滚脚本

### Q: 同步进度慢
A: 正常现象，等待同步完成

### Q: P2P 连接失败
A: 检查防火墙设置和端口开放

### Q: API 无法访问
A: 确认端口配置和防火墙规则

## 联系支持

如有问题，请访问: https://docs.aib.network
