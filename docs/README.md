# Code-Shield

基于 LLM 的代码专项静态扫描平台：插件化任务类型 + 6 阶段 Git 扫描流水线 + 5 种 LLM 执行引擎 + SWRR 加权轮询算力调度与槽位租约 + L1/L2 缺陷指纹增量比对 + React 管理前端。

## 目录

- [架构总览](#架构总览)
- [目录结构](#目录结构)
- [快速开始](#快速开始)
- [插件开发指南](#插件开发指南)
- [API 概览](#api-概览)
- [部署指南](#部署指南)
- [测试与架构门禁](#测试与架构门禁)
- [常见问题](#常见问题)

## 架构总览

```
┌──────────────────────────────────────────────────────────────────────────┐
│                       React 管理前端（Vite + AntD）                        │
│     Dashboard / TaskManagement / SystemDebug / ReportViewer(三Tab)        │
└─────────────────────────────────┬────────────────────────────────────────┘
                                  │ HTTP /api/*（JWT + traceId）
┌─────────────────────────────────▼────────────────────────────────────────┐
│                        Gin HTTP Handlers（接口层）                         │
│            仅做参数校验 / 鉴权 / 统一返回包装，业务下沉 service 门面          │
└─────────────────────────────────┬────────────────────────────────────────┘
                                  │
┌─────────────────────────────────▼────────────────────────────────────────┐
│                     services.go 统一门面（业务领域层）                      │
│  ┌───────────┐ ┌───────────┐ ┌────────────┐ ┌──────────────┐             │
│  │ runner    │ │ engines   │ │ dispatcher │ │ invoker      │             │
│  │ 6阶段流水线│ │ 5种引擎模式│ │ SWRR+槽位租约│ │ LLM多后端驱动 │             │
│  └───────────┘ └───────────┘ └────────────┘ └──────────────┘             │
│  ┌───────────┐ ┌───────────┐ ┌────────────┐                              │
│  │ queue     │ │ governance│ │ defects    │                              │
│  │ 持久化队列 │ │ 缺陷治理   │ │ L1/L2 指纹 │                              │
│  └───────────┘ └───────────┘ └────────────┘                              │
└─────────────────────────────────┬────────────────────────────────────────┘
                                  │ GORM
┌─────────────────────────────────▼────────────────────────────────────────┐
│                     PostgreSQL（生产）/ SQLite（开发）                     │
└──────────────────────────────────────────────────────────────────────────┘
```

### 6 阶段扫描流水线

```
STAGE01          STAGE02            STAGE03              STAGE04
git_sync   →   precondition   →   analysis       →   synthesis
Git 同步        准入校验             AI 分析（引擎）       汇总 + 四级漏斗增量比对
                                                         ↓
STAGE06          STAGE05
finalize   ←   postprocess
持久化（事务）    后处理：打分 / 治理 / issue 归并
```

### 5 种引擎模式

| engine_mode | 引擎实现 | 适用场景 |
| --- | --- | --- |
| `single` | SingleEngine | 单次全仓扫描，小仓库 / 演示场景 |
| `chunked` | ChunkedEngine | 经典分片并行扫描，大仓库默认模式 |
| `chunked_fast` | ChunkedEngine（轻量分片） | 语义分片快扫，减少 LLM 调用轮次（浮点比较等） |
| `debate_full` | DebateEngine（完整多轮辩论） | 高风险专项：内存泄漏 / Coredump（多轮质询交叉校验） |
| `debate_selective` | DebateEngine（选择性辩论） | 仅高风险分片启动辩论（cJSON 规范扫描等） |

### 核心机制

- **SWRR 加权轮询调度 + LLM 槽位租约**：按模型资源权重分配算力，`Acquire/Release` 租约防止并发超限，支持悬挂槽位一键校准。
- **L1/L2 缺陷指纹 + 四级漏斗增量比对**：L1 物理强指纹 + L2 语义弱指纹，输出 `NEW / EXISTED / RESOLVED / REOPENED` 状态机。
- **插件化任务类型**：`tasks/<name>/` 目录即插件，服务启动扫描 `meta.json` 同步入库，支持 API 动态增删改与激活停用。

## 目录结构

```
code-shield/
├── config.yaml.example          # 配置模板（提交到仓库）
├── config.yaml                  # 实际运行配置（.gitignore 忽略）
├── go.mod / main.go             # Go module 与程序入口
├── Makefile                     # 构建脚本，内置 lint-arch 架构门禁
├── models/                      # 数据模型层（config / db / GORM 模型）
├── handlers/                    # HTTP API 控制器（接口层）
├── services/                    # 核心业务领域层
│   ├── services.go              # 统一门面
│   ├── invoker/                 # LLM 驱动域（claude / opencode / native / agy / codex）
│   ├── dispatcher/              # 算力调度域（SWRR + 租约）
│   ├── queue/                   # 持久化队列域（Worker 池 / 重试）
│   ├── engines/                 # 扫描引擎域（纯内存规约，禁止 DB 访问）
│   ├── runner/                  # 6 阶段流水线协调域
│   ├── governance/              # 缺陷治理域（校准 / 专项 / 误报记忆）
│   └── defects/                 # 缺陷指纹域（锚点 / 指纹 / 增量比对）
├── tasks/                       # 任务类型插件目录（5 套示例插件）
├── frontend/                    # React + TS + Vite 前端工程
├── cron_jobs/                   # 定时任务领域包
├── db/migration/                # 数据库迁移 SQL
├── docs/                        # 项目文档
└── scripts/                     # 部署脚本（deploy / backup / init-db）
```

## 快速开始

### 依赖

| 依赖 | 版本要求 | 说明 |
| --- | --- | --- |
| Go | 1.21+ | 后端编译运行 |
| Node.js / npm | 18+ | 前端构建 |
| PostgreSQL | 14+ | 生产数据库（开发可换 SQLite） |
| LLM 后端 | 可选 | native HTTP / Claude CLI / OpenCode CLI / Codex CLI / Agy CLI |

### 1. 配置

```bash
cp config.yaml.example config.yaml
# 编辑 config.yaml：填写 database 连接信息、llm.resources 的 model 与 api_key
```

### 2. 初始化数据库

```bash
./scripts/init-db.sh --create-db    # 创建用户+数据库并执行建表迁移
# 或手动执行: psql -h 127.0.0.1 -U code_shield -d code_shield -f db/migration/001_init_schema.sql
```

### 3. 启动后端

```bash
go mod download
go run .                            # 默认监听 :8080
```

### 4. 启动前端（开发模式）

```bash
cd frontend
npm install
npm run dev                         # Vite 开发服务器，API 代理到后端
```

### 5. 构建产物

```bash
make build                          # 编译后端 code-shield-server（注入 git 版本）
cd frontend && npm run build        # 生成前端静态资源 frontend/dist
```

## 插件开发指南

### 插件目录结构

```
tasks/<task-name>/
├── meta.json             # 插件元数据（引擎模式、扫描配置、治理模式）
├── analysis_prompt.md    # 分析阶段提示词（必须约定 JSON 数组输出契约）
└── synthesis_prompt.md   # 汇总阶段提示词（输出 Markdown 报告）
```

服务启动时自动扫描 `tasks/` 目录，校验 `meta.json` 并同步入库 `task_types` 表；也可通过 `/api/task-types` 动态创建、修改、删除与激活停用。

### meta.json 字段说明

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| name | string | 是 | 插件唯一标识，须与目录名一致 |
| display_name | string | 是 | 前端展示名称 |
| description | string | 是 | 插件用途描述 |
| engine_mode | string | 是 | `single` / `chunked` / `chunked_fast` / `debate_full` / `debate_selective` |
| engine_config.file_extensions | string[] | 是 | 参与扫描的文件扩展名 |
| engine_config.content_keywords | string[] | 否 | 内容粗筛关键词，命中才纳入扫描 |
| engine_config.exclude_paths | string[] | 否 | 排除的路径关键字（如 thirdparts / vendor / test） |
| engine_config.max_files | number | 否 | 单次扫描最大文件数 |
| engine_config.depth | number | 否 | 目录扫描深度 |
| engine_config.concurrency | number | 否 | 分片并发数 |
| ai_backend | string | 否 | 指定 LLM 资源 ID，空串表示使用默认资源 |
| target_scope | string | 是 | 扫描范围，`business` 表示业务代码 |
| notify_threshold | number | 否 | 缺陷数达到阈值触发通知 |
| timeout | number | 否 | 单任务超时（分钟） |
| is_active | boolean | 是 | 是否激活 |
| governance_mode | string | 是 | `standard` / `defect_tracking` / `feedback` |
| is_campaign | boolean | 是 | 是否专项扫描（前端菜单按此分组） |

### 内置插件

| 插件 | 引擎模式 | 治理模式 | 说明 |
| --- | --- | --- | --- |
| memory_leak | debate_full | defect_tracking | 内存泄漏检测 |
| coredump_risk | debate_full | defect_tracking | Coredump 风险排查 |
| float_comparison | chunked_fast | standard | 浮点比较检测 |
| cjson_scan | debate_selective | standard | cJSON 使用规范扫描 |
| unused_var | chunked | feedback | 未使用变量检测 |

### 提示词编写约定

1. `analysis_prompt.md` 必须要求输出**严格 JSON 数组**，元素字段：`severity / category / file_path / line_number / title / detail / suggestion / confidence`；
2. 强调"只报告真实缺陷、宁缺毋滥、误报率高会被扣分、按严重度排序"；
3. `synthesis_prompt.md` 输出**纯文本 Markdown 报告**，包含总体结论、缺陷统计、TOP 风险点、整改建议、0-100 质量评分；
4. 提示词中的行号以原始文件为准，分片截断场景需谨慎判断并降低置信度。

## API 概览

统一返回格式：

```json
{ "code": 0, "msg": "ok", "data": {}, "traceId": "uuid-string" }
```

分页公共参数：`page`、`pageSize`；分页返回：`{ "list": [], "total": 0, "page": 1, "pageSize": 20 }`。

### 任务接口 `/api/tasks/*`

| Method | Endpoint | 说明 |
| --- | --- | --- |
| POST | `/api/tasks/trigger` | 手动触发扫描任务 |
| POST | `/api/tasks/:id/resume` | 失败分片恢复续跑 |
| GET | `/api/tasks` | 任务列表（分页 + 多状态筛选） |
| GET | `/api/tasks/:id` | 任务详情 |
| DELETE | `/api/tasks/:id` | 删除任务报告（running 任务禁止删除） |
| GET | `/api/tasks/:id/report/export` | 导出报告（markdown / json / excel） |
| POST | `/api/tasks/cancel-running` | 批量取消正在执行任务 |

### 插件管理 `/api/task-types/*`

| Method | Endpoint | 说明 |
| --- | --- | --- |
| GET | `/api/task-types` | 任务类型列表，支持 `active_only=true` |
| POST | `/api/task-types` | 创建任务插件 |
| PUT | `/api/task-types/:id` | 更新插件配置 |
| DELETE | `/api/task-types/:id` | 删除插件 |
| POST | `/api/task-types/:id/activate` | 激活插件 |
| POST | `/api/task-types/:id/deactivate` | 停用插件 |

### 系统诊断 `/api/debug/*`（管理员 JWT）

| Method | Endpoint | 说明 |
| --- | --- | --- |
| GET | `/api/debug` | 系统诊断大盘 |
| POST | `/api/debug/reset-slots` | 槽位校准，释放悬挂租约 |
| GET | `/api/debug/leases` | LLMSlotLease 租约透视 |
| GET | `/api/debug/queue` | 任务队列状态透视 |

## 部署指南

### 一键部署

```bash
sudo ./scripts/deploy.sh            # 环境检查 → 构建前后端 → 安装到 /opt/code-shield → 重启服务
```

### systemd 服务

```bash
sudo cp code-shield.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now code-shield
journalctl -u code-shield -f        # 查看日志
```

服务单元要点：`ExecStart=/opt/code-shield/code-shield-server`，`WorkingDirectory=/opt/code-shield`，`Environment=CONFIG_PATH=/opt/code-shield/config.yaml`，`Restart=on-failure`，日志输出到 journald。

### Nginx 反向代理

```bash
sudo cp nginx.conf.example /etc/nginx/conf.d/code-shield.conf
# 替换 server_name 与 SSL 证书路径后：
sudo nginx -t && sudo systemctl reload nginx
```

配置要点：`/api/` 与 `/` 代理到 `http://127.0.0.1:8080`，`client_max_body_size 50m`，开启 gzip，`/assets/` 静态资源长缓存，SPA 路由 `try_files $uri $uri/ /index.html` 回退。

### 数据备份

```bash
./scripts/backup.sh /var/backups/code-shield   # 数据库 dump + 报告目录 + 日志，按日期打包
# crontab 每日执行，自动保留最近 30 天：
0 2 * * * /opt/code-shield/scripts/backup.sh /var/backups/code-shield >> /var/log/code-shield-backup.log 2>&1
```

## 测试与架构门禁

```bash
go test -race ./...                 # 后端单元测试（含竞态检测）
cd frontend && npm test             # 前端单元测试
make lint                           # 全量检查（架构门禁 + 前端 lint）
make lint-arch                      # 仅架构门禁
```

`lint-arch` 架构门禁强制以下硬性约束，任一违反即构建失败：

1. `services/engines/` **禁止访问 `models.DB / gorm.DB`**，引擎只做纯内存计算，DB 操作全部在 runner 持久化阶段；
2. `services/invoker/` 禁止依赖 `services/engines/`；
3. 依赖方向单向：`runner → engines → invoker`、`runner → defects`，禁止循环依赖与非法反向依赖；
4. `services/` 根目录仅保留 `services.go` 统一门面。

## 常见问题

**Q1：服务启动报数据库连接失败？**
检查 `config.yaml` 中 `database` 配置与 PostgreSQL 实际参数是否一致；首次部署先执行 `./scripts/init-db.sh --create-db` 完成建库建表。

**Q2：前端刷新页面 404 / 白屏？**
Nginx 需要 SPA 路由回退配置：`try_files $uri $uri/ /index.html`（见 `nginx.conf.example`）。

**Q3：任务一直处于 pending 状态？**
依次检查：`scanner.worker_count` 是否大于 0；`queue_tasks` 表是否有积压；LLM 资源是否可达（`/api/debug` 诊断大盘可查看槽位与队列状态）。

**Q4：LLM 槽位悬挂（active slots 不释放）？**
进入 SystemDebug 页面点击"一键校准"，或调用 `POST /api/debug/reset-slots` 释放悬挂租约。

**Q5：修改插件后不生效？**
确认 `meta.json` 为合法 JSON 且 `is_active=true`；服务启动时才会重新扫描 `tasks/` 目录并同步 `task_types` 表，修改后需重启服务，或通过 `/api/task-types` 接口在线更新。

**Q6：扫描结果误报太多？**
可将插件 `governance_mode` 调整为 `feedback` 启用误报记忆库，抑制重复误报；同时收紧 `analysis_prompt.md` 中的报告条件与 confidence 要求。

**Q7：报告导出失败 / 上传大文件报 413？**
检查 Nginx `client_max_body_size`（示例已设为 50m）并 reload；同时确认后端 `proxy_read_timeout` 足够覆盖导出耗时。
