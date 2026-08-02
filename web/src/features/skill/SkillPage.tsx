import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  App, Button, Card, Drawer, Input, Modal, Popconfirm, Select, Space, Table, Tabs, Tag, Tooltip,
} from 'antd';
import {
  PlusOutlined, ReloadOutlined, SearchOutlined, EyeOutlined, DeleteOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';

import { skillApi, Skill } from '@/api/skill';

const SOURCE_LABEL: Record<string, { color: string; text: string }> = {
  component: { color: 'blue', text: '系统组件' },
  chain: { color: 'green', text: '规则链' },
  upload: { color: 'orange', text: '业务组件' },
  agent: { color: 'purple', text: 'Agent' },
};

function SourceTag({ value }: { value: string }) {
  const m = SOURCE_LABEL[value] || { color: 'default', text: value };
  return <Tag color={m.color}>{m.text}</Tag>;
}

export default function SkillPage() {
  const { message } = App.useApp();
  const [data, setData] = useState<Skill[]>([]);
  const [loading, setLoading] = useState(false);
  const [keyword, setKeyword] = useState('');
  const [source, setSource] = useState<string | undefined>(undefined);
  const requestSeq = useRef(0);
  const detailRequestSeq = useRef(0);

  // 查看详情
  const [viewing, setViewing] = useState<Skill | null>(null);
  const [viewLoading, setViewLoading] = useState(false);

  // 上传
  const [uploadOpen, setUploadOpen] = useState(false);
  const [uploadText, setUploadText] = useState('');
  const [uploading, setUploading] = useState(false);

  const load = useCallback(async () => {
    const seq = ++requestSeq.current;
    setLoading(true);
    try {
      const res = await skillApi.list({ source, keyword });
      if (seq === requestSeq.current) {
        setData(res.list || []);
      }
    } catch {
      /* 拦截器已提示 */
    } finally {
      if (seq === requestSeq.current) {
        setLoading(false);
      }
    }
  }, [source, keyword]);

  useEffect(() => {
    load();
  }, [load]);

  const onView = useCallback(async (r: Skill) => {
    const seq = ++detailRequestSeq.current;
    setViewLoading(true);
    setViewing(r);
    try {
      const full = await skillApi.get(r.id);
      if (seq === detailRequestSeq.current) {
        setViewing(full);
      }
    } catch {
      /* 拦截器已提示 */
    } finally {
      if (seq === detailRequestSeq.current) {
        setViewLoading(false);
      }
    }
  }, []);

  const closeViewing = useCallback(() => {
    detailRequestSeq.current += 1;
    setViewing(null);
  }, []);

  const onUpload = async () => {
    if (!uploadText.trim()) {
      message.warning('请粘贴 SKILL.md 内容');
      return;
    }
    setUploading(true);
    try {
      await skillApi.upload(uploadText, 'upload');
      message.success('SKILL 已保存（同名覆盖）');
      setUploadOpen(false);
      setUploadText('');
      load();
    } catch {
      /* 拦截器已提示 */
    } finally {
      setUploading(false);
    }
  };

  const onDelete = useCallback(async (r: Skill) => {
    await skillApi.remove(r.id);
    message.success('已删除');
    load();
  }, [load, message]);

  const columns: ColumnsType<Skill> = useMemo(() => [
    {
      title: '名称', dataIndex: 'name', key: 'name',
      render: (v, r) => (
        <Button type="link" style={{ padding: 0 }} onClick={() => onView(r)}>
          {v}
        </Button>
      ),
    },
    { title: '描述', dataIndex: 'description', key: 'description', ellipsis: true },
    { title: '来源', dataIndex: 'source', key: 'source', width: 90, render: (v) => <SourceTag value={v} /> },
    {
      title: '关联规则链', dataIndex: 'chainId', key: 'chainId', width: 160,
      render: (v) => (v ? <code style={{ fontSize: 12 }}>{v}</code> : '—'),
    },
    { title: '创建时间', dataIndex: 'createdAt', key: 'createdAt', width: 170 },
    {
      title: '操作', key: 'op', width: 120, fixed: 'right',
      render: (_, r) => (
        <Space size="small">
          <Tooltip title="查看">
            <Button aria-label="查看 SKILL" size="small" icon={<EyeOutlined />} onClick={() => onView(r)} />
          </Tooltip>
          <Popconfirm title="确认删除该 SKILL？" onConfirm={() => onDelete(r)}>
            <Button aria-label="删除 SKILL" size="small" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ], [onDelete, onView]);

  return (
    <div className="bf-page">
      <Card
        title="SKILL"
        extra={
          <Space>
            <Select
              allowClear
              placeholder="来源"
              style={{ width: 120 }}
              value={source}
              onChange={(v) => setSource(v)}
              options={[
                { value: 'component', label: '系统组件' },
                { value: 'chain', label: '规则链' },
                { value: 'upload', label: '业务组件' },
                { value: 'agent', label: 'Agent' },
              ]}
            />
            <Input
              allowClear
              prefix={<SearchOutlined />}
              placeholder="搜索名称 / 描述"
              style={{ width: 220 }}
              onChange={(e) => setKeyword(e.target.value)}
            />
            <Button aria-label="刷新 SKILL 列表" icon={<ReloadOutlined />} onClick={load} />
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setUploadOpen(true)}>
              上传 SKILL
            </Button>
          </Space>
        }
      >
        <Tabs
          activeKey={source || 'all'}
          onChange={(key) => setSource(key === 'all' ? undefined : key)}
          items={[
            { key: 'all', label: '全部 SKILL' },
            { key: 'component', label: '系统组件' },
            { key: 'upload', label: '业务组件' },
            { key: 'chain', label: '规则链' },
            { key: 'agent', label: 'Agent' },
          ]}
          style={{ marginTop: -8 }}
        />
        <Table
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={data}
          pagination={{ pageSize: 20, showTotal: (t) => `共 ${t} 条` }}
        />
      </Card>

      {/* 查看详情 */}
      <Drawer
        title={viewing ? `SKILL · ${viewing.name}` : 'SKILL'}
        width={720}
        open={!!viewing}
        onClose={closeViewing}
        loading={viewLoading}
      >
        {viewing && (
          <>
            <Space style={{ marginBottom: 12 }} wrap>
              <SourceTag value={viewing.source} />
              {viewing.description && <span style={{ color: '#666' }}>{viewing.description}</span>}
            </Space>
            <pre
              style={{
                background: '#0d1117', color: '#e6edf3', padding: 16, borderRadius: 8,
                overflow: 'auto', fontSize: 13, lineHeight: 1.6, maxHeight: '70vh',
              }}
            >
              {viewing.content || '（无内容）'}
            </pre>
          </>
        )}
      </Drawer>

      {/* 上传 */}
      <Modal
        title="上传 SKILL.md"
        open={uploadOpen}
        onOk={onUpload}
        onCancel={() => setUploadOpen(false)}
        confirmLoading={uploading}
        okText="保存"
        cancelText="取消"
        width={720}
        destroyOnClose
      >
        <p style={{ color: '#888', marginTop: 0 }}>
          需含 YAML frontmatter（<code>name</code> / <code>description</code>）。同名 SKILL 将被覆盖。
        </p>
        <Input.TextArea
          rows={16}
          value={uploadText}
          onChange={(e) => setUploadText(e.target.value)}
          placeholder={'---\nname: my-skill\ndescription: 做什么用\n---\n\n# 使用说明\n…'}
          style={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace', fontSize: 13 }}
        />
      </Modal>
    </div>
  );
}
