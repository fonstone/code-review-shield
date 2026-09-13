/**
 * 后端 API 契约类型定义（字段与后端 VO 严格对齐，snake_case 保持一致）
 */

/** 统一响应包装 */
export interface ApiResponse<T> {
  code: number;
  msg: string;
  data: T;
  traceId: string;
}

/** 分页数据结构 */
export interface Paged<T> {
  list: T[];
  total: number;
  page: number;
  pageSize: number;
}

/** 登录结果 */
export interface AuthResultVO {
  token: string;
  username: string;
  role: string;
}

/** 仓库 */
export interface RepoVO {
  id: number;
  name: string;
  url: string;
  branch: string;
}

/** 新建仓库请求体 */
export interface RepoForm {
  name: string;
  url: string;
  branch: string;
}

/** 任务类型 */
export interface TaskTypeVO {
  id: number;
  name: string;
  display_name: string;
  description?: string;
  engine_mode: string;
  engine_config?: string;
  ai_backend?: string;
  is_active: boolean;
  is_campaign: boolean;
  governance_mode?: string;
  notify_threshold?: number;
  timeout_seconds?: number;
  target_scope?: string;
  created_at?: string;
  updated_at?: string;
}

/** 任务类型创建/更新表单 */
export interface TaskTypeForm {
  name: string;
  display_name: string;
  description?: string;
  engine_mode: string;
  engine_config?: string;
  ai_backend?: string;
  is_campaign: boolean;
  governance_mode?: string;
  notify_threshold?: number;
  timeout_seconds?: number;
  target_scope?: string;
}

/** 扫描任务 */
export interface TaskVO {
  id: number;
  repo_id: number;
  task_type_id: number;
  status: string;
  report_path?: string;
  ai_summary?: string;
  score?: number;
  metrics?: TaskMetricsVO;
  clone_status?: string;
  clone_message?: string;
  total_chunks: number;
  processed_chunks: number;
  success_chunks: number;
  failed_chunks: number;
  has_failed_chunks: boolean;
  error_message?: string;
  started_at?: string;
  finished_at?: string;
  created_at: string;
  updated_at: string;
  repo?: RepoVO;
  task_type?: TaskTypeVO;
}

/** 任务指标（含运行轨迹 stages） */
export interface TaskMetricsVO {
  stages: StageInfo[];
  total_findings?: number;
  [key: string]: unknown;
}

/** 运行阶段信息 */
export interface StageInfo {
  stage: string;
  status: 'success' | 'error' | 'processing';
  duration_ms?: number;
  log?: string;
  error?: string;
}

/** 任务详情（列表项字段 + 缺陷清单 + 指标） */
export interface TaskDetailVO extends TaskVO {
  findings: FindingVO[];
  metrics: TaskMetricsVO;
}

/** 缺陷条目 */
export interface FindingVO {
  id: number;
  task_report_id: number;
  severity: string;
  category: string;
  file_path: string;
  line_number: number;
  code_snippet?: string;
  title: string;
  detail?: string;
  suggestion?: string;
  status?: string;
  diff_status?: string;
  l1_fingerprint?: string;
  l2_fingerprint?: string;
  confidence?: number;
  created_at?: string;
}

/** 触发扫描任务请求 */
export interface TriggerTaskReq {
  repo_id?: number;
  repo_name?: string;
  repo_url?: string;
  branch?: string;
  task_type_id: number;
  since_days?: number;
  diff_base?: string;
}

/** 任务列表查询参数 */
export interface TaskListQuery {
  page?: number;
  pageSize?: number;
  status?: string;
  repo_id?: number;
  task_type_id?: number;
  start_time?: string;
  end_time?: string;
}

/** LLM 算力资源 */
export interface ModelResourceVO {
  id: number;
  driver: string;
  model: string;
  concurrent: number;
  active_slots: number;
  load_ratio: number;
  total_calls: number;
  failed_calls: number;
  avg_duration_ms: number;
}

/** Worker 池状态 */
export interface WorkerPoolVO {
  worker_count: number;
  busy_workers: number;
  queue_capacity: number;
  queued: number;
}

/** 运行中任务 */
export interface RunningTaskVO {
  task_report_id: number;
  stage: string;
  status: string;
}

/** 系统诊断信息 */
export interface SystemDiagnosticVO {
  resources: ModelResourceVO[];
  workers: WorkerPoolVO;
  running_tasks: RunningTaskVO[];
}

/** 队列条目 */
export interface QueueItemVO {
  id: number;
  task_report_id: number;
  status: string;
  priority: number;
  attempts: number;
  max_attempts: number;
  next_retry_at?: string;
  created_at: string;
}

/** 队列诊断信息 */
export interface QueueDiagnosticVO {
  worker_count: number;
  busy_workers: number;
  queue_capacity: number;
  queued: number;
  items: QueueItemVO[];
}

/** LLM 槽位租约 */
export interface LLMSlotLeaseVO {
  id: number;
  resource_id: number;
  model: string;
  status: string;
  acquired_at?: string;
  released_at?: string;
  expires_at?: string;
  task_report_id?: number;
  hung: boolean;
}

/** 仪表盘汇总指标 */
export interface DashboardSummaryVO {
  total_tasks: number;
  running_tasks: number;
  active_slots: number;
  pending_findings: number;
  today_new: number;
  today_fixed: number;
  today_reopened: number;
}

/** 趋势点 */
export interface TrendPointVO {
  date: string;
  new: number;
  fixed: number;
  reopened: number;
}

/** 仓库质量排行 */
export interface RepoRankVO {
  repo_id: number;
  repo_name: string;
  total_findings: number;
  critical_count: number;
  high_count: number;
  score: number;
}

/** AI 模型调用统计 */
export interface ModelStatVO {
  resource_id: number;
  model: string;
  total_calls: number;
  failed_calls: number;
  failure_rate: number;
  avg_duration_ms: number;
}