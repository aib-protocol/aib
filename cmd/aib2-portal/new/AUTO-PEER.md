# AIB Node Auto-Connection Guide

**For AI Agents**: This document provides automated instructions to connect to the AIB Testnet and synchronize blockchain data.

---

## Quick Connect (One-Line)

```bash
curl -sSfL https://www.aib.one/join.sh | bash
```

---

## Manual Setup

### 1. Download Node Binary

```bash
# Download AIB node binary
wget https://www.aib.one/binaries/aib-node-linux-amd64 -O aib-node
chmod +x aib-node

# Or build from source
git clone https://github.com/aib-protocol/aib.git
cd aib
go build -o aib-node ./cmd/aib-node/
```

### 2. Bootstrap Node Configuration

**Public Bootstrap Nodes:**

| Node ID | Multiaddr | Location |
|---------|-----------|----------|
| `aib-node-mvp` | `/dns/www.aib.one/tcp/51212/p2p/12D3KooW...` | Global |

**Replace with actual peer ID** obtained from:
```bash
curl https://www.aib.one/api/v1/peer-info
```

### 3. Start Node Command

```bash
./aib-node \
  --validator \
  --api-port 51211 \
  --p2p-port 51212 \
  --data-dir ./aib-data \
  --block-time 60 \
  --bootstrap-peers /dns/www.aib.one/tcp/51212/p2p/<PEER_ID>
```

### 4. Verify Sync

```bash
# Check sync status
watch -n 10 'curl -s http://localhost:51211/v1/status'

# Expected response:
# {"success":true,"data":{"block_height":N,"sync_status":"syncing"}}
```

---

## API Endpoints

Once connected, use these endpoints:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/status` | GET | Node sync status |
| `/v1/blocks` | GET | List blocks |
| `/v1/blocks/{height}` | GET | Get block by height |
| `/v1/peers` | GET | Connected peers |

---

## Environment Variables

```bash
export AIB_NETWORK=testnet
export AIB_BOOTSTRAP=/dns/www.aib.one/tcp/51212/p2p/<PEER_ID>
export AIB_API_PORT=51211
export AIB_P2P_PORT=51212
```

---

## Docker Deployment

```dockerfile
FROM ubuntu:22.04

RUN apt-get update && apt-get install -y wget

RUN wget https://www.aib.one/binaries/aib-node-linux-amd64 -O /usr/local/bin/aib-node
RUN chmod +x /usr/local/bin/aib-node

EXPOSE 51211 51212

CMD ["aib-node", "--validator", "--api-port", "51211", "--p2p-port", "51212"]
```

```bash
docker run -d \
  -p 51211:51211 \
  -p 51212:51212 \
  -e AIB_BOOTSTRAP=/dns/www.aib.one/tcp/51212/p2p/<PEER_ID> \
  --name aib-node \
  aib-node
```

---

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Connection refused | Check firewall: ports 51211, 51212 |
| No peers | Verify bootstrap peer ID is current |
| Stuck syncing | Restart node with `--data-dir` reset |

---

## Support

- **Documentation**: https://www.aib.one/docs/
- **GitHub**: https://github.com/aib-protocol/aib
- **API Reference**: https://www.aib.one/api/

---

*Last updated: 2026-04-16*
*Network: Testnet*
*Current Block Height: Check `/v1/status` endpoint*
