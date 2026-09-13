# Code-Shield 项目全量需求与工程规范文档

>
> 用途：作为AI编码Agent执行工程生成的完整提示词，包含全部需求、架构约束、目录结构、API、数据库、验证标准、执行Phase。Agent必须严格遵守所有约束，按阶段执行，全部验证项通过才算交付完成。

## 项目简介

Code-Shield：基于LLM的代码专项静态扫描平台。
核心能力：插件化任务类型、6阶段Git扫描流水线、5种LLM执行引擎、SWRR加权轮询算力调度+槽位租约、L1/L2缺陷指纹+四级漏斗增量比对、React管理前端。

# 一、验证标准（**全部必须通过**，缺一不可）

## 功能完整性

- ✅ 5种执行引擎实现：`single / chunked / chunked_fast / debate_full / debate_selective`
- ✅ 6阶段流水线完整：**Git同步 → 准入 → 分析 → 汇总 → 后处理 → 持久化**（`services/runner`）
- ✅ SWRR加权轮询调度器 + LLM槽位租约管理（dispatcher+lease）
- ✅ 确定性L1/L2指纹计算 + 四级漏斗增量比对状态机 + Scope范围守卫（defects包）
- ✅ 前端4核心页面：Dashboard / TaskManagement / SystemDebug / ReportViewer（三Tab报告组件）
- ✅ 任务类型插件化机制：`tasks/<name>/meta.json + prompt md`；支持动态创建/编辑/删除/激活停用

## 架构合规性

- ✅ `services/`根目录仅包含`services.go`（统一门面，749行门面代码）
- ✅ 7大领域子包完整：`invoker / dispatcher / queue / engines / runner / governance / defects`
- ✅ 无循环依赖、无非法反向依赖；`make lint-arch` 100%通过
- ✅ `services/engines/` 目录**0处 models.DB/gorm.DB 访问，纯内存规约**；引擎只接收内存结构体输入、返回内存EngineResult，DB操作全部在runner上层
- ✅ 依赖方向单向：`runner → engines → invoker`；`runner → defects`；禁止engines反向import runner/invoker业务逻辑；invoker禁止依赖engines

## 部署就绪性

- ✅ `config.yaml.example` 完整，覆盖server/storage/database/llm/scanner/debate/governance全配置项
- ✅ Makefile 实现：`all / build / install / clean / run / test / lint / lint-arch`，架构门禁不可跳过
- ✅ 前端`package.json`包含`build / dev / lint`脚本
- ✅ 可生成systemd服务单元脚本 `code-shield.service`
- ✅ 可生成Nginx反向代理配置示例 `nginx.conf.example`

# 二、最终交付物清单

1. **完整Git源码工程**
   - Go后端：7大领域子包，6阶段流水线，5种执行引擎，Gin API handlers，GORM模型、数据库迁移SQL
   - React前端：4核心页面 + ReportViewer三Tab组件，api/client.ts，动态菜单menu.ts
   - `tasks/`目录至少5套任务插件（meta.json + analysis_prompt.md + synthesis_prompt.md）
   示例插件：`memory_leak`、`coredump_risk`、`float_comparison`、`cjson_scan`、`unused_var`
2. **构建产物**
   - `code-shield-server` 二进制可执行文件（带git版本LDFLAGS注入）
   - `frontend/dist` 前端静态打包资源
3. **部署配置**
   - `config.yaml.example`
   - Makefile（内置lint-arch架构门禁）
   - `nginx.conf.example`
   - `code-shield.service` systemd服务
4. **验证证据**
   - `go test -race ./...` 全部通过
   - `npm test` 前端单元测试通过
   - `make lint-arch` 架构门禁通过输出
   - 服务启动、API调用、前端页面访问功能验证截图

# 三、目录结构（严格DDD分层）

