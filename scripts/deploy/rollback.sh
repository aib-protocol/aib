#!/bin/bash
#
# AIB 2.0 emergency rollback script
# Usage: ./rollback.sh [options]
# Options:
#   -b, --backup BACKUP_PATH    specify backup path
#   -l, --list                  list available backups
#   -s, --service SERVICE       service name (default: aib2-mainnet)
#   -f, --force                 force rollback, skip confirmation
#   -h, --help                  show help information
#
# Features:
#   1. list available backups
#   2. restore binary file
#   3. restore the config file
#   4. restore the data directory
#   5. restart the node service
#   6. validate rollback result
#
# Author: AIB Protocol Team
# Updated: 2026-03
#

set -e
set -u
set -o pipefail

# ========== script metadata ==========
SCRIPT_NAME="$(basename "$0")"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
VERSION="2.0.0"

# ========== colored output ==========
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# ========== logging functions ==========
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

# ========== config variables ==========
BACKUP_DIR="${SCRIPT_DIR}/backups"
SERVICE_NAME="aib2-mainnet"
FORCE_ROLLBACK=false
BINARY_DIR="${PROJECT_DIR}/bin"
DATA_DIR="${PROJECT_DIR}/data"
CONFIG_DIR="${PROJECT_DIR}/config"

# ========== help information ==========
show_help() {
    cat << EOF
AIB 2.0 emergency rollback script v${VERSION}

Usage: ${SCRIPT_NAME} [options]

Options:
  -b, --backup BACKUP_PATH    specify backup path
  -l, --list                  list available backups
  -s, --service SERVICE       service name (default: ${SERVICE_NAME})
  -f, --force                 force rollback, skip confirmation
  -h, --help                  show help information

Examples:
  # list available backups
  ${SCRIPT_NAME} -l

  # roll back to the specified backup
  ${SCRIPT_NAME} -b /path/to/backup

  # force rollback
  ${SCRIPT_NAME} -b backup_path -f

  # roll back to the most recent backup
  ${SCRIPT_NAME} -b latest

rollback flow:
  1. stop the node service
  2. restore binary file
  3. restore the data directory
  4. restore the config file
  5. start the node service
  6. validate rollback result

warning:
  - rollback will stop the node service
  - data restore is a destructive operation
  - re-sync may be needed after rollback
  - recommended for emergency use
EOF
    exit 0
}

# ========== parse arguments ==========
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
                log_error "unknown argument: $1"
                show_help
                ;;
        esac
    done
}

