#!/bin/bash
#
# AIB 2.0 紧急回滚脚本
# 用法: ./rollback.sh [选项]
# 选项:
#   -b, --backup BACKUP_PATH    指定备份路径
#   -l, --list                  列出可用备份
#   -s, --service SERVICE       服务名 (默认: aib2-mainnet)
#   -f, --force                 强制回滚，跳过确认
#   -h, --help                  显示帮助信息
#
# 功能:
#   1. 列出可用备份
#   2. 恢复二进制文件
#   3. 恢复配置文件
#   4. 恢复数据目录
#   5. 重启节点服务
#   6. 验证回滚结果
#
# 作者: AIB Protocol Team
# 更新: 2026-03
#

set -e
set -u
set -o pipefail

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
CYAN='\033[0;36m'
NC='\033[0m'

# ========== 日志函数 ==========
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

log_critical() {
    echo -e "${RED}[CRITICAL]${NC} $*" >&2
}

# ========== 配置变量 ==========
BACKUP_DIR="${SCRIPT_DIR}/backups"
SERVICE_NAME="aib2-mainnet"
FORCE_ROLLBACK=false
BINARY_DIR="${PROJECT_DIR}/bin"
DATA_DIR="${PROJECT_DIR}/data"
CONFIG_DIR="${PROJECT_DIR}/config"

# ========== 帮助信息 ==========
show_help() {
    cat << EOF
AIB 2.0 紧急回滚脚本 v${VERSION}

用法: ${SCRIPT_NAME} [选项]

选项:
  -b, --backup BACKUP_PATH    指定备份路径
  -l, --list                  列出可用备份
  -s, --service SERVICE       服务名 (默认: ${SERVICE_NAME})
  -f, --force                 强制回滚，跳过确认
  -h, --help                  显示帮助信息

示例:
  # 列出可用备份
  ${SCRIPT_NAME} -l

  # 回滚到指定备份
  ${SCRIPT_NAME} -b /path/to/backup

  # 强制回滚
  ${SCRIPT_NAME} -b backup_path -f

  # 回滚到最近的备份
  ${SCRIPT_NAME} -b latest

回滚流程:
  1. 停止节点服务
  2. 恢复二进制文件
  3. 恢复数据目录
  4. 恢复配置文件
  5. 启动节点服务
  6. 验证回滚结果

警告:
  - 回滚会停止节点服务
  - 数据恢复是破坏性操作
  - 回滚后需要重新同步
  - 建议在紧急情况下使用
EOF
    exit 0
}

# ========== 参数解析 ==========
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -b|--backup)
                BACKUP_PATH="$2"
                shift 2
                ;;
            -l|--list)
                LIST_BACKUPS=true
                shift
                ;;
            -s|--service)
                SERVICE_NAME="$2"
                shift 2
                ;;
            -f|--force)
                FORCE_ROLLBACK=true
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