```
code-shield/
├── config.yaml.example          # 配置模板（提交到仓库）
├── config.yaml                  # 实际运行配置，.gitignore忽略，不提交
├── go.mod                       # Go module依赖定义
├── Makefile                     # 构建脚本，内置lint-arch架构门禁
├── main.go                      # 程序入口：加载配置、初始化DB、注册路由、启动worker队列、http服务
├── models/                      # 数据模型层（数据结构、DB连接、配置结构体）
│   ├── config.go                # viper配置解析，映射config.yaml到内存结构体
│   ├── db.go                    # GORM DB连接池初始化、迁移入口
│   └── models.go                # GORM模型：TaskReport, TaskType, Repository, AnalysisFinding, QueueTask
├── handlers/                    # HTTP API控制器（接口层，只做参数校验、鉴权、调用services门面，无业务逻辑）
│   ├── auth.go                  # JWT中间件、token解析、权限拦截
│   ├── task.go                  # /api/tasks/* 任务触发/恢复/列表/详情/删除/导出/取消
│   ├── task_type.go             # /api/task-types/* 插件CRUD、激活/停用
│   ├── schedule.go              # 定时调度相关接口
│   ├── repo.go                  # 代码仓库管理接口
│   ├── issue.go                 # 缺陷追踪接口
│   ├── report_handler.go        # 报告导出、预览接口
│   └── config.go                # 系统配置读写接口
├── services/                    # 核心业务领域层【7大领域子包】
│   ├── services.go              # 统一门面（749行，暴露所有领域服务实例给handlers）
│   ├── invoker/                 # 【LLM驱动域】AIInvoker接口与多后端驱动注册表
│   │   ├── invoker.go           # AIInvoker接口 + 驱动注册工厂
│   │   ├── common.go            # 进程组管理、超时捕获、流式包装
│   │   ├── claude.go            # Claude CLI驱动
│   │   ├── opencode.go          # OpenCode CLI驱动
│   │   ├── native.go            # Native HTTP（OpenAI兼容API）驱动
│   │   ├── agy.go               # Agy CLI驱动
│   │   └── codex.go             # Codex CLI驱动
│   ├── dispatcher/              # 【算力调度域】SWRR平滑加权轮询 + LLM槽位租约
│   │   ├── dispatcher.go        # ModelDispatcher、SWRR选择逻辑、Acquire/Release
│   │   ├── health.go            # 资源健康心跳检测
│   │   ├── lease.go             # LLMSlotLease租约结构体、租约存储与状态
│   │   └── wrapper.go           # DispatchingInvoker装饰器（自动申请/释放槽位）
│   ├── queue/                   # 【持久化队列域】Worker池、任务抢占、重试
│   │   ├── queue.go             # WorkerPool实现，消费queue_tasks表
│   │   └── types.go             # 队列内部任务结构定义
│   ├── engines/                 # 【扫描引擎域】纯内存规约，禁止DB访问
│   │   ├── engine.go            # TaskEngine顶层接口 + EngineContext/EngineResult
│   │   ├── config.go            # ChunkConfig分片参数定义
│   │   ├── single/              # SingleEngine：single模式引擎
│   │   ├── chunked/             # ChunkedEngine：chunked / chunked_fast模式引擎
│   │   ├── debate/              # DebateEngine：debate_full / debate_selective模式引擎
│   │   └── chunker/             # 语义分片器：代码文件拆分为Chunk
│   ├── runner/                  # 【流水线协调域】6阶段任务生命周期
│   │   ├── runner.go            # 流水线主协调器，编排6阶段
│   │   ├── context.go           # TaskContext：流水线全生命周期内存上下文
│   │   ├── git_sync.go          # STAGE1 Git同步：拉取代码仓
│   │   ├── precondition.go      # STAGE2 准入校验：since_days、diff_base、快速跳过规则
│   │   ├── analysis.go          # STAGE3 AI分析：调用TaskEngine，结果清洗、重试
│   │   ├── synthesis.go         # STAGE4 汇总：调用defects.DiffEngine、四级漏斗、Scope守卫，生成diff_status
│   │   ├── postprocess.go       # STAGE5 后处理：缺陷打分、治理规则、issue归并
│   │   ├── finalize.go          # STAGE6 持久化：单一事务写入task_reports / analysis_findings
│   │   ├── notifier.go          # 通知：邮件/Webhook告警，notify_threshold触发
│   │   └── resume.go            # 断点续跑：恢复失败分片逻辑
│   ├── governance/              # 【缺陷治理域】确定性校准、专项管理、误报记忆
│   │   ├── calibrator.go        # 严重度决策树，自动校准severity
│   │   ├── campaign.go          # 专项扫描归并，campaign分组
│   │   ├── feedback.go          # 误报记忆库，抑制重复误报
│   │   └── taxonomy.go          # 缺陷分类白名单与标准化分类
│   └── defects/                 # 【缺陷指纹域】源码锚点、L1/L2指纹、增量比对引擎
│       ├── anchor.go            # AST源码锚点提取（ASTSymbol、CleanedLines）
│       ├── diff_engine.go       # 四级漏斗增量比对，NEW/EXISTED/RESOLVED/REOPENED状态机
│       ├── fingerprint.go       # L1物理强指纹、L2语义弱指纹计算
│       └── migration.go         # 指纹迁移工具（历史缺陷指纹回填）
├── tasks/                       # 任务类型插件目录
│   ├── memory_leak/
│   │   ├── meta.json
│   │   ├── analysis_prompt.md
│   │   └── synthesis_prompt.md
│   ├── coredump_risk/
│   │   ├── meta.json
│   │   ├── analysis_prompt.md
│   │   └── synthesis_prompt.md
│   ├── float_comparison/
│   │   ├── meta.json
│   │   ├── analysis_prompt.md
│   │   └── synthesis_prompt.md
│   ├── cjson_scan/
│   │   ├── meta.json
│   │   ├── analysis_prompt.md
│   │   └── synthesis_prompt.md
│   └── unused_var/
│       ├── meta.json
│       ├── analysis_prompt.md
│       └── synthesis_prompt.md
├── frontend/                    # React+TS+Vite前端工程
│   ├── package.json
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── index.html
│   └── src/
│       ├── main.tsx
│       ├── App.tsx
│       ├── components/
│       │   ├── report/
│       │   │   └── ReportViewer.tsx  # 三Tab：Summary / Findings / Diagnostics + Stepper流水线时序
│       │   ├── task/
│       │   │   └── TaskTable.tsx
│       │   └── analysis/
│       │       └── FindingCard.tsx
│       ├── pages/
│       │   ├── Dashboard.tsx         # 数据看板，ECharts趋势、仓排行、模型统计
│       │   ├── CampaignAnalysis.tsx   # 专项扫描分析页
│       │   ├── TaskManagement.tsx     # 任务列表、详情抽屉、resume/导出/取消
│       │   └── SystemDebug.tsx        # 算力槽位、租约、队列状态，槽位校准按钮
│       ├── api/
│       │   ├── client.ts              # axios封装，JWT拦截，全部API函数
│       │   └── types.ts               # TS接口定义（TaskVO、FindingVO、LeaseVO等）
│       ├── hooks/
│       │   └── useAuth.ts             # 鉴权Hook
│       └── utils/
│           └── menu.ts                # 动态菜单，从/api/task-types拉取并按campaign分组
├── cron_jobs/                    # 定时任务领域包
│   └── jobs.go
├── docs/
│   └── README.md
└── scripts/                      # 部署辅助脚本
    ├── deploy.sh
    ├── backup.sh
    └── init-db.sh
```

