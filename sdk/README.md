# AIB 2.0 SDK

Multi-language SDK for AIB 2.0 blockchain.

## AIB 2.0 Specification

- **Cryptography**: Ed25519
- **Address format**: Bech32m (HRP: "aib")
- **Address derivation**: SHA256(public key) = 32-byte address
- **Transaction model**: UTXO

## Directory Structure

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

### Installation

```bash
go get github.com/aib-protocol/aib/sdk/go
```

### Usage Example

```go
package main

import (
    "fmt"
    "github.com/aib-protocol/aib/sdk/go"
)

func main() {
    // Create a new wallet
    wallet, err := aib.NewWallet()
    if err != nil {
        panic(err)
    }

    fmt.Println("Address:", wallet.GetAddressString())
    fmt.Println("Public key:", wallet.GetPublicKeyHex())

    // Import from private key
    // wallet, err := aib.NewWalletFromPrivateKey("your-private-key-hex")

    // Sign a message
    message := []byte("Hello, AIB!")
    signature := wallet.Sign(message)
    fmt.Println("Signature:", hex.EncodeToString(signature))

    // Verify the signature
    valid := wallet.Verify(message, signature)
    fmt.Println("Valid:", valid)
}
```

### Transaction Operations

```go
// Build a transaction
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

// Sign the transaction
err = wallet.SignTransaction(tx)
if err != nil {
    panic(err)
}

// Send the transaction
client := aib.NewClient(aib.DefaultClientConfig())
txHash, err := client.SendTransaction(tx)
```

### API Client

```go
// Create a client
config := aib.ClientConfig{
    BaseURL: "http://localhost:8080/api/v1",
    Timeout: 30 * time.Second,
}
client := aib.NewClient(config)

// Query balance
balance, err := client.GetBalance("aib1...")
if err != nil {
    panic(err)
}
fmt.Printf("Balance: %d\n", balance.Confirmed)

// Query UTXOs
utxos, err := client.GetUTXOs("aib1...")

// Estimate fee
fee, err := client.EstimateFee(500)
```

## JavaScript SDK

### Installation

```bash
npm install aib-sdk
```

### Usage Example

```javascript
const { Wallet, Address, Transaction } = require('./wallet');

async function main() {
    // Create a new wallet
    const wallet = await Wallet.create();

    console.log('Address:', wallet.getAddressString());
    console.log('Public key:', Buffer.from(wallet.getPublicKey()).toString('hex'));

    // Import from private key
    // const wallet = await Wallet.fromPrivateKey('your-private-key-hex');

    // Sign a message
    const message = Buffer.from('Hello, AIB!');
    const signature = await wallet.sign(message);
    console.log('Signature:', signature.toString('hex'));

    // Verify the signature
    const valid = await wallet.verify(message, signature);
    console.log('Valid:', valid);
}

main();
```

### Transaction Operations

```javascript
const { Client } = require('./client');
const { Transaction, TXInput, TXOutput } = require('./transaction');

// Build a transaction
const tx = Transaction.build(
    [
        { txHash: 'previous-tx-hash', index: 0 }
    ],
    [
        { address: 'recipient-address', amount: 1000 }
    ]
);

// Sign
await tx.signWith(wallet);

// Serialize
const serialized = tx.serialize();
console.log('Transaction hex:', serialized.toString('hex'));

// Send
const client = new Client({ baseURL: 'http://localhost:8080/api/v1' });
const result = await client.sendTransaction(tx);
console.log('Transaction hash:', result.tx_hash);
```

## Python SDK

### Installation

```bash
pip install aib-sdk
```

### Usage Example

```python
from aib import Wallet, Address

# Create a new wallet
wallet = Wallet.create()

print("Address:", wallet.get_address_string())
print("Public key:", wallet.get_public_key_hex())

# Import from private key
# wallet = Wallet.from_private_key("your-private-key-hex")

# Sign a message
message = b"Hello, AIB!"
signature = wallet.sign(message)
print("Signature:", signature.hex())

# Verify the signature
is_valid = wallet.verify(message, signature)
print("Valid:", is_valid)
```

### Transaction Operations

```python
from aib import Transaction, TXInput, TXOutput, Wallet, Client, ClientConfig

# Build a transaction
tx = Transaction.build(
    inputs=[{"tx_hash": "previous-tx-hash", "index": 0}],
    outputs=[{"address": "recipient-address", "amount": 1000}]
)

# Sign
tx.sign_with(wallet)

# Serialize
serialized = tx.serialize()
print("Transaction hex:", serialized.hex())

# Send
client = Client(ClientConfig(base_url="http://localhost:8080/api/v1"))
result = client.send_transaction(tx)
print("Transaction hash:", result["tx_hash"])
```

### API Client

```python
from aib import Client, ClientConfig

# Create a client
client = Client(ClientConfig(base_url="http://localhost:8080/api/v1"))

# Query balance
balance = client.get_balance("aib1...")
print(f"Balance: {balance.get('confirmed', 0)}")

# Query UTXOs
utxos = client.get_utxos("aib1...")

# Estimate fee
fee = client.estimate_fee(500)
```

## Common Workflow

### 1. Create a Wallet

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

### 2. Get the Address

```go
address := wallet.GetAddressString()
```

```javascript
const address = wallet.getAddressString();
```

```python
address = wallet.get_address_string()
```

### 3. Query Balance After Funding

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

### 4. Build and Send a Transaction

```go
// Build -> Sign -> Send
tx, _ := aib.BuildTransaction(inputs, outputs)
wallet.SignTransaction(tx)
txHash, _ := client.SendTransaction(tx)
```

```javascript
// Build -> Sign -> Send
const tx = Transaction.build(inputs, outputs);
await tx.signWith(wallet);
const result = await client.sendTransaction(tx);
```

```python
# Build -> Sign -> Send
tx = Transaction.build(inputs, outputs)
tx.sign_with(wallet)
result = client.send_transaction(tx)
```

## Error Handling

All SDKs use exceptions/errors for error handling. Recommended:

```go
// Go
wallet, err := aib.NewWallet()
if err != nil {
    return fmt.Errorf("failed to create wallet: %w", err)
}
```

```javascript
// JavaScript
try {
    const wallet = await Wallet.create();
} catch (error) {
    console.error("Failed to create wallet:", error.message);
}
```

```python
# Python
try:
    wallet = Wallet.create()
except Exception as e:
    print(f"Failed to create wallet: {e}")
```

## Notes

1. **Private key security**: never expose private keys in insecure environments
2. **Address format**: AIB uses Bech32m format (prefix: "aib1")
3. **Amount units**: the smallest unit is the satoshi (1 AIB = 10^8 satoshi)
4. **Testnet**: use testnet APIs during development

## API Endpoints

| Endpoint | Method | Description |
|------|------|------|
| `/api/v1/balance/{address}` | GET | Query balance |
| `/api/v1/utxos/{address}` | GET | Query UTXOs |
| `/api/v1/tx/{tx_hash}` | GET | Query transaction |
| `/api/v1/tx` | POST | Send transaction |
| `/api/v1/network` | GET | Network info |
| `/api/v1/fee` | GET | Estimate fee |

## License

MIT License
