import { useCallback, useEffect, useState } from 'react';
import { Card, Select, Space, Table, Tag, Button } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';

import { auditApi, AuditLog, AUDIT_ACTIONS, auditActionLabel } from '@/api/audit';
import { toSafeNumber } from '@/api/http';

const ACTION_COLOR: Record<string, string> = {
  'auth.login_failed': 'red',
  'chain.delete': 'red',
  'skill.delete': 'red',
  'llm.delete': 'red',
  'mcp.remove_exposure': 'orange',
  'chain.offline': 'orange',
  'chain.publish': 'green',
  'auth.login': 'blue',
};

export default function AuditPage() {
  const [data, setData] = useState<AuditLog[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [action, setAction] = useState<string | undefined>(undefined);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await auditApi.list({ action, page, pageSize });
      setData(res.list || []);
      setTotal(res.page ? toSafeNumber(res.page.total, 'audit total') : 0);
    } catch {
      /* 拦截器已提示 */
    } finally {
      setLoading(false);
    }
  }, [action, page, pageSize]);

  useEffect(() => {
    load();
  }, [load]);

  const columns: ColumnsType<AuditLog> = [
    { title: 'ID', dataIndex: 'id', key: 'id', width: 70 },
    {
      title: '操作', dataIndex: 'action', key: 'action', width: 150,
      render: (v: string) => <Tag color={ACTION_COLOR[v] || 'default'}>{auditActionLabel(v)}</Tag>,
    },
    { title: '对象类型', dataIndex: 'targetType', key: 'targetType', width: 120 },
    {
      title: '对象', dataIndex: 'targetId', key: 'targetId', width: 200, ellipsis: true,
      render: (v: string) => (v ? <code style={{ fontSize: 12 }}>{v}</code> : '—'),
    },
    {
      title: '详情', dataIndex: 'detail', key: 'detail', ellipsis: true,
      render: (v?: Record<string, unknown>) =>
        v && Object.keys(v).length > 0 ? (
          <code style={{ fontSize: 12 }}>{JSON.stringify(v)}</code>
        ) : (
          '—'
        ),
    },
    { title: 'IP', dataIndex: 'ip', key: 'ip', width: 130 },
    {
      title: '时间', dataIndex: 'createdAt', key: 'createdAt', width: 170,
      render: (v: string) => dayjs(v).format('YYYY-MM-DD HH:mm:ss'),
    },
  ];

  return (
    <div className="bf-page">
      <Card
        title="审计日志"
        extra={
          <Space>
            <Select
              allowClear
              placeholder="操作类型"
              style={{ width: 170 }}
              value={action}
              onChange={(v) => { setAction(v); setPage(1); }}
              options={AUDIT_ACTIONS}
            />
            <Button icon={<ReloadOutlined />} onClick={load} />
          </Space>
        }
      >
        <Table
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={data}
          pagination={{
            current: page, pageSize, total,
            showSizeChanger: true, showTotal: (t) => `共 ${t} 条`,
            onChange: (p, ps) => { setPage(p); setPageSize(ps); },
          }}
        />
      </Card>
    </div>
  );
}