## .gitignore

```
# Go
*.out
*.test
code-shield-server
# Config
config.yaml
# Frontend
frontend/dist
frontend/node_modules
# Runtime
tmp/
logs/
# DB
*.sqlite
# IDE
.vscode/
.idea/
```

# 四、后端API Handler规范

统一返回JSON格式（全部接口）

```
{
  "code": 0,
  "msg": "ok",
  "data": {},
  "traceId": "uuid-string"
}
```

分页公共请求参数：`page, pageSize`；分页返回包装：`{list: [], total, page, pageSize}`

## handlers/task.go（必须实现endpoint）

| Method | Endpoint                     | 说明                                |
| ------ | ---------------------------- | ----------------------------------- |
| POST   | /api/tasks/trigger           | 手动触发扫描任务                    |
| POST   | /api/tasks/:id/resume        | 失败分片恢复续跑                    |
| GET    | /api/tasks                   | 任务列表（分页+多状态筛选）         |
| GET    | /api/tasks/:id               | 任务详情                            |
| DELETE | /api/tasks/:id               | 删除任务报告（running任务禁止删除） |
| GET    | /api/tasks/:id/report/export | 导出报告，支持markdown/json/excel   |
| POST   | /api/tasks/cancel-running    | 批量取消正在执行任务                |

## handlers/task_type.go（插件管理）

| Method | Endpoint                       | 说明                                       |
| ------ | ------------------------------ | ------------------------------------------ |
| GET    | /api/task-types                | 任务类型列表，支持active_only=true查询参数 |
| POST   | /api/task-types                | 创建任务插件                               |
| PUT    | /api/task-types/:id            | 更新插件配置                               |
| DELETE | /api/task-types/:id            | 删除插件                                   |
| POST   | /api/task-types/:id/activate   | 激活插件                                   |
| POST   | /api/task-types/:id/deactivate | 停用插件                                   |

## handlers/system_debug.go（管理员运维接口，JWT管理员权限保护）

