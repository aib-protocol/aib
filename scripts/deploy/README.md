# AIB 2.0 deployment automation scripts

This directory contains AIB 2.0 automation scripts for mainnet launch and node upgrades。

## directory structure

```
scripts/deploy/
├── upgrade.sh              # node upgrade script
├── mainnet-init.sh         # mainnet initialization script
├── validate-upgrade.sh     # upgrade validation script
├── rollback.sh             # emergency rollback script
├── templates/              # config template
│   ├── config.toml         # node config template
│   └── aib2-mainnet.service # systemd service template
├── backups/                # backup directory
├── logs/                   # log directory
└── README.md              # This document
```

## script description

### 1. upgrade.sh - node upgrade script

automatically upgrade the node from the current version to a new version。

**Usage:**
```bash
./upgrade.sh [options]
```

**Options:**
- `-v, --version VERSION` - specify the upgrade version (default: latest)
- `-b, --backup-dir DIR` - backup directory (default: ./backups)
- `-s, --skip-backup` - skip backup step (not recommended)
- `-f, --force` - force upgrade, skip confirmation
- `-d, --dry-run` - dry run, no actual upgrade performed
- `-h, --help` - show help information

**Examples:**
```bash
# upgrade to the latest version
./upgrade.sh

# upgrade to a specified version
./upgrade.sh -v 2.1.0

# dry run
./upgrade.sh -n

# use a custom backup directory
./upgrade.sh -b /custom/backup/path
```

**upgrade flow:**
1. check current node status
2. back up data and config
3. download the new binary
4. validate binary integrity
5. update the config file
6. stop the current service
7. deploy the new version
8. start the service
9. validate upgrade success

### 2. mainnet-init.sh - mainnet initialization script

initialize a new node to join mainnet or testnet。

**Usage:**
```bash
./mainnet-init.sh [options]
```

**Options:**
- `-n, --network NETWORK` - specify the network: mainnet|testnet (default: mainnet)
- `-i, --node-id NODE_ID` - specify the node ID (optional, auto-generated)
- `-p, --port PORT` - specify API port (default: 51200)
- `-m, --moniker NODE_NAME` - specify the node name (required)
- `-s, --stake AMOUNT` - stake amount (default: 1000000)
- `-d, --data-dir DIR` - data directory
- `-f, --force` - force initialization (overwrites existing data）
- `-h, --help` - show help information

**Examples:**
```bash
# initialize a mainnet validator node
./mainnet-init.sh -m "my-validator" -s 1000000

# initialize a testnet node
./mainnet-init.sh -n testnet -m "test-validator"

# specify a custom port
./mainnet-init.sh -p 51201 -m "secondary-node"
```

### 3. validate-upgrade.sh - upgrade validation script

validate node status and functionality after upgrade。

**Usage:**
```bash
./validate-upgrade.sh [options]
```

**Options:**
- `-c, --check CHECKS` - specify checks: all|version|consensus|api|p2p|sync (default: all)
- `-s, --service SERVICE` - specify service name (default: aib2-mainnet)
- `-p, --port PORT` - API port (default: 51200)
- `-t, --timeout SECONDS` - timeout (default: 30)
- `-v, --verbose` - verbose output
- `-h, --help` - show help information

**check item:**
- `version` - validate node version
- `consensus` - validate consensus status
- `api` - validate API availability
- `p2p` - validate P2P network connection
- `sync` - validatesync status
- `chain` - validate on-chain activity

**Examples:**
```bash
# validate all items
./validate-upgrade.sh

# validate version only
./validate-upgrade.sh -c version

# validate multiple items
./validate-upgrade.sh -c version,consensus,api

# verbose output
./validate-upgrade.sh -v
```

### 4. rollback.sh - emergency rollback script

roll back to the previous version if the upgrade fails。

**Usage:**
```bash
./rollback.sh [options]
```

**Options:**
- `-b, --backup BACKUP_PATH` - specify backup path
- `-l, --list` - list available backups
- `-s, --service SERVICE` - service name (default: aib2-mainnet)
- `-f, --force` - force rollback, skip confirmation
- `-h, --help` - show help information

**Examples:**
```bash
# list available backups
./rollback.sh -l

# roll back to the specified backup
./rollback.sh -b /path/to/backup

# force rollback
./rollback.sh -b backup_path -f

# roll back to the most recent backup
./rollback.sh -b latest
```

## usage flow

### initial deployment

1. Initialize node:
```bash
./mainnet-init.sh -m "my-validator" -s 1000000
```

2. validate node status:
```bash
./validate-upgrade.sh -c all -v
```

### node upgrade

1. simulated upgrade:
```bash
./upgrade.sh -d
```

2. perform the upgrade:
```bash
./upgrade.sh -v 2.1.0
```

3. validate upgrade:
```bash
./validate-upgrade.sh -c all -v
```

### emergency rollback

1. list backups:
```bash
./rollback.sh -l
```

2. execute rollback:
```bash
./rollback.sh -b latest -f
```

3. validate rollback:
```bash
./validate-upgrade.sh -c all
```

## permission requirements

All scripts need to be made executable:
```bash
chmod +x scripts/deploy/*.sh
```

## dependency requirements

- Bash 4.0+
- systemctl (systemd)
- curl
- jq
- sha256sum
- openssl

## notes

1. **backup**: all operations back up automatically; check the backup directory regularly
2. **test**: validate in a test environment before production
3. **monitoring**: continuously monitor node status after upgrade
4. **logs**: check logs to troubleshoot
5. **rollback**: if issues arise, use the rollback script promptly

## log location

- node logs: `./logs/mainnet.log`
- error logs: `./logs/mainnet.error.log`
- Systemd logs: `journalctl -u aib2-mainnet -f`

## FAQ

### Q: node fails to start after upgrade
A: check logs to confirm version compatibility; use the rollback script if needed

### Q: slow sync progress
A: normal, wait for sync to complete

### Q: P2P connection failed
A: check firewall settings and open ports

### Q: API inaccessible
A: confirm port config and firewall rules

## contact support

if you have issues, visit: https://docs.aib.network