# ========== list backups ==========
list_backups() {
    log_info "list available backups..."

    if [[ ! -d "${BACKUP_DIR}" ]]; then
        log_error "backup directory does not exist: ${BACKUP_DIR}"
        return 1
    fi

    local backups=()
    while IFS= read -r -d '' dir; do
        backups+=("$dir")
    done < <(find "${BACKUP_DIR}" -type d -name "upgrade_*" -print0 2>/dev/null | sort -rz)

    if [[ ${#backups[@]} -eq 0 ]]; then
        log_warning "backup not found"
        return 0
    fi

    echo
    echo "available backup list:"
    echo "==============="

    local i=0
    for backup in "${backups[@]}"; do
        i=$((i + 1))
        local info_file="${backup}/backup_info.txt"
        local timestamp=$(basename "${backup}" | sed 's/upgrade_//')

        echo
        echo "${i}. ${timestamp}"
        echo "   path: ${backup}"

        if [[ -f "${info_file}" ]]; then
            echo "   details:"
            while IFS= read -r line; do
                echo "     ${line}"
            done < "${info_file}"
        fi

        # check backup integrity
        local integrity="unknown"
        if [[ -f "${backup}/checksums.txt" ]]; then
            integrity="verified"
        elif [[ -f "${backup}/data.tar.gz" && -f "${backup}/config.tar.gz" ]]; then
            integrity="complete"
        else
            integrity="incomplete"
        fi
        echo "   status: ${integrity}"
    done

    echo
    echo "Usage: ${SCRIPT_NAME} -b <path> perform rollback"
    return 0
}

# ========== get backup info ==========
get_backup_info() {
    local backup_path="$1"
    local info_file="${backup_path}/backup_info.txt"

    if [[ ! -f "${info_file}" ]]; then
        echo "backup info file does not exist"
        return 1
    fi

    cat "${info_file}"
}

# ========== check backup integrity ==========
check_backup_integrity() {
    local backup_path="$1"

    log_info "check backup integrity: ${backup_path}"

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
        log_error "backup incomplete, missing files:"
        for file in "${missing[@]}"; do
            echo "  - ${file}"
        done
        return 1
    fi

    # validatechecksum
    if [[ -f "${backup_path}/checksums.txt" ]]; then
        cd "${backup_path}"
        if sha256sum -c checksums.txt > /dev/null 2>&1; then
            log_success "backup integrity validation passed"
        else
            log_error "backup integrity validation failed"
            return 1
        fi
        cd - > /dev/null
    else
        log_warning "notchecksum file not found, skipping integrity validation"
    fi

    return 0
}

# ========== Stop Service ==========
stop_service() {
    log_info "stop the node service..."

    if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
        log_info "Stop service: ${SERVICE_NAME}"
        systemctl stop "${SERVICE_NAME}" || {
            log_error "failed to stop service"
            return 1
        }

        # wait for the service to fully stop
        local count=0
        while systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; do
            sleep 1
            count=$((count + 1))
            if [[ ${count} -gt 30 ]]; then
                log_error "service stop timed out"
                return 1
            fi
        done

        log_success "service stopped"
    else
        log_info "service is not running"
    fi

    return 0
}

# ========== restore binary ==========
restore_binary() {
    local backup_path="$1"
    local backup_binary="${backup_path}/aib-node.backup"
    local target_binary="${BINARY_DIR}/aib-node"

    log_info "restore binary file..."

    if [[ ! -f "${backup_binary}" ]]; then
        log_error "backup binary does not exist: ${backup_binary}"
        return 1
    fi

    # check the current binary
    if [[ -f "${target_binary}" ]]; then
        log_info "back up the current binary: ${target_binary}.old"
        mv "${target_binary}" "${target_binary}.old" || {
            log_error "failed to back up the current binary"
            return 1
        }
    fi

    # restore the backup binary
    cp "${backup_binary}" "${target_binary}" || {
        log_error "failed to restore the binary"
        return 1
    }

    chmod +x "${target_binary}"

    log_success "binary restore complete"
    return 0
}

# ========== Restore Data ==========
restore_data() {
    local backup_path="$1"
    local backup_data="${backup_path}/data.tar.gz"

    log_info "restore the data directory..."

    if [[ ! -f "${backup_data}" ]]; then
        log_error "backup data does not exist: ${backup_data}"
        return 1
    fi

    # check current data
    if [[ -d "${DATA_DIR}" ]]; then
        log_info "back up current data: ${DATA_DIR}.old"
        mv "${DATA_DIR}" "${DATA_DIR}.old" || {
            log_error "failed to back up current data"
            return 1
        }
    fi

    # create the data directory
    mkdir -p "${DATA_DIR}"

    # Restore data
    tar -xzf "${backup_data}" -C "${DATA_DIR}" || {
        log_error "failed to restore data"
        return 1
    }

    log_success "data restore complete"
    return 0
}

# ========== Restore Config ==========
restore_config() {
    local backup_path="$1"
    local backup_config="${backup_path}/config.tar.gz"

    log_info "restore the config file..."

    if [[ ! -f "${backup_config}" ]]; then
        log_error "backup config does not exist: ${backup_config}"
        return 1
    fi

    # check current config
    if [[ -d "${CONFIG_DIR}" ]]; then
        log_info "back up the current config: ${CONFIG_DIR}.old"
        mv "${CONFIG_DIR}" "${CONFIG_DIR}.old" || {
            log_error "failed to back up the current config"
            return 1
        }
    fi

    # create the config directory
    mkdir -p "${CONFIG_DIR}"

    # Restore config
    tar -xzf "${backup_config}" -C "${CONFIG_DIR}" || {
        log_error "failed to restore the config"
        return 1
    }

    log_success "config restore complete"
    return 0
}

# ========== start the service ==========
start_service() {
    log_info "start the node service..."

    systemctl start "${SERVICE_NAME}" || {
        log_error "failed to start service"
        return 1
    }

    # wait for the service to start
    local count=0
    while ! systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; do
        sleep 1
        count=$((count + 1))
        if [[ ${count} -gt 30 ]]; then
            log_error "service start timed out"
            return 1
        fi
    done

    log_success "service started"
    return 0
}

# ========== validate rollback ==========
verify_rollback() {
    log_info "validate rollback result..."

    sleep 5

    # check service status
    if ! systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
        log_error "service is not running"
        return 1
    fi

    # check version
    local version
    version=$("${BINARY_DIR}/aib-node" --version 2>/dev/null || echo "unknown")
    log_success "current version: ${version}"

    # basic health check
    local health_url="http://localhost:51200/health"
    if command -v curl &> /dev/null; then
        if curl -sf "${health_url}" > /dev/null 2>&1; then
            log_success "node health check passed"
        else
            log_warning "node health check failed, please verify manually"
        fi
    fi

    return 0
}

# ========== create rollback record ==========
create_rollback_record() {
    local backup_path="$1"
    local rollback_dir="${BACKUP_DIR}/rollbacks"

    mkdir -p "${rollback_dir}"

    local timestamp=$(date '+%Y%m%d_%H%M%S')
    local record_file="${rollback_dir}/rollback_${timestamp}.txt"

    cat > "${record_file}" << EOF
rollback record
========
time: $(date)
backup path: ${backup_path}
service name: ${SERVICE_NAME}
rollback reason: emergency rollback
current version: $(get_current_version)
backup version: $(get_backup_info "${backup_path}" | grep "current version" | cut -d: -f2- | xargs)
operator: $(whoami)

backup info:
$(get_backup_info "${backup_path}")

post-rollback status:
- binary: restored
- config: restored
- Data: restored
- service: started
EOF

    log_info "rollback record saved: ${record_file}"
}

# ========== get the current version ==========
get_current_version() {
    if [[ -f "${BINARY_DIR}/aib-node" ]]; then
        "${BINARY_DIR}/aib-node" --version 2>/dev/null || echo "unknown"
    else
        echo "not_installed"
    fi
}

# ========== main flow ==========
main() {
    echo
    echo -e "${RED}AIB 2.0 emergency rollback script${NC}"
    echo -e "${YELLOW}version: ${VERSION}${NC}"
    echo

    # parse arguments
    parse_args "$@"

    # list backups
    if [[ "${LIST_BACKUPS}" == "true" ]]; then
        list_backups
        exit 0
    fi

    # check the backup path
    if [[ -z "${BACKUP_PATH}" ]]; then
        log_error "backup path must be specified (-b, --backup)"
        show_help
    fi

    # process latest
    if [[ "${BACKUP_PATH}" == "latest" ]]; then
        local latest_backup
        latest_backup=$(find "${BACKUP_DIR}" -type d -name "upgrade_*" -exec stat -c "%Y %n" {} \; 2>/dev/null | sort -nr | head -1 | cut -d' ' -f2-)
        if [[ -n "${latest_backup}" ]]; then
            BACKUP_PATH="${latest_backup}"
            log_info "use the latest backup: ${BACKUP_PATH}"
        else
            log_error "no usable backup found"
            exit 1
        fi
    fi

    # check the backup path
    if [[ ! -d "${BACKUP_PATH}" ]]; then
        log_error "backup path does not exist: ${BACKUP_PATH}"
        exit 1
    fi

    # confirm rollback
    if [[ "${FORCE_ROLLBACK}" != "true" ]]; then
        echo
        echo -e "${YELLOW}about to roll back to backup:${NC}"
        echo "path: ${BACKUP_PATH}"
        echo
        get_backup_info "${BACKUP_PATH}"
        echo
        echo -e "${RED}warning: rollback is destructive and will overwrite current data!${NC}"
        read -p "Continue? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "rollback cancelled"
            exit 0
        fi
    fi

    # check backup integrity
    check_backup_integrity "${BACKUP_PATH}" || {
        log_error "backup integrity check failed"
        exit 1
    }

    # Confirm operation
    echo
    log_critical "starting rollback..."
    echo

    # execute rollback steps
    local step=0
    local total_steps=6

    step=$((step + 1))
    log_info "[${step}/${total_steps}] Stopping service..."
    stop_service || {
        log_error "failed to stop service"
        exit 1
    }

    step=$((step + 1))
    log_info "[${step}/${total_steps}] restore binary..."
    restore_binary "${BACKUP_PATH}" || {
        log_error "failed to restore the binary"
        exit 1
    }

    step=$((step + 1))
    log_info "[${step}/${total_steps}] restore the data directory..."
    restore_data "${BACKUP_PATH}" || {
        log_error "failed to restore data"
        exit 1
    }

    step=$((step + 1))
    log_info "[${step}/${total_steps}] restore the config file..."
    restore_config "${BACKUP_PATH}" || {
        log_error "failed to restore the config"
        exit 1
    }

    step=$((step + 1))
    log_info "[${step}/${total_steps}] start the service..."
    start_service || {
        log_error "failed to start service"
        exit 1
    }

    step=$((step + 1))
    log_info "[${step}/${total_steps}] validate rollback..."
    verify_rollback || {
        log_warning "rollback validation failed, please check node status manually"
    }

    # create rollback record
    create_rollback_record "${BACKUP_PATH}"

    # complete
    echo
    echo "========================================"
    echo "  rollback complete"
    echo "========================================"
    echo
    echo "backup path: ${BACKUP_PATH}"
    echo "current version: $(get_current_version)"
    echo
    echo "notes:"
    echo "  - current data backed up to: ${DATA_DIR}.old"
    echo "  - current config backed up to: ${CONFIG_DIR}.old"
    echo "  - node may need to re-sync"
    echo "  - please monitor node status"
    echo
    echo "common commands:"
    echo "  status: systemctl status ${SERVICE_NAME}"
    echo "  logs: journalctl -u ${SERVICE_NAME} -f"
    echo "  Health: curl http://localhost:51200/health"
    echo "========================================"

    log_success "rollback complete!"
    exit 0
}

# ========== Entry Point ==========
main "$@"
