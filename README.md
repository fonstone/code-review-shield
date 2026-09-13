# code-review-shield

Code-Shield：基于 LLM 的代码专项静态扫描平台。

- 插件化任务类型（tasks/ 目录动态加载，支持 CRUD/激活停用）
- 6 阶段 Git 扫描流水线（同步 → 准入 → 分析 → 汇总 → 后处理 → 持久化）
- 5 种 LLM 执行引擎（single / chunked / chunked_fast / debate_full / debate_selective）
- SWRR 加权轮询算力调度 + LLM 槽位租约管理
- L1/L2 缺陷指纹 + 四级漏斗增量比对状态机（NEW/EXISTED/RESOLVED/REOPENED）+ Scope 范围守卫
- React 管理前端（Dashboard / TaskManagement / SystemDebug / ReportViewer 三 Tab 报告）

详见 [docs/README.md](docs/README.md)。

## 快速开始

```bash
cp config.yaml.example config.yaml   # 修改 LLM API Key
go build -o code-shield-server .
./code-shield-server                  # 启动后端 :8080

cd frontend && npm install && npm run dev   # 启动前端 :5173
```

## 验证

```bash
go test -race ./...
make lint-arch        # 架构防腐门禁
cd frontend && npm run build && npm test
```