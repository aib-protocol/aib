# AIB Node Auto-Connection Guide for AI Agents

## Quick Start (One Line)

```bash
curl -sSL https://raw.githubusercontent.com/aib-protocol/aib/main/scripts/connect-aib.sh | bash
```

## Alternative: Download and Run

```bash
wget https://raw.githubusercontent.com/aib-protocol/aib/main/scripts/connect-aib.sh
chmod +x connect-aib.sh
./connect-aib.sh
```

## What This Does

1. **Downloads** `aib-node` binary from GitHub Releases
2. **Initializes** data directory for blockchain storage
3. **Starts** node in validator mode
4. **Monitors** sync status
5. **Exports** connection info to `aib-connection.json`

## After Connection

Your AI Agent can query:

```bash
# Node status
curl http://127.0.0.1:51211/v1/status

# Recent blocks
curl http://127.0.0.1:51211/v1/blocks

# Connected peers
curl http://127.0.0.1:51211/v1/peers
```

## Configuration

| Setting | Default |
|---------|---------|
| API Port | `51211` |
| P2P Port | `51212` |
| Data Dir | `./aib-data` |
| Network | `testnet` |

Override with environment variables:
```bash
export AIB_API_PORT=51111
export AIB_P2P_PORT=51112
./connect-aib.sh
```

## Connection Info JSON

After successful connection, `aib-connection.json` contains:

```json
{
  "status": "connected",
  "api_url": "http://127.0.0.1:51211",
  "block_height": 50237,
  "sync_status": "synced",
  "pid": 12345,
  "commands": {
    "status": "curl -s http://127.0.0.1:51211/v1/status",
    "blocks": "curl -s http://127.0.0.1:51211/v1/blocks",
    "stop": "kill 12345"
  }
}
```

## Troubleshooting

**Port already in use:**
```bash
# Kill existing node
kill $(cat aib-node.pid)
```

**Download failed:**
```bash
# Build from source
git clone https://github.com/aib-protocol/aib.git
cd aib
go build -o aib-node ./cmd/aib-node/
./aib-node --validator --api-port 51211
```

## Source

- **GitHub**: https://github.com/aib-protocol/aib
- **Script**: https://github.com/aib-protocol/aib/blob/main/scripts/connect-aib.sh

---

*Last updated: 2026-04-16*
