#!/usr/bin/env bash
# AIB Node installer — no root required, installs to ~/.aib
# Usage: curl -sSfL https://aib.one/install.sh | bash
set -euo pipefail

VERSION=v0.11.45
REPO="aib-protocol/aib"
# Pinned artifact hashes (multi-source integrity anchor).
# Every source must match the pinned hash or the installer refuses to run.
PINNED_SHA256_AMD64="36f9756dc492dfd924414cc8fb5d3cbf81a470a488f3dd0b183b4957bf598952"
PINNED_SHA256_ARM64="93d2a7ec277607fcb7396657e47d36561033d8e8ca1325c0bbd50e97e12b18cd"
INSTALL_DIR="${AIB_HOME:-$HOME/.aib}"
BIN_DIR="$INSTALL_DIR/bin"
BIN="$BIN_DIR/aib-node"
SERVICE_NAME="aib-node"

# ---------- pretty output ----------
info()  { printf '\033[1;34m[AIB]\033[0m %s\n' "$*"; }
ok()    { printf '\033[1;32m  ✓\033[0m %s\n' "$*"; }
warn()  { printf '\033[1;33m  !\033[0m %s\n' "$*"; }
note()  { printf '\033[1;36m  •\033[0m %s\n' "$*"; }
die()   { printf '\033[1;31m[AIB] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

# ---------- verify mode: cross-check hashes against on-chain anchors ----------
# bash install.sh --verify
# Fetches /release.json (served from each node's P2P port, backed by the
# on-chain release anchor) from every known node, takes the MAJORITY
# record, and compares BOTH hashes: the running script and the binary.
VERIFY_NODES=(
  "http://182.61.43.222:51413"
  "http://212.56.43.128:51413"
  "http://212.56.43.128:51415"
  "http://154.53.40.40:51414"
)
if [ "${1:-}" = "--verify" ]; then
  SELF="$(sha256sum "$0" 2>/dev/null | awk '{print $1}')"
  info "self (install.sh) sha256 = $SELF"
  declare -A VOTES
  ANSWERED=0
  for N in "${VERIFY_NODES[@]}"; do
    J="$(curl -s --max-time 8 "$N/release.json" 2>/dev/null)" || continue
    R_NAME="$(printf '%s' "$J" | grep -o '"name":"[^"]*"' | cut -d'"' -f4)"
    R_BIN="$(printf '%s' "$J" | grep -o '"sha256":"[^"]*"' | cut -d'"' -f4)"
    R_INS="$(printf '%s' "$J" | grep -o '"installer_sha256":"[^"]*"' | cut -d'"' -f4)"
    [ -n "$R_NAME" ] || continue
    ANSWERED=$((ANSWERED+1))
    KEY="$R_NAME|$R_BIN|$R_INS"
    VOTES["$KEY"]=$(( ${VOTES["$KEY"]:-0} + 1 ))
    info "$N -> $R_NAME bin=${R_BIN:0:12}... ins=${R_INS:0:12}..."
  done
  [ "$ANSWERED" -ge 2 ] || die "fewer than 2 nodes answered /release.json — cannot establish majority"
  BEST=""; BEST_N=0
  for K in "${!VOTES[@]}"; do
    if [ "${VOTES[$K]}" -gt "$BEST_N" ]; then BEST_N="${VOTES[$K]}"; BEST="$K"; fi
  done
  [ "$BEST_N" -ge 2 ] || die "no majority among node answers (split answers = possible attack)"
  A_NAME="${BEST%%|*}"; REST="${BEST#*|}"
  A_BIN="${REST%%|*}"; A_INS="${REST#*|}"
  ok "Majority ($BEST_N/$ANSWERED): on-chain anchor for $A_NAME"
  PASS=1
  if [ -n "$A_INS" ] && [ "$A_INS" != "$SELF" ]; then
    warn "SCRIPT HASH MISMATCH: anchor=$A_INS self=$SELF"; PASS=0
  fi
  if [ -x "$BIN" ]; then
    LOCAL_BIN="$(sha256sum "$BIN" | awk '{print $1}')"
    if [ "$LOCAL_BIN" != "$A_BIN" ]; then
      warn "LOCAL BINARY MISMATCH: anchor=$A_BIN local=$LOCAL_BIN"; PASS=0
    else
      ok "local binary matches on-chain anchor"
    fi
  fi
  if [ "$A_NAME" != "$VERSION" ]; then
    warn "this script targets $VERSION but chain anchors $A_NAME (script may be stale, not malicious)"
  fi
  [ "$PASS" = "1" ] && ok "VERIFY PASS — script + binary match the PoS-chain anchored release" || die "VERIFY FAIL"
  exit 0
fi

# ---------- detect ----------
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$OS" in
  linux) OS="linux" ;;
  darwin) OS="darwin" ;;
  *) die "Unsupported OS: $OS (linux/darwin only for now)" ;;
esac
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) die "Unsupported architecture: $ARCH" ;;
esac
ASSET="aib-node-${OS}-${ARCH}"
info "Detected: ${OS}/${ARCH} → ${ASSET}"

