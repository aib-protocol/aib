#!/bin/bash
#
# AIB 2.0 mainnet node startup script
# Usage: ./start-mainnet-node.sh [options]
# Options:
#   -p, --port PORT     listen port (default: 51200)
#   -d, --daemon        run as daemon
#   -s, --systemd       install systemd service
#   -r, --restart       restart systemd service
#   -h, --help          show help
#

set -e

# ========== Config variables ==========
PROJECT_DIR="."
BINARY_PATH="${PROJECT_DIR}/bin/aib2-portal"
DEFAULT_PORT="51200"
SERVICE_NAME="aib2-mainnet"
LOG_DIR="${PROJECT_DIR}/logs"
LOG_FILE="${LOG_DIR}/mainnet.log"
PID_FILE="${PROJECT_DIR}/aib2-portal.pid"

# node configuration
NODE_IP="www.aib.one"

# ========== Color definitions ==========
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ========== Functions ==========

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

show_help() {
    cat << EOF
AIB 2.0 Mainnet Node Startup Script

Usage: $0 [options]

Options:
  -p, --port PORT     listen port (default: ${DEFAULT_PORT})
  -d, --daemon        run as daemon
  -s, --systemd       install systemd service
  -r, --restart       restart systemd service
  -t, --stop          stop the service
  -S, --status        show service status
  -h, --help          show help

Examples:
  $0 --port 51200                    # start on port 51200
  $0 --daemon                         # run as daemon
  $0 --systemd                        # install systemd service
  $0 --restart                        # restart the service
  $0 --status                         # show status
EOF
}

# check port availability
check_port() {
    local port=$1
    if ss -tlnp 2>/dev/null | grep -q ":${port} " || netstat -tlnp 2>/dev/null | grep -q ":${port} "; then
        log_error "Port ${port} is already in use!"
        return 1
    fi
    log_info "Port ${port} is available"
    return 0
}

# create log directory
setup_log_dir() {
    if [ ! -d "${LOG_DIR}" ]; then
        mkdir -p "${LOG_DIR}"
        log_info "Creating log directory: ${LOG_DIR}"
    fi
}

# check dependencies
check_dependencies() {
    if [ ! -f "${BINARY_PATH}" ]; then
        log_error "Binary not found: ${BINARY_PATH}"
        log_info "Build first: cd ${PROJECT_DIR} && go build -o bin/aib2-portal ./cmd/aib2-portal"
        exit 1
    fi

    if [ ! -x "${BINARY_PATH}" ]; then
        log_error "Binary is not executable: ${BINARY_PATH}"
        chmod +x "${BINARY_PATH}"
    fi

    log_success "Dependency check passed"
}

# start node
start_node() {
    local port=$1
    local daemon=$2

    check_port "${port}" || exit 1
    setup_log_dir
    check_dependencies

    log_info "Starting AIB 2.0 mainnet node..."
    log_info "Listen address: https://${NODE_IP}:${port}"
    log_info "Log file: ${LOG_FILE}"

    if [ "${daemon}" = "true" ]; then
        nohup "${BINARY_PATH}" -addr ":${port}" >> "${LOG_FILE}" 2>&1 &
        echo $! > "${PID_FILE}"
        sleep 2

        if kill -0 $(cat "${PID_FILE}") 2>/dev/null; then
            log_success "Node started successfully! PID: $(cat ${PID_FILE})"
            log_info "URL: https://${NODE_IP}:${port}"
        else
            log_error "Node failed to start, check the log: ${LOG_FILE}"
            exit 1
        fi
    else
        exec "${BINARY_PATH}" -addr ":${port}"
    fi
}

# stop node
stop_node() {
    if [ -f "${PID_FILE}" ]; then
        local pid=$(cat "${PID_FILE}")
        if kill -0 "${pid}" 2>/dev/null; then
            log_info "Stopping node (PID: ${pid})..."
            kill "${pid}"
            sleep 2
            if kill -0 "${pid}" 2>/dev/null; then
                kill -9 "${pid}"
            fi
            rm -f "${PID_FILE}"
            log_success "Node stopped"
        else
            log_warn "Node is not running"
            rm -f "${PID_FILE}"
        fi
    else
        # try to find and kill the process
        local pid=$(pgrep -f "aib2-portal.*addr.*:${DEFAULT_PORT}")
        if [ -n "${pid}" ]; then
            log_info "Found running node (PID: ${pid}), stopping..."
            kill "${pid}" 2>/dev/null || true
            sleep 2
            log_success "Node stopped"
        else
            log_warn "No running node found"
        fi
    fi
}