# ========== 列出备份 ==========
list_backups() {
    log_info "列出可用备份..."

    if [[ ! -d "${BACKUP_DIR}" ]]; then
        log_error "备份目录不存在: ${BACKUP_DIR}"
        return 1
    fi

    local backups=()
    while IFS= read -r -d '' dir; do
        backups+=("$dir")
    done < <(find "${BACKUP_DIR}" -type d -name "upgrade_*" -print0 2>/dev/null | sort -rz)

    if [[ ${#backups[@]} -eq 0 ]]; then
        log_warning "未找到备份"
        return 0
    fi

    echo
    echo "可用备份列表:"
    echo "==============="

    local i=0
    for backup in "${backups[@]}"; do
        i=$((i + 1))
        local info_file="${backup}/backup_info.txt"
        local timestamp=$(basename "${backup}" | sed 's/upgrade_//')

        echo
        echo "${i}. ${timestamp}"
        echo "   路径: ${backup}"

        if [[ -f "${info_file}" ]]; then
            echo "   详情:"
            while IFS= read -r line; do
                echo "     ${line}"
            done < "${info_file}"
        fi

        # 检查备份完整性
        local integrity="未知"
        if [[ -f "${backup}/checksums.txt" ]]; then
            integrity="已验证"
        elif [[ -f "${backup}/data.tar.gz" && -f "${backup}/config.tar.gz" ]]; then
            integrity="完整"
        else
            integrity="不完整"
        fi
        echo "   状态: ${integrity}"
    done

    echo
    echo "使用: ${SCRIPT_NAME} -b <路径> 进行回滚"
    return 0
}

# ========== 获取备份信息 ==========
get_backup_info() {
    local backup_path="$1"
    local info_file="${backup_path}/backup_info.txt"

    if [[ ! -f "${info_file}" ]]; then
        echo "备份信息文件不存在"
        return 1
    fi

    cat "${info_file}"
}

# ========== 检查备份完整性 ==========
check_backup_integrity() {
    local backup_path="$1"

    log_info "检查备份完整性: ${backup_path}"

    local files=(
        "data.tar.gz"
        "config.tar.gz"
        "aib-node.backup"
        "checksums.txt"
    )

    local missing=()
    for file in "${files[@]}"; do
        if [[ ! -f "${backup_path}/${file}" ]]; then
            missing+=("${file}")
        fi
    done

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "备份不完整，缺少文件:"
        for file in "${missing[@]}"; do
            echo "  - ${file}"
        done
        return 1
    fi

    # 验证校验和
    if [[ -f "${backup_path}/checksums.txt" ]]; then
        cd "${backup_path}"
        if sha256sum -c checksums.txt > /dev/null 2>&1; then
            log_success "备份完整性验证通过"
        else
            log_error "备份完整性验证失败"
            return 1
        fi
        cd - > /dev/null
    else
        log_warning "未找到校验和文件，跳过完整性验证"
    fi

    return 0
}

# ========== 停止服务 ==========
stop_service() {
    log_info "停止节点服务..."

    if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
        log_info "停止服务: ${SERVICE_NAME}"
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

# ========== 恢复二进制 ==========
restore_binary() {
    local backup_path="$1"
    local backup_binary="${backup_path}/aib-node.backup"
    local target_binary="${BINARY_DIR}/aib-node"

    log_info "恢复二进制文件..."

    if [[ ! -f "${backup_binary}" ]]; then
        log_error "备份二进制文件不存在: ${backup_binary}"
        return 1
    fi

    # 检查当前二进制
    if [[ -f "${target_binary}" ]]; then
        log_info "备份当前二进制: ${target_binary}.old"
        mv "${target_binary}" "${target_binary}.old" || {
            log_error "备份当前二进制失败"
            return 1
        }
    fi

    # 恢复备份二进制
    cp "${backup_binary}" "${target_binary}" || {
        log_error "恢复二进制失败"
        return 1
    }

    chmod +x "${target_binary}"

    log_success "二进制恢复完成"
    return 0
}

# ========== 恢复数据 ==========
restore_data() {
    local backup_path="$1"
    local backup_data="${backup_path}/data.tar.gz"

    log_info "恢复数据目录..."

    if [[ ! -f "${backup_data}" ]]; then
        log_error "备份数据不存在: ${backup_data}"
        return 1
    fi

    # 检查当前数据
    if [[ -d "${DATA_DIR}" ]]; then
        log_info "备份当前数据: ${DATA_DIR}.old"
        mv "${DATA_DIR}" "${DATA_DIR}.old" || {
            log_error "备份当前数据失败"
            return 1
        }
    fi

    # 创建数据目录
    mkdir -p "${DATA_DIR}"

    # 恢复数据
    tar -xzf "${backup_data}" -C "${DATA_DIR}" || {
        log_error "恢复数据失败"
        return 1
    }

    log_success "数据恢复完成"
    return 0
}

# ========== 恢复配置 ==========
restore_config() {
    local backup_path="$1"
    local backup_config="${backup_path}/config.tar.gz"

    log_info "恢复配置文件..."

    if [[ ! -f "${backup_config}" ]]; then
        log_error "备份配置不存在: ${backup_config}"
        return 1
    fi

    # 检查当前配置
    if [[ -d "${CONFIG_DIR}" ]]; then
        log_info "备份当前配置: ${CONFIG_DIR}.old"
        mv "${CONFIG_DIR}" "${CONFIG_DIR}.old" || {
            log_error "备份当前配置失败"
            return 1
        }
    fi

    # 创建配置目录
    mkdir -p "${CONFIG_DIR}"

    # 恢复配置
    tar -xzf "${backup_config}" -C "${CONFIG_DIR}" || {
        log_error "恢复配置失败"
        return 1
    }

    log_success "配置恢复完成"
    return 0
}

# ========== 启动服务 ==========
start_service() {
    log_info "启动节点服务..."

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

# ========== 验证回滚 ==========
verify_rollback() {
    log_info "验证回滚结果..."

    sleep 5

    # 检查服务状态
    if ! systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
        log_error "服务未运行"
        return 1
    fi

    # 检查版本
    local version
    version=$("${BINARY_DIR}/aib-node" --version 2>/dev/null || echo "unknown")
    log_success "当前版本: ${version}"

    # 基本健康检查
    local health_url="http://localhost:51200/health"
    if command -v curl &> /dev/null; then
        if curl -sf "${health_url}" > /dev/null 2>&1; then
            log_success "节点健康检查通过"
        else
            log_warning "节点健康检查失败，请手动验证"
        fi
    fi

    return 0
}

# ========== 创建回滚记录 ==========
create_rollback_record() {
    local backup_path="$1"
    local rollback_dir="${BACKUP_DIR}/rollbacks"

    mkdir -p "${rollback_dir}"

    local timestamp=$(date '+%Y%m%d_%H%M%S')
    local record_file="${rollback_dir}/rollback_${timestamp}.txt"

    cat > "${record_file}" << EOF
回滚记录
========
时间: $(date)
备份路径: ${backup_path}
服务名: ${SERVICE_NAME}
回滚原因: 紧急回滚
当前版本: $(get_current_version)
备份版本: $(get_backup_info "${backup_path}" | grep "当前版本" | cut -d: -f2- | xargs)
操作人员: $(whoami)

备份信息:
$(get_backup_info "${backup_path}")

回滚后的状态:
- 二进制: 已恢复
- 配置: 已恢复
- 数据: 已恢复
- 服务: 已启动
EOF

    log_info "回滚记录已保存: ${record_file}"
}

# ========== 获取当前版本 ==========
get_current_version() {
    if [[ -f "${BINARY_DIR}/aib-node" ]]; then
        "${BINARY_DIR}/aib-node" --version 2>/dev/null || echo "unknown"
    else
        echo "not_installed"
    fi
}

# ========== 主流程 ==========
main() {
    echo
    echo -e "${RED}AIB 2.0 紧急回滚脚本${NC}"
    echo -e "${YELLOW}版本: ${VERSION}${NC}"
    echo

    # 解析参数
    parse_args "$@"

    # 列出备份
    if [[ "${LIST_BACKUPS}" == "true" ]]; then
        list_backups
        exit 0
    fi

    # 检查备份路径
    if [[ -z "${BACKUP_PATH}" ]]; then
        log_error "必须指定备份路径 (-b, --backup)"
        show_help
    fi

    # 处理 latest
    if [[ "${BACKUP_PATH}" == "latest" ]]; then
        local latest_backup
        latest_backup=$(find "${BACKUP_DIR}" -type d -name "upgrade_*" -exec stat -c "%Y %n" {} \; 2>/dev/null | sort -nr | head -1 | cut -d' ' -f2-)
        if [[ -n "${latest_backup}" ]]; then
            BACKUP_PATH="${latest_backup}"
            log_info "使用最新备份: ${BACKUP_PATH}"
        else
            log_error "未找到可用备份"
            exit 1
        fi
    fi

    # 检查备份路径
    if [[ ! -d "${BACKUP_PATH}" ]]; then
        log_error "备份路径不存在: ${BACKUP_PATH}"
        exit 1
    fi

    # 确认回滚
    if [[ "${FORCE_ROLLBACK}" != "true" ]]; then
        echo
        echo -e "${YELLOW}即将回滚到备份:${NC}"
        echo "路径: ${BACKUP_PATH}"
        echo
        get_backup_info "${BACKUP_PATH}"
        echo
        echo -e "${RED}警告: 回滚是破坏性操作，将覆盖当前数据!${NC}"
        read -p "确认继续? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "回滚已取消"
            exit 0
        fi
    fi

    # 检查备份完整性
    check_backup_integrity "${BACKUP_PATH}" || {
        log_error "备份完整性检查失败"
        exit 1
    }

    # 确认操作
    echo
    log_critical "开始回滚操作..."
    echo

    # 执行回滚步骤
    local step=0
    local total_steps=6

    step=$((step + 1))
    log_info "[${step}/${total_steps}] 停止服务..."
    stop_service || {
        log_error "停止服务失败"
        exit 1
    }

    step=$((step + 1))
    log_info "[${step}/${total_steps}] 恢复二进制..."
    restore_binary "${BACKUP_PATH}" || {
        log_error "恢复二进制失败"
        exit 1
    }

    step=$((step + 1))
    log_info "[${step}/${total_steps}] 恢复数据目录..."
    restore_data "${BACKUP_PATH}" || {
        log_error "恢复数据失败"
        exit 1
    }

    step=$((step + 1))
    log_info "[${step}/${total_steps}] 恢复配置文件..."
    restore_config "${BACKUP_PATH}" || {
        log_error "恢复配置失败"
        exit 1
    }

    step=$((step + 1))
    log_info "[${step}/${total_steps}] 启动服务..."
    start_service || {
        log_error "启动服务失败"
        exit 1
    }

    step=$((step + 1))
    log_info "[${step}/${total_steps}] 验证回滚..."
    verify_rollback || {
        log_warning "回滚验证失败，请手动检查节点状态"
    }

    # 创建回滚记录
    create_rollback_record "${BACKUP_PATH}"

    # 完成
    echo
    echo "========================================"
    echo "  回滚完成"
    echo "========================================"
    echo
    echo "备份路径: ${BACKUP_PATH}"
    echo "当前版本: $(get_current_version)"
    echo
    echo "注意事项:"
    echo "  - 当前数据已备份到: ${DATA_DIR}.old"
    echo "  - 当前配置已备份到: ${CONFIG_DIR}.old"
    echo "  - 节点可能需要重新同步"
    echo "  - 请监控节点状态"
    echo
    echo "常用命令:"
    echo "  状态: systemctl status ${SERVICE_NAME}"
    echo "  日志: journalctl -u ${SERVICE_NAME} -f"
    echo "  健康: curl http://localhost:51200/health"
    echo "========================================"

    log_success "回滚操作完成!"
    exit 0
}

# ========== 入口 ==========
main "$@"
