# AIB 2.0 SDK

Multi-language SDK for AIB 2.0 blockchain.

## AIB 2.0 规范

- **加密算法**: Ed25519
- **地址格式**: Bech32m (HRP: "aib")
- **地址派生**: SHA256(公钥) = 32字节地址
- **交易模型**: UTXO

## 目录结构

```
sdk/
├── go/           # Go SDK
│   ├── wallet.go
│   ├── transaction.go
│   └── client.go
├── js/           # JavaScript/TypeScript SDK
│   ├── wallet.js
│   ├── transaction.js
│   └── client.js
└── python/       # Python SDK
    ├── wallet.py
    ├── transaction.py
    ├── client.py
    └── __init__.py
```

## Go SDK

### 安装

```bash
go get github.com/aib-protocol/aib/sdk/go
```

### 使用示例

```go
package main

import (
    "fmt"
    "github.com/aib-protocol/aib/sdk/go"
)

func main() {
    // 创建新钱包
    wallet, err := aib.NewWallet()
    if err != nil {
        panic(err)
    }

    fmt.Println("地址:", wallet.GetAddressString())
    fmt.Println("公钥:", wallet.GetPublicKeyHex())

    // 从私钥导入
    // wallet, err := aib.NewWalletFromPrivateKey("your-private-key-hex")

    // 签名消息
    message := []byte("Hello, AIB!")
    signature := wallet.Sign(message)
    fmt.Println("签名:", hex.EncodeToString(signature))

    // 验证签名
    valid := wallet.Verify(message, signature)
    fmt.Println("验证结果:", valid)
}
```

### 交易操作

```go
// 构建交易
inputs := []aib.TXInputParams{
    {TxHash: "previous-tx-hash", Index: 0},
}
outputs := []aib.TXOutputParams{
    {Address: "recipient-address", Amount: 1000},
}

tx, err := aib.BuildTransaction(inputs, outputs)
if err != nil {
    panic(err)
}

// 签名交易
err = wallet.SignTransaction(tx)
if err != nil {
    panic(err)
}

// 发送交易
client := aib.NewClient(aib.DefaultClientConfig())
txHash, err := client.SendTransaction(tx)
```

### API 客户端

```go
// 创建客户端
config := aib.ClientConfig{
    BaseURL: "http://localhost:8080/api/v1",
    Timeout: 30 * time.Second,
}
client := aib.NewClient(config)

// 查询余额
balance, err := client.GetBalance("aib1...")
if err != nil {
    panic(err)
}
fmt.Printf("余额: %d\n", balance.Confirmed)

// 查询 UTXO
utxos, err := client.GetUTXOs("aib1...")

// 估算费用
fee, err := client.EstimateFee(500)
```

## JavaScript SDK

### 安装

```bash
npm install aib-sdk
```

### 使用示例

```javascript
const { Wallet, Address, Transaction } = require('./wallet');

async function main() {
    // 创建新钱包
    const wallet = await Wallet.create();

    console.log('地址:', wallet.getAddressString());
    console.log('公钥:', Buffer.from(wallet.getPublicKey()).toString('hex'));

    // 从私钥导入
    // const wallet = await Wallet.fromPrivateKey('your-private-key-hex');

    // 签名消息
    const message = Buffer.from('Hello, AIB!');
    const signature = await wallet.sign(message);
    console.log('签名:', signature.toString('hex'));

    // 验证签名
    const valid = await wallet.verify(message, signature);
    console.log('验证结果:', valid);
}

main();
```

### 交易操作

```javascript
const { Client } = require('./client');
const { Transaction, TXInput, TXOutput } = require('./transaction');

// 构建交易
const tx = Transaction.build(
    [
        { txHash: 'previous-tx-hash', index: 0 }
    ],
    [
        { address: 'recipient-address', amount: 1000 }
    ]
);

// 签名
await tx.signWith(wallet);

// 序列化
const serialized = tx.serialize();
console.log('交易Hex:', serialized.toString('hex'));

// 发送
const client = new Client({ baseURL: 'http://localhost:8080/api/v1' });
const result = await client.sendTransaction(tx);
console.log('交易哈希:', result.tx_hash);
```

## Python SDK

### 安装

```bash
pip install aib-sdk
```

