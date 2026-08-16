#!/bin/bash
#
# AIB 2.0 主网节点启动脚本
# 用法: ./start-mainnet-node.sh [选项]
# 选项:
#   -p, --port PORT     指定监听端口 (默认: 51200)
#   -d, --daemon        作为守护进程运行
#   -s, --systemd       安装 systemd 服务
#   -r, --restart       重启 systemd 服务
#   -h, --help          显示帮助信息
#

set -e

# ========== 配置变量 ==========
PROJECT_DIR="."
BINARY_PATH="${PROJECT_DIR}/bin/aib2-portal"
DEFAULT_PORT="51200"
SERVICE_NAME="aib2-mainnet"
LOG_DIR="${PROJECT_DIR}/logs"
LOG_FILE="${LOG_DIR}/mainnet.log"
PID_FILE="${PROJECT_DIR}/aib2-portal.pid"

# 节点配置
NODE_IP="www.aib.one"

# ========== 颜色定义 ==========
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ========== 函数定义 ==========

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
AIB 2.0 主网节点启动脚本

用法: $0 [选项]

选项:
  -p, --port PORT     指定监听端口 (默认: ${DEFAULT_PORT})
  -d, --daemon        作为守护进程运行
  -s, --systemd       安装 systemd 服务
  -r, --restart       重启 systemd 服务
  -t, --stop          停止服务
  -S, --status        查看服务状态
  -h, --help          显示帮助信息

示例:
  $0 --port 51200                    # 在端口 51200 启动
  $0 --daemon                         # 作为守护进程运行
  $0 --systemd                        # 安装 systemd 服务
  $0 --restart                        # 重启服务
  $0 --status                         # 查看状态
EOF
}

# 检查端口可用性
check_port() {
    local port=$1
    if ss -tlnp 2>/dev/null | grep -q ":${port} " || netstat -tlnp 2>/dev/null | grep -q ":${port} "; then
        log_error "端口 ${port} 已被占用!"
        return 1
    fi
    log_info "端口 ${port} 可用"
    return 0
}

# 创建日志目录
setup_log_dir() {
    if [ ! -d "${LOG_DIR}" ]; then
        mkdir -p "${LOG_DIR}"
        log_info "创建日志目录: ${LOG_DIR}"
    fi
}

# 检查依赖
check_dependencies() {
    if [ ! -f "${BINARY_PATH}" ]; then
        log_error "找不到二进制文件: ${BINARY_PATH}"
        log_info "请先构建: cd ${PROJECT_DIR} && go build -o bin/aib2-portal ./cmd/aib2-portal"
        exit 1
    fi

    if [ ! -x "${BINARY_PATH}" ]; then
        log_error "二进制文件没有执行权限: ${BINARY_PATH}"
        chmod +x "${BINARY_PATH}"
    fi

    log_success "依赖检查通过"
}

# 启动节点
start_node() {
    local port=$1
    local daemon=$2

    check_port "${port}" || exit 1
    setup_log_dir
    check_dependencies

    log_info "启动 AIB 2.0 主网节点..."
    log_info "监听地址: https://${NODE_IP}:${port}"
    log_info "日志文件: ${LOG_FILE}"

    if [ "${daemon}" = "true" ]; then
        nohup "${BINARY_PATH}" -addr ":${port}" >> "${LOG_FILE}" 2>&1 &
        echo $! > "${PID_FILE}"
        sleep 2

        if kill -0 $(cat "${PID_FILE}") 2>/dev/null; then
            log_success "节点启动成功! PID: $(cat ${PID_FILE})"
            log_info "访问地址: https://${NODE_IP}:${port}"
        else
            log_error "节点启动失败，请查看日志: ${LOG_FILE}"
            exit 1
        fi
    else
        exec "${BINARY_PATH}" -addr ":${port}"
    fi
}

# 停止节点
stop_node() {
    if [ -f "${PID_FILE}" ]; then
        local pid=$(cat "${PID_FILE}")
        if kill -0 "${pid}" 2>/dev/null; then
            log_info "停止节点 (PID: ${pid})..."
            kill "${pid}"
            sleep 2
            if kill -0 "${pid}" 2>/dev/null; then
                kill -9 "${pid}"
            fi
            rm -f "${PID_FILE}"
            log_success "节点已停止"
        else
            log_warn "节点未运行"
            rm -f "${PID_FILE}"
        fi
    else
        # 尝试查找并杀死进程
        local pid=$(pgrep -f "aib2-portal.*addr.*:${DEFAULT_PORT}")
        if [ -n "${pid}" ]; then
            log_info "找到运行中的节点 (PID: ${pid})，正在停止..."
            kill "${pid}" 2>/dev/null || true
            sleep 2
            log_success "节点已停止"
        else
            log_warn "未找到运行中的节点"
        fi
    fi
}

# 检查节点状态
check_status() {
    if [ -f "${PID_FILE}" ]; then
        local pid=$(cat "${PID_FILE}")
        if kill -0 "${pid}" 2>/dev/null; then
            log_success "节点运行中 (PID: ${pid})"
            return 0
        else
            log_error "PID 文件存在但进程已退出"
            return 1
        fi
    else
        local pid=$(pgrep -f "aib2-portal.*addr.*:${DEFAULT_PORT}")
        if [ -n "${pid}" ]; then
            log_success "节点运行中 (PID: ${pid})"
            return 0
        else
            log_error "节点未运行"
            return 1
        fi
    fi
}

# 安装 systemd 服务
install_systemd_service() {
    log_info "安装 systemd 服务: ${SERVICE_NAME}"

    # 创建 systemd 服务文件
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

# 环境变量
Environment=PROJECT_DIR=${PROJECT_DIR}
Environment=PORT=${DEFAULT_PORT}

[Install]
WantedBy=multi-user.target
EOF

    # 复制到 systemd 目录
    if [ -d /etc/systemd/system ]; then
        cp /tmp/${SERVICE_NAME}.service /etc/systemd/system/
        systemctl daemon-reload
        log_success "systemd 服务已安装"
        log_info "使用以下命令管理服务:"
        echo "  systemctl start ${SERVICE_NAME}     # 启动"
        echo "  systemctl stop ${SERVICE_NAME}      # 停止"
        echo "  systemctl restart ${SERVICE_NAME}   # 重启"
        echo "  systemctl status ${SERVICE_NAME}    # 状态"
        echo "  journalctl -u ${SERVICE_NAME} -f    # 查看日志"
    else
        log_error "systemd 未安装或不可用"
        exit 1
    fi
}

# 启动 systemd 服务
start_systemd_service() {
    log_info "启动 systemd 服务..."
    systemctl start ${SERVICE_NAME}
    log_success "服务已启动"
    systemctl status ${SERVICE_NAME} --no-pager
}

# 重启 systemd 服务
restart_systemd_service() {
    log_info "重启 systemd 服务..."
    systemctl restart ${SERVICE_NAME}
    log_success "服务已重启"
    systemctl status ${SERVICE_NAME} --no-pager
}

# ========== 主程序 ==========

# 解析命令行参数
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
            log_error "未知选项: $1"
            show_help
            exit 1
            ;;
    esac
done

# 执行操作
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
