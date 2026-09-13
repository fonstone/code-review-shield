#!/usr/bin/env bash
# =============================================================================
# Code-Shield 数据库初始化脚本（PostgreSQL）
# 功能：
#   1. （可选）创建数据库用户与数据库
#   2. 执行 db/migration/001_init_schema.sql 完成建表与索引
# 用法：
#   ./scripts/init-db.sh                  # 仅执行建表迁移
#   ./scripts/init-db.sh --create-db      # 先创建用户与数据库，再执行迁移
# 环境变量（均有默认值，可覆盖）：
#   PGHOST / PGPORT / PGUSER / PGPASSWORD / PGDATABASE
#   PGADMINUSER（--create-db 时使用的超级用户，默认 postgres）
# 示例：
#   PGADMINUSER=postgres ./scripts/init-db.sh --create-db
# =============================================================================
set -euo pipefail

# ---------------------------------------------------------------------------
# 基础变量
# ---------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
MIGRATION_SQL="${PROJECT_ROOT}/db/migration/001_init_schema.sql"

PG_HOST="${PGHOST:-127.0.0.1}"
PG_PORT="${PGPORT:-5432}"
DB_USER="${PGUSER:-code_shield}"
DB_PASSWORD="${PGPASSWORD:-CodeShield618!}"
DB_NAME="${PGDATABASE:-code_shield}"
ADMIN_USER="${PGADMINUSER:-postgres}"

CREATE_DB=false
for arg in "$@"; do
    case "${arg}" in
        --create-db) CREATE_DB=true ;;
        *) echo "[ERROR] 未知参数: ${arg}（仅支持 --create-db）" >&2; exit 1 ;;
    esac
done

log()  { echo "[INFO]  $*"; }
warn() { echo "[WARN]  $*"; }
err()  { echo "[ERROR] $*" >&2; }

# ---------------------------------------------------------------------------
# 0. 环境检查
# ---------------------------------------------------------------------------
if ! command -v psql >/dev/null 2>&1; then
    err "未找到 psql 客户端，请先安装 PostgreSQL 客户端工具"
    exit 1
fi

if [ ! -f "${MIGRATION_SQL}" ]; then
    err "迁移脚本不存在: ${MIGRATION_SQL}"
    exit 1
fi

# ---------------------------------------------------------------------------
# 1. 可选：创建数据库用户与数据库（幂等，已存在则跳过）
# ---------------------------------------------------------------------------
if [ "${CREATE_DB}" = true ]; then
    log "以管理员 ${ADMIN_USER} 创建用户与数据库（已存在则跳过）..."

    # 创建角色（用户）：存在性检查与创建通过 \gexec 动态执行
    psql -h "${PG_HOST}" -p "${PG_PORT}" -U "${ADMIN_USER}" -d postgres \
        -v ON_ERROR_STOP=1 \
        -v db_user="${DB_USER}" -v db_pass="${DB_PASSWORD}" <<'SQL'
SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'db_user', :'db_pass')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = :'db_user')
\gexec
SQL

    # 创建数据库（CREATE DATABASE 不能在事务块中执行，用 \gexec 逐条执行）
    psql -h "${PG_HOST}" -p "${PG_PORT}" -U "${ADMIN_USER}" -d postgres \
        -v ON_ERROR_STOP=1 \
        -v db_name="${DB_NAME}" -v db_user="${DB_USER}" <<'SQL'
SELECT format('CREATE DATABASE %I OWNER %I ENCODING ''UTF8''', :'db_name', :'db_user')
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'db_name')
\gexec
SQL

    log "用户 ${DB_USER} 与数据库 ${DB_NAME} 已就绪"
else
    warn "未指定 --create-db，跳过用户与数据库创建，直接执行迁移"
fi

# ---------------------------------------------------------------------------
# 2. 执行建表迁移（幂等：IF NOT EXISTS，可重复执行）
# ---------------------------------------------------------------------------
log "执行建表迁移: ${MIGRATION_SQL}"
PGPASSWORD="${DB_PASSWORD}" psql \
    -h "${PG_HOST}" -p "${PG_PORT}" \
    -U "${DB_USER}" -d "${DB_NAME}" \
    -v ON_ERROR_STOP=1 \
    -f "${MIGRATION_SQL}"

log "数据库初始化完成: ${DB_NAME}@${PG_HOST}:${PG_PORT}"