| Method | Endpoint               | 说明                   |
| ------ | ---------------------- | ---------------------- |
| GET    | /api/debug             | 系统诊断大盘           |
| POST   | /api/debug/reset-slots | 槽位校准，释放悬挂租约 |
| GET    | /api/debug/leases      | LLMSlotLease租约透视   |
| GET    | /api/debug/queue       | 任务队列状态透视       |

>
> Handler层约束：handler仅负责HTTP参数校验、鉴权、traceId，**禁止在handler内部写业务逻辑，业务下沉service门面**

# 五、前端核心页面与组件

## frontend/src/pages/Dashboard.tsx

- 近期缺陷趋势图表（ECharts折线图，按天统计新增/修复/重开缺陷）
- 代码仓质量排行卡片（缺陷密度排序）
- AI模型调用统计图表（调用次数、失败率、平均耗时，按LLM资源分组）
- 快速触发扫描任务入口
- 全局指标卡片：总任务、运行中任务、活跃LLM槽位、待处理缺陷

## frontend/src/pages/TaskManagement.tsx

- 任务列表表格，状态筛选：pending/analyzing/success/failed；仓库、时间范围筛选；分页
- 每行操作：查看详情、导出报告、失败分片恢复resume、取消、删除
- 任务详情抽屉（内嵌ReportViewer）
- 缺陷条目一键复制文件名+行号剪贴板
- 失败任务高亮失败分片数量，提供Resume按钮

## frontend/src/pages/SystemDebug.tsx

- 算力槽位实时透视表：每个ModelResource最大并发、活跃槽位、负载比例
- 一键校准活跃槽位按钮（调用reset-slots接口）
- Worker池状态面板：worker数量、繁忙worker、队列容量、当前排队数
- 队列排队进度条
- LLMSlotLease租约详情表格；hung/悬挂租约标红

## frontend/src/components/report/ReportViewer.tsx（可复用组件）

Tab三标签布局

1. Tab1 📑 审计总结报告 Summary：Markdown渲染总结，指标卡片
2. Tab2 📋 详细问题清单 Findings：分页表格；支持严重度筛选、文件路径搜索；点击跳转代码位置；展示L1/L2指纹
3. Tab3 🔬 运行轨迹与诊断：流水线时序流
   - 现代化Stepper：STAGE01 git_sync / STAGE02 precondition / STAGE03 analysis / STAGE04 synthesis / STAGE05 postprocess / STAGE06 finalize
   - 每个阶段：状态标记、耗时、阶段日志预览、错误告警

- 高质感AntD卡片布局，报告下载按钮

## frontend/src/api/client.ts（axios封装）

导出函数清单

```
// Task API
createTask(params: TriggerTaskReq): Promise<TaskVO>
getTaskList(query: TaskListQuery): Promise<Paged<TaskVO>>
getTaskDetail(taskId: number): Promise<TaskDetailVO>
cancelTask(taskId: number): Promise<void>
resumeFailedChunks(taskId: number): Promise<void>
exportReport(taskId: number, format: 'markdown'|'json'|'excel'): Promise<Blob>

// TaskType API
getTaskTypes(activeOnly?:boolean): Promise<TaskTypeVO[]>
createTaskType(payload: TaskTypeForm): Promise<TaskTypeVO>
updateTaskType(id:number, payload: TaskTypeForm): Promise<TaskTypeVO>

// System Debug API
getSystemDebugInfo(): Promise<SystemDiagnosticVO>
resetSlots(): Promise<void>
getLeases(): Promise<LLMSlotLeaseVO[]>
getQueueStatus(): Promise<QueueDiagnosticVO>
```

特性：统一错误拦截、自动toast提示、traceId透传、JWT请求拦截器。

## frontend/src/utils/menu.ts（动态侧边菜单）

逻辑：

1. 页面初始化请求 `/api/task-types?active_only=true`
2. 根据`is_campaign`字段分组：专项扫描、普通扫描两大菜单组
3. 动态生成侧边菜单；菜单数据缓存入sessionStorage，过期重新拉取
4. 路由匹配自动高亮菜单
5. 系统调试页面仅管理员可见

# 六、任务类型插件规范

插件目录：`tasks/<task-name>/`

```
tasks/<task-name>/
├── meta.json
├── analysis_prompt.md
└── synthesis_prompt.md
```

## meta.json Schema

