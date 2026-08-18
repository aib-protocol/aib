#!/bin/bash
#
# AIB 2.0 node upgrade script
# Usage: ./upgrade.sh [options]
# Options:
#   -v, --version VERSION    specify the upgrade version (default: latest)
#   -b, --backup-dir DIR     backup directory (default: ./backups)
#   -s, --skip-backup        skip backup step (not recommended)
#   -f, --force              force upgrade, skip confirmation
#   -d, --dry-run            dry run, no actual upgrade performed
#   -h, --help               show help information
#
# Features:
#   1. back up current node data and config
#   2. download and verify the new binary
#   3. update the config file
#   4. restart the node service
#   5. validate upgrade success
#
# Author: AIB Protocol Team
# Updated: 2026-03
#

set -e  # exit immediately on errors
set -u  # exit on undefined variables
set -o pipefail  # exit on failed pipe commands

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
NC='\033[0m' # No Color

# ========== logging functions ==========
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

# ========== default config ==========
DEFAULT_VERSION="latest"
DEFAULT_BACKUP_DIR="${SCRIPT_DIR}/backups"
DEFAULT_DATA_DIR="${PROJECT_DIR}/data"
DEFAULT_CONFIG_DIR="${PROJECT_DIR}/config"
BINARY_DIR="${PROJECT_DIR}/bin"
LOG_DIR="${SCRIPT_DIR}/logs"
DOWNLOAD_CACHE="/tmp/aib-upgrade"

# runtime config
VERSION="${DEFAULT_VERSION}"
BACKUP_DIR="${DEFAULT_BACKUP_DIR}"
DATA_DIR="${DEFAULT_DATA_DIR}"
CONFIG_DIR="${DEFAULT_CONFIG_DIR}"
SKIP_BACKUP=false
FORCE=false
DRY_RUN=false
SERVICE_NAME="aib2-mainnet"

# ========== help information ==========
show_help() {
    cat << EOF
AIB 2.0 node upgrade script v${VERSION}

Usage: ${SCRIPT_NAME} [options]

Options:
  -v, --version VERSION    specify the upgrade version (default: ${DEFAULT_VERSION})
  -b, --backup-dir DIR     backup directory (default: ${DEFAULT_BACKUP_DIR})
  -d, --data-dir DIR       data directory (default: ${DEFAULT_DATA_DIR})
  -s, --skip-backup        skip backup step (not recommended)
  -f, --force              force upgrade, skip confirmation
  -n, --dry-run            dry run, no actual upgrade performed
  -h, --help               show this help message

Examples:
  ${SCRIPT_NAME}                              # upgrade to the latest version
  ${SCRIPT_NAME} -v 2.1.0                     # upgrade to a specified version
  ${SCRIPT_NAME} -n                           # dry run
  ${SCRIPT_NAME} -b /custom/backup/path       # use a custom backup directory

upgrade steps:
  1. check current node status
  2. back up data and config
  3. download the new binary
  4. validate binary integrity
  5. update the config file
  6. stop the current service
  7. deploy the new version
  8. start the service
  9. validate upgrade success

for more information visit: https://docs.aib.network/upgrade
EOF
    exit 0
}

# ========== parse arguments ==========
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
                log_error "unknown argument: $1"
                show_help
                ;;
        esac
    done
}

# ========== environment check ==========
check_environment() {
    log_info "check the runtime environment..."

    # check whether it is root user
    if [[ $EUID -eq 0 ]]; then
        log_warning "not recommended root user runs this script"
    fi

    # check required commands
    local required_commands=("curl" "jq" "sha256sum" "systemctl")
    for cmd in "${required_commands[@]}"; do
        if ! command -v "$cmd" &> /dev/null; then
            log_error "missing required command: $cmd"
            return 1
        fi
    done

    # check directory permissions
    if [[ ! -w "${PROJECT_DIR}" ]]; then
        log_error "no write permission on the project directory: ${PROJECT_DIR}"
        return 1
    fi

    # create necessary directories
    mkdir -p "${BACKUP_DIR}" "${LOG_DIR}" "${DOWNLOAD_CACHE}"

    log_success "environment check passed"
    return 0
}

