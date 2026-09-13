#!/usr/bin/env bash
# =============================================================================
# Code-Shield 一键部署脚本
# 功能：环境检查 → 构建后端 → 构建前端 → 安装产物 → 重启 systemd 服务
# 用法：
#   ./scripts/deploy.sh              # 默认安装到 /opt/code-shield
#   ./scripts/deploy.sh /srv/code-shield   # 自定义安装目录
# 说明：非 root 用户执行时会自动使用 sudo 提权完成安装与服务重启
# =============================================================================
set -euo pipefail

# ---------------------------------------------------------------------------
# 基础变量
# ---------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
INSTALL_DIR="${1:-/opt/code-shield}"
SERVICE_NAME="code-shield"
BINARY_NAME="code-shield-server"

# 非 root 用户自动加 sudo
if [ "$(id -u)" -ne 0 ]; then
    SUDO="sudo"
else
    SUDO=""
fi

# 日志函数
log()  { echo "[INFO]  $*"; }
warn() { echo "[WARN]  $*"; }
err()  { echo "[ERROR] $*" >&2; }

# ---------------------------------------------------------------------------
# 1. 环境检查：Go / Node / npm 工具链
# ---------------------------------------------------------------------------
log "检查构建环境..."
if ! command -v go >/dev/null 2>&1; then
    err "未找到 Go 工具链，请先安装 Go 1.21+"
    exit 1
fi
log "Go 版本: $(go version)"

if ! command -v node >/dev/null 2>&1; then
    err "未找到 Node.js，请先安装 Node.js 18+"
    exit 1
fi
log "Node 版本: $(node --version)"

if ! command -v npm >/dev/null 2>&1; then
    err "未找到 npm，请先安装 npm"
    exit 1
fi
log "npm 版本: $(npm --version)"

# ---------------------------------------------------------------------------
# 2. 构建后端二进制（注入 git 版本号）
# ---------------------------------------------------------------------------
cd "${PROJECT_ROOT}"
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
log "构建后端二进制（版本 ${VERSION}）..."
CGO_ENABLED=0 go build -ldflags "-X 'main.Version=${VERSION}'" -o "${BINARY_NAME}" .
log "后端构建完成: ${PROJECT_ROOT}/${BINARY_NAME}"

# ---------------------------------------------------------------------------
# 3. 构建前端静态资源（frontend/dist）
# ---------------------------------------------------------------------------
log "构建前端静态资源..."
cd "${PROJECT_ROOT}/frontend"
if [ -f package-lock.json ]; then
    npm ci
else
    warn "未找到 package-lock.json，回退为 npm install"
    npm install
fi
npm run build
log "前端构建完成: ${PROJECT_ROOT}/frontend/dist"

# ---------------------------------------------------------------------------
# 4. 安装产物到目标目录
# ---------------------------------------------------------------------------
log "安装产物到 ${INSTALL_DIR} ..."
${SUDO} mkdir -p "${INSTALL_DIR}"
${SUDO} cp "${PROJECT_ROOT}/${BINARY_NAME}" "${INSTALL_DIR}/"
${SUDO} rm -rf "${INSTALL_DIR}/dist"
${SUDO} cp -r "${PROJECT_ROOT}/frontend/dist" "${INSTALL_DIR}/dist"
${SUDO} rm -rf "${INSTALL_DIR}/tasks"
${SUDO} cp -r "${PROJECT_ROOT}/tasks" "${INSTALL_DIR}/tasks"

# 首次部署时保留已有线上配置，仅提示缺失
if [ ! -f "${INSTALL_DIR}/config.yaml" ]; then
    warn "未发现 ${INSTALL_DIR}/config.yaml，请从 config.yaml.example 复制并填写数据库/LLM 配置后再启动服务"
fi

# ---------------------------------------------------------------------------
# 5. 重载并重启 systemd 服务
# ---------------------------------------------------------------------------
if command -v systemctl >/dev/null 2>&1; then
    log "重载 systemd 配置并重启 ${SERVICE_NAME} ..."
    ${SUDO} systemctl daemon-reload
    ${SUDO} systemctl restart "${SERVICE_NAME}"
    ${SUDO} systemctl status "${SERVICE_NAME}" --no-pager || true
    log "服务 ${SERVICE_NAME} 已重启"
else
    warn "未检测到 systemd，请手动启动: ${INSTALL_DIR}/${BINARY_NAME}"
fi

log "部署完成"
