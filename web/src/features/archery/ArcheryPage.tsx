import { useCallback, useEffect, useState } from 'react';
import {
  App, Button, Card, Empty, Form, Input, Modal, Popconfirm,
  Space, Switch, Table, Tag, Tooltip,
} from 'antd';
import {
  PlusOutlined, ThunderboltOutlined, EditOutlined, DeleteOutlined, SyncOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';

import {
  archeryApi, ArcheryConnection, ArcheryConnectionInput, ArcheryInstance,
} from '@/api/archery';

export default function ArcheryPage() {
  const { message } = App.useApp();
  const [list, setList] = useState<ArcheryConnection[]>([]);
  const [loading, setLoading] = useState(false);
  const [testingId, setTestingId] = useState<number | null>(null);
  const [syncingId, setSyncingId] = useState<number | null>(null);
  // 各连接已同步实例（展开行内展示）。
  const [instances, setInstances] = useState<Record<number, ArcheryInstance[]>>({});

  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<ArcheryConnection | null>(null);
  const [form] = Form.useForm<ArcheryConnectionInput>();

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await archeryApi.listConnections();
      const conns = res.list || [];
      setList(conns);
      // 预取各连接实例，供展开行与"实例数"列展示。
      const grouped = await Promise.all(
        conns.map(async (c) => [c.id, (await archeryApi.listInstances(c.id)).list] as const),
      );
      setInstances(Object.fromEntries(grouped));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    setOpen(true);
  };
  const openEdit = (c: ArcheryConnection) => {
    setEditing(c);
    form.setFieldsValue({
      name: c.name, endpoint: c.endpoint,
      username: c.username, insecure: c.insecure, caCert: c.caCert, remark: c.remark,
      // password 留空 = 不修改
    });
    setOpen(true);
  };
  const save = async () => {
    const v = await form.validateFields();
    if (editing) {
      await archeryApi.updateConnection(editing.id, v);
      message.success('连接已更新');
    } else {
      await archeryApi.createConnection(v);
      message.success('连接已创建，可点击"更新实例"拉取其实例');
    }
    setOpen(false);
    load();
  };
  const remove = async (c: ArcheryConnection) => {
    await archeryApi.deleteConnection(c.id);
    message.success('已删除');
    load();
  };
  const test = async (c: ArcheryConnection) => {
    setTestingId(c.id);
    try {
      const r = await archeryApi.testConnection(c.id);
      if (r.ok) {
        message.success(`连通正常，可访问 ${r.instances ?? 0} 个实例`);
      } else {
        message.error(r.error || '连接失败');
      }
    } finally {
      setTestingId(null);
    }
  };
  // 更新实例：重新从该 Archery 地址拉取所有实例并 upsert（新建/更新/清理）。
  const sync = async (c: ArcheryConnection) => {
    setSyncingId(c.id);
    try {
      const r = await archeryApi.syncInstances(c.id);
      setInstances((m) => ({ ...m, [c.id]: r.list || [] }));
      message.success(`已同步 ${r.list?.length ?? 0} 个实例`);
      load();
    } catch (e) {
      message.error(e instanceof Error ? e.message : '同步失败');
    } finally {
      setSyncingId(null);
    }
  };

  const cols: ColumnsType<ArcheryConnection> = [
    {
      title: '名称', dataIndex: 'name', key: 'name',
      render: (v, c) => (
        <Space>
          <span style={{ fontWeight: 600 }}>{v}</span>
          {c.insecure && <Tag color="orange">跳过TLS校验</Tag>}
        </Space>
      ),
    },
    {
      title: '地址', dataIndex: 'endpoint', key: 'endpoint',
      render: (v) => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{v}</span>,
    },
    { title: '用户名', dataIndex: 'username', key: 'username' },
    {
      title: '实例数', dataIndex: 'instanceCount', key: 'instanceCount', width: 90,
      render: (v: number) => <Tag color={v > 0 ? 'blue' : 'default'}>{v}</Tag>,
    },
    { title: '备注', dataIndex: 'remark', key: 'remark', ellipsis: true },
    {
      title: '操作', key: 'op', width: 190,
      render: (_, c) => (
        <Space size="small">
          <Tooltip title="测试连接">
            <Button
              size="small" type="text" icon={<ThunderboltOutlined />}
              loading={testingId === c.id} onClick={() => test(c)}
            />
          </Tooltip>
          <Tooltip title="更新实例：重新拉取该地址下所有实例并新建/更新">
            <Button
              size="small" type="text" icon={<SyncOutlined />}
              loading={syncingId === c.id} onClick={() => sync(c)}
            />
          </Tooltip>
          <Button size="small" type="text" icon={<EditOutlined />} onClick={() => openEdit(c)} />
          <Popconfirm title="删除该连接？其下实例与引用它们的 archery 节点将不可用" onConfirm={() => remove(c)}>
            <Button size="small" type="text" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div className="bf-page">
      <h2 style={{ marginTop: 0 }}>Archery 连接</h2>
      <Card
        title="连接（Archery 平台地址 + 账号）；实例由「更新实例」自动拉取"
        extra={<Button size="small" type="primary" icon={<PlusOutlined />} onClick={openCreate}>新建</Button>}
      >
        <Table
          rowKey="id"
          size="middle"
          loading={loading}
          columns={cols}
          dataSource={list}
          pagination={false}
          locale={{ emptyText: <Empty description="暂无连接，点击右上新建" /> }}
          expandable={{
            rowExpandable: (c) => (instances[c.id]?.length ?? 0) > 0,
            expandedRowRender: (c) => (
              <Table
                rowKey="id"
                size="small"
                pagination={false}
                dataSource={instances[c.id] ?? []}
                columns={[
                  { title: '实例名', dataIndex: 'instanceName', key: 'instanceName' },
                  {
                    title: '类型', dataIndex: 'dbType', key: 'dbType', width: 140,
                    render: (v) => (v ? <Tag>{v}</Tag> : '—'),
                  },
                ]}
              />
            ),
          }}
        />
      </Card>

      <Modal
        title={editing ? '编辑连接' : '新建连接'}
        open={open}
        onOk={save}
        onCancel={() => setOpen(false)}
        okText="保存"
        cancelText="取消"
        destroyOnClose
      >
        <Form form={form} layout="vertical" style={{ marginTop: 12 }}>
          <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input placeholder="如：生产 Archery" />
          </Form.Item>
          <Form.Item name="endpoint" label="Archery 地址" rules={[{ required: true, message: '请输入地址' }]}>
            <Input placeholder="https://archery.example.com" />
          </Form.Item>
          <Form.Item name="username" label="用户名" rules={[{ required: true, message: '请输入用户名' }]}>
            <Input autoComplete="off" />
          </Form.Item>
          <Form.Item
            name="password"
            label={editing ? '密码（留空不修改）' : '密码'}
            rules={editing ? [] : [{ required: true, message: '请输入密码' }]}
          >
            <Input.Password
              placeholder={editing ? '留空保持不变' : '登录密码'}
              autoComplete="new-password"
            />
          </Form.Item>
          <Form.Item name="insecure" label="跳过 TLS 证书校验" valuePropName="checked" initialValue={false}>
            <Switch />
          </Form.Item>
          <Form.Item
            name="caCert" label="自定义 CA 证书（PEM）"
            extra="内部/私有 CA 时粘贴 PEM 文本；开启跳过校验时无需填写"
          >
            <Input.TextArea rows={3} placeholder="-----BEGIN CERTIFICATE-----" />
          </Form.Item>
          <Form.Item name="remark" label="备注">
            <Input.TextArea rows={2} placeholder="可选" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