# ========== get the current version ==========
get_current_version() {
    if [[ -f "${BINARY_DIR}/aib-node" ]]; then
        "${BINARY_DIR}/aib-node" --version 2>/dev/null || echo "unknown"
    else
        echo "not_installed"
    fi
}

# ========== get the latest version ==========
get_latest_version() {
    log_info "check the latest available version..."

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

# ========== check node status ==========
check_node_status() {
    log_info "check node status..."

    local is_running=false

    if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
        is_running=true
        log_success "node service is running"
    elif pgrep -f "aib-node" > /dev/null; then
        is_running=true
        log_warning "detected aib-node process, but not managed by systemd"
    else
        log_warning "node service is not running"
    fi

    # check node health status
    if [[ "${is_running}" == "true" ]]; then
        local health_url="http://localhost:51200/health"
        if command -v curl &> /dev/null; then
            if curl -sf "${health_url}" > /dev/null 2>&1; then
                log_success "node health check passed"
            else
                log_warning "node health check failed, there may be issues"
            fi
        fi
    fi

    return 0
}

# ========== backup data ==========
backup_data() {
    if [[ "${SKIP_BACKUP}" == "true" ]]; then
        log_warning "backup step skipped (not recommended)"
        return 0
    fi

    log_info "start backing up data..."

    local timestamp=$(date '+%Y%m%d_%H%M%S')
    local backup_path="${BACKUP_DIR}/upgrade_${timestamp}"
    local current_version=$(get_current_version)

    mkdir -p "${backup_path}"

    # back up the data directory
    if [[ -d "${DATA_DIR}" ]]; then
        log_info "back up the data directory..."
        tar -czf "${backup_path}/data.tar.gz" -C "${DATA_DIR}" . || {
            log_error "failed to back up the data directory"
            return 1
        }
    fi

    # back up the config directory
    if [[ -d "${CONFIG_DIR}" ]]; then
        log_info "back up the config directory..."
        tar -czf "${backup_path}/config.tar.gz" -C "${CONFIG_DIR}" . || {
            log_error "failed to back up the config directory"
            return 1
        }
    fi

    # back up the current binary
    if [[ -f "${BINARY_DIR}/aib-node" ]]; then
        log_info "back up the current binary..."
        cp "${BINARY_DIR}/aib-node" "${backup_path}/aib-node.backup" || {
            log_error "binary backup failed"
            return 1
        }
    fi

    # record version info
    cat > "${backup_path}/backup_info.txt" << EOF
backup time: ${timestamp}
current version: ${current_version}
target version: ${VERSION}
data directory: ${DATA_DIR}
config directory: ${CONFIG_DIR}
EOF

    # create backup checksum
    cd "${backup_path}"
    sha256sum *.tar.gz *.backup > checksums.txt 2>/dev/null || true
    cd - > /dev/null

    log_success "backupcomplete: ${backup_path}"
    echo "${backup_path}" > "${BACKUP_DIR}/.last_backup"

    return 0
}

# ========== download the new version ==========
download_version() {
    local target_version="$1"
    log_info "download version ${target_version}..."

    local download_url="https://github.com/aib-protocol/aib/releases/download/v${target_version}/aib-node-linux-amd64"
    local checksum_url="https://github.com/aib-protocol/aib/releases/download/v${target_version}/checksums.txt"

    if [[ "${target_version}" == "latest" ]]; then
        target_version=$(get_latest_version)
        download_url="https://github.com/aib-protocol/aib/releases/latest/download/aib-node-linux-amd64"
    fi

    log_info "download URL: ${download_url}"

    # download binary
    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY-RUN] will download: ${download_url}"
        return 0
    fi

    curl -L -o "${DOWNLOAD_CACHE}/aib-node" "${download_url}" || {
        log_error "download failed"
        return 1
    }

    # download checksum
    curl -L -o "${DOWNLOAD_CACHE}/checksums.txt" "${checksum_url}" || {
        log_warning "checksum file download failed, skipping validation"
    }

    # set executable permissions
    chmod +x "${DOWNLOAD_CACHE}/aib-node"

    log_success "download complete"
    return 0
}

