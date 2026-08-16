#!/bin/bash
#
# AIB 2.0 节点升级脚本
# 用法: ./upgrade.sh [选项]
# 选项:
#   -v, --version VERSION    指定升级版本 (默认: latest)
#   -b, --backup-dir DIR     备份目录 (默认: ./backups)
#   -s, --skip-backup        跳过备份步骤 (不推荐)
#   -f, --force              强制升级，跳过确认
#   -d, --dry-run            模拟运行，不执行实际升级
#   -h, --help               显示帮助信息
#
# 功能:
#   1. 备份当前节点数据和配置
#   2. 下载并验证新版本二进制
#   3. 更新配置文件
#   4. 重启节点服务
#   5. 验证升级成功
#
# 作者: AIB Protocol Team
# 更新: 2026-03
#

set -e  # 遇到错误立即退出
set -u  # 使用未定义变量时退出
set -o pipefail  # 管道命令失败时退出

# ========== 脚本元数据 ==========
SCRIPT_NAME="$(basename "$0")"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
VERSION="2.0.0"

# ========== 颜色输出 ==========
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ========== 日志函数 ==========
log_info() {
    echo -e "${BLUE}[INFO]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $*"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $*" >&2
}

# ========== 默认配置 ==========
DEFAULT_VERSION="latest"
DEFAULT_BACKUP_DIR="${SCRIPT_DIR}/backups"
DEFAULT_DATA_DIR="${PROJECT_DIR}/data"
DEFAULT_CONFIG_DIR="${PROJECT_DIR}/config"
BINARY_DIR="${PROJECT_DIR}/bin"
LOG_DIR="${SCRIPT_DIR}/logs"
DOWNLOAD_CACHE="/tmp/aib-upgrade"

# 运行时配置
VERSION="${DEFAULT_VERSION}"
BACKUP_DIR="${DEFAULT_BACKUP_DIR}"
DATA_DIR="${DEFAULT_DATA_DIR}"
CONFIG_DIR="${DEFAULT_CONFIG_DIR}"
SKIP_BACKUP=false
FORCE=false
DRY_RUN=false
SERVICE_NAME="aib2-mainnet"

# ========== 帮助信息 ==========
show_help() {
    cat << EOF
AIB 2.0 节点升级脚本 v${VERSION}

用法: ${SCRIPT_NAME} [选项]

选项:
  -v, --version VERSION    指定升级版本 (默认: ${DEFAULT_VERSION})
  -b, --backup-dir DIR     备份目录 (默认: ${DEFAULT_BACKUP_DIR})
  -d, --data-dir DIR       数据目录 (默认: ${DEFAULT_DATA_DIR})
  -s, --skip-backup        跳过备份步骤 (不推荐)
  -f, --force              强制升级，跳过确认
  -n, --dry-run            模拟运行，不执行实际升级
  -h, --help               显示此帮助信息

示例:
  ${SCRIPT_NAME}                              # 升级到最新版本
  ${SCRIPT_NAME} -v 2.1.0                     # 升级到指定版本
  ${SCRIPT_NAME} -n                           # 模拟运行
  ${SCRIPT_NAME} -b /custom/backup/path       # 使用自定义备份目录

升级步骤:
  1. 检查当前节点状态
  2. 备份数据和配置
  3. 下载新版本二进制
  4. 验证二进制完整性
  5. 更新配置文件
  6. 停止当前服务
  7. 部署新版本
  8. 启动服务
  9. 验证升级成功

更多信息请访问: https://docs.aib.network/upgrade
EOF
    exit 0
}

# ========== 参数解析 ==========
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -v|--version)
                VERSION="$2"
                shift 2
                ;;
            -b|--backup-dir)
                BACKUP_DIR="$2"
                shift 2
                ;;
            -d|--data-dir)
                DATA_DIR="$2"
                shift 2
                ;;
            -s|--skip-backup)
                SKIP_BACKUP=true
                shift
                ;;
            -f|--force)
                FORCE=true
                shift
                ;;
            -n|--dry-run)
                DRY_RUN=true
                shift
                ;;
            -h|--help)
                show_help
                ;;
            *)
                log_error "未知参数: $1"
                show_help
                ;;
        esac
    done
}

