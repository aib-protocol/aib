# AIB DeFi 合约部署工具

## 概述

AIB DeFi 合约部署工具是一套完整的部署和验证解决方案，用于在 AIB 区块链上部署和验证 DeFi 合约。

## 功能特性

- ✅ **完整的DeFi合约套件**: WETH, Uniswap V2 Factory, Router
- ✅ **自动化部署工具**: Go语言编写的部署主程序
- ✅ **多环境支持**: 开发网、测试网、主网配置
- ✅ **验证脚本**: 自动化合约验证和测试
- ✅ **详细文档**: 部署指南和使用手册

## 目录结构

```
./cmd/deploy-contracts/
├── main.go              # 部署主程序 (Go)
├── config.yaml          # 配置文件
├── go.mod              # Go模块文件
├── networks.json        # 网络配置
├── contracts/          # 合约源码
│   ├── WETH.sol        # WETH包装合约
│   ├── UniswapV2.sol   # Factory和Pair合约
│   ├── Router.sol      # Router合约
│   └── AIBTestToken.sol # 测试代币
├── deploy_genesis.sh   # Genesis初始化脚本
├── deploy_contracts.sh # 合约部署脚本
└── verify_contracts.sh # 验证脚本
```

## 快速开始

### 1. 环境准备

```bash
# 安装Go (1.22+)
go version

# 设置环境变量
export RPC_ENDPOINT="http://localhost:8545"
export PRIVATE_KEY="0xYOUR_PRIVATE_KEY"
```

### 2. 配置

编辑 `config.yaml` 文件:

```yaml
rpc_endpoint: "http://localhost:8545"
chain_id: 314159
private_key: "0xYOUR_PRIVATE_KEY"
gas_limit: 8000000
timeout_sec: 300
```

### 3. 部署

```bash
cd ./cmd/deploy-contracts

# 部署所有合约
./deploy_contracts.sh --contract all

# 单独部署
./deploy_contracts.sh --contract weth
./deploy_contracts.sh --contract factory
./deploy_contracts.sh --contract router
```

### 4. 验证

```bash
# 验证已部署的合约
./verify_contracts.sh
```

## 合约说明

### WETH (Wrapped Ether)
- 将ETH包装为ERC20代币
- 支持存款和提取功能
- 用于与DeFi协议交互

### UniswapV2Factory
- 创建和管理代币交易对
- 核心DEX工厂合约
- 支持任意代币对创建

### UniswapV2Router
- 用户交互的主要入口
- 提供交换和流动性管理
- 支持多种交换路径

## 文档链接

- [部署验证报告](https://www.aib.one:51200/docs/deployment/verification-report.html)
- [用户部署指南](https://www.aib.one:51200/docs/developers/defi-deploy-guide.html)
- [计划文档](https://www.aib.one:51200/plans/deploy-defi-verification.md)

## 技术栈

- **部署工具**: Go 1.22+
- **合约语言**: Solidity 0.8.20+
- **网络协议**: JSON-RPC
- **脚本语言**: Bash

## 测试

工具包含完整的测试套件:

- 合约部署测试
- 功能验证测试
- 性能测试
- 用户体验测试

## 许可证

MIT License

## 贡献

欢迎提交Issue和Pull Request来改进这个工具。