# root not needed — refuse if running as root unnecessarily
if [ "$(id -u)" = "0" ]; then
  warn "Running as root — AIB does not need root. Installing for root user anyway."
fi

# ---------- download (multi-source, censorship resistant) ----------
# Order: GitHub -> aib.one mirror -> community P2P nodes.
# Every source serves the SAME file; the pinned hash above is the only
# trust anchor - a malicious mirror cannot make us execute bad code.
DIST_DIR="${VERSION}-testnet"
# NOTE: aib.one is behind Cloudflare which caches 404s for hours; the
# direct node sources are canonical. aib.one kept last as convenience.
SOURCES=(
  "http://212.56.43.128:51413/${DIST_DIR}"
  "http://154.53.40.40:51414/${DIST_DIR}"
  "http://216.180.75.219:51413/${DIST_DIR}"
  "http://144.91.108.90:51413/${DIST_DIR}"
  "https://aib.one/releases/${DIST_DIR}"
)
PINNED=""
case "$ARCH" in
  amd64) PINNED="$PINNED_SHA256_AMD64" ;;
  arm64) PINNED="$PINNED_SHA256_ARM64" ;;
esac
case "$PINNED" in ""|__*) die "pinned hash missing for $ARCH" ;; esac

mkdir -p "$BIN_DIR"
GOT=""
for SRC in "${SOURCES[@]}"; do
  info "Trying ${SRC} ..."
  rm -f "$BIN.tmp"
  if command -v curl >/dev/null 2>&1; then
    curl -sSfL --max-time 60 "${SRC}/${ASSET}" -o "$BIN.tmp" 2>/dev/null || continue
  else
    wget -q --timeout=60 "${SRC}/${ASSET}" -O "$BIN.tmp" 2>/dev/null || continue
  fi
  [ -s "$BIN.tmp" ] || continue
  GOT="$(sha256sum "$BIN.tmp" 2>/dev/null | awk '{print $1}' || shasum -a 256 "$BIN.tmp" | awk '{print $1}')"
  if [ "$GOT" = "$PINNED" ]; then
    ok "Downloaded + pinned sha256 verified from ${SRC}"
    break
  fi
  warn "Hash mismatch from ${SRC} - trying next source"
  GOT=""
done
[ -n "$GOT" ] || die "all download sources failed or returned bad hashes"

chmod +x "$BIN.tmp"
mv "$BIN.tmp" "$BIN"
ok "Installed: $BIN ($("$BIN" --help >/dev/null 2>&1; echo v0.11.40))"

# ---------- config / data ----------
mkdir -p "$INSTALL_DIR/data"
[ -f "$INSTALL_DIR/config.toml" ] || cat > "$INSTALL_DIR/config.toml" <<'CFG'
# AIB node configuration — defaults are fine for testnet
network = "testnet"
block_time = 30
# api_port = 8080
# p2p_port = 51413
# bootstrap = ""
# nickname = ""
CFG
ok "Config: $INSTALL_DIR/config.toml"

# ---------- PATH hint ----------
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) warn "Add to PATH:  export PATH=\"\$PATH:$BIN_DIR\"" ;;
esac

