#!/bin/bash
#
# AIB 2.0 部署脚本测试工具
# 用法: ./test-deploy.sh
#
# 功能: 验证所有部署脚本的语法和基本功能
#

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# 颜色输出
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
echo "  AIB 2.0 部署脚本测试"
echo "========================================="
echo

# ========== 测试 1: 文件存在性 ==========
log_info "测试 1: 检查文件存在性..."

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
        log_success "${script} 存在"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_error "${script} 不存在"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
done

for template in "${templates[@]}"; do
    if [[ -f "${template}" ]]; then
        log_success "${template} 存在"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_error "${template} 不存在"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
done

# ========== 测试 2: 执行权限 ==========
echo
log_info "测试 2: 检查执行权限..."

for script in "${scripts[@]}"; do
    if [[ -x "${script}" ]]; then
        log_success "${script} 可执行"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_error "${script} 不可执行"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
done

# ========== 测试 3: 语法检查 ==========
echo
log_info "测试 3: 检查脚本语法..."

for script in "${scripts[@]}"; do
    if bash -n "${script}" 2>/dev/null; then
        log_success "${script} 语法正确"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_error "${script} 语法错误"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
done

# ========== 测试 4: 帮助信息 ==========
echo
log_info "测试 4: 检查帮助信息..."

for script in "${scripts[@]}"; do
    if "${script}" -h &>/dev/null || "${script}" --help &>/dev/null; then
        log_success "${script} 帮助信息可用"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_warning "${script} 帮助信息测试跳过"
    fi
done

# ========== 测试 5: 依赖检查 ==========
echo
log_info "测试 5: 检查依赖命令..."

dependencies=("bash" "curl" "jq" "sha256sum" "systemctl" "openssl" "nc")

for cmd in "${dependencies[@]}"; do
    if command -v "${cmd}" &>/dev/null; then
        log_success "${cmd} 已安装"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_warning "${cmd} 未安装（可选）"
    fi
done

# ========== 测试 6: 目录结构 ==========
echo
log_info "测试 6: 检查目录结构..."

dirs=(
    "${SCRIPT_DIR}/backups"
    "${SCRIPT_DIR}/logs"
    "${SCRIPT_DIR}/templates"
)

for dir in "${dirs[@]}"; do
    if [[ -d "${dir}" ]]; then
        log_success "${dir} 目录存在"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        log_error "${dir} 目录不存在"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
done

# ========== 结果摘要 ==========
echo
echo "========================================="
echo "  测试结果"
echo "========================================="
echo
echo "通过: ${PASS_COUNT}"
echo "失败: ${FAIL_COUNT}"
echo

if [[ ${FAIL_COUNT} -eq 0 ]]; then
    echo -e "${GREEN}所有测试通过!${NC}"
    exit 0
else
    echo -e "${RED}部分测试失败${NC}"
    exit 1
fi
