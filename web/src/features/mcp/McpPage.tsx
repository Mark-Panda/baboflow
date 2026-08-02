import { useCallback, useEffect, useState } from 'react';
import {
  App, Button, Card, Form, Input, Modal, Popconfirm, Radio, Select, Space, Switch,
  Table, Tabs, Tag, Tooltip,
} from 'antd';
import {
  PlusOutlined, ReloadOutlined, DeleteOutlined, ApiOutlined, ThunderboltOutlined,
  EditOutlined, LinkOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';

import { mcpApi, McpServer, McpServerInput, McpExposure, McpTransport } from '@/api/mcp';
import { chainApi, ChainListItem } from '@/api/chain';

const TRANSPORT_LABEL: Record<string, string> = {
  sse: 'SSE',
  stdio: 'Stdio',
  'streamable-http': 'HTTP',
};

// ============ MCP Server 配置 ============

function ServerPanel() {
  const { message } = App.useApp();
  const [data, setData] = useState<McpServer[]>([]);
  const [loading, setLoading] = useState(false);
  const [testing, setTesting] = useState<number | null>(null);

  const [editOpen, setEditOpen] = useState(false);
  const [editing, setEditing] = useState<McpServer | null>(null);
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm<McpServerInput & { argsText?: string; envText?: string }>();
  const transport = Form.useWatch('transport', form);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await mcpApi.listServers();
      setData(res.list || []);
    } catch {
      /* 拦截器已提示 */
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ transport: 'sse' } as never);
    setEditOpen(true);
  };

  const openEdit = (r: McpServer) => {
    setEditing(r);
    form.setFieldsValue({
      name: r.name,
      transport: r.transport,
      endpoint: r.endpoint,
      command: r.command,
      argsText: (r.args || []).join(' '),
      envText: r.env ? JSON.stringify(r.env) : '',
    } as never);
    setEditOpen(true);
  };

  const onSave = async () => {
    const v = await form.validateFields();
    setSaving(true);
    try {
      const args = (v.argsText || '').split(/\s+/).filter(Boolean);
      let env: Record<string, string> | undefined;
      if (v.envText && v.envText.trim()) {
        try {
          env = JSON.parse(v.envText);
        } catch {
          message.error('env 需为 JSON 对象');
          setSaving(false);
          return;
        }
      }
      const payload: McpServerInput = {
        name: v.name, transport: v.transport, endpoint: v.endpoint,
        command: v.command, args, env,
      };
      if (editing) {
        await mcpApi.updateServer(editing.id, payload);
        message.success('已更新');
      } else {
        await mcpApi.createServer(payload);
        message.success('已创建');
      }
      setEditOpen(false);
      load();
    } catch {
      /* 拦截器已提示 */
    } finally {
      setSaving(false);
    }
  };

  const onToggle = async (r: McpServer) => {
    await mcpApi.toggleServer(r.id);
    load();
  };

  const onTest = async (r: McpServer) => {
    setTesting(r.id);
    try {
      const res = await mcpApi.testServer(r.id);
      if (res.ok) {
        Modal.success({
          title: `连接成功 · ${r.name}`,
          content: (
            <div>
              <p>发现 {res.tools?.length || 0} 个工具：</p>
              <div style={{ maxHeight: 300, overflow: 'auto' }}>
                {(res.tools || []).map((t) => <Tag key={t} style={{ marginBottom: 4 }}>{t}</Tag>)}
              </div>
            </div>
          ),
        });
      } else {
        Modal.error({ title: `连接失败 · ${r.name}`, content: res.error || '未知错误' });
      }
    } catch {
      /* 拦截器已提示 */
    } finally {
      setTesting(null);
    }
  };

  const onDelete = async (r: McpServer) => {
    await mcpApi.removeServer(r.id);
    message.success('已删除');
    load();
  };

  const columns: ColumnsType<McpServer> = [
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '传输', dataIndex: 'transport', key: 'transport', width: 90, render: (v: McpTransport) => <Tag>{TRANSPORT_LABEL[v] || v}</Tag> },
    {
      title: '端点 / 命令', key: 'addr', ellipsis: true,
      render: (_, r) => (
        <code style={{ fontSize: 12 }}>{r.transport === 'stdio' ? `${r.command} ${(r.args || []).join(' ')}` : r.endpoint}</code>
      ),
    },
    {
      title: '启用', dataIndex: 'status', key: 'status', width: 80,
      render: (v, r) => <Switch size="small" checked={v === 'enabled'} onChange={() => onToggle(r)} />,
    },
    {
      title: '操作', key: 'op', width: 180, fixed: 'right',
      render: (_, r) => (
        <Space size="small">
          <Tooltip title="测试连接">
            <Button size="small" icon={<ThunderboltOutlined />} loading={testing === r.id} onClick={() => onTest(r)} />
          </Tooltip>
          <Tooltip title="编辑">
            <Button size="small" icon={<EditOutlined />} onClick={() => openEdit(r)} />
          </Tooltip>
          <Popconfirm title="确认删除该 MCP 服务？" onConfirm={() => onDelete(r)}>
            <Button size="small" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <Card
      size="small"
      title="MCP 服务配置"
      extra={
        <Space>
          <Button size="small" icon={<ReloadOutlined />} onClick={load} />
          <Button size="small" type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增服务</Button>
        </Space>
      }
    >
      <Table rowKey="id" size="small" loading={loading} columns={columns} dataSource={data}
        pagination={{ pageSize: 10, showTotal: (t) => `共 ${t} 条` }} />

      <Modal
        title={editing ? '编辑 MCP 服务' : '新增 MCP 服务'}
        open={editOpen}
        onOk={onSave}
        onCancel={() => setEditOpen(false)}
        confirmLoading={saving}
        okText="保存"
        cancelText="取消"
        destroyOnClose
      >
        <Form form={form} layout="vertical" style={{ marginTop: 12 }}>
          <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input placeholder="如 filesystem" />
          </Form.Item>
          <Form.Item name="transport" label="传输方式" rules={[{ required: true }]}>
            <Radio.Group
              options={[
                { value: 'sse', label: 'SSE' },
                { value: 'streamable-http', label: 'Streamable HTTP' },
                { value: 'stdio', label: 'Stdio' },
              ]}
              optionType="button"
            />
          </Form.Item>
          {transport === 'stdio' ? (
            <>
              <Form.Item name="command" label="命令" rules={[{ required: true, message: '请输入命令' }]}>
                <Input placeholder="如 npx" />
              </Form.Item>
              <Form.Item name="argsText" label="参数（空格分隔）">
                <Input placeholder="-y @modelcontextprotocol/server-filesystem /tmp" />
              </Form.Item>
              <Form.Item name="envText" label="环境变量（JSON 对象，可选）">
                <Input.TextArea rows={2} placeholder='{"API_KEY":"..."}' style={{ fontFamily: 'monospace' }} />
              </Form.Item>
            </>
          ) : (
            <>
              <Form.Item name="endpoint" label="端点 URL" rules={[{ required: true, message: '请输入端点 URL' }]}>
                <Input placeholder="http://host:port/sse" />
              </Form.Item>
              <Form.Item name="envText" label="HTTP Headers（JSON 对象，可选）">
                <Input.TextArea rows={2} placeholder='{"Authorization":"Bearer ..."}' style={{ fontFamily: 'monospace' }} />
              </Form.Item>
            </>
          )}
        </Form>
      </Modal>
    </Card>
  );
}