# P2P port picked AFTER stale-process kill below (see pick_p2p_port) —
# checking before the kill would see the zombie's port and hide the node
# on 51414 forever (Explorer OUTDATED root cause on TK5-138).
# ---------- storage selection (protect the system disk) ----------
# Blockchain + logs grow unbounded. Enumerate ALL non-root mounts with
# >=2GB free, show a picker (interactive), auto-pick largest (non-interactive),
# and MIGRATE existing chain data to the chosen location when it moves.
DATA_DIR="$INSTALL_DIR"   # default fallback: system disk

list_mounts() {
  awk '$3 ~ /^(ext[234]|xfs|btrfs|f2fs|exfat|vfat|ntfs|apfs)$/ && $2!="/" && $2!~/^\/boot/ && $2!~/^\/snap/ && $2!~/^\/proc/ && $2!~/^\/sys/ && $2!~/^\/dev/ && $2!~/^\/run/ && $2!~/^\/var\/lib\/docker/ && $2!~/^\/etc/ {print $2}' /proc/mounts | sort -u | while read -r m; do
    free_kb=$(df -k "$m" 2>/dev/null | awk 'NR==2{print $4}')
    dev=$(df "$m" 2>/dev/null | awk 'NR==2{print $1}')
    root_dev=$(df / 2>/dev/null | awk 'NR==2{print $1}')
    [ -n "$free_kb" ] && [ "$free_kb" -ge 2097152 ] && [ "$dev" != "$root_dev" ] && echo "$free_kb|$m|$(df -h "$m" | awk 'NR==2{print $2" total, "$4" free"}')|$(df "$m" | awk 'NR==2{print $1}')"
  done | sort -t'|' -k1 -rn
}

MOUNTS=$(list_mounts)
if [ -n "$MOUNTS" ]; then
  if [ -t 0 ]; then
    echo
    note "Blockchain storage grows fast — pick where to store it (NOT the system disk if avoidable):"
    i=0; MOUNT_ARR=""
    echo "  $((++i)). System disk  $HOME/.aib  ($(df -h / | awk 'NR==2{print $4}') free)  ← not recommended"
    while IFS='|' read -r _ m desc _d; do
      MOUNT_ARR="$MOUNT_ARR $m"
      echo "  $((++i)). $m  ($desc)"
    done <<EOF2
$MOUNTS
EOF2
    echo "  $((++i)). Custom path…"
    N_M=$(echo "$MOUNTS" | wc -l)
    printf "  Choose [1-%d, default=2 (largest free)]: " "$((N_M+2))"
    read -r pick || pick=2
    pick=${pick:-2}
    case "$pick" in
      1) info "System disk selected — chain data stays in $INSTALL_DIR" ;;
      "$((N_M+2))")
        printf "  Enter custom data dir: "; read -r cust || cust=""
        if [ -n "$cust" ] && mkdir -p "$cust" 2>/dev/null; then
          DATA_DIR="$cust"; ok "Data will be stored on: $DATA_DIR"
        else warn "Invalid path — falling back to system disk"; fi ;;
      *)
        sel=$(echo "$MOUNTS" | sed -n "${pick}p" | cut -d'|' -f2)
        if [ -n "$sel" ]; then DATA_DIR="$sel/aib-node"; ok "Data will be stored on: $DATA_DIR"
        else warn "Bad choice — auto-picking largest free disk"; DATA_DIR=$(echo "$MOUNTS" | head -1 | cut -d'|' -f2)/aib-node; ok "Data: $DATA_DIR"; fi ;;
    esac
  else
    # Non-interactive (curl|bash): auto-pick the largest-free external disk.
    DATA_DIR=$(echo "$MOUNTS" | head -1 | cut -d'|' -f2)/aib-node
    ok "External disk auto-picked (non-interactive) — data goes to $DATA_DIR"
    note "To choose a different disk:  curl … | bash -s -- datadir=/mnt/xxx   (or run interactively)"
  fi
fi
# CLI override: datadir=/path
for a in "$@"; do
  case "$a" in
    datadir=*) DATA_DIR="${a#datadir=}" ;;
  esac
done
[ -n "${AIB_DATA_DIR:-}" ] && DATA_DIR="$AIB_DATA_DIR"   # env override wins

