import { useState } from 'react';
import {
  Button,
  DatePicker,
  Dropdown,
  Progress,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import type { MenuProps } from 'antd';
import {
  ClearOutlined,
  DeleteOutlined,
  ExportOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';
import type { Dayjs } from 'dayjs';
import type { Paged, RepoVO, TaskListQuery, TaskTypeVO, TaskVO } from '../../api/types';
import { RUNNING_STATUSES, TASK_STATUS_META } from '../../utils/meta';

/** 报告导出格式 */
export type ReportFormat = 'markdown' | 'json' | 'excel';

interface TaskTableProps {
  loading: boolean;
  data: Paged<TaskVO>;
  taskTypes: TaskTypeVO[];
  repos: RepoVO[];
  /** 查询条件变化（含分页、筛选） */
  onQueryChange: (query: TaskListQuery) => void;
  onDetail: (task: TaskVO) => void;
  onExport: (task: TaskVO, format: ReportFormat) => void;
  onResume: (task: TaskVO) => void;
  onCancel: (task: TaskVO) => void;
  onDelete: (task: TaskVO) => void;
}

/** 状态筛选选项 */
const STATUS_OPTIONS = [
  { value: 'pending', label: '等待中' },
  { value: 'analyzing', label: '分析中' },
  { value: 'success', label: '成功' },
  { value: 'failed', label: '失败' },
];

const EXPORT_ITEMS: MenuProps['items'] = [
  { key: 'markdown', label: 'Markdown (.md)' },
  { key: 'json', label: 'JSON (.json)' },
  { key: 'excel', label: 'Excel (.xlsx)' },
];

/**
 * 任务表格（可复用组件）：筛选栏 + 分页表格 + 行操作。
 * 内部维护筛选/分页状态，变化时通过 onQueryChange 通知父组件拉取数据。
 */
export default function TaskTable({
  loading,
  data,
  taskTypes,
  repos,
  onQueryChange,
  onDetail,
  onExport,
  onResume,
  onCancel,
  onDelete,
}: TaskTableProps) {
  const [status, setStatus] = useState<string>();
  const [repoId, setRepoId] = useState<number>();
  const [taskTypeId, setTaskTypeId] = useState<number>();
  const [range, setRange] = useState<[Dayjs | null, Dayjs | null] | null>(null);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const fmt = (d: Dayjs | null) => (d ? d.format('YYYY-MM-DD HH:mm:ss') : undefined);

  /** 由当前状态 + 覆盖项拼出完整查询参数 */
  const buildQuery = (overrides: Partial<TaskListQuery> = {}): TaskListQuery => ({
    page,
    pageSize,
    status,
    repo_id: repoId,
    task_type_id: taskTypeId,
    start_time: range?.[0] ? fmt(range[0]) : undefined,
    end_time: range?.[1] ? fmt(range[1]) : undefined,
    ...overrides,
  });

  const changeStatus = (v?: string) => {
    setStatus(v);
    onQueryChange(buildQuery({ page: 1, status: v }));
  };
  const changeRepo = (v?: number) => {
    setRepoId(v);
    onQueryChange(buildQuery({ page: 1, repo_id: v }));
  };
  const changeTaskType = (v?: number) => {
    setTaskTypeId(v);
    onQueryChange(buildQuery({ page: 1, task_type_id: v }));
  };
  const changeRange = (v: [Dayjs | null, Dayjs | null] | null) => {
    setRange(v);
    onQueryChange(
      buildQuery({
        page: 1,
        start_time: v?.[0] ? fmt(v[0]) : undefined,
        end_time: v?.[1] ? fmt(v[1]) : undefined,
      }),
    );
  };
  const changePage = (p: number, ps: number) => {
    setPage(p);
    setPageSize(ps);
    onQueryChange(buildQuery({ page: p, pageSize: ps }));
  };
  const resetFilters = () => {
    setStatus(undefined);
    setRepoId(undefined);
    setTaskTypeId(undefined);
    setRange(null);
    setPage(1);
    onQueryChange({ page: 1, pageSize });
  };

  const columns: ColumnsType<TaskVO> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: '仓库', key: 'repo', width: 160, render: (_, r) => r.repo?.name ?? '-' },
    {
      title: '任务类型',
      key: 'task_type',
      width: 140,
      render: (_, r) => r.task_type?.display_name ?? r.task_type?.name ?? '-',
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (v: string) => {
        const meta = TASK_STATUS_META[v] ?? { color: 'default', label: v };
        return <Tag color={meta.color}>{meta.label}</Tag>;
      },
    },
    {
      title: '分片进度',
      key: 'progress',
      width: 190,
      render: (_, r) => {
        const percent = r.total_chunks > 0 ? Math.round((r.processed_chunks / r.total_chunks) * 100) : 0;
        return (
          <Space direction="vertical" size={0} style={{ width: '100%' }}>
            <Progress
              percent={percent}
              size="small"
              status={r.status === 'failed' ? 'exception' : undefined}
            />
            <Space size={4}>
              <Typography.Text type="secondary">
                {r.processed_chunks}/{r.total_chunks}
              </Typography.Text>
              {r.has_failed_chunks && <Tag color="error">失败 {r.failed_chunks}</Tag>}
            </Space>
          </Space>
        );
      },
    },
    { title: '评分', dataIndex: 'score', width: 80, render: (v?: number) => (v != null ? v : '-') },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      width: 170,
      render: (v?: string) => (v ? dayjs(v).format('YYYY-MM-DD HH:mm:ss') : '-'),
    },
    {
      title: '操作',
      key: 'actions',
      width: 230,
      fixed: 'right',
      render: (_, r) => {
        const running = RUNNING_STATUSES.includes(r.status);
        return (
          <Space size={0} wrap>
            <Button size="small" type="link" onClick={() => onDetail(r)}>
              详情
            </Button>
            <Dropdown
              menu={{ items: EXPORT_ITEMS, onClick: ({ key }) => onExport(r, key as ReportFormat) }}
            >
              <Button size="small" type="link" icon={<ExportOutlined />}>
                导出
              </Button>
            </Dropdown>
            {r.has_failed_chunks && (
              <Tooltip title={`${r.failed_chunks} 个失败分片可续跑`}>
                <Button size="small" type="link" danger icon={<PlayCircleOutlined />} onClick={() => onResume(r)}>
                  续跑
                </Button>
              </Tooltip>
            )}
            {running ? (
              <Tooltip title="取消将终止所有运行中任务">
                <Button size="small" type="link" danger icon={<PauseCircleOutlined />} onClick={() => onCancel(r)}>
                  取消
                </Button>
              </Tooltip>
            ) : (
              <Button size="small" type="link" danger icon={<DeleteOutlined />} onClick={() => onDelete(r)}>
                删除
              </Button>
            )}
          </Space>
        );
      },
    },
  ];

  return (
    <>
      {/* 筛选栏 */}
      <Space wrap size={8} style={{ marginBottom: 16 }}>
        <Select
          allowClear
          placeholder="全部状态"
          style={{ width: 120 }}
          options={STATUS_OPTIONS}
          value={status}
          onChange={changeStatus}
        />
        <Select
          allowClear
          placeholder="仓库"
          style={{ width: 180 }}
          showSearch
          optionFilterProp="label"
          options={repos.map((r) => ({ value: r.id, label: r.name }))}
          value={repoId}
          onChange={changeRepo}
        />
        <Select
          allowClear
          placeholder="任务类型"
          style={{ width: 180 }}
          showSearch
          optionFilterProp="label"
          options={taskTypes.map((t) => ({ value: t.id, label: t.display_name }))}
          value={taskTypeId}
          onChange={changeTaskType}
        />
        <DatePicker.RangePicker value={range} onChange={changeRange} />
        <Button icon={<ClearOutlined />} onClick={resetFilters}>
          重置
        </Button>
      </Space>

      {/* 任务表格 */}
      <Table<TaskVO>
        rowKey="id"
        loading={loading}
        columns={columns}
        dataSource={data.list}
        scroll={{ x: 1100 }}
        pagination={{
          current: page,
          pageSize,
          total: data.total,
          showSizeChanger: true,
          showQuickJumper: true,
          showTotal: (t) => `共 ${t} 条`,
          onChange: changePage,
        }}
      />
    </>
  );
}