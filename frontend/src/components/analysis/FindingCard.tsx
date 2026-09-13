import { Card, Space, Tag, Typography } from 'antd';
import { CopyOutlined } from '@ant-design/icons';
import type { FindingVO } from '../../api/types';
import { copyText } from '../../utils/clipboard';
import { DIFF_META, SEVERITY_META } from '../../utils/meta';

interface FindingCardProps {
  finding: FindingVO;
}

/**
 * 缺陷卡片（可复用组件）：严重度 + 标题 + 文件:行号定位 + 代码片段 + 建议。
 */
export default function FindingCard({ finding }: FindingCardProps) {
  const sev = SEVERITY_META[finding.severity] ?? { color: 'default', label: finding.severity };
  const diff = finding.diff_status
    ? DIFF_META[finding.diff_status] ?? { color: 'default', label: finding.diff_status }
    : null;

  return (
    <Card
      size="small"
      title={
        <Space wrap>
          <Tag color={sev.color}>{sev.label}</Tag>
          <Typography.Text strong>{finding.title}</Typography.Text>
          {diff && <Tag color={diff.color}>{diff.label}</Tag>}
        </Space>
      }
      extra={
        <Typography.Text code>
          {finding.file_path}:{finding.line_number}
          <Typography.Text
            style={{ marginLeft: 8, cursor: 'pointer' }}
            onClick={() => copyText(`${finding.file_path}:${finding.line_number}`, '已复制定位信息')}
          >
            <CopyOutlined />
          </Typography.Text>
        </Typography.Text>
      }
    >
      <Space direction="vertical" size={8} style={{ width: '100%' }}>
        <Space size={8} wrap>
          <Tag>{finding.category}</Tag>
          {finding.confidence != null && (
            <Typography.Text type="secondary">
              置信度 {Math.round(finding.confidence * 100)}%
            </Typography.Text>
          )}
        </Space>
        {finding.code_snippet && <pre className="finding-snippet">{finding.code_snippet}</pre>}
        {finding.detail && (
          <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
            {finding.detail}
          </Typography.Paragraph>
        )}
        {finding.suggestion && (
          <Typography.Text style={{ color: '#52c41a' }}>建议：{finding.suggestion}</Typography.Text>
        )}
      </Space>
    </Card>
  );
}