# ---------- migrate existing chain data if the storage location changed ----------
migrate_chain_data() {
  local NEW="$1" OLD_FOUND=""
  local OLD_LOC="${AIB_PREV_DATA_DIR:-}"
  [ -z "$OLD_LOC" ] && [ -f "$INSTALL_DIR/.data-dir" ] && OLD_LOC=$(cat "$INSTALL_DIR/.data-dir" 2>/dev/null)
  # also scan common previous locations for real chain data
  for d in "$OLD_LOC" "$INSTALL_DIR" "$HOME/.aib" /nvme/aib-node; do
    [ -n "$d" ] && [ "$d" != "$NEW" ] && { [ -f "$d/chain.db" ] || [ -f "$d/blocks" ] || [ -d "$d/blocks" ]; } && OLD_FOUND="$d" && break
  done
  [ -z "$OLD_FOUND" ] && return 0
  [ "$(readlink -f "$OLD_FOUND" 2>/dev/null)" = "$(readlink -f "$NEW" 2>/dev/null)" ] && return 0
  echo
  note "Existing blockchain data found at: $OLD_FOUND ($(du -sh "$OLD_FOUND" 2>/dev/null | awk '{print $1}'))"
  if [ -t 0 ]; then
    printf "  Migrate it to %s (recommended — keeps your chain + wallet)? [Y/n] " "$NEW"
    read -r mans || ans=y; case "$ans" in n|N*) info "Skipping migration — node will sync from scratch on the new disk"; return 0 ;; esac
  fi
  mkdir -p "$NEW"
  ok "Migrating chain data $OLD_FOUND → $NEW (copy + verify, source kept as backup)…"
  if command -v rsync >/dev/null 2>&1; then
    rsync -a "$OLD_FOUND"/ "$NEW"/ 2>/dev/null || cp -a "$OLD_FOUND"/. "$NEW"/ 2>/dev/null
  else
    cp -a "$OLD_FOUND"/. "$NEW"/ 2>/dev/null
  fi
  # verify: chain.db present in the new location
  if [ -f "$NEW/chain.db" ] || [ -d "$NEW/blocks" ]; then
    ok "Migration verified — node_key/chain/utxo now on $NEW"
    note "Old copy kept at $OLD_FOUND (safe to delete manually after confirming the node runs)"
  else
    warn "Migration incomplete — node will resync on the new disk (no data lost: old copy intact)"
  fi
}
if [ "$DATA_DIR" != "$INSTALL_DIR" ]; then
  migrate_chain_data "$DATA_DIR"
  mkdir -p "$DATA_DIR" 2>/dev/null || { warn "Cannot write $DATA_DIR — falling back to system disk"; DATA_DIR="$INSTALL_DIR"; }
fi
echo "$DATA_DIR" > "$INSTALL_DIR/.data-dir" 2>/dev/null   # remember for future migrations

[ -n "${AIB_DATA_DIR:-}" ] && DATA_DIR="$AIB_DATA_DIR"   # explicit override wins


# ---------- kill any stale node from a previous install ----------
# A stale process may still serve an OLD chain on port 8080 and fool the
# health check below. Stop it so the freshly installed binary takes over.
if pgrep -f aib-node >/dev/null 2>&1; then
  info "Stopping existing aib-node process(es)..."
  systemctl --user stop aib-node >/dev/null 2>&1 || true
  pkill -f aib-node >/dev/null 2>&1 || true
  sleep 2
  pgrep -f aib-node >/dev/null 2>&1 && { pkill -9 -f aib-node >/dev/null 2>&1 || true; }
  # wait for the process(es) to fully exit AND the P2P port to be released
  for i in $(seq 1 15); do
    pgrep -f aib-node >/dev/null 2>&1 || break
    sleep 1
  done
  pgrep -f aib-node >/dev/null 2>&1 && warn "Some aib-node process still running — it may hold port $P2P_PORT; investigate: pgrep -af aib-node"

  systemctl --user reset-failed aib-node >/dev/null 2>&1 || true
  ok "Old node stopped"

fi

# ---------- pick a free P2P port AFTER any kill (51413 default) ----------
pick_p2p_port() {
  P2P_PORT=51413
  while ss -tlnH "( sport = :$P2P_PORT )" 2>/dev/null | grep -q .; do
    P2P_PORT=$((P2P_PORT+1))
  done
  [ "$P2P_PORT" != "51413" ] && info "Port 51413 busy — using P2P port $P2P_PORT"
}
pick_p2p_port

