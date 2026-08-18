# AIB 2.0 ZKML 3-Node Testnet

This directory contains the deployment configuration for a 3-node test network of the AIB 2.0 ZKML consensus mechanism.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    3-Node Testnet                               │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   ┌──────────┐   ┌──────────┐   ┌──────────┐                  │
│   │  Node 1  │   │  Node 2  │   │  Node 3  │                  │
│   │ :51210   │   │ :51211   │   │ :51212   │                  │
│   └────┬─────┘   └────┬─────┘   └────┬─────┘                  │
│        │              │              │                          │
│        └──────────────┼──────────────┘                          │
│                       │                                         │
│                       ▼                                         │
│              ┌─────────────────┐                               │
│              │     Ollama      │                               │
│              │  (localhost)    │                               │
│              └─────────────────┘                               │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Prerequisites

1. **Go 1.22+**: Go compiler installed
2. **Ollama**: Ollama service running
3. **llama2 model**: llama2 model downloaded

### Install Ollama and the model

```bash
# Install Ollama (Linux)
curl -fsSL https://ollama.com/install.sh | sh

# Start the Ollama service
ollama serve &

# Download the llama2 model
ollama pull llama2
```

## Quick Start

### 1. Build and start

```bash
cd ./testnet
chmod +x deploy.sh stop.sh
./deploy.sh start
```

### 2. Check status

```bash
./deploy.sh status
```

### 3. View logs

```bash
tail -f logs/node1.log
tail -f logs/node2.log
tail -f logs/node3.log
```

### 4. Stop the testnet

```bash
./deploy.sh stop
# or
./stop.sh
```

## Configuration Files

Each node uses its own configuration file:

| Node | Config File | Port | Data Directory |
|------|------------|------|----------------|
| Node 1 | `config/node1.json` | 51210 | `data/node1` |
| Node 2 | `config/node2.json` | 51211 | `data/node2` |
| Node 3 | `config/node3.json` | 51212 | `data/node3` |

### Example configuration

```json
{
  "node_id": "testnet-node-1",
  "ollama_url": "http://localhost:11434",
  "model": "llama2",
  "stake_amount": 1000.0,
  "listen_addr": "127.0.0.1:51210",
  "data_dir": "./data/node1",
  "log_level": "info"
}
```

## Testing

### Run unit tests

```bash
cd .
go test ./zkml/testnet/... -v
```

### Run integration tests

```bash
cd .
go test ./zkml/... -v -count=1
```

## Directory Structure

```
testnet/
├── config/
│   ├── node1.json
│   ├── node2.json
│   └── node3.json
├── data/
│   ├── node1/
│   ├── node2/
│   └── node3/
├── logs/
│   ├── node1.log
│   ├── node1.pid
│   ├── node2.log
│   ├── node2.pid
│   ├── node3.log
│   └── node3.pid
├── deploy.sh
├── stop.sh
└── README.md
```

## Troubleshooting

### Ollama connection failure

```bash
# Check whether Ollama is running
curl http://localhost:11434/api/tags

# Restart Ollama
pkill ollama
ollama serve &
```

### Node startup failure

```bash
# View detailed logs
cat logs/node1.log

# Check port usage
lsof -i :51210
lsof -i :51211
lsof -i :51212
```

### Clean up data

```bash
rm -rf data/node*/*
rm -rf logs/*.log logs/*.pid
```

## Access Links

- **Ollama API**: http://localhost:11434
- **Node 1**: http://127.0.0.1:51210
- **Node 2**: http://127.0.0.1:51211
- **Node 3**: http://127.0.0.1:51212

---

*More documentation: https://www.aib.one:51200/plans/zkml-testnet.md*