```
{
  "name": "memory_leak",
  "display_name": "内存泄漏检测",
  "description": "专项检测代码中的内存泄漏风险",
  "engine_mode": "debate_full",
  "engine_config": {
    "file_extensions": [".c", ".cpp", ".cc", ".cxx", ".h", ".hpp"],
    "content_keywords": ["malloc", "free", "new", "delete"],
    "exclude_paths": ["thirdparts", "vendor", "test"],
    "max_files": 20,
    "depth": 2,
    "concurrency": 6
  },
  "ai_backend": "",
  "target_scope": "business",
  "notify_threshold": 10,
  "timeout": 60,
  "is_active": true,
  "governance_mode": "defect_tracking",
  "is_campaign": false
}
```

## 5种引擎模式（engine_mode枚举）

- `debate_full`：全量多智能体辩论（内存泄漏/Coredump等高风险专项扫描，多轮质询交叉校验）→ DebateEngine
- `debate_selective`：选择性辩论（仅高风险分片启动辩论，普通分片单模型；cJSON_SCAN/无序集合）→ DebateEngine
- `chunked_fast`：语义分片快扫，轻量分片，减少LLM调用轮次（浮点比较/线程创建）→ ChunkedEngine
- `chunked`：经典分片并行扫描，大仓库默认模式，全文件分片并发 → ChunkedEngine
- `single`：单次全仓扫描，不拆分代码分片，小仓库/演示场景 → SingleEngine

>
> 服务启动扫描tasks目录meta.json，校验并同步入库task_types表；API支持动态新增/修改插件。

# 七、PostgreSQL 核心表DDL

```
-- 任务报告表
CREATE TABLE task_reports (
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

-- 任务类型插件表
CREATE TABLE task_types (
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

-- 缺陷问题表
CREATE TABLE analysis_findings (
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
    diff_status VARCHAR(20), -- NEW/EXISTED/RESOLVED/REOPENED
    created_at TIMESTAMP DEFAULT NOW()
);

-- 持久化队列表
CREATE TABLE queue_tasks (
    id SERIAL PRIMARY KEY,
    task_report_id INTEGER REFERENCES task_reports(id),
    status VARCHAR(20),
    priority INTEGER,
    attempts INTEGER DEFAULT 0,
    max_attempts INTEGER DEFAULT 3,
    next_retry_at TIMESTAMP
);
```

# 八、配置文件 config.yaml.example

```
server:
  port: ":8080"
  external_url: "http://localhost:8080"

storage:
  root: "."

database:
  driver: "postgres"
  host: "127.0.0.1"
  port: 5432
  user: "code_shield"
  password: "CodeShield618!"
  dbname: "code_shield"

llm:
  default_resource: "native"
  resources:
    - id: "native"
      driver: "native"
      model: "deepseek-v4-flash"
      concurrent: 10
      base_url: "https://api.deepseek.com/v1/chat/completions"
      api_key: "sk-your-api-key"

scanner:
  worker_count: 5
  max_queue_size: 2000
  analysis:
    max_retries: 3
    retry_backoff_ms: 2000

debate:
  enabled: true
  fast_pass_enabled: true
  tiers:
    tier1_hunter:
      resource: "native"
      timeout_seconds: 7200
    tier2_challenger:
      resource: "native"
      timeout_seconds: 1200
    tier3_judge:
      resource: "native"
      timeout_seconds: 1800
    tier4_synthesis:
      resource: "native"
      timeout_seconds: 300

governance:
  fingerprint:
    enabled: true
    similarity_threshold: 0.85
  lifecycle:
    scope_guard_enabled: true
    auto_resolve_missing: true
```

# 九、Makefile（含架构防腐门禁 lint-arch）

```
BINARY := code-shield-server
FRONTEND_DIR := frontend
VERSION := $(shell git describe --tags --always --dirty)
LDFLAGS := -X 'main.Version=$(VERSION)'

.PHONY: all build install frontend backend clean run test lint lint-arch

all: build

install: frontend/node_modules

frontend: frontend/dist

frontend/dist:
	cd $(FRONTEND_DIR) && npm run build

backend: $(BINARY)

$(BINARY):
	go build -ldflags "$(LDFLAGS)" -o $(BINARY)

clean:
	rm -rf frontend/dist $(BINARY)

run: build
	./$(BINARY)

test:
	go test ./...

lint:
	make lint-arch
	cd frontend && npm run lint

lint-arch:
	@echo "Checking architecture boundaries..."
	@if grep -rn "models\.DB" services/engines/; then \
		echo "❌ ERROR: engines/ should not access models.DB"; exit 1; fi
	@if grep -rn "code-shield/services/engines" services/invoker/; then \
		echo "❌ ERROR: invoker/ should not depend on engines/"; exit 1; fi
	@echo "✅ ArchGuard Passed"
```

