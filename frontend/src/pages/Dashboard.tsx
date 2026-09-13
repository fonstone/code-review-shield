import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Card,
  Col,
  Empty,
  Form,
  Input,
  InputNumber,
  List,
  Modal,
  Progress,
  Row,
  Select,
  Space,
  Spin,
  Statistic,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import {
  BugOutlined,
  CloudServerOutlined,
  FileDoneOutlined,
  SyncOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import * as echarts from 'echarts';
import { useNavigate } from 'react-router-dom';
import {
  createTask,
  getDashboardModelStats,
  getDashboardRepoRank,
  getDashboardSummary,
  getDashboardTrends,
  getRepos,
  getTaskTypes,
} from '../api/client';
import type {
  DashboardSummaryVO,
  ModelStatVO,
  RepoRankVO,
  RepoVO,
  TaskTypeVO,
  TrendPointVO,
} from '../api/types';
import { useECharts } from '../hooks/useECharts';

/** 快速触发扫描的表单值 */
interface TriggerForm {
  repo_id: number;
  task_type_id: number;
  since_days?: number;
  diff_base?: string;
}

/** 仪表盘：指标卡片 + 缺陷趋势图 + 仓库质量排行 + 模型调用统计 + 快速触发扫描 */
export default function Dashboard() {
  const navigate = useNavigate();
  const [summary, setSummary] = useState<DashboardSummaryVO | null>(null);
  const [trends, setTrends] = useState<TrendPointVO[]>([]);
  const [repoRank, setRepoRank] = useState<RepoRankVO[]>([]);
  const [modelStats, setModelStats] = useState<ModelStatVO[]>([]);
  const [repos, setRepos] = useState<RepoVO[]>([]);
  const [taskTypes, setTaskTypes] = useState<TaskTypeVO[]>([]);
  const [loading, setLoading] = useState(false);
  const [triggerOpen, setTriggerOpen] = useState(false);
  const [triggerLoading, setTriggerLoading] = useState(false);
  const [triggerForm] = Form.useForm<TriggerForm>();

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [s, t, r, m] = await Promise.all([
        getDashboardSummary(),
        getDashboardTrends(14),
        getDashboardRepoRank(),
        getDashboardModelStats(),
      ]);
      setSummary(s);
      setTrends(t);
      setRepoRank(r);
      setModelStats(m);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  // 预加载下拉选项，打开触发弹窗时无需等待
  useEffect(() => {
    void Promise.all([getRepos(), getTaskTypes(true)]).then(([r, t]) => {
      setRepos(r);
      setTaskTypes(t);
    });
  }, []);

  /** 缺陷密度 = findings / score（score 越低代码质量越差） */
  const density = (r: RepoRankVO) => (r.score > 0 ? r.total_findings / r.score : r.total_findings);
  const rankedRepos = useMemo(() => [...repoRank].sort((a, b) => density(b) - density(a)), [repoRank]);

  // 缺陷趋势折线图：按天新增/修复/重开三条线
  const trendOption = useMemo<echarts.EChartsOption | null>(() => {
    if (trends.length === 0) return null;
    return {
      tooltip: { trigger: 'axis' },
      legend: { data: ['新增', '修复', '重开'] },
      grid: { left: 48, right: 24, top: 48, bottom: 32 },
      xAxis: { type: 'category', boundaryGap: false, data: trends.map((t) => t.date) },
      yAxis: { type: 'value', minInterval: 1 },
      series: [
        {
          name: '新增',
          type: 'line',
          smooth: true,
          symbolSize: 6,
          data: trends.map((t) => t.new),
          itemStyle: { color: '#f5222d' },
        },
        {
          name: '修复',
          type: 'line',
          smooth: true,
          symbolSize: 6,
          data: trends.map((t) => t.fixed),
          itemStyle: { color: '#52c41a' },
        },
        {
          name: '重开',
          type: 'line',
          smooth: true,
          symbolSize: 6,
          data: trends.map((t) => t.reopened),
          itemStyle: { color: '#faad14' },
        },
      ],
    };
  }, [trends]);
  const trendRef = useECharts(trendOption, [trendOption]);

  // 顶部四个指标卡片
  const metricCards = [
    { title: '总任务', value: summary?.total_tasks ?? 0, icon: <FileDoneOutlined />, color: '#1677ff' },
    { title: '运行中任务', value: summary?.running_tasks ?? 0, icon: <SyncOutlined spin />, color: '#f5222d' },
    { title: '活跃 LLM 槽位', value: summary?.active_slots ?? 0, icon: <CloudServerOutlined />, color: '#722ed1' },
    { title: '待处理缺陷', value: summary?.pending_findings ?? 0, icon: <BugOutlined />, color: '#faad14' },
  ];

  const modelColumns: ColumnsType<ModelStatVO> = [
    { title: '资源 ID', dataIndex: 'resource_id', width: 90 },
    { title: '模型', dataIndex: 'model' },
    { title: '调用次数', dataIndex: 'total_calls', width: 110 },
    { title: '失败次数', dataIndex: 'failed_calls', width: 110 },
    {
      title: '失败率',
      dataIndex: 'failure_rate',
      width: 170,
      render: (v: number) => (
        <Space size={8}>
          <Progress percent={Math.round((v ?? 0) * 100)} size="small" status={v > 0.1 ? 'exception' : 'normal'} />
        </Space>
      ),
    },
    {
      title: '平均耗时',
      dataIndex: 'avg_duration_ms',
      width: 110,
      render: (v: number) => (v != null ? `${v} ms` : '-'),
    },
  ];

  const submitTrigger = async (values: TriggerForm) => {
    setTriggerLoading(true);
    try {
      const task = await createTask({
        repo_id: values.repo_id,
        task_type_id: values.task_type_id,
        since_days: values.since_days ?? 7,
        diff_base: values.diff_base || undefined,
      });
      message.success(`任务 #${task.id} 已触发`);
      setTriggerOpen(false);
      triggerForm.resetFields();
      void load(); // 触发后刷新指标
    } finally {
      setTriggerLoading(false);
    }
  };

  return (
    <Spin spinning={loading}>
      <Row gutter={[16, 16]}>
        {metricCards.map((c) => (
          <Col key={c.title} xs={12} lg={6}>
            <Card>
              <Statistic title={c.title} value={c.value} prefix={c.icon} valueStyle={{ color: c.color }} />
            </Card>
          </Col>
        ))}

        <Col span={24}>
          <Row gutter={[16, 16]}>
            {/* 缺陷趋势 */}
            <Col xs={24} lg={16}>
              <Card
                title="近期缺陷趋势（近 14 天）"
                extra={
                  <Space size={8}>
                    <Tag color="red">今日新增 {summary?.today_new ?? 0}</Tag>
                    <Tag color="green">今日修复 {summary?.today_fixed ?? 0}</Tag>
                    <Tag color="orange">今日重开 {summary?.today_reopened ?? 0}</Tag>
                  </Space>
                }
              >
                {trends.length > 0 ? (
                  <div ref={trendRef} style={{ height: 320 }} />
                ) : (
                  <Empty description="暂无趋势数据" />
                )}
              </Card>
            </Col>

            {/* 仓库质量排行：按缺陷密度排序 */}
            <Col xs={24} lg={8}>
              <Card title="仓库质量排行" extra={<Typography.Text type="secondary">按缺陷密度排序</Typography.Text>}>
                <List
                  dataSource={rankedRepos}
                  locale={{ emptyText: '暂无数据' }}
                  renderItem={(item, index) => (
                    <List.Item>
                      <List.Item.Meta
                        avatar={<Tag color={index < 3 ? 'gold' : 'default'}>{index + 1}</Tag>}
                        title={
                          <Space size={4} wrap>
                            <Typography.Text strong>{item.repo_name}</Typography.Text>
                            <Tag color="magenta">严重 {item.critical_count}</Tag>
                            <Tag color="orange">高危 {item.high_count}</Tag>
                          </Space>
                        }
                        description={`缺陷总数 ${item.total_findings} · 评分 ${item.score}`}
                      />
                      <Tag color="geekblue">密度 {density(item).toFixed(2)}</Tag>
                    </List.Item>
                  )}
                />
              </Card>
            </Col>

            {/* AI 模型调用统计 */}
            <Col xs={24} lg={16}>
              <Card title="AI 模型调用统计">
                <Table
                  rowKey="resource_id"
                  columns={modelColumns}
                  dataSource={modelStats}
                  size="small"
                  pagination={false}
                  scroll={{ x: 720 }}
                  locale={{ emptyText: '暂无数据' }}
                />
              </Card>
            </Col>

            {/* 快捷操作 */}
            <Col xs={24} lg={8}>
              <Card title="快捷操作">
                <Space direction="vertical" size={12} style={{ width: '100%' }}>
                  <Button type="primary" block icon={<ThunderboltOutlined />} onClick={() => setTriggerOpen(true)}>
                    快速触发扫描
                  </Button>
                  <Button block onClick={() => navigate('/tasks')}>
                    前往任务管理
                  </Button>
                  <Typography.Text type="secondary">
                    触发后将自动克隆仓库并执行代码扫描，结果可在任务管理查看。
                  </Typography.Text>
                </Space>
              </Card>
            </Col>
          </Row>
        </Col>
      </Row>

      {/* 快速触发扫描任务 Modal */}
      <Modal
        title="快速触发扫描任务"
        open={triggerOpen}
        onCancel={() => setTriggerOpen(false)}
        onOk={() => triggerForm.submit()}
        confirmLoading={triggerLoading}
        okText="触发"
      >
        <Form form={triggerForm} layout="vertical" onFinish={submitTrigger} initialValues={{ since_days: 7 }}>
          <Form.Item name="repo_id" label="仓库" rules={[{ required: true, message: '请选择仓库' }]}>
            <Select
              placeholder="选择仓库"
              showSearch
              optionFilterProp="label"
              options={repos.map((r) => ({ value: r.id, label: `${r.name}（${r.branch}）` }))}
            />
          </Form.Item>
          <Form.Item name="task_type_id" label="任务类型" rules={[{ required: true, message: '请选择任务类型' }]}>
            <Select
              placeholder="选择任务类型"
              options={taskTypes.map((t) => ({ value: t.id, label: t.display_name }))}
            />
          </Form.Item>
          <Form.Item name="since_days" label="扫描时间范围（天）">
            <InputNumber min={1} max={365} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="diff_base" label="diff 基准（可选）">
            <Input placeholder="如：main" />
          </Form.Item>
        </Form>
      </Modal>
    </Spin>
  );
}