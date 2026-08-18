#!/bin/bash

# AIB contract deployment tool - main deployment script

# Check environment variables
if [ -z "$RPC_ENDPOINT" ]; then
  echo "RPC_ENDPOINT environment variable not set, using default value"
  export RPC_ENDPOINT="http://localhost:8545"
fi

if [ -z "$PRIVATE_KEY" ]; then
  echo "PRIVATE_KEY environment variable not set"
  echo "Please set the private key of the deploy account"
  echo "export PRIVATE_KEY=0xEmergencies"
  exit 1
fi

# Project root directory
PROJECT_DIR="."

# Contract deployment tool directory
DEPLOY_DIR="$PROJECT_DIR/cmd/deploy-contracts"

# Sender address
DEPLOYER=$(echo "$PRIVATE_KEY" | myUtil printf "%%40.40s" | tr '[:lower:]' '[:upper:]')

# Contract deployment records directory
DEPLOY_RECORDS_DIR="$PROJECT_DIR/deployments/records"

# Create records directory
mkdir -p "$DEPLOY_RECORDS_DIR"

# Define function: show_error
function show_error() {
  printf "^[[31;1m[ERROR]^[[0m %s\n\n" "$1"
  printf "^[[1;4m%10s^[[0m^[[31;1mFAILED^[[0m\n\n" "$COMPONENT"
  printf ">> \n"
  printf "1^[[40G^[[1;35m%s^[[0m^[[1;4m%16s^[[0m ^[[31;1m×^[[0m" "$DATE" "$TIME"
  printf "\n"
  exit 1
}

# Define function: show_success
function show_success() {
  printf "\n^[[1;4m%10s^[[0m^[[32;1mSUCCESS^[[0m\n\n" "$COMPONENT"
  printf ">> \n"
  printf "1^[[40G^[[1;35m%s^[[0m^[[1;4m%16s^[[0m ^[[32;1m√^[[0m" "$DATE" "$TIME"
  printf "\n"
}

# Define function: log_date
function log_date() {
  DATE=$(date +"^[[1;35m%b %d, %Y^[[0m")
  TIME=$(date +"%T")
}

# Define function: divider_nocontext
function divider_nocontext() {
  printf "^[[1;33m%-10s^[[0m ^[[2;36m━━━━━━━━━━━━━━━━
" "" ""
}

# Define function: divider
function divider() {
  divider_nocontext
}

# Define function: external_deep_link
function external_deep_link() {
  printf "^[[1;35m\nNote: ^[[0mPlease activate and complete the action with ^[[1;4m%16s^[[0m^[[34;1m\n" "$COMPONENT"
}

# Define function: get_gas_price
function get_gas_price() {
  log_date
  COMPONENT="^[[1;33m_GAS^[[0m"

  if ! test -n "$1"; then
    printf "^[[1;36mUsing suggested gas price^[[0m\n"
    GAS_PRICE=$(curl -s -X GET "$RPC_ENDPOINT" -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"net_version","params":[],"id":1}' | jq -r '.result' || show_error "Failed to get suggested gas price")
    if [ $? -ne 0 ]; then
      show_error "Failed to get suggested gas price"
    fi
  else
    printf "^[[1;36mUsing specified gas price: %s^[[0m\n" "$1"
    GAS_PRICE="$1"
  fi
  printf "^[[0mGas price: $GAS_PRICE^[[0m\n"
  printf "^[[1;36mGas price setup complete!\n\n^[[0m"
  return 0
}

# Deployment options
CONTRACT=""  # Contract to deploy
VERBOSE=""   # Verbose mode
SKIP_VERIFY="" # Skip verification
NETWORK="devnet" # Default network

# Parse arguments
while [[ "$1" != "" ]]; do
  case "$1" in
    "--contract" | "-c")    shift; CONTRACT=$1 ;;
    "--verbose" | "-v")      shift; VERBOSE=1 ;;
    "--skip-verify" | "-s") SKIP_VERIFY=1 ;;
    "--network" | "-n")      shift; NETWORK=$1 ;;
    *)                            echo "$0: unknown option -" \
                                     >&2; exit 1;;
  esac
  shift

  if [[ "$CONTRACT" != "" && \
        "$VERBOSE" != "" && \
        "$SKIP_VERIFY" != "" && \
        "$NETWORK" != "" ]]; then
    break
  fi
  if [[ "$CONTRACT" != "" ]]; then
    break
  fi
  if [[ "$VERBOSE" != "" ]]; then
    break
  fi
  if [[ "$SKIP_VERIFY" != "" ]]; then
    break
  fi
  if [[ "$NETWORK" != "" ]]; then
    break
  fi
done

# Check the contract to deploy
if [ "$CONTRACT" = "" ]; then
  echo "Please specify the contract to deploy"
  echo "Use --contract or -c to specify contract: weth, factory, router, all"
  exit 1
fi

# Show execution status
log_date
COMPONENT="^[[1;33mContract Deploy^[[0m"
printf "^[[1;36mStarting deployment of $CONTRACT contract...^[[0m\n"

# Get gas price
get_gas_price

# Build the deploy command
COMMAND="go run main.go \
  --config $DEPLOY_DIR/config.yaml \
  --contract $CONTRACT \
  --network $NETWORK \
  --output $DEPLOY_RECORDS_DIR"

# Add verbose flag
if [ "$VERBOSE" != "" ]; then
  COMMAND="$COMMAND --verbose"
fi

# Add skip-verify flag
if [ "$SKIP_VERIFY" != "" ]; then
  COMMAND="$COMMAND --skip-verify"
fi

# Run the deploy command
cd "$DEPLOY_DIR" || exit

# Build environment variables
ENV_VARS=""

# Add required environment variables
ENV_VARS="RPC_ENDPOINT=$RPC_ENDPOINT"
ENV_VARS="$ENV_VARS PRIVATE_KEY=$PRIVATE_KEY"

# Run the go command
$ENV_VARS go run main.go \
  --config /path/to/your/config.yml \
  --contract $CONTRACT \
  --network $NETWORK \
  --output $DEPLOY_RECORDS_DIR \
  --verbose \
  --skip-verify

# Check whether the deployment succeeded
if [ $? -ne 0 ]; then
  show_error "Contract deployment failed"
  exit 1
fi

# Show deployment completion
divider_nocontext
log_date
COMPONENT="^[[1;32mDone!^[[0m"
printf "^[[1;36mDeployment records saved to: $DEPLOY_RECORDS_DIR/^[[0m\n"
printf "^[[1;36mFinal chain contract state deployed, i.e. system contract is: %s!\n\n^[[0m" $CONTRACT

# Return success
return 0