# 十、Agent严格执行顺序（禁止跳阶段，上一阶段全部验收通过，才能进入下一阶段）

## Phase1：初始化工程骨架

1. 创建完整目录树，初始化`go.mod`、`frontend/package.json`
2. 编写`models/`：GORM模型（repositories、task_types、task_reports、analysis_findings、queue_tasks）
3. 编写db迁移SQL（`db/migration/001_init_schema.sql`）
4. 基础中间件：JWT鉴权、traceId、统一返回包装、panic recover> 
> 验收标准：go mod tidy无报错；数据库模型编译无错误

## Phase2：实现 services/runner（6阶段流水线）

6阶段：Git同步 → 准入 → 分析 → 汇总 → 后处理 → 持久化

- runner/主调度、状态管理、任务上下文
- analysis.go 调用TaskEngine接口
- synthesis.go 调用defects.DiffEngine做四级漏斗增量比对、Scope守卫
- resume失败分片恢复逻辑、任务取消逻辑> 
> 验收标准：流水线可顺序执行；可中断恢复；无DB泄漏；engines仅接收内存结构体

## Phase3：实现 services/engines 引擎层（纯内存规约）

1. `engine.go`：`TaskEngine`接口、`EngineContext`、`EngineResult`
2. 实现3个基础引擎：SingleEngine、ChunkedEngine、DebateEngine
3. 在引擎内部区分5种引擎模式分支：
   - `single` → SingleEngine
   - `chunked` → ChunkedEngine
   - `chunked_fast` → ChunkedEngine（轻量快速配置）
   - `debate_full` → DebateEngine（完整多轮辩论）
   - `debate_selective` → DebateEngine（仅高风险分片辩论）> 
   
   > 验收标准：`grep models.DB services/engines/` 返回空；无DB读写；不反向依赖runner

## Phase4：实现调度、队列、LLM调用层

1. `services/dispatcher`：ModelResource、SWRR加权轮询、Acquire/Release、槽位租约LLMSlotLease、DispatchingInvoker包装器、ResetActiveSlots悬挂槽位校准
2. `services/invoker`：底层LLM HTTP调用，AIRequest/AIInvoker接口
3. `services/queue`：持久化任务队列、worker池、限流、重试逻辑，对接queue_tasks表
4. `services/defects`：PhysicalToken、L1/L2指纹计算、DiffEngine四级漏斗、Scope守卫
5. `services/governance`：缺陷生命周期管理> 
> 验收标准：SWRR负载均衡正常；租约异常时槽位自动释放；四级漏斗匹配逻辑正确

## Phase5：Gin API Handlers + 前端React实现

后端 handlers：task.go / task_type.go / system_debug.go，全部endpoint按规范实现
前端：

1. `src/api/client.ts`：API请求封装、JWT拦截器
2. `src/utils/menu.ts`：动态侧边菜单（读取task-types，按is_campaign分组，sessionStorage缓存）
3. 页面：Dashboard.tsx、TaskManagement.tsx、SystemDebug.tsx
4. 组件：`ReportViewer.tsx`三Tab组件，流水线Stepper时序展示> 
> 验收标准：所有API可访问；前端页面渲染正常；报告导出、resume、取消任务、槽位校准功能可用

## Phase6：配置、构建脚本、部署文件

1. `config.yaml.example` 完整配置
2. Makefile，包含`lint-arch`架构门禁
3. `nginx.conf.example` 反向代理配置
4. `code-shield.service` systemd单元文件
5. `tasks/`目录，至少5套任务插件meta.json + prompt文件> 
> 验收标准：make build可编译后端；`cd frontend && npm run build`生成dist静态资源

## Phase7：测试、架构门禁、部署验证

1. `go test -race ./...` 单元测试
2. 前端`npm test`
3. `make lint-arch` 架构边界校验
4. 本地完整部署验证：启动服务，触发任务、查看报告、调试算力槽位> 
> 验收标准：全部测试通过，门禁无报错，前后端联调正常

# 十一、全局硬性约束（全程强制执行）

1. engines包**绝对禁止访问models.DB/gorm.DB**，lint-arch会校验
2. 依赖方向单向，禁止循环依赖；invoker不能import engines
3. 所有数据库写入逻辑放在runner/persistence阶段，引擎只做内存计算
4. 任务插件meta.json在服务启动时扫描并同步入库task_types表
5. analysis_findings.diffStatus 由defects.DiffEngine在汇总阶段输出

