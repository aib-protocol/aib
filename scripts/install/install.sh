#!/bin/bash
# AIB 2.0 Node Installation Script
# Usage: curl -sL https://www.aib.one/install.sh | bash

set -e

echo "Installing AIB 2.0 Node..."

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    echo "Docker not found. Installing Docker..."
    curl -fsSL https://get.docker.com | sh
    sudo usermod -aG docker $USER
    echo "Docker installed. Please log out and log back in, then run this script again."
    exit 1
fi

# Pull and run AIB 2.0 node
echo "Starting AIB 2.0 node..."
docker run -d \
    --name aib-node \
    -p 8080:8080 \
    -p 30303:30303 \
    -v aib-data:/data \
    aibprotocol/aib2-node:latest \
    --testnet

echo "AIB 2.0 node started!"
echo "API: http://localhost:8080"
echo "Check status: curl http://localhost:8080/status"