NODE_ARGS="-data-dir $DATA_DIR -api-port 8080 -p2p-port $P2P_PORT"
# AIB_VALIDATOR=1 -> run as PoS validator (mine AIB). Off by default.
# NOTE the shell pitfall: `AIB_VALIDATOR=1 curl ... | bash` does NOT pass the
# variable through the pipe (prefix applies to curl only!). Accepted forms:
#   curl ... | AIB_VALIDATOR=1 bash        <- prefix on bash
#   curl ... | bash -s -- validator        <- CLI arg (safest, documented)
#   AIB_VALIDATOR=1 bash <(curl ...)       <- process substitution
for a in "$@"; do
  case "$a" in
    validator|--validator|-v) AIB_VALIDATOR=1 ;;
  esac
done
if [ "${AIB_VALIDATOR:-0}" = "1" ]; then
  NODE_ARGS="$NODE_ARGS -validator"
fi


# ---------- forked/stale chain auto-heal ----------
# A chain frozen at a PRE-FORK height that never advances across reinstall
# cycles (classic symptom: same height for days, always OUTDATED) means the
# local chain data is on a dead fork with finality marks — the node is
# cryptographically forbidden from switching to the canonical chain. The ONLY
# cure is archiving chain DBs (wallet keys are KEPT) and resyncing fresh.
if [ -n "$DATA_DIR" ] && [ -d "$DATA_DIR" ] && command -v curl >/dev/null 2>&1; then
  LOCAL_H=$(curl -s -m3 http://127.0.0.1:8080/v1/block/latest 2>/dev/null | grep -oE '"height":[0-9]+' | head -1 | grep -oE '[0-9]+' || echo 0)
  if [ "${LOCAL_H:-0}" -gt 0 ] && [ "${LOCAL_H:-0}" -lt 13000 ]; then
    warn "Local chain frozen at h$LOCAL_H — this is a DEAD FORK (pre-VRF-fix era, h~13274)"
    if [ -t 0 ]; then
      printf "  Archive chain data + resync fresh (wallet keys kept)? [Y/n] "
      read -r heal_ans || heal_ans=y
    else
      heal_ans=y   # non-interactive: auto-heal, keys kept, old chain archived not deleted
    fi
    case "$heal_ans" in
      n|N*) warn "Keeping forked chain — node will stay OUTDATED forever until healed" ;;
      *)
        TS=$(date +%Y%m%d-%H%M%S); BAK="$HOME/.aib-oldfork-$TS"; mkdir -p "$BAK"
        for f in chain.db utxo.db block_index.db blocks node.log; do
          [ -e "$DATA_DIR/$f" ] && mv "$DATA_DIR/$f" "$BAK/" 2>/dev/null || true
        done
        ok "Dead-fork chain archived to $BAK (node_key/wallet untouched)"
        note "Node will resync the canonical chain from genesis"
        ;;
    esac
  fi
fi

# ---------- systemd --user (Linux only) ----------
RUN_NOW=0
if [ "$OS" = "linux" ] && command -v systemctl >/dev/null 2>&1; then
  mkdir -p "$HOME/.config/systemd/user"
  cat > "$HOME/.config/systemd/user/${SERVICE_NAME}.service" <<UNIT
[Unit]
Description=AIB Node (testnet)
After=network-online.target

[Service]
ExecStart=$BIN $NODE_ARGS
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
UNIT
  systemctl --user daemon-reload 2>/dev/null || true
  systemctl --user enable --now "${SERVICE_NAME}.service" 2>/dev/null && ok "Service started (systemd --user: aib-node)" || {
    info "systemd --user unavailable — starting in background automatically"
    RUN_BG=1
  }
  # Without linger, the user manager (and the node with it) dies when the
  # user logs out and never starts at boot. Linger keeps it alive forever.
  if loginctl enable-linger "${USER:-$(id -un)}" 2>/dev/null; then
    ok "Linger enabled — service survives logout and starts at boot"
  elif [ "$(id -u)" = "0" ] && loginctl enable-linger root 2>/dev/null; then
    ok "Linger enabled for root"
  else
    warn "Could not enable linger — node may stop when you log out"
  fi
