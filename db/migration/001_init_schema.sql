-- ============================================================================
-- Code-Shield 数据库初始化脚本（001_init_schema）
-- 目标数据库：PostgreSQL 14+
-- ----------------------------------------------------------------------------
-- 【与 GORM AutoMigrate 的关系】
--   1. models/models.go 中的 GORM 模型与本文件表结构一一对应，本文件是生产
--      环境的建表基准（source of truth）；
--   2. 开发环境可通过 GORM AutoMigrate 自动建表/补列，但 AutoMigrate 不会
--      删除列、不会创建本文件中的全部索引，生产上线与升级必须以迁移脚本为准；
--   3. 后续模型变更请新增 002_xxx.sql 增量迁移脚本，不要直接修改本文件在
--      线上重复执行；
--   4. 本脚本全部使用 IF NOT EXISTS，可安全重复执行，不会破坏已有数据。
-- 执行方式：
--   psql -h 127.0.0.1 -U code_shield -d code_shield -f db/migration/001_init_schema.sql
-- 或使用脚本：
--   ./scripts/init-db.sh [--create-db]
-- ============================================================================

-- ---------------------------------------------------------------------------
-- 1. repositories：代码仓库表
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS repositories (
    id SERIAL PRIMARY KEY,
    name VARCHAR(200),
    url TEXT,
    branch VARCHAR(100),
    local_path TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- ---------------------------------------------------------------------------
-- 2. users：平台用户表
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(100) UNIQUE,
    password_hash VARCHAR(255),
    role VARCHAR(50),
    created_at TIMESTAMP DEFAULT NOW()
);

-- ---------------------------------------------------------------------------
-- 3. task_types：任务类型插件表（对应 tasks/<name>/meta.json 启动时同步入库）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS task_types (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE,
    display_name VARCHAR(200),
    description TEXT,
    engine_mode VARCHAR(50),
    engine_config JSONB,
    ai_backend VARCHAR(50),
    is_active BOOLEAN DEFAULT TRUE,
    is_campaign BOOLEAN DEFAULT FALSE,
    governance_mode VARCHAR(50)
);

-- ---------------------------------------------------------------------------
-- 4. task_reports：任务报告表（依赖 repositories / task_types）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS task_reports (
    id SERIAL PRIMARY KEY,
    repo_id INTEGER REFERENCES repositories(id),
    task_type_id INTEGER REFERENCES task_types(id),
    status VARCHAR(50),
    report_path TEXT,
    ai_summary TEXT,
    score INTEGER,
    metrics JSONB,
    clone_status VARCHAR(20),
    clone_message TEXT,
    total_chunks INTEGER DEFAULT 0,
    processed_chunks INTEGER DEFAULT 0,
    success_chunks INTEGER DEFAULT 0,
    failed_chunks INTEGER DEFAULT 0,
    has_failed_chunks BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- ---------------------------------------------------------------------------
-- 5. analysis_findings：缺陷问题表
--    注：l1_fingerprint / l2_fingerprint 为缺陷指纹列，由 services/defects 包
--        在汇总阶段计算写入，用于四级漏斗增量比对
--        （NEW / EXISTED / RESOLVED / REOPENED）。
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS analysis_findings (
    id SERIAL PRIMARY KEY,
    task_report_id INTEGER REFERENCES task_reports(id),
    task_type_id INTEGER REFERENCES task_types(id),
    repo_id INTEGER REFERENCES repositories(id),
    severity VARCHAR(20),
    category VARCHAR(100),
    file_path TEXT,
    line_number TEXT,
    code_snippet TEXT,
    title VARCHAR(500),
    detail TEXT,
    suggestion TEXT,
    status VARCHAR(20) DEFAULT 'open',
    diff_status VARCHAR(20),
    l1_fingerprint VARCHAR(64),
    l2_fingerprint VARCHAR(64),
    created_at TIMESTAMP DEFAULT NOW()
);

-- ---------------------------------------------------------------------------
-- 6. queue_tasks：持久化任务队列表
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS queue_tasks (
    id SERIAL PRIMARY KEY,
    task_report_id INTEGER REFERENCES task_reports(id),
    status VARCHAR(20),
    priority INTEGER,
    attempts INTEGER DEFAULT 0,
    max_attempts INTEGER DEFAULT 3,
    next_retry_at TIMESTAMP
);

-- ---------------------------------------------------------------------------
-- 索引
-- ---------------------------------------------------------------------------
-- 任务列表按状态 + 时间筛选
CREATE INDEX IF NOT EXISTS idx_task_reports_status_created_at
    ON task_reports(status, created_at);

-- 报告详情页加载缺陷清单
CREATE INDEX IF NOT EXISTS idx_analysis_findings_task_report_id
    ON analysis_findings(task_report_id);

-- 增量比对按 diff_status 过滤（NEW/EXISTED/RESOLVED/REOPENED）
CREATE INDEX IF NOT EXISTS idx_analysis_findings_diff_status
    ON analysis_findings(diff_status);

-- 四级漏斗 L1 物理强指纹匹配
CREATE INDEX IF NOT EXISTS idx_analysis_findings_l1_fingerprint
    ON analysis_findings(l1_fingerprint);

-- 仓库维度缺陷查询与质量排行（补充）
CREATE INDEX IF NOT EXISTS idx_analysis_findings_repo_id
    ON analysis_findings(repo_id);

-- Worker 池按状态消费队列
CREATE INDEX IF NOT EXISTS idx_queue_tasks_status
    ON queue_tasks(status);

-- 失败任务重试扫描（补充）
CREATE INDEX IF NOT EXISTS idx_queue_tasks_next_retry_at
    ON queue_tasks(next_retry_at);

-- 任务类型名称唯一（列定义已有 UNIQUE 约束，此处显式声明唯一索引以对齐门禁要求）
CREATE UNIQUE INDEX IF NOT EXISTS uq_task_types_name
    ON task_types(name);