// ============ 规则链暴露为 MCP 工具 ============

function ExposurePanel() {
  const { message } = App.useApp();
  const [data, setData] = useState<McpExposure[]>([]);
  const [loading, setLoading] = useState(false);

  const [exposeOpen, setExposeOpen] = useState(false);
  const [chains, setChains] = useState<ChainListItem[]>([]);
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm<{ chainId: string; toolName: string; description?: string; schemaText?: string }>();

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await mcpApi.listExposures();
      setData(res.list || []);
    } catch {
      /* 拦截器已提示 */
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const openExpose = async () => {
    form.resetFields();
    setExposeOpen(true);
    try {
      const res = await chainApi.list({ status: 'published', pageSize: 200 });
      setChains(res.list || []);
    } catch {
      /* 拦截器已提示 */
    }
  };

  const onExpose = async () => {
    const v = await form.validateFields();
    let schema: Record<string, unknown> | undefined;
    if (v.schemaText && v.schemaText.trim()) {
      try {
        schema = JSON.parse(v.schemaText);
      } catch {
        message.error('入参 schema 需为合法 JSON');
        return;
      }
    }
    setSaving(true);
    try {
      const res = await mcpApi.expose(v.chainId, { toolName: v.toolName, description: v.description, inputSchema: schema });
      Modal.success({
        title: '已暴露为 MCP 工具',
        content: (
          <div>
            <p>工具名：<code>{res.toolName}</code></p>
            <p>MCP 端点（SSE）：<code>{location.origin}{res.mcpEndpoint}/sse</code></p>
            <p style={{ color: '#888', marginBottom: 0 }}>将该地址填入任意 MCP 客户端即可调用此规则链。</p>
          </div>
        ),
      });
      setExposeOpen(false);
      load();
    } catch {
      /* 拦截器已提示 */
    } finally {
      setSaving(false);
    }
  };

  const onRemove = async (r: McpExposure) => {
    await mcpApi.removeExposure(r.id);
    message.success('已取消暴露');
    load();
  };

  const columns: ColumnsType<McpExposure> = [
    { title: '工具名', dataIndex: 'toolName', key: 'toolName', render: (v) => <Tag color="geekblue" icon={<ApiOutlined />}>{v}</Tag> },
    { title: '描述', dataIndex: 'description', key: 'description', ellipsis: true },
    { title: '规则链', dataIndex: 'chainId', key: 'chainId', width: 180, render: (v) => <code style={{ fontSize: 12 }}>{v}</code> },
    {
      title: '状态', dataIndex: 'enabled', key: 'enabled', width: 90,
      render: (v) => (v ? <Tag color="green">已启用</Tag> : <Tag>停用</Tag>),
    },
    {
      title: '操作', key: 'op', width: 110, fixed: 'right',
      render: (_, r) => (
        <Popconfirm title="取消暴露后外部将无法调用" onConfirm={() => onRemove(r)}>
          <Button size="small" danger icon={<DeleteOutlined />} />
        </Popconfirm>
      ),
    },
  ];

  return (
    <Card
      size="small"
      title="规则链 → MCP 工具"
      extra={
        <Space>
          <Button size="small" icon={<ReloadOutlined />} onClick={load} />
          <Button size="small" type="primary" icon={<LinkOutlined />} onClick={openExpose}>暴露规则链</Button>
        </Space>
      }
    >
      <Table rowKey="id" size="small" loading={loading} columns={columns} dataSource={data}
        pagination={{ pageSize: 10, showTotal: (t) => `共 ${t} 条` }} />

      <Modal
        title="把已发布规则链暴露为 MCP 工具"
        open={exposeOpen}
        onOk={onExpose}
        onCancel={() => setExposeOpen(false)}
        confirmLoading={saving}
        okText="暴露"
        cancelText="取消"
        destroyOnClose
      >
        <Form form={form} layout="vertical" style={{ marginTop: 12 }}>
          <Form.Item name="chainId" label="规则链（仅已发布）" rules={[{ required: true, message: '请选择规则链' }]}>
            <Select
              showSearch
              optionFilterProp="label"
              placeholder="选择已发布的规则链"
              options={chains.map((c) => ({ value: c.id, label: `${c.name} (v${c.version})` }))}
            />
          </Form.Item>
          <Form.Item
            name="toolName"
            label="工具名"
            rules={[
              { required: true, message: '请输入工具名' },
              { pattern: /^[a-zA-Z][a-zA-Z0-9_]*$/, message: '字母开头，仅字母/数字/下划线' },
            ]}
          >
            <Input placeholder="如 temp_alert" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={2} placeholder="工具用途，供 MCP 客户端/LLM 理解" />
          </Form.Item>
          <Form.Item name="schemaText" label="入参 JSON Schema（可选，默认 {data: string}）">
            <Input.TextArea rows={5} placeholder='{"type":"object","properties":{...}}' style={{ fontFamily: 'monospace' }} />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
}

export default function McpPage() {
  return (
    <div className="bf-page">
      <Card>
        <Tabs
          defaultActiveKey="exposures"
          items={[
            { key: 'exposures', label: '规则链暴露', children: <ExposurePanel /> },
            { key: 'servers', label: 'MCP 服务配置', children: <ServerPanel /> },
          ]}
        />
      </Card>
    </div>
  );
}