else
  info "No systemd (or macOS) — starting in background automatically"
  RUN_BG=1
fi

# ---------- fallback: direct background run ----------
if [ "${RUN_BG:-0}" = "1" ]; then
  mkdir -p "$INSTALL_DIR"
  setsid nohup "$BIN" $NODE_ARGS >> "$INSTALL_DIR/node.log" 2>&1 < /dev/null &
  BG_PID=$!
  ok "Node started in background (pid $BG_PID, log: $INSTALL_DIR/node.log)"
fi

# ---------- health check: behavior-driven self-heal ----------
# A node that keeps failing to start (corrupt/old chain DB, genesis change)
# shows up as: service active but /health never answers. We do NOT parse logs
# (journald may be unreadable for the user); we watch behavior instead.
info "Waiting for node to come up..."
UP=0
for i in $(seq 1 20); do
  if curl -s -o /dev/null --max-time 2 "http://127.0.0.1:8080/health" 2>/dev/null; then UP=1; break; fi
  sleep 1
done

heal_chain() {  # archive chain DBs (keep wallet keys), restart
  warn "Node failing to start — archiving broken/old chain data (wallet keys kept)"
  systemctl --user stop aib-node >/dev/null 2>&1 || true
  pkill -f aib-node >/dev/null 2>&1 || true
  sleep 1
  local TS BAK f d
  TS=$(date +%Y%m%d-%H%M%S); BAK="$HOME/.aib-oldchain-$TS"; mkdir -p "$BAK"
  for f in chain.db utxo.db block_index.db node.log; do
    for d in "$INSTALL_DIR" "$DATA_DIR"; do
      [ -f "$d/$f" ] && mv "$d/$f" "$BAK/" 2>/dev/null || true
    done
  done
  [ -d "$DATA_DIR/blocks" ] && mv "$DATA_DIR/blocks" "$BAK/" 2>/dev/null || true
  ok "Old chain data archived to $BAK"
  systemctl --user reset-failed aib-node >/dev/null 2>&1 || true
  systemctl --user start aib-node >/dev/null 2>&1 || {
    setsid nohup "$BIN" $NODE_ARGS >> "$INSTALL_DIR/node.log" 2>&1 < /dev/null &
  }
  for i in $(seq 1 25); do
    if curl -s -o /dev/null --max-time 2 "http://127.0.0.1:8080/health" 2>/dev/null; then return 0; fi
    sleep 1
  done
  return 1
}

if [ "$UP" = "0" ]; then
  warn "Node did not come up in 20s — attempting automatic repair..."
  if heal_chain; then
    ok "Repair successful — node is UP with a fresh chain (resyncing)"
    UP=1
  fi
fi

if [ "$UP" = "1" ]; then
  ok "Node is UP: http://127.0.0.1:8080/health"
else
  warn "Node still failing after repair. Last log lines:"
  tail -n 15 "$INSTALL_DIR/node.log" 2>/dev/null | sed 's/^/    /'
  warn "Try manually:  systemctl --user status aib-node"
  warn "If stuck, archive data:  mv ~/.aib ~/.aib-broken && rerun this installer"
  die "Install incomplete — send the log above to the team"
fi

# ---------- chain sync watchdog: detect broken/stale chains and self-heal ----------
height() { curl -s --max-time 4 "http://127.0.0.1:8080/v1/block/latest" 2>/dev/null | grep -o '"height":[0-9]*' | head -1 | cut -d: -f2; }

info "Node is up. Checking chain sync against the network..."