# ========== validatebinary ==========
verify_binary() {
    log_info "validate the binary file..."

    local downloaded="${DOWNLOAD_CACHE}/aib-node"

    if [[ ! -f "${downloaded}" ]]; then
        log_error "downloaded file does not exist"
        return 1
    fi

    # check file size
    local size=$(stat -f%z "${downloaded}" 2>/dev/null || stat -c%s "${downloaded}" 2>/dev/null)
    if [[ ${size} -lt 1000000 ]]; then
        log_error "abnormal file size: ${size} bytes"
        return 1
    fi

    # validate checksum (if available)
    if [[ -f "${DOWNLOAD_CACHE}/checksums.txt" ]]; then
        local calculated=$(sha256sum "${downloaded}" | awk '{print $1}')
        local expected=$(grep "aib-node-linux-amd64" "${DOWNLOAD_CACHE}/checksums.txt" | awk '{print $1}')

        if [[ -n "${expected}" && "${calculated}" != "${expected}" ]]; then
            log_error "checksum mismatch"
            log_error "expected: ${expected}"
            log_error "actual: ${calculated}"
            return 1
        fi
        log_success "checksum validation passed"
    else
        log_warning "checksum file not found, skipping checksum validation"
    fi

    # check whether the binary is executable
    if ! "${downloaded}" --version &> /dev/null; then
        log_error "binary not executable or corrupted"
        return 1
    fi

    log_success "binaryvalidatepassed"
    return 0
}

# ========== update the config file ==========
update_config() {
    log_info "check config file updates..."

    local config_file="${CONFIG_DIR}/config.toml"

    if [[ ! -f "${config_file}" ]]; then
        log_warning "config file does not exist: ${config_file}"
        return 0
    fi

    # config migration logic (per version changes）
    # example: add a new config item
    log_info "config file needs no update"

    return 0
}

# ========== Stop Service ==========
stop_service() {
    log_info "stop the node service..."

    if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
        if [[ "${DRY_RUN}" == "true" ]]; then
            log_info "[DRY-RUN] will stop the service: ${SERVICE_NAME}"
            return 0
        fi

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

# ========== deploy the new version ==========
deploy_binary() {
    log_info "deploy the new binary..."

    local source="${DOWNLOAD_CACHE}/aib-node"
    local target="${BINARY_DIR}/aib-node"

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY-RUN] will deploy: ${source} -> ${target}"
        return 0
    fi

    # back up the old version
    if [[ -f "${target}" ]]; then
        mv "${target}" "${target}.old" || {
            log_error "failed to back up the old version"
            return 1
        }
    fi

    # deploy the new version
    cp "${source}" "${target}" || {
        log_error "failed to deploy the new version"
        # attempt to restore
        if [[ -f "${target}.old" ]]; then
            mv "${target}.old" "${target}"
        fi
        return 1
    }

    chmod +x "${target}"

    log_success "new version deployed"
    return 0
}

# ========== start the service ==========
start_service() {
    log_info "start the node service..."

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY-RUN] will start the service: ${SERVICE_NAME}"
        return 0
    fi

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

# ========== validate upgrade ==========
verify_upgrade() {
    log_info "validate upgrade result..."

    sleep 5  # wait for the service to fully start

    # check service status
    if ! systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
        log_error "service is not running"
        return 1
    fi

    # check version
    local new_version=$("${BINARY_DIR}/aib-node" --version 2>/dev/null)
    log_success "current version: ${new_version}"

    # check node sync status
    local health_url="http://localhost:51200/health"
    if curl -sf "${health_url}" > /dev/null 2>&1; then
        log_success "node health check passed"
    else
        log_warning "node health check failed, please verify manually"
    fi

    return 0
}

