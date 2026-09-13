import { useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Col,
  Dropdown,
  Empty,
  Input,
  Progress,
  Row,
  Select,
  Space,
  Statistic,
  Steps,
  Table,
  Tabs,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
import type { MenuProps } from 'antd';
import { CopyOutlined, DownloadOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import ReactMarkdown from 'react-markdown';
import { exportReport } from '../../api/client';
import type { FindingVO, StageInfo, TaskDetailVO } from '../../api/types';
import { copyText } from '../../utils/clipboard';
import { DIFF_META, SEVERITY_META } from '../../utils/meta';
import type { ReportFormat } from '../task/TaskTable';

interface ReportViewerProps {
  /** 任务详情（含 findings 与 metrics.stages） */
  task: TaskDetailVO;
}

const EXPORT_ITEMS: MenuProps['items'] = [
  { key: 'markdown', label: 'Markdown (.md)' },
  { key: 'json', label: 'JSON (.json)' },
  { key: 'excel', label: 'Excel (.xlsx)' },
];

const EXPORT_EXT: Record<ReportFormat, string> = { markdown: 'md', json: 'json', excel: 'xlsx' };

/** 耗时格式化：>= 1s 显示秒，否则显示毫秒 */
function fmtDuration(ms?: number): string {
  if (ms == null) return '-';
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)} s` : `${ms} ms`;
}

/** 阶段状态 → Steps 状态 */
function stageStepStatus(status: string): 'finish' | 'error' | 'process' | 'wait' {
  if (status === 'success') return 'finish';
  if (status === 'error') return 'error';
  if (status === 'processing') return 'process';
  return 'wait';
}

/**
 * 报告查看器（可复用组件）：三 Tab —— 审计总结 / 详细问题清单 / 运行轨迹。
 */
export default function ReportViewer({ task }: ReportViewerProps) {
  const [severityFilter, setSeverityFilter] = useState<string>();
  const [keyword, setKeyword] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  // 严重度分布（来自缺陷清单）
  const severityCount = useMemo(() => {
    const map = new Map<string, number>();
    for (const f of task.findings) {
      map.set(f.severity, (map.get(f.severity) ?? 0) + 1);
    }
    return map;
  }, [task.findings]);

  // 分片成功率
  const chunkSuccessRate =
    task.processed_chunks > 0 ? Math.round((task.success_chunks / task.processed_chunks) * 100) : 0;

  // 缺陷清单：客户端筛选（严重度 + 文件路径搜索）
  const filteredFindings = useMemo(() => {
    let list = task.findings;
    if (severityFilter) list = list.filter((f) => f.severity === severityFilter);
    if (keyword) {
      const kw = keyword.toLowerCase();
      list = list.filter((f) => f.file_path.toLowerCase().includes(kw));
    }
    return list;
  }, [task.findings, severityFilter, keyword]);

  const findingsColumns: ColumnsType<FindingVO> = [
    {
      title: '严重度',
      dataIndex: 'severity',
      width: 90,
      render: (v: string) => {
        const meta = SEVERITY_META[v] ?? { color: 'default', label: v };
        return <Tag color={meta.color}>{meta.label}</Tag>;
      },
    },
    { title: '标题', dataIndex: 'title', ellipsis: true },
    { title: '文件路径', dataIndex: 'file_path', width: 220, ellipsis: true },
    { title: '行号', dataIndex: 'line_number', width: 70 },
    { title: '分类', dataIndex: 'category', width: 110 },
    {
      title: 'diff 状态',
      dataIndex: 'diff_status',
      width: 100,
      render: (v?: string) => {
        if (!v) return '-';
        const meta = DIFF_META[v] ?? { color: 'default', label: v };
        return <Tag color={meta.color}>{meta.label}</Tag>;
      },
    },
    { title: '状态', dataIndex: 'status', width: 90, render: (v?: string) => (v ? <Tag>{v}</Tag> : '-') },
    {
      title: '置信度',
      dataIndex: 'confidence',
      width: 90,
      render: (v?: number) => (v != null ? `${Math.round(v * 100)}%` : '-'),
    },
    {
      title: '指纹 (L1/L2)',
      key: 'fingerprint',
      width: 150,
      render: (_, f) => (
        <Space size={6}>
          {f.l1_fingerprint ? (
            <Tooltip title={`L1: ${f.l1_fingerprint}`}>
              <Typography.Text code>{f.l1_fingerprint.slice(0, 8)}</Typography.Text>
            </Tooltip>
          ) : (
            <Typography.Text type="secondary">-</Typography.Text>
          )}
          {f.l2_fingerprint ? (
            <Tooltip title={`L2: ${f.l2_fingerprint}`}>
              <Typography.Text code type="secondary">
                {f.l2_fingerprint.slice(0, 8)}
              </Typography.Text>
            </Tooltip>
          ) : null}
        </Space>
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      fixed: 'right',
      render: (_, f) => (
        <Button
          size="small"
          type="link"
          icon={<CopyOutlined />}
          onClick={() => copyText(`${f.file_path}:${f.line_number}`, '已复制定位信息')}
        >
          复制定位
        </Button>
      ),
    },
  ];

  const handleExport = async (format: ReportFormat) => {
    await exportReport(task.id, format);
    message.success(`报告已导出：code-shield-report-${task.id}.${EXPORT_EXT[format]}`);
  };

  const stages: StageInfo[] = task.metrics?.stages ?? [];
  const currentIdx = stages.findIndex((s) => s.status === 'processing');

  // Tab1 审计总结
  const summaryTab = (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Row gutter={[16, 16]}>
        <Col xs={12} lg={6}>
          <Card size="small">
            <Statistic
              title="质量评分"
              value={task.score ?? 0}
              suffix="/100"
              valueStyle={{
                color:
                  (task.score ?? 0) >= 80 ? '#52c41a' : (task.score ?? 0) >= 60 ? '#faad14' : '#f5222d',
              }}
            />
          </Card>
        </Col>
        <Col xs={12} lg={6}>
          <Card size="small">
            <Statistic title="缺陷总数" value={task.findings.length} />
          </Card>
        </Col>
        <Col xs={12} lg={6}>
          <Card size="small" title="严重度分布">
            <Space wrap>
              {[...severityCount.entries()].map(([sev, count]) => {
                const meta = SEVERITY_META[sev] ?? { color: 'default', label: sev };
                return (
                  <Tag key={sev} color={meta.color}>
                    {meta.label} ×{count}
                  </Tag>
                );
              })}
              {severityCount.size === 0 && <Typography.Text type="secondary">无缺陷</Typography.Text>}
            </Space>
          </Card>
        </Col>
        <Col xs={12} lg={6}>
          <Card size="small" title="分片成功率">
            <Progress
              percent={chunkSuccessRate}
              size="small"
              status={chunkSuccessRate >= 100 ? 'success' : chunkSuccessRate === 0 ? 'exception' : 'active'}
            />
            <Typography.Text type="secondary">
              {task.success_chunks}/{task.processed_chunks} 成功 · 失败 {task.failed_chunks}
            </Typography.Text>
          </Card>
        </Col>
      </Row>
      <Card size="small" title="AI 审计总结">
        {task.ai_summary ? (
          <div className="markdown-body">
            <ReactMarkdown>{task.ai_summary}</ReactMarkdown>
          </div>
        ) : (
          <Empty description="暂无审计总结" />
        )}
      </Card>
    </Space>
  );

  // Tab2 详细问题清单
  const findingsTab = (
    <Space direction="vertical" size={12} style={{ width: '100%' }}>
      <Space wrap>
        <Select
          allowClear
          placeholder="严重度筛选"
          style={{ width: 140 }}
          value={severityFilter}
          onChange={(v) => {
            setSeverityFilter(v);
            setPage(1);
          }}
          options={Object.entries(SEVERITY_META).map(([value, meta]) => ({
            value,
            label: meta.label,
          }))}
        />
        <Input.Search
          allowClear
          placeholder="搜索文件路径"
          style={{ width: 260 }}
          value={keyword}
          onChange={(e) => {
            setKeyword(e.target.value);
            setPage(1);
          }}
        />
      </Space>
      <Table<FindingVO>
        rowKey="id"
        size="small"
        columns={findingsColumns}
        dataSource={filteredFindings}
        scroll={{ x: 1100 }}
        pagination={{
          current: page,
          pageSize,
          total: filteredFindings.length,
          showSizeChanger: true,
          onChange: (p, ps) => {
            setPage(p);
            setPageSize(ps);
          },
        }}
        locale={{ emptyText: '暂无缺陷条目' }}
      />
    </Space>
  );

  // Tab3 运行轨迹（Steps 垂直 Stepper + 报告下载）
  const diagnosticsTab = (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Space style={{ justifyContent: 'space-between', width: '100%' }}>
        <Typography.Text strong>任务执行阶段：{stages.length} 个</Typography.Text>
        <Dropdown
          menu={{ items: EXPORT_ITEMS, onClick: ({ key }) => void handleExport(key as ReportFormat) }}
        >
          <Button icon={<DownloadOutlined />}>下载报告</Button>
        </Dropdown>
      </Space>
      {stages.length === 0 ? (
        <Empty description="暂无运行轨迹数据" />
      ) : (
        <Steps
          direction="vertical"
          current={currentIdx >= 0 ? currentIdx : stages.length}
          items={stages.map((s) => ({
            status: stageStepStatus(s.status),
            title: s.stage,
            description: (
              <Space direction="vertical" size={4} style={{ width: '100%' }}>
                <Typography.Text type="secondary">耗时：{fmtDuration(s.duration_ms)}</Typography.Text>
                {s.log ? <pre className="stage-log">{s.log}</pre> : null}
                {s.error ? (
                  <Alert type="error" showIcon message="阶段异常" description={s.error} />
                ) : null}
              </Space>
            ),
          }))}
        />
      )}
    </Space>
  );

  return (
    <Tabs
      defaultActiveKey="summary"
      items={[
        { key: 'summary', label: '审计总结', children: summaryTab },
        { key: 'findings', label: `详细问题清单（${task.findings.length}）`, children: findingsTab },
        { key: 'diagnostics', label: '运行轨迹', children: diagnosticsTab },
      ]}
    />
  );
}