# ========== 环境检查 ==========
check_environment() {
    log_info "检查运行环境..."

    # 检查是否为 root 用户
    if [[ $EUID -eq 0 ]]; then
        log_warning "不建议使用 root 用户运行此脚本"
    fi

    # 检查必要的命令
    local required_commands=("curl" "jq" "sha256sum" "systemctl")
    for cmd in "${required_commands[@]}"; do
        if ! command -v "$cmd" &> /dev/null; then
            log_error "缺少必要命令: $cmd"
            return 1
        fi
    done

    # 检查目录权限
    if [[ ! -w "${PROJECT_DIR}" ]]; then
        log_error "没有项目目录写权限: ${PROJECT_DIR}"
        return 1
    fi

    # 创建必要的目录
    mkdir -p "${BACKUP_DIR}" "${LOG_DIR}" "${DOWNLOAD_CACHE}"

    log_success "环境检查通过"
    return 0
}

# ========== 获取当前版本 ==========
get_current_version() {
    if [[ -f "${BINARY_DIR}/aib-node" ]]; then
        "${BINARY_DIR}/aib-node" --version 2>/dev/null || echo "unknown"
    else
        echo "not_installed"
    fi
}

# ========== 获取最新版本 ==========
get_latest_version() {
    log_info "检查最新可用版本..."

    local api_url="https://api.github.com/repos/aib-protocol/aib/releases/latest"
    local version

    if command -v curl &> /dev/null; then
        version=$(curl -s "${api_url}" | jq -r '.tag_name' | sed 's/^v//')
    else
        version="2.0.0"
    fi

    if [[ -z "${version}" || "${version}" == "null" ]]; then
        version="2.0.0"
    fi

    echo "${version}"
}

# ========== 检查节点状态 ==========
check_node_status() {
    log_info "检查节点状态..."

    local is_running=false

    if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
        is_running=true
        log_success "节点服务正在运行"
    elif pgrep -f "aib-node" > /dev/null; then
        is_running=true
        log_warning "检测到 aib-node 进程，但未通过 systemd 管理"
    else
        log_warning "节点服务未运行"
    fi

    # 检查节点健康状态
    if [[ "${is_running}" == "true" ]]; then
        local health_url="http://localhost:51200/health"
        if command -v curl &> /dev/null; then
            if curl -sf "${health_url}" > /dev/null 2>&1; then
                log_success "节点健康检查通过"
            else
                log_warning "节点健康检查失败，可能存在问题"
            fi
        fi
    fi

    return 0
}

# ========== 备份数据 ==========
backup_data() {
    if [[ "${SKIP_BACKUP}" == "true" ]]; then
        log_warning "已跳过备份步骤 (不推荐)"
        return 0
    fi

    log_info "开始备份数据..."

    local timestamp=$(date '+%Y%m%d_%H%M%S')
    local backup_path="${BACKUP_DIR}/upgrade_${timestamp}"
    local current_version=$(get_current_version)

    mkdir -p "${backup_path}"

    # 备份数据目录
    if [[ -d "${DATA_DIR}" ]]; then
        log_info "备份数据目录..."
        tar -czf "${backup_path}/data.tar.gz" -C "${DATA_DIR}" . || {
            log_error "数据目录备份失败"
            return 1
        }
    fi

    # 备份配置目录
    if [[ -d "${CONFIG_DIR}" ]]; then
        log_info "备份配置目录..."
        tar -czf "${backup_path}/config.tar.gz" -C "${CONFIG_DIR}" . || {
            log_error "配置目录备份失败"
            return 1
        }
    fi

    # 备份当前二进制
    if [[ -f "${BINARY_DIR}/aib-node" ]]; then
        log_info "备份当前二进制..."
        cp "${BINARY_DIR}/aib-node" "${backup_path}/aib-node.backup" || {
            log_error "二进制备份失败"
            return 1
        }
    fi

    # 记录版本信息
    cat > "${backup_path}/backup_info.txt" << EOF
备份时间: ${timestamp}
当前版本: ${current_version}
目标版本: ${VERSION}
数据目录: ${DATA_DIR}
配置目录: ${CONFIG_DIR}
EOF

    # 创建备份校验和
    cd "${backup_path}"
    sha256sum *.tar.gz *.backup > checksums.txt 2>/dev/null || true
    cd - > /dev/null

    log_success "备份完成: ${backup_path}"
    echo "${backup_path}" > "${BACKUP_DIR}/.last_backup"

    return 0
}

