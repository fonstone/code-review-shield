import { useCallback, useEffect, useState } from 'react';
import { Button, Card, Drawer, Modal, Spin, message } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import TaskTable from '../components/task/TaskTable';
import type { ReportFormat } from '../components/task/TaskTable';
import ReportViewer from '../components/report/ReportViewer';
import {
  cancelRunningTasks,
  deleteTask,
  exportReport,
  getRepos,
  getTaskDetail,
  getTaskList,
  getTaskTypes,
  resumeFailedChunks,
} from '../api/client';
import type { Paged, RepoVO, TaskDetailVO, TaskListQuery, TaskTypeVO, TaskVO } from '../api/types';

/** 导出格式 → 文件扩展名 */
const FORMAT_EXT: Record<ReportFormat, string> = { markdown: 'md', json: 'json', excel: 'xlsx' };

/** 任务管理：筛选 + 分页表格 + 详情抽屉（内嵌 ReportViewer） */
export default function TaskManagement() {
  const [query, setQuery] = useState<TaskListQuery>({ page: 1, pageSize: 10 });
  const [data, setData] = useState<Paged<TaskVO>>({ list: [], total: 0, page: 1, pageSize: 10 });
  const [loading, setLoading] = useState(false);
  const [taskTypes, setTaskTypes] = useState<TaskTypeVO[]>([]);
  const [repos, setRepos] = useState<RepoVO[]>([]);
  const [detail, setDetail] = useState<TaskDetailVO | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);

  const fetchList = useCallback(async (q: TaskListQuery) => {
    setLoading(true);
    try {
      setData(await getTaskList(q));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void fetchList(query);
  }, [fetchList, query]);

  // 加载筛选下拉选项
  useEffect(() => {
    void Promise.all([getTaskTypes(), getRepos()]).then(([t, r]) => {
      setTaskTypes(t);
      setRepos(r);
    });
  }, []);

  const handleQueryChange = (q: TaskListQuery) => setQuery(q);

  // 打开详情抽屉并拉取任务详情
  const openDetail = async (task: TaskVO) => {
    setDrawerOpen(true);
    setDetailLoading(true);
    try {
      setDetail(await getTaskDetail(task.id));
    } catch {
      setDetail(null);
    } finally {
      setDetailLoading(false);
    }
  };

  const handleExport = async (task: TaskVO, format: ReportFormat) => {
    await exportReport(task.id, format);
    message.success(`报告已导出：code-shield-report-${task.id}.${FORMAT_EXT[format]}`);
  };

  const handleResume = (task: TaskVO) => {
    Modal.confirm({
      title: '续跑失败分片',
      content: `任务 #${task.id} 有 ${task.failed_chunks} 个失败分片，确认重新执行？`,
      onOk: async () => {
        await resumeFailedChunks(task.id);
        message.success('已提交续跑');
        void fetchList(query);
      },
    });
  };

  const handleCancel = (_task: TaskVO) => {
    Modal.confirm({
      title: '取消任务',
      content: '后端仅提供全局取消接口，将终止所有运行中的任务，确认继续？',
      onOk: async () => {
        await cancelRunningTasks();
        message.success('已取消运行中任务');
        void fetchList(query);
      },
    });
  };

  const handleDelete = (task: TaskVO) => {
    Modal.confirm({
      title: '删除任务',
      content: `确认删除任务 #${task.id}？该操作不可恢复。`,
      okButtonProps: { danger: true },
      onOk: async () => {
        await deleteTask(task.id);
        message.success('已删除');
        void fetchList(query);
      },
    });
  };

  return (
    <Card
      title="任务管理"
      extra={
        <Button icon={<ReloadOutlined />} onClick={() => void fetchList(query)}>
          刷新
        </Button>
      }
    >
      <TaskTable
        loading={loading}
        data={data}
        taskTypes={taskTypes}
        repos={repos}
        onQueryChange={handleQueryChange}
        onDetail={openDetail}
        onExport={handleExport}
        onResume={handleResume}
        onCancel={handleCancel}
        onDelete={handleDelete}
      />

      <Drawer
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        width={960}
        title={detail ? `任务详情 #${detail.id}` : '任务详情'}
      >
        {detailLoading ? (
          <Spin style={{ display: 'block', margin: '80px auto' }} />
        ) : detail ? (
          <ReportViewer task={detail} />
        ) : null}
      </Drawer>
    </Card>
  );
}