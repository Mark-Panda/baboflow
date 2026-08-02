import { useCallback, useEffect, useState } from 'react';
import {
  App, Button, Card, Input, Modal, Popconfirm, Space, Table, Tooltip,
} from 'antd';
import {
  PlusOutlined, ReloadOutlined, EditOutlined, DeleteOutlined,
  CloudUploadOutlined, CloudDownloadOutlined, SearchOutlined,
  FileTextOutlined, LinkOutlined,
} from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';

import { chainApi, ChainListItem } from '@/api/chain';
import { skillApi } from '@/api/skill';
import StatusTag from '@/components/StatusTag';

export default function ChainListPage() {
  const { message } = App.useApp();
  const navigate = useNavigate();
  const [data, setData] = useState<ChainListItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [keyword, setKeyword] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await chainApi.list({ keyword, page, pageSize });
      setData(res.list || []);
      setTotal(res.total || 0);
    } catch {
      /* 拦截器已提示 */
    } finally {
      setLoading(false);
    }
  }, [keyword, page, pageSize]);

  useEffect(() => {
    load();
  }, [load]);

  const onPublish = async (r: ChainListItem) => {
    await chainApi.publish(r.id);
    message.success(`已发布 v${r.version + 1}`);
    load();
  };
  const onOffline = async (r: ChainListItem) => {
    await chainApi.offline(r.id);
    message.success('已撤销发布');
    load();
  };
  const onDelete = async (r: ChainListItem) => {
    await chainApi.remove(r.id);
    message.success('已删除');
    load();
  };

  // M6：Agent2 反生成 SKILL
  const [genSkillId, setGenSkillId] = useState<string | null>(null);
  const onGenSkill = async (r: ChainListItem) => {
    setGenSkillId(r.id);
    try {
      const s = await skillApi.generateFromChain(r.id);
      Modal.success({
        title: '已生成 SKILL',
        content: (
          <div>
            <p>名称：<code>{s.name}</code></p>
            <p style={{ color: '#888', marginBottom: 0 }}>{s.description || '已保存，可前往 SKILL 页查看。'}</p>
          </div>
        ),
      });
    } catch {
      /* 拦截器已提示 */
    } finally {
      setGenSkillId(null);
    }
  };

  const columns: ColumnsType<ChainListItem> = [
    {
      title: '名称', dataIndex: 'name', key: 'name',
      render: (v, r) => (
        <a onClick={() => navigate(`/chains/${r.id}/edit`)}>{v}</a>
      ),
    },
    { title: '描述', dataIndex: 'description', key: 'description', ellipsis: true },
    { title: '状态', dataIndex: 'status', key: 'status', width: 100, render: (v) => <StatusTag value={v} /> },
    { title: '版本', dataIndex: 'version', key: 'version', width: 80, render: (v) => `v${v}` },
    {
      title: '来源', dataIndex: 'source', key: 'source', width: 90,
      render: (v) => (v === 'agent' ? 'Agent' : '手动'),
    },
    {
      title: '更新时间', dataIndex: 'updatedAt', key: 'updatedAt', width: 170,
      render: (v) => dayjs(v).format('YYYY-MM-DD HH:mm'),
    },
    {
      title: '操作', key: 'op', width: 320, fixed: 'right',
      render: (_, r) => (
        <Space size="small" wrap>
          <Tooltip title="编辑画布">
            <Button size="small" icon={<EditOutlined />} onClick={() => navigate(`/chains/${r.id}/edit`)} />
          </Tooltip>
          {r.status === 'published' ? (
            <>
              <Popconfirm title="撤销发布后该链将停止对外服务" onConfirm={() => onOffline(r)}>
                <Button size="small" icon={<CloudDownloadOutlined />}>撤销</Button>
              </Popconfirm>
              <Tooltip title="Agent2 反生成 SKILL">
                <Button
                  size="small"
                  icon={<FileTextOutlined />}
                  loading={genSkillId === r.id}
                  onClick={() => onGenSkill(r)}
                >
                  SKILL
                </Button>
              </Tooltip>
              <Tooltip title="到 MCP 页暴露为工具">
                <Button size="small" icon={<LinkOutlined />} onClick={() => navigate('/mcp')} />
              </Tooltip>
            </>
          ) : (
            <Button size="small" type="primary" ghost icon={<CloudUploadOutlined />} onClick={() => onPublish(r)}>
              发布
            </Button>
          )}
          <Popconfirm title="确认删除该规则链？" onConfirm={() => onDelete(r)}>
            <Button size="small" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div className="bf-page">
      <Card
        title="规则链"
        extra={
          <Space>
            <Input
              allowClear
              prefix={<SearchOutlined />}
              placeholder="搜索名称 / 描述"
              style={{ width: 240 }}
              onChange={(e) => { setKeyword(e.target.value); setPage(1); }}
            />
            <Button icon={<ReloadOutlined />} onClick={load} />
            <Button type="primary" icon={<PlusOutlined />} onClick={() => navigate('/chains/new/edit')}>
              新建规则链
            </Button>
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