# ========== Cleanup ==========
cleanup() {
    log_info "clean up temporary files..."

    rm -rf "${DOWNLOAD_CACHE}"

    log_success "cleanup complete"
}

# ========== rollback ==========
rollback() {
    local backup_path="$1"

    log_error "upgrade failed, starting rollback..."

    if [[ -z "${backup_path}" || ! -d "${backup_path}" ]]; then
        log_error "invalid backup path: ${backup_path}"
        return 1
    fi

    # restore binary
    if [[ -f "${backup_path}/aib-node.backup" ]]; then
        cp "${backup_path}/aib-node.backup" "${BINARY_DIR}/aib-node"
        chmod +x "${BINARY_DIR}/aib-node"
        log_info "binary restored"
    fi

    # restore data (optional, requires user confirmation）
    log_warning "data restore must be run manually, use rollback.sh script"

    return 0
}

# ========== main flow ==========
main() {
    log_info "AIB 2.0 node upgrade script started"
    log_info "Project directory: ${PROJECT_DIR}"

    # parse arguments
    parse_args "$@"

    # environment check
    check_environment || exit 1

    # check node status
    check_node_status || exit 1

    # get version info
    local current_version=$(get_current_version)
    log_info "current version: ${current_version}"

    if [[ "${VERSION}" == "latest" ]]; then
        VERSION=$(get_latest_version)
    fi
    log_info "target version: ${VERSION}"

    # confirm upgrade
    if [[ "${FORCE}" != "true" && "${DRY_RUN}" != "true" ]]; then
        echo
        echo -e "${YELLOW}about to ${current_version} upgrade to ${VERSION}${NC}"
        read -p "Continue? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "upgrade cancelled"
            exit 0
        fi
    fi

    # execute the upgrade flow
    local backup_path=""
    local step=0
    local total_steps=8

    # 1. backup
    step=$((step + 1))
    log_info "[${step}/${total_steps}] backup data..."
    backup_data || {
        log_error "backup failed, upgrade aborted"
        exit 1
    }
    backup_path=$(cat "${BACKUP_DIR}/.last_backup" 2>/dev/null)

    # 2. download
    step=$((step + 1))
    log_info "[${step}/${total_steps}] download the new version..."
    download_version "${VERSION}" || {
        log_error "download failed"
        rollback "${backup_path}"
        exit 1
    }

    # 3. validate
    step=$((step + 1))
    log_info "[${step}/${total_steps}] validatebinary..."
    verify_binary || {
        log_error "validatefailed"
        exit 1
    }

    # 4. update config
    step=$((step + 1))
    log_info "[${step}/${total_steps}] update config..."
    update_config || {
        log_warning "config update failed, continuing upgrade"
    }

    # 5. Stop service
    step=$((step + 1))
    log_info "[${step}/${total_steps}] Stopping service..."
    stop_service || {
        log_error "failed to stop service"
        exit 1
    }

    # 6. deploy
    step=$((step + 1))
    log_info "[${step}/${total_steps}] deploy the new version..."
    deploy_binary || {
        log_error "deploy failed, attempting rollback..."
        rollback "${backup_path}"
        exit 1
    }

    # 7. start the service
    step=$((step + 1))
    log_info "[${step}/${total_steps}] start the service..."
    start_service || {
        log_error "start failed, attempting rollback..."
        rollback "${backup_path}"
        exit 1
    }

    # 8. validate
    step=$((step + 1))
    log_info "[${step}/${total_steps}] validate upgrade..."
    verify_upgrade || {
        log_warning "upgrade validation failed, please check node status manually"
    }

    # Cleanup
    cleanup

    # complete
    echo
    log_success "====== upgrade complete ======"
    log_success "backup location: ${backup_path}"
    log_success "if issues arise, use rollback.sh script rollback"

    exit 0
}

# ========== Entry Point ==========
main "$@"
