#!/bin/bash

# AIB 2.0 ZKML 3-Node Testnet Deployment Script
# This script deploys a 3-node testnet for ZKML consensus testing

set -e

echo "======================================"
echo "AIB 2.0 ZKML 3-Node Testnet Deployer"
echo "======================================"
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
TESTNET_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR="$TESTNET_DIR/config"
DATA_DIR="$TESTNET_DIR/data"
LOG_DIR="$TESTNET_DIR/logs"

# Check if Ollama is running
check_ollama() {
    echo -e "${BLUE}Checking Ollama service...${NC}"
    if curl -s http://localhost:11434/api/tags > /dev/null 2>&1; then
        echo -e "${GREEN}✓ Ollama is running${NC}"

        # Check if llama2 model is available
        if curl -s http://localhost:11434/api/tags | grep -q "llama2"; then
            echo -e "${GREEN}✓ llama2 model is available${NC}"
        else
            echo -e "${YELLOW}⚠ llama2 model not found. Pulling...${NC}"
            echo -e "${YELLOW}Please run: ollama pull llama2${NC}"
            exit 1
        fi
    else
        echo -e "${RED}✗ Ollama is not running${NC}"
        echo -e "${YELLOW}Please start Ollama first: ollama serve${NC}"
        exit 1
    fi
}

# Create necessary directories
setup_directories() {
    echo -e "${BLUE}Setting up directories...${NC}"

    mkdir -p "$DATA_DIR"/node{1,2,3}
    mkdir -p "$LOG_DIR"

    echo -e "${GREEN}✓ Directories created${NC}"
}

# Build aib-miner
build_miner() {
    echo -e "${BLUE}Building aib-miner...${NC}"

    cd .
    if /usr/local/go/bin/go build -o "$TESTNET_DIR/aib-miner" ./cmd/aib-miner/; then
        echo -e "${GREEN}✓ aib-miner built successfully${NC}"
    else
        echo -e "${RED}✗ Failed to build aib-miner${NC}"
        exit 1
    fi
}

# Start a node
start_node() {
    local node_id=$1
    local config_file="$CONFIG_DIR/node$node_id.json"
    local log_file="$LOG_DIR/node$node_id.log"

    echo -e "${BLUE}Starting node $node_id...${NC}"

    cd "$TESTNET_DIR"

    # Start node in background
    ./aib-miner start --config "$config_file" > "$log_file" 2>&1 &
    local pid=$!

    # Store PID
    echo $pid > "$LOG_DIR/node$node_id.pid"

    # Wait a bit and check if it's running
    sleep 2

    if kill -0 $pid 2>/dev/null; then
        echo -e "${GREEN}✓ Node $node_id started (PID: $pid)${NC}"
        echo -e "${YELLOW}  Log: $log_file${NC}"
    else
        echo -e "${RED}✗ Node $node_id failed to start${NC}"
        echo -e "${YELLOW}  Check log: $log_file${NC}"
        return 1
    fi
}

# Check node status
check_node() {
    local node_id=$1
    local pid_file="$LOG_DIR/node$node_id.pid"

    if [ -f "$pid_file" ]; then
        local pid=$(cat "$pid_file")
        if kill -0 $pid 2>/dev/null; then
            echo -e "${GREEN}✓ Node $node_id is running (PID: $pid)${NC}"
            return 0
        else
            echo -e "${RED}✗ Node $node_id is not running${NC}"
            return 1
        fi
    else
        echo -e "${YELLOW}⚠ Node $node_id PID file not found${NC}"
        return 1
    fi
}

# Stop a node
stop_node() {
    local node_id=$1
    local pid_file="$LOG_DIR/node$node_id.pid"

    if [ -f "$pid_file" ]; then
        local pid=$(cat "$pid_file")
        if kill -0 $pid 2>/dev/null; then
            echo -e "${BLUE}Stopping node $node_id (PID: $pid)...${NC}"
            kill $pid

            # Wait for graceful shutdown
            local attempts=0
            while kill -0 $pid 2>/dev/null && [ $attempts -lt 10 ]; do
                sleep 1
                ((attempts++))
            done

            if kill -0 $pid 2>/dev/null; then
                echo -e "${YELLOW}Force killing node $node_id...${NC}"
                kill -9 $pid
            fi

            rm -f "$pid_file"
            echo -e "${GREEN}✓ Node $node_id stopped${NC}"
        else
            echo -e "${YELLOW}⚠ Node $node_id was not running${NC}"
            rm -f "$pid_file"
        fi
    else
        echo -e "${YELLOW}⚠ Node $node_id PID file not found${NC}"
    fi
}

# Show usage
usage() {
    echo "Usage: $0 {start|stop|status|restart}"
    echo ""
    echo "Commands:"
    echo "  start   - Start the 3-node testnet"
    echo "  stop    - Stop the 3-node testnet"
    echo "  status  - Check status of all nodes"
    echo "  restart - Restart the 3-node testnet"
    echo ""
    echo "Examples:"
    echo "  $0 start"
    echo "  $0 status"
    echo "  $0 stop"
}

# Main command handler
case "${1:-}" in
    start)
        echo -e "${GREEN}Starting AIB 2.0 ZKML 3-Node Testnet${NC}"
        echo ""
        check_ollama
        echo ""
        setup_directories
        echo ""
        build_miner
        echo ""

        # Start all nodes
        for i in 1 2 3; do
            start_node $i
            echo ""
        done

        echo -e "${GREEN}======================================${NC}"
        echo -e "${GREEN}Testnet started successfully!${NC}"
        echo -e "${GREEN}======================================${NC}"
        echo ""
        echo -e "${YELLOW}Next steps:${NC}"
        echo "1. Check node status: $0 status"
        echo "2. View logs: tail -f $LOG_DIR/node*.log"
        echo "3. Run tests: cd $TESTNET_DIR/../.. && go test ./zkml/testnet/..."
        ;;

    stop)
        echo -e "${BLUE}Stopping AIB 2.0 ZKML 3-Node Testnet${NC}"
        echo ""

        for i in 1 2 3; do
            stop_node $i
            echo ""
        done

        echo -e "${GREEN}======================================${NC}"
        echo -e "${GREEN}Testnet stopped${NC}"
        echo -e "${GREEN}======================================${NC}"
        ;;

    status)
        echo -e "${BLUE}Checking AIB 2.0 ZKML 3-Node Testnet Status${NC}"
        echo ""

        # Check Ollama
        if curl -s http://localhost:11434/api/tags > /dev/null 2>&1; then
            echo -e "${GREEN}✓ Ollama is running${NC}"
        else
            echo -e "${RED}✗ Ollama is not running${NC}"
        fi
        echo ""

        # Check nodes
        local running_nodes=0
        for i in 1 2 3; do
            if check_node $i; then
                ((running_nodes++))
            fi
            echo ""
        done

        echo -e "${BLUE}Summary: $running_nodes/3 nodes running${NC}"
        ;;

    restart)
        $0 stop
        sleep 2
        $0 start
        ;;

    *)
        usage
        exit 1
        ;;
esac