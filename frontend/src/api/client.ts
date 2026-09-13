import axios, { AxiosError } from 'axios';
import type { AxiosRequestConfig } from 'axios';
import { message } from 'antd';
import type {
  ApiResponse,
  AuthResultVO,
  DashboardSummaryVO,
  LLMSlotLeaseVO,
  ModelStatVO,
  Paged,
  QueueDiagnosticVO,
  RepoForm,
  RepoRankVO,
  RepoVO,
  SystemDiagnosticVO,
  TaskDetailVO,
  TaskListQuery,
  TaskTypeForm,
  TaskTypeVO,
  TaskVO,
  TrendPointVO,
  TriggerTaskReq,
} from './types';

/** Token 在 localStorage 中的存储键 */
export const TOKEN_KEY = 'code-shield-token';

/** axios 实例：baseURL 统一指向 /api，由 Vite 代理转发到后端 */
const http = axios.create({
  baseURL: '/api',
  timeout: 60_000,
});

// 请求拦截器：注入 Bearer Token
http.interceptors.request.use((config) => {
  const token = localStorage.getItem(TOKEN_KEY);
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// 响应拦截器：解包 {code, msg, data, traceId}，code !== 0 统一报错；401 跳转登录页
http.interceptors.response.use(
  (response) => {
    // 文件流（blob）响应直接透传
    if (response.config.responseType === 'blob') return response;
    const body = response.data as ApiResponse<unknown>;
    if (body.code !== 0) {
      message.error(body.msg || '请求失败');
      return Promise.reject(new Error(body.msg));
    }
    return response;
  },
  (error: AxiosError<unknown>) => {
    const status = error.response?.status;
    if (status === 401 && window.location.pathname !== '/login') {
      localStorage.removeItem(TOKEN_KEY);
      message.error('登录已过期，请重新登录');
      window.location.href = '/login';
      return Promise.reject(error);
    }
    const body = error.response?.data;
    const msg =
      body && typeof body === 'object' && 'msg' in body
        ? (body as { msg?: string }).msg
        : undefined;
    message.error(msg || error.message || '网络错误');
    return Promise.reject(error);
  },
);

/** 通用请求封装：自动解包 ApiResponse 的 data 字段 */
async function request<T>(config: AxiosRequestConfig): Promise<T> {
  const response = await http.request<ApiResponse<T>>(config);
  return response.data.data;
}

/* ------------------------------ 认证 ------------------------------ */

export function login(username: string, password: string): Promise<AuthResultVO> {
  return request({ url: '/auth/login', method: 'post', data: { username, password } });
}

/* ------------------------------ 任务 ------------------------------ */

export function createTask(params: TriggerTaskReq): Promise<TaskVO> {
  return request({ url: '/tasks/trigger', method: 'post', data: params });
}

export function getTaskList(query: TaskListQuery): Promise<Paged<TaskVO>> {
  return request({ url: '/tasks', method: 'get', params: query });
}

export function getTaskDetail(taskId: number): Promise<TaskDetailVO> {
  return request({ url: `/tasks/${taskId}`, method: 'get' });
}

export function deleteTask(taskId: number): Promise<void> {
  return request({ url: `/tasks/${taskId}`, method: 'delete' });
}

export function cancelRunningTasks(): Promise<void> {
  return request({ url: '/tasks/cancel-running', method: 'post' });
}

export function resumeFailedChunks(taskId: number): Promise<void> {
  return request({ url: `/tasks/${taskId}/resume`, method: 'post' });
}

/** 导出报告：按 format 生成文件名 code-shield-report-{taskId}.{md|json|xlsx} 并触发下载 */
export async function exportReport(
  taskId: number,
  format: 'markdown' | 'json' | 'excel',
): Promise<Blob> {
  const response = await http.get<Blob>(`/tasks/${taskId}/report/export`, {
    params: { format },
    responseType: 'blob',
  });
  const blob = response.data;
  const ext = format === 'excel' ? 'xlsx' : format;
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = `code-shield-report-${taskId}.${ext}`;
  link.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  return blob;
}

/* ------------------------------ 任务类型 ------------------------------ */

export function getTaskTypes(activeOnly?: boolean): Promise<TaskTypeVO[]> {
  return request({
    url: '/task-types',
    method: 'get',
    params: { active_only: activeOnly },
  });
}

export function createTaskType(payload: TaskTypeForm): Promise<TaskTypeVO> {
  return request({ url: '/task-types', method: 'post', data: payload });
}

export function updateTaskType(id: number, payload: TaskTypeForm): Promise<TaskTypeVO> {
  return request({ url: `/task-types/${id}`, method: 'put', data: payload });
}

export function deleteTaskType(id: number): Promise<void> {
  return request({ url: `/task-types/${id}`, method: 'delete' });
}

export function activateTaskType(id: number): Promise<void> {
  return request({ url: `/task-types/${id}/activate`, method: 'post' });
}

export function deactivateTaskType(id: number): Promise<void> {
  return request({ url: `/task-types/${id}/deactivate`, method: 'post' });
}

/* ------------------------------ 系统调试 ------------------------------ */

export function getSystemDebugInfo(): Promise<SystemDiagnosticVO> {
  return request({ url: '/debug', method: 'get' });
}

export function resetSlots(): Promise<void> {
  return request({ url: '/debug/reset-slots', method: 'post' });
}

export function getLeases(): Promise<LLMSlotLeaseVO[]> {
  return request({ url: '/debug/leases', method: 'get' });
}

export function getQueueStatus(): Promise<QueueDiagnosticVO> {
  return request({ url: '/debug/queue', method: 'get' });
}

/* ------------------------------ 仪表盘 ------------------------------ */

export function getDashboardSummary(): Promise<DashboardSummaryVO> {
  return request({ url: '/dashboard/summary', method: 'get' });
}

export function getDashboardTrends(days?: number): Promise<TrendPointVO[]> {
  return request({
    url: '/dashboard/trends',
    method: 'get',
    params: { days: days ?? 14 },
  });
}

export function getDashboardRepoRank(): Promise<RepoRankVO[]> {
  return request({ url: '/dashboard/repo-rank', method: 'get' });
}

export function getDashboardModelStats(): Promise<ModelStatVO[]> {
  return request({ url: '/dashboard/model-stats', method: 'get' });
}

/* ------------------------------ 仓库 ------------------------------ */

export function getRepos(): Promise<RepoVO[]> {
  return request({ url: '/repos', method: 'get' });
}

export function createRepo(payload: RepoForm): Promise<RepoVO> {
  return request({ url: '/repos', method: 'post', data: payload });
}