### 使用示例

```python
from aib import Wallet, Address

# 创建新钱包
wallet = Wallet.create()

print("地址:", wallet.get_address_string())
print("公钥:", wallet.get_public_key_hex())

# 从私钥导入
# wallet = Wallet.from_private_key("your-private-key-hex")

# 签名消息
message = b"Hello, AIB!"
signature = wallet.sign(message)
print("签名:", signature.hex())

# 验证签名
is_valid = wallet.verify(message, signature)
print("验证结果:", is_valid)
```

### 交易操作

```python
from aib import Transaction, TXInput, TXOutput, Wallet, Client, ClientConfig

# 构建交易
tx = Transaction.build(
    inputs=[{"tx_hash": "previous-tx-hash", "index": 0}],
    outputs=[{"address": "recipient-address", "amount": 1000}]
)

# 签名
tx.sign_with(wallet)

# 序列化
serialized = tx.serialize()
print("交易Hex:", serialized.hex())

# 发送
client = Client(ClientConfig(base_url="http://localhost:8080/api/v1"))
result = client.send_transaction(tx)
print("交易哈希:", result["tx_hash"])
```

### API 客户端

```python
from aib import Client, ClientConfig

# 创建客户端
client = Client(ClientConfig(base_url="http://localhost:8080/api/v1"))

# 查询余额
balance = client.get_balance("aib1...")
print(f"余额: {balance.get('confirmed', 0)}")

# 查询 UTXO
utxos = client.get_utxos("aib1...")

# 估算费用
fee = client.estimate_fee(500)
```

## 通用工作流程

### 1. 创建钱包

```go
// Go
wallet, _ := aib.NewWallet()
```

```javascript
// JavaScript
const wallet = await Wallet.create();
```

```python
# Python
wallet = Wallet.create()
```

### 2. 获取地址

```go
address := wallet.GetAddressString()
```

```javascript
const address = wallet.getAddressString();
```

```python
address = wallet.get_address_string()
```

### 3. 充值后查询余额

```go
balance, _ := client.GetBalance(address)
fmt.Println(balance.Confirmed)
```

```javascript
const balance = await client.getBalance(address);
console.log(balance.confirmed);
```

```python
balance = client.get_balance(address)
print(balance["confirmed"])
```

### 4. 构建并发送交易

```go
// 构建 -> 签名 -> 发送
tx, _ := aib.BuildTransaction(inputs, outputs)
wallet.SignTransaction(tx)
txHash, _ := client.SendTransaction(tx)
```

```javascript
// 构建 -> 签名 -> 发送
const tx = Transaction.build(inputs, outputs);
await tx.signWith(wallet);
const result = await client.sendTransaction(tx);
```

```python
# 构建 -> 签名 -> 发送
tx = Transaction.build(inputs, outputs)
tx.sign_with(wallet)
result = client.send_transaction(tx)
```

## 错误处理

所有 SDK 都使用异常/错误机制处理错误。建议:

```go
// Go
wallet, err := aib.NewWallet()
if err != nil {
    return fmt.Errorf("创建钱包失败: %w", err)
}
```

```javascript
// JavaScript
try {
    const wallet = await Wallet.create();
} catch (error) {
    console.error("创建钱包失败:", error.message);
}
```

```python
# Python
try:
    wallet = Wallet.create()
except Exception as e:
    print(f"创建钱包失败: {e}")
```

## 注意事项

1. **私钥安全**: 切勿在不安全的环境下暴露私钥
2. **地址格式**: AIB 使用 Bech32m 格式 (前缀: "aib1")
3. **金额单位**: 最小单位为 satoshi (1 AIB = 10^8 satoshi)
4. **测试网**: 开发时使用测试网 API

## API 端点

| 端点 | 方法 | 描述 |
|------|------|------|
| `/api/v1/balance/{address}` | GET | 查询余额 |
| `/api/v1/utxos/{address}` | GET | 查询 UTXO |
| `/api/v1/tx/{tx_hash}` | GET | 查询交易 |
| `/api/v1/tx` | POST | 发送交易 |
| `/api/v1/network` | GET | 网络信息 |
| `/api/v1/fee` | GET | 估算费用 |

## 许可证

MIT License
