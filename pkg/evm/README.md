# EVM 兼容性测试套件

## 概述

完整的 EVM 兼容性测试套件，确保可以部署和运行 DeFi 应用。

## 文件结构

```
./pkg/evm/
├── evm_test.go          # 核心功能测试
├── precompiled_test.go  # 预编译合约测试
├── defi_test.go         # DeFi 场景测试
├── security_test.go     # 安全测试
├── doc.go               # 包文档
├── executor.go          # EVM 执行器
├── state.go             # 状态管理
├── gas.go               # Gas 计算
├── precompiled.go       # 预编译合约
└── journal.go           # 状态日志

./testdata/evm/
├── Interfaces.sol       # Solidity 接口定义
└── testvectors.json     # 测试向量

./scripts/
└── test-evm-coverage.sh # 测试覆盖率脚本
```

## 测试范围

### 1. 核心功能测试 (evm_test.go)

- 地址类型转换
- 哈希运算 (Keccak256)
- 状态管理
- 交易执行
- 合约部署
- Gas 计算
- EVM 操作码测试
- 边缘情况处理

### 2. 预编译合约测试 (precompiled_test.go)

- **ecrecover (0x01)**: ECDSA 签名恢复
- **SHA256 (0x02)**: SHA-256 哈希
- **RIPEMD160 (0x03)**: RIPEMD-160 哈希
- **Identity (0x04)**: 数据复制
- **Modexp (0x05)**: 模幂运算
- **BN128Add (0x06)**: 椭圆曲线点加法
- **BN128Mul (0x07)**: 椭圆曲线点乘法
- **BN128Pairing (0x08)**: 椭圆曲线配对
- **Blake2F (0x09)**: Blake2 压缩函数

### 3. DeFi 场景测试 (defi_test.go)

#### ERC20 标准
- `transfer` - 代币转账
- `approve` - 授权额度
- `transferFrom` - 从授权账户转账
- `allowance` - 查询授权额度
- `totalSupply` - 查询总供应量
- `balanceOf` - 查询余额

#### ERC721 标准
- `mint` - 铸造 NFT
- `transfer` - 转移 NFT
- `approve` - 授权 NFT
- `ownerOf` - 查询所有者

#### Uniswap V2 风格交易
- 代币交换
- 添加/移除流动性
- 滑点保护

#### Flashloan
- 借款和还款
- 费用计算
- 失败回滚

#### 价格预言机
- 价格更新
- 价格过期检测
- 访问控制

### 4. 安全测试 (security_test.go)

- **重入攻击防护**: 测试重入守卫模式
- **整数溢出防护**: SafeMath 操作测试
- **访问控制**: 角色权限测试
- **Gas 优化**: Gas 成本分析
- **DoS 防护**: 外部调用限制
- **输入验证**: 边界检查
- **状态不变量**: 状态一致性验证

## 运行测试

### 基本测试
```bash
cd .
go test -v ./pkg/aal/...
```

### 覆盖率测试
```bash
go test -v -cover ./pkg/aal/...
go test -v -coverprofile=coverage.out ./pkg/aal/...
go tool cover -html=coverage.out
```

### 基准测试
```bash
go test -bench=. -benchmem ./pkg/aal/...
```

### 使用覆盖率脚本
```bash
bash ./scripts/test-evm-coverage.sh
```

## 测试覆盖率目标

- **目标**: 80% 以上
- **当前**: 请查看覆盖率报告

## 性能基准

关键操作的性能基准测试：

- `BenchmarkKeccak256`: Keccak256 哈希计算
- `BenchmarkStateGetBalance`: 余额查询
- `BenchmarkStateSetBalance`: 余额设置
- `BenchmarkAddressConversion`: 地址转换
- `BenchmarkERC20Transfer`: ERC20 转账
- `BenchmarkUniswapSwap`: Uniswap 交换
- `BenchmarkFlashloan`: 闪电贷

## 测试数据

### Solidity 接口 (`Interfaces.sol`)

包含完整的 DeFi 接口定义：
- ERC20/ERC721 标准
- Uniswap V2 Factory/Pair/Router
- Flashloan 接收器
- 借贷池接口
- 价格预言机接口
- 治理接口
- 质押接口
- 多签钱包接口

### 测试向量 (`testvectors.json`)

标准化的测试输入和预期输出。

## 贡献指南

1. 新增测试时，请确保覆盖率达到 80% 以上
2. 使用描述性的测试名称
3. 为复杂的测试添加注释
4. 运行所有测试和基准测试后再提交

## 相关文档

- [EVM 开发者指南](https://www.aib.one:51200/docs/evm-dev-guide.html)
- [DeFi 部署教程](https://www.aib.one:51200/docs/defi-deployment.html)
- [安全文档](https://www.aib.one:51200/docs/security.html)
