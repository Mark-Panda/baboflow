import { useCallback, useEffect, useState } from 'react';
import {
  App, Button, Card, Empty, Form, Input, Modal, Popconfirm, Space, Table,
} from 'antd';
import { PlusOutlined, ReloadOutlined, DeleteOutlined, AppstoreOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';

import { boardApi, Board } from '@/api/board';

export default function BoardListPage() {
  const { message } = App.useApp();
  const navigate = useNavigate();
  const [data, setData] = useState<Board[]>([]);
  const [loading, setLoading] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm<{ name: string; description?: string }>();

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await boardApi.list();
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

  const onCreate = async () => {
    const v = await form.validateFields();
    setSaving(true);
    try {
      const b = await boardApi.create(v);
      message.success('已创建（含默认三列）');
      setCreateOpen(false);
      form.resetFields();
      navigate(`/boards/${b.id}`);
    } catch {
      /* 拦截器已提示 */
    } finally {
      setSaving(false);
    }
  };

  const onDelete = async (r: Board) => {
    await boardApi.remove(r.id);
    message.success('已删除');
    load();
  };

  const columns: ColumnsType<Board> = [
    {
      title: '名称', dataIndex: 'name', key: 'name',
      render: (v, r) => <a onClick={() => navigate(`/boards/${r.id}`)}><AppstoreOutlined /> {v}</a>,
    },
    { title: '描述', dataIndex: 'description', key: 'description', ellipsis: true },
    { title: '创建时间', dataIndex: 'createdAt', key: 'createdAt', width: 170, render: (v) => dayjs(v).format('YYYY-MM-DD HH:mm') },
    {
      title: '操作', key: 'op', width: 100, fixed: 'right',
      render: (_, r) => (
        <Popconfirm title="确认删除该看板及其全部任务？" onConfirm={() => onDelete(r)}>
          <Button size="small" danger icon={<DeleteOutlined />} />
        </Popconfirm>
      ),
    },
  ];

  return (
    <div className="bf-page">
      <Card
        title="看板"
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={load} />
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>新建看板</Button>
          </Space>
        }
      >
        <Table
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={data}
          locale={{ emptyText: <Empty description="还没有看板，点击右上角新建" /> }}
          pagination={{ pageSize: 20, showTotal: (t) => `共 ${t} 条` }}
        />
      </Card>

      <Modal
        title="新建看板"
        open={createOpen}
        onOk={onCreate}
        onCancel={() => setCreateOpen(false)}
        confirmLoading={saving}
        okText="创建"
        cancelText="取消"
        destroyOnClose
      >
        <Form form={form} layout="vertical" style={{ marginTop: 12 }}>
          <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input placeholder="如 运维任务" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={2} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
