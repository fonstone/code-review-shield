#!/usr/bin/env bash
# =============================================================================
# Code-Shield 数据备份脚本
# 功能：备份数据库（PostgreSQL dump / SQLite 文件）+ 报告目录 + 日志目录，
#       按日期打包为 tar.gz，并自动清理 30 天前的旧备份
# 用法：
#   ./scripts/backup.sh                      # 默认备份到 /var/backups/code-shield
#   ./scripts/backup.sh /data/backups        # 自定义备份目录
# 定时执行（crontab 示例，每天凌晨 2 点）：
#   0 2 * * * /opt/code-shield/scripts/backup.sh /var/backups/code-shield >> /var/log/code-shield-backup.log 2>&1
# =============================================================================
set -euo pipefail

# ---------------------------------------------------------------------------
# 配置区（可通过环境变量覆盖）
# ---------------------------------------------------------------------------
PROJECT_ROOT="${PROJECT_ROOT:-/opt/code-shield}"
BACKUP_ROOT="${1:-/var/backups/code-shield}"
RETAIN_DAYS="${RETAIN_DAYS:-30}"                # 备份保留天数
REPORT_DIR="${PROJECT_ROOT}/reports"            # 报告输出目录
LOG_DIR="${PROJECT_ROOT}/logs"                  # 运行日志目录
SQLITE_DB="${PROJECT_ROOT}/code-shield.sqlite"  # SQLite 数据库文件（如使用）

# PostgreSQL 连接参数（使用 PostgreSQL 时生效）
PG_HOST="${PG_HOST:-127.0.0.1}"
PG_PORT="${PG_PORT:-5432}"
PG_USER="${PG_USER:-code_shield}"
PG_PASSWORD="${PG_PASSWORD:-}"
PG_DB="${PG_DB:-code_shield}"

TIMESTAMP="$(date +%Y%m%d_%H%M%S)"
ARCHIVE="${BACKUP_ROOT}/code-shield-backup-${TIMESTAMP}.tar.gz"

log()  { echo "[INFO]  $(date '+%Y-%m-%d %H:%M:%S') $*"; }
warn() { echo "[WARN]  $(date '+%Y-%m-%d %H:%M:%S') $*"; }
err()  { echo "[ERROR] $(date '+%Y-%m-%d %H:%M:%S') $*" >&2; }

# ---------------------------------------------------------------------------
# 准备工作目录
# ---------------------------------------------------------------------------
mkdir -p "${BACKUP_ROOT}"
WORK_DIR="$(mktemp -d)"
# 脚本退出时清理临时目录
trap 'rm -rf "${WORK_DIR}"' EXIT

log "开始备份，临时目录: ${WORK_DIR}"

# ---------------------------------------------------------------------------
# 1. 备份数据库：优先 PostgreSQL dump，其次 SQLite 文件
# ---------------------------------------------------------------------------
DB_BACKED_UP=false

if command -v pg_dump >/dev/null 2>&1; then
    log "检测到 pg_dump，导出 PostgreSQL 数据库 ${PG_DB} ..."
    export PGPASSWORD="${PG_PASSWORD}"
    if pg_dump -h "${PG_HOST}" -p "${PG_PORT}" -U "${PG_USER}" -d "${PG_DB}" \
        -Fc -f "${WORK_DIR}/database.pgdump"; then
        DB_BACKED_UP=true
        log "PostgreSQL 导出完成: database.pgdump"
    else
        warn "PostgreSQL 导出失败（请检查连接参数与 PGPASSWORD）"
    fi
    unset PGPASSWORD
else
    warn "未检测到 pg_dump，跳过 PostgreSQL 备份"
fi

if [ -f "${SQLITE_DB}" ]; then
    log "备份 SQLite 数据库: ${SQLITE_DB}"
    cp "${SQLITE_DB}" "${WORK_DIR}/code-shield.sqlite"
    DB_BACKED_UP=true
fi

if [ "${DB_BACKED_UP}" = false ]; then
    warn "未找到可备份的数据库（PostgreSQL 与 SQLite 均不可用）"
fi

# ---------------------------------------------------------------------------
# 2. 备份报告目录与日志目录
# ---------------------------------------------------------------------------
if [ -d "${REPORT_DIR}" ]; then
    log "备份报告目录: ${REPORT_DIR}"
    cp -r "${REPORT_DIR}" "${WORK_DIR}/reports"
else
    warn "报告目录不存在，跳过: ${REPORT_DIR}"
fi

if [ -d "${LOG_DIR}" ]; then
    log "备份日志目录: ${LOG_DIR}"
    cp -r "${LOG_DIR}" "${WORK_DIR}/logs"
else
    warn "日志目录不存在，跳过: ${LOG_DIR}"
fi

# ---------------------------------------------------------------------------
# 3. 打包压缩
# ---------------------------------------------------------------------------
log "打包为 ${ARCHIVE} ..."
tar -czf "${ARCHIVE}" -C "${WORK_DIR}" .

log "备份完成: ${ARCHIVE} ($(du -h "${ARCHIVE}" | cut -f1))"

# ---------------------------------------------------------------------------
# 4. 清理超过保留天数的旧备份
# ---------------------------------------------------------------------------
log "清理 ${RETAIN_DAYS} 天前的旧备份..."
DELETED="$(find "${BACKUP_ROOT}" -maxdepth 1 -name 'code-shield-backup-*.tar.gz' -mtime +"${RETAIN_DAYS}" -print -delete | wc -l)"
log "已清理 ${DELETED} 个旧备份文件"
