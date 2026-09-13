/**
 * 展示元数据映射：Tag 颜色与中文文案（跨页面复用，保证风格统一）
 */

export interface TagMeta {
  color: string;
  label: string;
}

/** 缺陷严重度 */
export const SEVERITY_META: Record<string, TagMeta> = {
  critical: { color: 'magenta', label: '严重' },
  high: { color: 'red', label: '高危' },
  medium: { color: 'orange', label: '中危' },
  low: { color: 'blue', label: '低危' },
  info: { color: 'default', label: '提示' },
};

/** diff 状态：NEW 绿 / EXISTED 蓝 / RESOLVED 灰 / REOPENED 橙 */
export const DIFF_META: Record<string, TagMeta> = {
  NEW: { color: 'green', label: '新增' },
  EXISTED: { color: 'blue', label: '既有' },
  RESOLVED: { color: 'default', label: '已解决' },
  REOPENED: { color: 'orange', label: '重开' },
};

/** 任务状态 */
export const TASK_STATUS_META: Record<string, TagMeta> = {
  pending: { color: 'default', label: '等待中' },
  analyzing: { color: 'processing', label: '分析中' },
  success: { color: 'success', label: '成功' },
  failed: { color: 'error', label: '失败' },
};

/** 槽位租约状态 */
export const LEASE_STATUS_META: Record<string, TagMeta> = {
  active: { color: 'processing', label: '占用中' },
  released: { color: 'default', label: '已释放' },
  expired: { color: 'error', label: '已过期' },
  waiting: { color: 'warning', label: '排队中' },
  queued: { color: 'warning', label: '排队中' },
};

/** 处于运行中的任务状态（不可删除） */
export const RUNNING_STATUSES: readonly string[] = ['pending', 'analyzing'] as const;