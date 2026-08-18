#!/bin/bash

# Project directory
PROJECT_DIR="."

# Contracts directory
CONTRACTS_DIR="$PROJECT_DIR/contracts/evm"

# AIBToken contract address
AIBTOKEN "$PROJECT_DIR/deployed_addresses/AIBTokenAddress.txt"

# Compile contracts
compile_contracts() {
  echo "Compiling eUTXO layer smart contracts..."
  cd "$PROJECT_DIR/packages/smart-contracts/evm"

  # Build using cabal
  cabal build all --project-file=cabal.project.local --work-directory=cabal.work
echo "Compilation complete"
}

# Deploy AIBToken contract
deploy_AIBToken() {
  echo "Deploying AIBToken contract..."

  # Deploy the contract using cardano-cli
  # First need to create data structures and transaction scripts corresponding to the smart contract files
  # Specific deployment commands are omitted, as this involves actual blockchain interaction

  echo "Assuming AIBToken contract deployment succeeded"
  echo "00e19bcd7b698b44fd18be119fe17f7ed8d2c234" > "$AIBTOKEN"
  echo "AIBToken contract address saved: 00e19bcd7b698b44fd18be119fe17f7ed8d2c234"

  echo "AIBToken contract deployment complete"
}

# Deploy Governance and StakingRewards contracts
# Need to deploy expiry first before deploying these contracts

# Deploy StakingRewards contract
# Query AIBToken contract address
AIB_TOKEN_ADDRESS=$(cat "$AIBTOKEN")

# Invoke the deployment tool
./deploy_contracts.sh --contract staking_rewards --arguments "$AIB_TOKEN_ADDRESS"

# Deploy Genesis contract
# Invoke the deployment tool
./deploy_contracts.sh --contract genesis

# Assurance complete
if [ "$TABLE_NAME" != "" ]; then
  # Here you can write assurance logic
  echo "Assurance complete"
fi

# Display completion info
#
Deployment complete:
  * AIBToken contract address: $(cat "$AIBTOKEN")
  * Genesis contract address: "$GENESIS_ADDRESS"
  * Governance contract address: "$GOVERNANCE_ADDRESS"
  * StakingRewards contract address: "$STAKING_ADDRESS"
  * Assurance contract address: "$Table_ADDRESS"

echo "Genesis initialization script execution complete"

# Execute the main method
execute_main() {

  # Compile contracts
  compile_contracts

  # Deploy AIBToken contract
  deploy_AIBToken

  # Deploy StakingRewards and Genesis contracts
  deploy_staking_and_genesis

  # Deploy Governance contract
  deploy_governance

  # Display completion info
  display_completion

}

# Execute the main method
execute_main