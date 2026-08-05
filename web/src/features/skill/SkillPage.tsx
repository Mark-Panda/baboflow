import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  App, Button, Card, Drawer, Empty, Input, List, Modal, Popconfirm, Select, Space, Table, Tabs, Tag, Tooltip, Upload,
} from 'antd';
import {
  PlusOutlined, ReloadOutlined, SearchOutlined, EyeOutlined, DeleteOutlined,
  InboxOutlined, FileTextOutlined, FolderOutlined, DownloadOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import type { UploadProps } from 'antd';

import { skillApi, Skill, SkillFileItem } from '@/api/skill';
import type { ProtoInt64 } from '@/api/http';

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

function formatSize(value: ProtoInt64): string {
  const bytes = BigInt(value);
  if (bytes < 1024n) return `${bytes} B`;
  const divisor = bytes < 1024n * 1024n ? 1024n : 1024n * 1024n;
  const unit = divisor === 1024n ? 'KB' : 'MB';
  return `${bytes / divisor}.${(bytes % divisor) * 10n / divisor} ${unit}`;
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

  // 包文件（详情内）
  const [files, setFiles] = useState<SkillFileItem[]>([]);
  const [filesLoading, setFilesLoading] = useState(false);
  const [fileContent, setFileContent] = useState<{ path: string; content: string } | null>(null);
  const [fileContentLoading, setFileContentLoading] = useState(false);

  // 上传
  const [uploadOpen, setUploadOpen] = useState(false);
  const [uploadTab, setUploadTab] = useState<'text' | 'package'>('text');
  const [uploadText, setUploadText] = useState('');
  const [uploading, setUploading] = useState(false);
  const [pkgFile, setPkgFile] = useState<File | null>(null);

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

  // 加载包文件清单（仅含包技能）
  const loadFiles = useCallback(async (id: ProtoInt64) => {
    setFilesLoading(true);
    try {
      const res = await skillApi.listFiles(id);
      setFiles(res.list || []);
    } catch {
      setFiles([]);
    } finally {
      setFilesLoading(false);
    }
  }, []);

  const onView = useCallback(async (r: Skill) => {
    const seq = ++detailRequestSeq.current;
    setViewLoading(true);
    setViewing(r);
    setFiles([]);
    setFileContent(null);
    try {
      const full = await skillApi.get(r.id);
      if (seq === detailRequestSeq.current) {
        setViewing(full);
        if (full.hasFiles) {
          loadFiles(full.id);
        }
      }
    } catch {
      /* 拦截器已提示 */
    } finally {
      if (seq === detailRequestSeq.current) {
        setViewLoading(false);
      }
    }
  }, [loadFiles]);

  const closeViewing = useCallback(() => {
    detailRequestSeq.current += 1;
    setViewing(null);
    setFiles([]);
    setFileContent(null);
  }, []);

  const onReadFile = useCallback(async (path: string) => {
    if (!viewing) return;
    setFileContentLoading(true);
    try {
      const res = await skillApi.readFile(viewing.id, path);
      setFileContent(res);
    } catch {
      /* 拦截器已提示 */
    } finally {
      setFileContentLoading(false);
    }
  }, [viewing]);

  const onUploadText = async () => {
    if (!uploadText.trim()) {
      message.warning('请粘贴 SKILL.md 内容');
      return;
    }
    setUploading(true);
    try {
      await skillApi.upload(uploadText, 'upload');
      message.success('SKILL 已保存（同名覆盖）');
      closeUpload();
      load();
    } catch {
      /* 拦截器已提示 */
    } finally {
      setUploading(false);
    }
  };

  const onUploadPackage = async () => {
    if (!pkgFile) {
      message.warning('请选择 .zip 技能包');
      return;
    }
    setUploading(true);
    try {
      await skillApi.uploadPackage(pkgFile, 'upload');
      message.success('技能包已保存（同名覆盖）');
      closeUpload();
      load();
    } catch {
      /* 拦截器已提示 */
    } finally {
      setUploading(false);
    }
  };

  const closeUpload = () => {
    setUploadOpen(false);
    setUploadText('');
    setPkgFile(null);
    setUploadTab('text');
  };

  const draggerProps: UploadProps = {
    accept: '.zip',
    maxCount: 1,
    beforeUpload: (file) => {
      if (!/\.zip$/i.test(file.name)) {
        message.error('仅支持 .zip 技能包');
        return Upload.LIST_IGNORE;
      }
      setPkgFile(file);
      return false; // 不自动上传，点保存时提交
    },
    onRemove: () => setPkgFile(null),
    fileList: pkgFile ? ([{ uid: '-1', name: pkgFile.name, size: pkgFile.size }] as never) : [],
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
        <Space size={6}>
          <Button type="link" style={{ padding: 0 }} onClick={() => onView(r)}>
            {v}
          </Button>
          {r.hasFiles && (
            <Tooltip title="含技能包文件">
              <Tag color="cyan" style={{ marginInlineEnd: 0 }}>📦 含文件</Tag>
            </Tooltip>
          )}
        </Space>
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
        extra={viewing?.hasFiles && (
          <Button
            size="small"
            icon={<DownloadOutlined />}
            href={skillApi.packageUrl(viewing.id)}
            download
          >
            下载技能包
          </Button>
        )}
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
                overflow: 'auto', fontSize: 13, lineHeight: 1.6, maxHeight: '50vh',
              }}
            >
              {viewing.content || '（无内容）'}
            </pre>

            {/* 包文件 */}
            {viewing.hasFiles && (
              <Card
                size="small"
                title="包文件"
                style={{ marginTop: 16 }}
                styles={{ body: { paddingTop: 4, paddingBottom: 4 } }}
              >
                <List
                  size="small"
                  loading={filesLoading}
                  dataSource={files}
                  locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="无文件" /> }}
                  renderItem={(f) => (
                    <List.Item style={{ padding: '6px 0' }}>
                      <Space size={8} style={{ width: '100%', justifyContent: 'space-between' }}>
                        <Space size={8} style={{ minWidth: 0 }}>
                          {f.isDir ? <FolderOutlined /> : <FileTextOutlined />}
                          {f.isDir ? (
                            <span style={{ fontFamily: 'ui-monospace, Menlo, monospace', fontSize: 12 }}>{f.path}/</span>
                          ) : (
                            <Button
                              type="link"
                              size="small"
                              style={{ padding: 0, fontFamily: 'ui-monospace, Menlo, monospace', fontSize: 12 }}
                              onClick={() => onReadFile(f.path)}
                            >
                              {f.path}
                            </Button>
                          )}
                        </Space>
                        {!f.isDir && (
                          <span style={{ color: '#999', fontSize: 12, flexShrink: 0 }}>{formatSize(f.size)}</span>
                        )}
                      </Space>
                    </List.Item>
                  )}
                />
              </Card>
            )}
          </>
        )}
      </Drawer>

      {/* 文件内容查看 */}
      <Modal
        title={fileContent?.path || '文件内容'}
        open={!!fileContent}
        onCancel={() => setFileContent(null)}
        footer={null}
        width={760}
        loading={fileContentLoading}
      >
        <pre
          style={{
            background: '#0d1117', color: '#e6edf3', padding: 16, borderRadius: 8,
            overflow: 'auto', fontSize: 13, lineHeight: 1.6, maxHeight: '64vh', margin: 0,
          }}
        >
          {fileContent?.content}
        </pre>
      </Modal>

      {/* 上传 */}
      <Modal
        title="上传 SKILL"
        open={uploadOpen}
        onOk={uploadTab === 'text' ? onUploadText : onUploadPackage}
        onCancel={closeUpload}
        confirmLoading={uploading}
        okText="保存"
        cancelText="取消"
        width={720}
        destroyOnClose
      >
        <Tabs
          activeKey={uploadTab}
          onChange={(k) => setUploadTab(k as 'text' | 'package')}
          items={[
            {
              key: 'text',
              label: '粘贴文本',
              children: (
                <>
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
                </>
              ),
            },
            {
              key: 'package',
              label: '上传技能包(.zip)',
              children: (
                <>
                  <p style={{ color: '#888', marginTop: 0 }}>
                    上传标准 SKILL 技能包（ZIP，内含 SKILL.md 及 references/ scripts/ 等附属文件）。
                    将自动定位包内 SKILL.md 并解压落盘；同名 SKILL 将被覆盖。
                  </p>
                  <Upload.Dragger {...draggerProps}>
                    <p className="ant-upload-drag-icon"><InboxOutlined /></p>
                    <p className="ant-upload-text">点击或拖拽 .zip 文件到此</p>
                    <p className="ant-upload-hint">仅支持单个 .zip 技能包（≤20MB）</p>
                  </Upload.Dragger>
                </>
              ),
            },
          ]}
        />
      </Modal>
    </div>
  );
}