# ========== 下载新版本 ==========
download_version() {
    local target_version="$1"
    log_info "下载版本 ${target_version}..."

    local download_url="https://github.com/aib-protocol/aib/releases/download/v${target_version}/aib-node-linux-amd64"
    local checksum_url="https://github.com/aib-protocol/aib/releases/download/v${target_version}/checksums.txt"

    if [[ "${target_version}" == "latest" ]]; then
        target_version=$(get_latest_version)
        download_url="https://github.com/aib-protocol/aib/releases/latest/download/aib-node-linux-amd64"
    fi

    log_info "下载地址: ${download_url}"

    # 下载二进制
    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY-RUN] 将下载: ${download_url}"
        return 0
    fi

    curl -L -o "${DOWNLOAD_CACHE}/aib-node" "${download_url}" || {
        log_error "下载失败"
        return 1
    }

    # 下载校验和
    curl -L -o "${DOWNLOAD_CACHE}/checksums.txt" "${checksum_url}" || {
        log_warning "校验和文件下载失败，跳过验证"
    }

    # 设置执行权限
    chmod +x "${DOWNLOAD_CACHE}/aib-node"

    log_success "下载完成"
    return 0
}

# ========== 验证二进制 ==========
verify_binary() {
    log_info "验证二进制文件..."

    local downloaded="${DOWNLOAD_CACHE}/aib-node"

    if [[ ! -f "${downloaded}" ]]; then
        log_error "下载的文件不存在"
        return 1
    fi

    # 检查文件大小
    local size=$(stat -f%z "${downloaded}" 2>/dev/null || stat -c%s "${downloaded}" 2>/dev/null)
    if [[ ${size} -lt 1000000 ]]; then
        log_error "文件大小异常: ${size} bytes"
        return 1
    fi

    # 验证校验和（如果可用）
    if [[ -f "${DOWNLOAD_CACHE}/checksums.txt" ]]; then
        local calculated=$(sha256sum "${downloaded}" | awk '{print $1}')
        local expected=$(grep "aib-node-linux-amd64" "${DOWNLOAD_CACHE}/checksums.txt" | awk '{print $1}')

        if [[ -n "${expected}" && "${calculated}" != "${expected}" ]]; then
            log_error "校验和不匹配"
            log_error "期望: ${expected}"
            log_error "实际: ${calculated}"
            return 1
        fi
        log_success "校验和验证通过"
    else
        log_warning "未找到校验和文件，跳过校验和验证"
    fi

    # 检查二进制是否可执行
    if ! "${downloaded}" --version &> /dev/null; then
        log_error "二进制文件无法执行或损坏"
        return 1
    fi

    log_success "二进制验证通过"
    return 0
}

# ========== 更新配置文件 ==========
update_config() {
    log_info "检查配置文件更新..."

    local config_file="${CONFIG_DIR}/config.toml"

    if [[ ! -f "${config_file}" ]]; then
        log_warning "配置文件不存在: ${config_file}"
        return 0
    fi

    # 配置迁移逻辑（根据版本变化）
    # 示例：添加新的配置项
    log_info "配置文件无需更新"

    return 0
}

# ========== 停止服务 ==========
stop_service() {
    log_info "停止节点服务..."

    if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
        if [[ "${DRY_RUN}" == "true" ]]; then
            log_info "[DRY-RUN] 将停止服务: ${SERVICE_NAME}"
            return 0
        fi

        systemctl stop "${SERVICE_NAME}" || {
            log_error "停止服务失败"
            return 1
        }

        # 等待服务完全停止
        local count=0
        while systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; do
            sleep 1
            count=$((count + 1))
            if [[ ${count} -gt 30 ]]; then
                log_error "服务停止超时"
                return 1
            fi
        done

        log_success "服务已停止"
    else
        log_info "服务未在运行"
    fi

    return 0
}

# ========== 部署新版本 ==========
deploy_binary() {
    log_info "部署新版本二进制..."

    local source="${DOWNLOAD_CACHE}/aib-node"
    local target="${BINARY_DIR}/aib-node"

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY-RUN] 将部署: ${source} -> ${target}"
        return 0
    fi

    # 备份旧版本
    if [[ -f "${target}" ]]; then
        mv "${target}" "${target}.old" || {
            log_error "备份旧版本失败"
            return 1
        }
    fi

    # 部署新版本
    cp "${source}" "${target}" || {
        log_error "部署新版本失败"
        # 尝试恢复
        if [[ -f "${target}.old" ]]; then
            mv "${target}.old" "${target}"
        fi
        return 1
    }

    chmod +x "${target}"

    log_success "新版本部署完成"
    return 0
}

# ========== 启动服务 ==========
start_service() {
    log_info "启动节点服务..."

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY-RUN] 将启动服务: ${SERVICE_NAME}"
        return 0
    fi

    systemctl start "${SERVICE_NAME}" || {
        log_error "启动服务失败"
        return 1
    }

    # 等待服务启动
    local count=0
    while ! systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; do
        sleep 1
        count=$((count + 1))
        if [[ ${count} -gt 30 ]]; then
            log_error "服务启动超时"
            return 1
        fi
    done

    log_success "服务已启动"
    return 0
}

