#!/bin/bash
#
# AIB 2.0 deploy script test tool
# Usage: ./test-deploy.sh
#
# Features: validate syntax and basic functionality of all deploy scripts
#

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# colored output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASS_COUNT=0
FAIL_COUNT=0

log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_success() { echo -e "${GREEN}[PASS]${NC} $*"; }
log_error() { echo -e "${RED}[FAIL]${NC} $*" >&2; }
log_warning() { echo -e "${YELLOW}[WARN]${NC} $*"; }

echo "========================================="
echo "  AIB 2.0 deploy script tests"
echo "========================================="
echo

# ========== test 1: file existence ==========
log_info "test 1: check file existence..."

scripts=(
    "${SCRIPT_DIR}/upgrade.sh"
    "${SCRIPT_DIR}/mainnet-init.sh"
    "${SCRIPT_DIR}/validate-upgrade.sh"
    "${SCRIPT_DIR}/rollback.sh"
)

templates=(
    "${SCRIPT_DIR}/templates/config.toml"
    "${SCRIPT_DIR}/templates/aib2-mainnet.service"
)

for script in "${scripts[@]}"; do
    if [[ -f "${script}" ]]; then
        log_success "${script} exists"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_error "${script} does not exist"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
done

for template in "${templates[@]}"; do
    if [[ -f "${template}" ]]; then
        log_success "${template} exists"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_error "${template} does not exist"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
done

# ========== test 2: executable permission ==========
echo
log_info "test 2: check executable permissions..."

for script in "${scripts[@]}"; do
    if [[ -x "${script}" ]]; then
        log_success "${script} executable"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_error "${script} not executable"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
done

# ========== test 3: syntax check ==========
echo
log_info "test 3: check script syntax..."

for script in "${scripts[@]}"; do
    if bash -n "${script}" 2>/dev/null; then
        log_success "${script} syntax OK"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_error "${script} syntax error"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
done

# ========== test 4: help information ==========
echo
log_info "test 4: check help output..."

for script in "${scripts[@]}"; do
    if "${script}" -h &>/dev/null || "${script}" --help &>/dev/null; then
        log_success "${script} help output available"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_warning "${script} help output test skipped"
    fi
done

# ========== test 5: dependency check ==========
echo
log_info "test 5: check dependency commands..."

dependencies=("bash" "curl" "jq" "sha256sum" "systemctl" "openssl" "nc")

for cmd in "${dependencies[@]}"; do
    if command -v "${cmd}" &>/dev/null; then
        log_success "${cmd} installed"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_warning "${cmd} not installed (optional）"
    fi
done

# ========== test 6: directory structure ==========
echo
log_info "test 6: check directory structure..."

dirs=(
    "${SCRIPT_DIR}/backups"
    "${SCRIPT_DIR}/logs"
    "${SCRIPT_DIR}/templates"
)

for dir in "${dirs[@]}"; do
    if [[ -d "${dir}" ]]; then
        log_success "${dir} directory exists"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_error "${dir} directory does not exist"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
done

# ========== result summary ==========
echo
echo "========================================="
echo "  test results"
echo "========================================="
echo
echo "passed: ${PASS_COUNT}"
echo "failed: ${FAIL_COUNT}"
echo

if [[ ${FAIL_COUNT} -eq 0 ]]; then
    echo -e "${GREEN}all tests passed!${NC}"
    exit 0
else
    echo -e "${RED}some tests failed${NC}"
    exit 1
fi