# check node status
check_status() {
    if [ -f "${PID_FILE}" ]; then
        local pid=$(cat "${PID_FILE}")
        if kill -0 "${pid}" 2>/dev/null; then
            log_success "Node is running (PID: ${pid})"
            return 0
        else
            log_error "PID file exists but process has exited"
            return 1
        fi
    else
        local pid=$(pgrep -f "aib2-portal.*addr.*:${DEFAULT_PORT}")
        if [ -n "${pid}" ]; then
            log_success "Node is running (PID: ${pid})"
            return 0
        else
            log_error "Node is not running"
            return 1
        fi
    fi
}

# install systemd service
install_systemd_service() {
    log_info "Installing systemd service: ${SERVICE_NAME}"

    # create systemd service file
    cat > /tmp/${SERVICE_NAME}.service << EOF
[Unit]
Description=AIB 2.0 Mainnet Node
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=${SERVICE_USER:-aib}
WorkingDirectory=${PROJECT_DIR}
ExecStart=${BINARY_PATH} -addr :${DEFAULT_PORT}
Restart=on-failure
RestartSec=10
StandardOutput=append:${LOG_FILE}
StandardError=append:${LOG_FILE}

# environment variables
Environment=PROJECT_DIR=${PROJECT_DIR}
Environment=PORT=${DEFAULT_PORT}

[Install]
WantedBy=multi-user.target
EOF

    # copy to systemd directory
    if [ -d /etc/systemd/system ]; then
        cp /tmp/${SERVICE_NAME}.service /etc/systemd/system/
        systemctl daemon-reload
        log_success "systemd service installed"
        log_info "Manage the service with:"
        echo "  systemctl start ${SERVICE_NAME}     # start"
        echo "  systemctl stop ${SERVICE_NAME}      # stop"
        echo "  systemctl restart ${SERVICE_NAME}   # restart"
        echo "  systemctl status ${SERVICE_NAME}    # status"
        echo "  journalctl -u ${SERVICE_NAME} -f    # view logs"
    else
        log_error "systemd not installed or unavailable"
        exit 1
    fi
}

# start systemd service
start_systemd_service() {
    log_info "Starting systemd service..."
    systemctl start ${SERVICE_NAME}
    log_success "Service started"
    systemctl status ${SERVICE_NAME} --no-pager
}

# restart systemd service
restart_systemd_service() {
    log_info "Restarting systemd service..."
    systemctl restart ${SERVICE_NAME}
    log_success "Service restarted"
    systemctl status ${SERVICE_NAME} --no-pager
}

# ========== Main program ==========

# parse command-line arguments
PORT="${DEFAULT_PORT}"
DAEMON="false"
SYSTEMD="false"
RESTART="false"
STOP="false"
STATUS="false"

while [[ $# -gt 0 ]]; do
    case $1 in
        -p|--port)
            PORT="$2"
            shift 2
            ;;
        -d|--daemon)
            DAEMON="true"
            shift
            ;;
        -s|--systemd)
            SYSTEMD="true"
            shift
            ;;
        -r|--restart)
            RESTART="true"
            shift
            ;;
        -t|--stop)
            STOP="true"
            shift
            ;;
        -S|--status)
            STATUS="true"
            shift
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# execute action
if [ "${SYSTEMD}" = "true" ]; then
    install_systemd_service
    start_systemd_service
elif [ "${RESTART}" = "true" ]; then
    restart_systemd_service
elif [ "${STOP}" = "true" ]; then
    systemctl stop ${SERVICE_NAME} 2>/dev/null || stop_node
elif [ "${STATUS}" = "true" ]; then
    systemctl status ${SERVICE_NAME} --no-pager 2>/dev/null || check_status
else
    start_node "${PORT}" "${DAEMON}"
fi
