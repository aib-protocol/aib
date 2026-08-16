#!/bin/bash

# AIB 2.0 ZKML Testnet Stop Script

TESTNET_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_DIR="$TESTNET_DIR/logs"

echo "Stopping AIB 2.0 ZKML Testnet..."

for i in 1 2 3; do
    pid_file="$LOG_DIR/node$i.pid"
    if [ -f "$pid_file" ]; then
        pid=$(cat "$pid_file")
        if kill -0 $pid 2>/dev/null; then
            echo "Stopping node $i (PID: $pid)..."
            kill $pid
            rm -f "$pid_file"
        fi
    fi
done

# Kill any remaining aib-miner processes
pkill -f "aib-miner start" 2>/dev/null || true

echo "Testnet stopped."