# ========== 验证升级 ==========
verify_upgrade() {
    log_info "验证升级结果..."

    sleep 5  # 等待服务完全启动

    # 检查服务状态
    if ! systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
        log_error "服务未运行"
        return 1
    fi

    # 检查版本
    local new_version=$("${BINARY_DIR}/aib-node" --version 2>/dev/null)
    log_success "当前版本: ${new_version}"

    # 检查节点同步状态
    local health_url="http://localhost:51200/health"
    if curl -sf "${health_url}" > /dev/null 2>&1; then
        log_success "节点健康检查通过"
    else
        log_warning "节点健康检查失败，请手动验证"
    fi

    return 0
}

# ========== 清理 ==========
cleanup() {
    log_info "清理临时文件..."

    rm -rf "${DOWNLOAD_CACHE}"

    log_success "清理完成"
}

# ========== 回滚 ==========
rollback() {
    local backup_path="$1"

    log_error "升级失败，开始回滚..."

    if [[ -z "${backup_path}" || ! -d "${backup_path}" ]]; then
        log_error "备份路径无效: ${backup_path}"
        return 1
    fi

    # 恢复二进制
    if [[ -f "${backup_path}/aib-node.backup" ]]; then
        cp "${backup_path}/aib-node.backup" "${BINARY_DIR}/aib-node"
        chmod +x "${BINARY_DIR}/aib-node"
        log_info "已恢复二进制文件"
    fi

    # 恢复数据（可选，需要用户确认）
    log_warning "数据恢复需要手动执行，请使用 rollback.sh 脚本"

    return 0
}

# ========== 主流程 ==========
main() {
    log_info "AIB 2.0 节点升级脚本启动"
    log_info "项目目录: ${PROJECT_DIR}"

    # 解析参数
    parse_args "$@"

    # 环境检查
    check_environment || exit 1

    # 检查节点状态
    check_node_status || exit 1

    # 获取版本信息
    local current_version=$(get_current_version)
    log_info "当前版本: ${current_version}"

    if [[ "${VERSION}" == "latest" ]]; then
        VERSION=$(get_latest_version)
    fi
    log_info "目标版本: ${VERSION}"

    # 确认升级
    if [[ "${FORCE}" != "true" && "${DRY_RUN}" != "true" ]]; then
        echo
        echo -e "${YELLOW}即将从 ${current_version} 升级到 ${VERSION}${NC}"
        read -p "确认继续? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "升级已取消"
            exit 0
        fi
    fi

    # 执行升级流程
    local backup_path=""
    local step=0
    local total_steps=8

    # 1. 备份
    step=$((step + 1))
    log_info "[${step}/${total_steps}] 备份数据..."
    backup_data || {
        log_error "备份失败，升级中止"
        exit 1
    }
    backup_path=$(cat "${BACKUP_DIR}/.last_backup" 2>/dev/null)

    # 2. 下载
    step=$((step + 1))
    log_info "[${step}/${total_steps}] 下载新版本..."
    download_version "${VERSION}" || {
        log_error "下载失败"
        rollback "${backup_path}"
        exit 1
    }

    # 3. 验证
    step=$((step + 1))
    log_info "[${step}/${total_steps}] 验证二进制..."
    verify_binary || {
        log_error "验证失败"
        exit 1
    }

    # 4. 更新配置
    step=$((step + 1))
    log_info "[${step}/${total_steps}] 更新配置..."
    update_config || {
        log_warning "配置更新失败，继续升级"
    }

    # 5. 停止服务
    step=$((step + 1))
    log_info "[${step}/${total_steps}] 停止服务..."
    stop_service || {
        log_error "停止服务失败"
        exit 1
    }

    # 6. 部署
    step=$((step + 1))
    log_info "[${step}/${total_steps}] 部署新版本..."
    deploy_binary || {
        log_error "部署失败，尝试回滚..."
        rollback "${backup_path}"
        exit 1
    }

    # 7. 启动服务
    step=$((step + 1))
    log_info "[${step}/${total_steps}] 启动服务..."
    start_service || {
        log_error "启动失败，尝试回滚..."
        rollback "${backup_path}"
        exit 1
    }

    # 8. 验证
    step=$((step + 1))
    log_info "[${step}/${total_steps}] 验证升级..."
    verify_upgrade || {
        log_warning "升级验证失败，请手动检查节点状态"
    }

    # 清理
    cleanup

    # 完成
    echo
    log_success "====== 升级完成 ======"
    log_success "备份位置: ${backup_path}"
    log_success "如遇问题，请使用 rollback.sh 脚本回滚"

    exit 0
}

# ========== 入口 ==========
main "$@"
