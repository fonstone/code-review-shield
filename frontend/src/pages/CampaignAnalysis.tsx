import { useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Col,
  Empty,
  Form,
  Input,
  InputNumber,
  List,
  Modal,
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
import { ThunderboltOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';
import { useParams } from 'react-router-dom';
import { createTask, getRepos, getTaskDetail, getTaskList, getTaskTypes } from '../api/client';
import type { RepoVO, TaskDetailVO, TaskTypeVO, TaskVO } from '../api/types';
import FindingCard from '../components/analysis/FindingCard';
import { filterCampaignByName, getMenuFromCache } from '../utils/menu';
import { SEVERITY_META, TASK_STATUS_META } from '../utils/meta';

/** 触发扫描表单值 */
interface TriggerForm {
  repo_id: number;
  since_days?: number;
  diff_base?: string;
}

/** 严重度排序权重（用于挑选 Top 缺陷） */
const SEVERITY_WEIGHT: Record<string, number> = {
  critical: 0,
  high: 1,
  medium: 2,
  low: 3,
  info: 4,
};

/** 专项扫描分析页：读取 /campaign/:name 参数，聚合该专项最新报告 */
export default function CampaignAnalysis() {
  const { name = '' } = useParams();
  const [taskType, setTaskType] = useState<TaskTypeVO | null>(null);
  const [cachedName, setCachedName] = useState<string | undefined>();
  const [latest, setLatest] = useState<TaskDetailVO | null>(null);
  const [recentTasks, setRecentTasks] = useState<TaskVO[]>([]);
  const [totalReports, setTotalReports] = useState(0);
  const [repos, setRepos] = useState<RepoVO[]>([]);
  const [loading, setLoading] = useState(false);
  const [triggerOpen, setTriggerOpen] = useState(false);
  const [triggerLoading, setTriggerLoading] = useState(false);
  const [triggerForm] = Form.useForm<TriggerForm>();

  // 从菜单缓存取 display_name 兜底（缓存中无完整 TaskTypeVO）
  useEffect(() => {
    const groups = getMenuFromCache();
    const item = groups?.flatMap((g) => g.items).find((i) => i.name === name);
    setCachedName(item?.display_name);
  }, [name]);

  // 拉取任务类型 + 该专项最近的成功报告做前端聚合
  useEffect(() => {
    (async () => {
      setLoading(true);
      try {
        const types = await getTaskTypes(true);
        const type = filterCampaignByName(types, name);
        setTaskType(type ?? null);
        if (!type) return;
        const page = await getTaskList({ task_type_id: type.id, page: 1, pageSize: 10, status: 'success' });
        setTotalReports(page.total);
        // 按创建时间倒序，取最近一份成功报告
        const recent = [...page.list].sort((a, b) =>
          (b.created_at ?? '').localeCompare(a.created_at ?? ''),
        );
        setRecentTasks(recent);
        const top = recent[0];
        if (top) {
          try {
            setLatest(await getTaskDetail(top.id));
          } catch {
            setLatest(null);
          }
        }
      } finally {
        setLoading(false);
      }
    })();
  }, [name]);

  useEffect(() => {
    void getRepos().then(setRepos);
  }, []);

  // 严重度统计
  const severityStats = useMemo(() => {
    const map = new Map<string, number>();
    for (const f of latest?.findings ?? []) {
      map.set(f.severity, (map.get(f.severity) ?? 0) + 1);
    }
    return map;
  }, [latest]);

  // 文件 Top：按缺陷数降序取前 10
  const fileTop = useMemo(() => {
    const map = new Map<string, number>();
    for (const f of latest?.findings ?? []) {
      map.set(f.file_path, (map.get(f.file_path) ?? 0) + 1);
    }
    return [...map.entries()].sort((a, b) => b[1] - a[1]).slice(0, 10);
  }, [latest]);

  // 按严重度排序取 Top 20 缺陷
  const topFindings = useMemo(
    () =>
      [...(latest?.findings ?? [])]
        .sort((a, b) => (SEVERITY_WEIGHT[a.severity] ?? 9) - (SEVERITY_WEIGHT[b.severity] ?? 9))
        .slice(0, 20),
    [latest],
  );

  const recentColumns: ColumnsType<TaskVO> = [
    { title: 'ID', dataIndex: 'id', width: 80 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (v: string) => {
        const meta = TASK_STATUS_META[v] ?? { color: 'default', label: v };
        return <Tag color={meta.color}>{meta.label}</Tag>;
      },
    },
    { title: '评分', dataIndex: 'score', width: 90, render: (v?: number) => v ?? '-' },
    {
      title: '分片',
      key: 'chunks',
      width: 120,
      render: (_, r) => `${r.processed_chunks}/${r.total_chunks}`,
    },
    { title: '完成时间', dataIndex: 'finished_at', render: (v?: string) => (v ? dayjs(v).format('YYYY-MM-DD HH:mm:ss') : '-') },
  ];

  const submitTrigger = async (values: TriggerForm) => {
    if (!taskType) return;
    setTriggerLoading(true);
    try {
      const task = await createTask({
        repo_id: values.repo_id,
        task_type_id: taskType.id,
        since_days: values.since_days ?? 7,
        diff_base: values.diff_base || undefined,
      });
      message.success(`任务 #${task.id} 已触发，可在任务管理查看`);
      setTriggerOpen(false);
      triggerForm.resetFields();
    } finally {
      setTriggerLoading(false);
    }
  };

  return (
    <Spin spinning={loading}>
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Card
          title={taskType ? `${taskType.display_name}（${taskType.name}）` : `专项扫描：${cachedName ?? name}`}
          extra={
            <Space>
              <Typography.Text type="secondary">历史成功报告 {totalReports} 份</Typography.Text>
              <Button
                type="primary"
                icon={<ThunderboltOutlined />}
                disabled={!taskType}
                onClick={() => setTriggerOpen(true)}
              >
                触发扫描
              </Button>
            </Space>
          }
        >
          {taskType ? (
            <Row gutter={[16, 16]}>
              <Col xs={12} lg={6}>
                <Card size="small">
                  <Statistic title="最新报告评分" value={latest?.score ?? 0} suffix="/100" />
                </Card>
              </Col>
              <Col xs={12} lg={6}>
                <Card size="small">
                  <Statistic title="缺陷总数" value={latest?.findings.length ?? 0} />
                </Card>
              </Col>
              <Col xs={12} lg={6}>
                <Card size="small">
                  <Statistic
                    title="严重缺陷"
                    value={severityStats.get('critical') ?? 0}
                    valueStyle={{ color: '#f5222d' }}
                  />
                </Card>
              </Col>
              <Col xs={12} lg={6}>
                <Card size="small">
                  <Statistic
                    title="高危缺陷"
                    value={severityStats.get('high') ?? 0}
                    valueStyle={{ color: '#fa541c' }}
                  />
                </Card>
              </Col>
            </Row>
          ) : (
            <Alert
              type="warning"
              showIcon
              message={`未找到专项任务类型：${name}`}
              description="请确认任务类型名称与后端配置一致。"
            />
          )}
        </Card>

        <Row gutter={[16, 16]}>
          {/* 严重度统计 */}
          <Col xs={24} lg={12}>
            <Card title="严重度分布" size="small">
              <Space wrap>
                {[...severityStats.entries()].map(([sev, count]) => {
                  const meta = SEVERITY_META[sev] ?? { color: 'default', label: sev };
                  return (
                    <Tag key={sev} color={meta.color}>
                      {meta.label} ×{count}
                    </Tag>
                  );
                })}
                {severityStats.size === 0 && (
                  <Typography.Text type="secondary">暂无缺陷数据</Typography.Text>
                )}
              </Space>
            </Card>
          </Col>

          {/* 文件 Top */}
          <Col xs={24} lg={12}>
            <Card title="文件 Top（按缺陷数）" size="small">
              <List
                size="small"
                dataSource={fileTop}
                locale={{ emptyText: '暂无数据' }}
                renderItem={([file, count], index) => (
                  <List.Item>
                    <List.Item.Meta
                      avatar={<Tag color={index < 3 ? 'gold' : 'default'}>{index + 1}</Tag>}
                      title={<Typography.Text code>{file}</Typography.Text>}
                      description={`${count} 个缺陷`}
                    />
                  </List.Item>
                )}
              />
            </Card>
          </Col>
        </Row>

        {/* 最新报告缺陷 Top */}
        <Card title={latest ? `最新报告 #${latest.id} 缺陷 Top` : '缺陷明细'} size="small">
          {topFindings.length > 0 ? (
            <Space direction="vertical" size={12} style={{ width: '100%' }}>
              {topFindings.map((finding) => (
                <FindingCard key={finding.id} finding={finding} />
              ))}
            </Space>
          ) : (
            <Empty description="暂无缺陷" />
          )}
        </Card>

        {/* 最近成功报告 */}
        <Card title="最近成功报告" size="small">
          <Table
            rowKey="id"
            size="small"
            columns={recentColumns}
            dataSource={recentTasks}
            pagination={false}
            locale={{ emptyText: '暂无报告' }}
          />
        </Card>
      </Space>

      {/* 触发扫描 Modal */}
      <Modal
        title={`触发扫描：${taskType?.display_name ?? name}`}
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