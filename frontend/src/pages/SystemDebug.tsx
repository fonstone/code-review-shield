import { useCallback, useEffect, useState } from 'react';
import {
  Button,
  Card,
  Col,
  Modal,
  Progress,
  Row,
  Space,
  Statistic,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import { ReloadOutlined, SlidersOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';
import { getLeases, getQueueStatus, getSystemDebugInfo, resetSlots } from '../api/client';
import type {
  LLMSlotLeaseVO,
  ModelResourceVO,
  QueueDiagnosticVO,
  QueueItemVO,
  SystemDiagnosticVO,
} from '../api/types';
import { LEASE_STATUS_META, TASK_STATUS_META } from '../utils/meta';

const fmtTime = (v?: string) => (v ? dayjs(v).format('YYYY-MM-DD HH:mm:ss') : '-');

/** 系统调试：算力槽位透视 + Worker 池状态 + 队列 + LLM 槽位租约 */
export default function SystemDebug() {
  const [sysInfo, setSysInfo] = useState<SystemDiagnosticVO | null>(null);
  const [leases, setLeases] = useState<LLMSlotLeaseVO[]>([]);
  const [queue, setQueue] = useState<QueueDiagnosticVO | null>(null);
  const [loading, setLoading] = useState(false);

  const loadAll = useCallback(async () => {
    setLoading(true);
    try {
      const [s, l, q] = await Promise.all([getSystemDebugInfo(), getLeases(), getQueueStatus()]);
      setSysInfo(s);
      setLeases(l);
      setQueue(q);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadAll();
  }, [loadAll]);

  // 调试页每 15 秒自动轮询刷新
  useEffect(() => {
    const timer = window.setInterval(() => void loadAll(), 15_000);
    return () => window.clearInterval(timer);
  }, [loadAll]);

  const confirmReset = () => {
    Modal.confirm({
      title: '校准算力槽位',
      content: '将释放所有悬挂的 LLM 槽位租约并重置资源计数，确认继续？',
      onOk: async () => {
        await resetSlots();
        message.success('槽位已校准');
        void loadAll();
      },
    });
  };

  // 算力槽位透视表
  const resourceColumns: ColumnsType<ModelResourceVO> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: '驱动', dataIndex: 'driver', width: 130 },
    { title: '模型', dataIndex: 'model' },
    { title: '最大并发', dataIndex: 'concurrent', width: 90 },
    { title: '活跃槽位', key: 'active', width: 100, render: (_, r) => `${r.active_slots}/${r.concurrent}` },
    {
      title: '负载比例',
      dataIndex: 'load_ratio',
      width: 180,
      render: (v: number) => {
        const percent = Math.round((v ?? 0) * 100);
        return (
          <Progress
            percent={percent}
            size="small"
            status={percent >= 90 ? 'exception' : percent >= 70 ? 'active' : 'normal'}
          />
        );
      },
    },
    { title: '总调用', dataIndex: 'total_calls', width: 100 },
    { title: '失败调用', dataIndex: 'failed_calls', width: 100 },
    { title: '平均耗时', dataIndex: 'avg_duration_ms', width: 110, render: (v: number) => `${v} ms` },
  ];

  // 租约表格（hung 标红）
  const leaseColumns: ColumnsType<LLMSlotLeaseVO> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: '资源', dataIndex: 'resource_id', width: 80 },
    { title: '模型', dataIndex: 'model' },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (v: string) => {
        const meta = LEASE_STATUS_META[v] ?? { color: 'default', label: v };
        return <Tag color={meta.color}>{meta.label}</Tag>;
      },
    },
    { title: '任务', dataIndex: 'task_report_id', width: 90, render: (v?: number) => v ?? '-' },
    { title: '获取时间', dataIndex: 'acquired_at', width: 170, render: fmtTime },
    { title: '释放时间', dataIndex: 'released_at', width: 170, render: fmtTime },
    {
      title: '过期时间',
      dataIndex: 'expires_at',
      width: 170,
      render: (v?: string, r?: LLMSlotLeaseVO) => {
        // 占用中且已过过期时间视为异常，标红提示
        const overdue = (r?.status ?? '') === 'active' && !!v && dayjs(v).isBefore(dayjs());
        return (
          <Typography.Text style={overdue ? { color: '#f5222d' } : undefined}>{fmtTime(v)}</Typography.Text>
        );
      },
    },
    {
      title: '悬挂',
      dataIndex: 'hung',
      width: 80,
      render: (v: boolean) => (v ? <Tag color="error">HUNG</Tag> : '-'),
    },
  ];

  // 队列明细表
  const queueColumns: ColumnsType<QueueItemVO> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: '任务', dataIndex: 'task_report_id', width: 100 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (v: string) => <Tag color={TASK_STATUS_META[v]?.color ?? 'default'}>{v}</Tag>,
    },
    { title: '优先级', dataIndex: 'priority', width: 80 },
    { title: '重试', key: 'attempts', width: 100, render: (_, r) => `${r.attempts}/${r.max_attempts}` },
    { title: '下次重试', dataIndex: 'next_retry_at', width: 170, render: fmtTime },
    { title: '入队时间', dataIndex: 'created_at', width: 170, render: fmtTime },
  ];

  // 队列占用进度
  const queueCapacity = queue?.queue_capacity ?? 0;
  const queuePercent = queueCapacity > 0 ? Math.round(((queue?.queued ?? 0) / queueCapacity) * 100) : 0;

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      {/* 算力槽位透视 */}
      <Card
        title="算力槽位透视"
        extra={
          <Space>
            <Button size="small" icon={<ReloadOutlined />} onClick={() => void loadAll()} loading={loading}>
              刷新
            </Button>
            <Button size="small" danger icon={<SlidersOutlined />} onClick={confirmReset}>
              一键校准槽位
            </Button>
          </Space>
        }
      >
        <Table
          rowKey="id"
          size="small"
          columns={resourceColumns}
          dataSource={sysInfo?.resources ?? []}
          pagination={false}
          locale={{ emptyText: '暂无算力资源' }}
        />
      </Card>

      <Row gutter={[16, 16]}>
        {/* Worker 池状态 + 队列进度 */}
        <Col xs={24} lg={10}>
          <Card title="Worker 池状态">
            <Row gutter={16}>
              <Col span={6}>
                <Statistic title="Worker" value={queue?.worker_count ?? 0} />
              </Col>
              <Col span={6}>
                <Statistic
                  title="繁忙"
                  value={queue?.busy_workers ?? 0}
                  valueStyle={{ color: (queue?.busy_workers ?? 0) > 0 ? '#faad14' : undefined }}
                />
              </Col>
              <Col span={6}>
                <Statistic title="队列容量" value={queue?.queue_capacity ?? 0} />
              </Col>
              <Col span={6}>
                <Statistic
                  title="排队数"
                  value={queue?.queued ?? 0}
                  valueStyle={{ color: (queue?.queued ?? 0) > 0 ? '#f5222d' : undefined }}
                />
              </Col>
            </Row>
            <Typography.Title level={5} style={{ marginTop: 16 }}>
              队列占用
            </Typography.Title>
            <Progress
              percent={queuePercent}
              status={queuePercent >= 90 ? 'exception' : queuePercent >= 70 ? 'active' : 'normal'}
            />
          </Card>
        </Col>

        {/* 队列明细 */}
        <Col xs={24} lg={14}>
          <Card title="队列明细">
            <Table
              rowKey="id"
              size="small"
              columns={queueColumns}
              dataSource={queue?.items ?? []}
              pagination={{ pageSize: 8, showSizeChanger: false }}
              locale={{ emptyText: '队列为空' }}
            />
          </Card>
        </Col>
      </Row>

      {/* LLM 槽位租约 */}
      <Card title="LLM 槽位租约">
        <Table
          rowKey="id"
          size="small"
          columns={leaseColumns}
          dataSource={leases}
          rowClassName={(r) => (r.hung ? 'lease-hung' : '')}
          pagination={{ pageSize: 10 }}
          scroll={{ x: 1000 }}
          locale={{ emptyText: '暂无租约' }}
        />
      </Card>
    </Space>
  );
}