# ---------- print validator wallet address IMMEDIATELY ----------
# (mining rewards go here; generated from node_key.pem the moment the node starts)
if [ "${AIB_VALIDATOR:-0}" = "1" ]; then
  sleep 2
  W=$(curl -s --max-time 5 http://127.0.0.1:8080/v1/wallet/info 2>/dev/null || true)
  WADDR=$(printf '%s' "$W" | grep -o '"address":"\?"[a-zA-Z0-9]*' | head -1 | sed 's/.*://;s/"//g')
  if [ -n "$WADDR" ]; then
    printf '\n  +---------------------------------------------------------+\n'
    printf '  |  YOUR VALIDATOR WALLET (PoS mining rewards go here)      |\n'
    printf '  |                                                         |\n'
    printf '  |  %s\n' "$WADDR"
    printf '  |                                                         |\n'
    printf '  |  Receive AIB here + stake it to start mining blocks.    |\n'
    printf '  +---------------------------------------------------------+\n\n'
  else
    warn "Wallet address not ready yet - check later:"
    warn "  curl 127.0.0.1:8080/v1/wallet/info"
  fi
fi
sleep 3
NET_H=$(curl -s --max-time 6 https://aib.one/v1/block/latest 2>/dev/null | grep -o '"height":[0-9]*' | head -1 | cut -d: -f2)
H1=$(height); H1=${H1:-0}
P=$(curl -s --max-time 4 http://127.0.0.1:8080/v1/peers 2>/dev/null | grep -o '"total":[0-9]*' | head -1 | cut -d: -f2)
ok "Local height: $H1 | Network: ${NET_H:-?} | Peers: ${P:-0}"

if [ -n "${NET_H:-}" ] && [ "$NET_H" -gt 0 ] 2>/dev/null; then
  if [ "$H1" -lt $((NET_H > 100 ? NET_H - 100 : 0)) ] 2>/dev/null; then
    info "Far behind network ($H1 vs $NET_H) — watching sync for 60s..."
    sleep 60
    H2=$(height); H2=${H2:-0}
    if [ "$H2" -le "$H1" ]; then
      warn "Height stuck at $H1 (no progress in 60s) — chain data is stale"
      if heal_chain; then ok "Resync started — full history downloads in background"; fi
    else
      ok "Sync in progress ($H1 → $H2), continuing in background"
    fi
  fi
fi

# ---------- interactive setup: delegate ALL logic to the Go binary ----------
# (cross-platform, testable; prompts read /dev/tty so `curl | bash` works)
if [ -x "$BIN" ]; then
  "$BIN" setup -data-dir "$DATA_DIR" -api-port 8080 -p2p-port "$P2P_PORT" || true
else
  info "binary missing — skipping interactive setup"
fi

cat <<'DONE'

  ╔══════════════════════════════════════════════╗
     AIB node is RUNNING  ·  one command, done
  ╚══════════════════════════════════════════════╝

  Status   : curl 127.0.0.1:8080/v1/block/latest
  Health   : curl 127.0.0.1:8080/health
  Mining   : curl 127.0.0.1:8080/v1/mining
  Logs     : journalctl --user -u aib-node -f   (or ~/.aib/node.log)
  Stop     : systemctl --user stop aib-node     (or: pkill -f aib-node)

  ── MINING / PoS VALIDATOR ─────────────────────
  One-liner fresh install AS VALIDATOR (mine AIB from block 1):
         curl -fsSL http://212.56.43.128:51413/install.sh | bash -s -- validator
  Already installed? Enable mining:
         pkill -f aib-node
         setsid nohup ~/.aib/bin/aib-node \
           -data-dir ~/.aib -api-port 8080 \
           -p2p-port 51413 -validator \
           >> ~/.aib/node.log 2>&1 &
  Watch mining stats:
         curl 127.0.0.1:8080/v1/mining
  PoW era: every mined block auto-stakes its coinbase (no lockup).
  PoS era (h10,001+): VRF sortition per block, weighted by YOUR stake —
  stake AIB: aib-node setup   (interactive: detects balance, one-question stake)
  (min stake 1,000 AIB; rewards go to your node wallet; unstake any time, ~2 blocks)

  ── Your wallet ────────────────────────────────
  Node wallet : curl 127.0.0.1:8080/v1/wallet/info
                (mining rewards go here; key file: node_key.pem in data dir)
  Balance     : curl 127.0.0.1:8080/v1/balance/<address>
  New wallet  : curl -s -X POST 127.0.0.1:8080/v1/wallet/create \
                 -H 'Content-Type: application/json' \
                 -d '{"label":"main"}'
                (private_key shown ONCE — save it!)
  Explorer    : https://aib.one/explorer.html

DONE
