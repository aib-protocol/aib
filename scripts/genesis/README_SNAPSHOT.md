# AIB1 Bridge Snapshot Tool

A snapshot generation tool for the AIB1-to-AIB2 bridge migration.

## Features

- Import account balance data from CSV/JSON
- Generate a Merkle Tree
- Compute the Merkle Proof for each account
- Export the complete snapshot data for verification

## Files

| File | Description |
|------|-------------|
| `aib1_snapshot.json` | Snapshot config template (to be filled with actual data) |
| `snapshot_config.json` | Tool runtime configuration file |
| `aib1_accounts_sample.csv` | Sample account data in CSV format |
| `snapshot_tool.go` | Tool source code |
| `snapshot_tool` | Compiled executable |

## Usage

### 1. Prepare account data

Create a CSV file (format: `address,balance,timestamp,nonce`):

```csv
0x1234567890123456789012345678901234567890,1000,1735689599,0
0xabcdefabcdefabcdefabcdefabcdefabcdefabcd,5000,1735689599,0
```

Or use JSON format:

```json
[
  {"address": "0x1234...7890", "balance": "1000", "timestamp": 1735689599, "nonce": 0},
  {"address": "0xabcd...efcd", "balance": "5000", "timestamp": 1735689599, "nonce": 0}
]
```

### 2. Generate the snapshot

```bash
# Using command-line arguments
./snapshot_tool \
  -input accounts.csv \
  -output snapshot_result.json \
  -deadline "2027-12-31T23:59:59Z" \
  -id "aib1-snapshot-2025" \
  -v

# Or using a config file
./snapshot_tool -config snapshot_config.json -v
```

### 3. Validate data only

```bash
./snapshot_tool -input accounts.csv -deadline "2027-12-31T23:59:59Z" -validate
```

## Command-Line Arguments

| Argument | Description | Default |
|----------|-------------|---------|
| `-config` | Config file path | - |
| `-input` | Input file path | - |
| `-output` | Output file path | - |
| `-id` | Snapshot ID | Auto-generated |
| `-time` | Snapshot timestamp (RFC3339) | Current time |
| `-deadline` | Claim deadline (RFC3339) | - |
| `-network` | Network identifier | aib1-mainnet |
| `-hash` | Hash algorithm | sha256 |
| `-v` | Verbose output | false |
| `-validate` | Validate data only | false |

## Output Format

The generated snapshot contains:

```json
{
  "snapshot_id": "...",
  "snapshot_root": "...",      // Merkle Root (hex)
  "total_accounts": N,
  "total_amount": "...",
  "merkle_tree": [...],         // Full Merkle Tree
  "proofs": {                   // Proof for each address
    "0x1234...": {
      "leaf_hash": "...",
      "path": ["...", "..."],
      "indices": [0, 0]
    }
  }
}
```

## Address Format Requirements

- 40-64 hexadecimal characters
- Optional 0x prefix
- Example: `0x1234567890123456789012345678901234567890`

## Merkle Tree Structure

- Standard binary Merkle Tree
- For an odd number of nodes, the last node is duplicated (Bitcoin convention)
- SHA-256 hash algorithm
- Leaf data format: `address:balance:timestamp:nonce`
