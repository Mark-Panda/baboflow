import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  App, Button, Card, DatePicker, Form, Input, InputNumber, Modal, Popconfirm,
  Radio, Select, Space, Switch, Table, Tag, Tooltip,
} from 'antd';
import {
  PlusOutlined, ReloadOutlined, DeleteOutlined, EditOutlined, ClockCircleOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import dayjs, { Dayjs } from 'dayjs';

import { cronApi, CronJob, CronInput } from '@/api/cron';
import { chainApi, ChainListItem } from '@/api/chain';
import { listAgents, Agent } from '@/api/agent';
import { toSafeNumber } from '@/api/http';

type FormValues = {
  name?: string;
  targetType: 'chain' | 'agent';
  targetId: string;
  scheduleType: 'once' | 'interval' | 'cron';
  cronExpr?: string;
  intervalSec?: number;
  runAt?: Dayjs;
  payloadText?: string;
};

function scheduleText(j: CronJob): string {
  switch (j.scheduleType) {
    case 'interval':
      return `每 ${j.intervalSec}s`;
    case 'once':
      return j.runAt ? `一次性 · ${dayjs(j.runAt).format('MM-DD HH:mm')}` : '一次性';
    default:
      return j.cronExpr;
  }
}

export default function CronPage() {
  const { message } = App.useApp();
  const [data, setData] = useState<CronJob[]>([]);
  const [loading, setLoading] = useState(false);

  const [editOpen, setEditOpen] = useState(false);
  const [editing, setEditing] = useState<CronJob | null>(null);
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm<FormValues>();
  const targetType = Form.useWatch('targetType', form);
  const scheduleType = Form.useWatch('scheduleType', form);

  const [chains, setChains] = useState<ChainListItem[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await cronApi.list();
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

  useEffect(() => {
    chainApi.list({ status: 'published', pageSize: 200 }).then((r) => setChains(r.list || [])).catch(() => {});
    listAgents().then((r) => setAgents(r.list || [])).catch(() => {});
  }, []);

  const targetOptions = useMemo(() => {
    if (targetType === 'agent') {
      return agents.map((a) => ({ value: a.key, label: a.name }));
    }
    return chains.map((c) => ({ value: c.id, label: `${c.name} (v${c.version})` }));
  }, [targetType, chains, agents]);

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ targetType: 'chain', scheduleType: 'interval', intervalSec: 60 } as never);
    setEditOpen(true);
  };

  const openEdit = (r: CronJob) => {
    setEditing(r);
    form.setFieldsValue({
      name: r.name,
      targetType: r.targetType,
      targetId: r.targetId,
      scheduleType: r.scheduleType,
      cronExpr: r.cronExpr,
      intervalSec: toSafeNumber(r.intervalSec, 'cron intervalSec'),
      runAt: r.runAt ? dayjs(r.runAt) : undefined,
      payloadText: r.payload ? JSON.stringify(r.payload) : '',
    });
    setEditOpen(true);
  };

  const onSave = async () => {
    const v = await form.validateFields();
    let payload: Record<string, unknown> | undefined;
    if (v.payloadText && v.payloadText.trim()) {
      try {
        payload = JSON.parse(v.payloadText);
      } catch {
        message.error('Payload 需为合法 JSON');
        return;
      }
    }
    const body: CronInput = {
      name: v.name,
      targetType: v.targetType,
      targetId: v.targetId,
      scheduleType: v.scheduleType,
      cronExpr: v.cronExpr,
      intervalSec: v.intervalSec === undefined ? undefined : String(v.intervalSec),
      runAt: v.runAt ? v.runAt.toISOString() : undefined,
      payload,
    };
    setSaving(true);
    try {
      if (editing) {
        await cronApi.update(editing.id, body);
        message.success('已更新');
      } else {
        await cronApi.create(body);
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

  const onToggle = async (r: CronJob) => {
    await cronApi.toggle(r.id);
    load();
  };

  const onDelete = async (r: CronJob) => {
    await cronApi.remove(r.id);
    message.success('已删除');
    load();
  };

  const columns: ColumnsType<CronJob> = [
    { title: '名称', dataIndex: 'name', key: 'name', render: (v) => v || '—' },
    {
      title: '目标', key: 'target',
      render: (_, r) => (
        <Space size={4}>
          <Tag color={r.targetType === 'chain' ? 'geekblue' : 'purple'}>{r.targetType === 'chain' ? '规则链' : 'Agent'}</Tag>
          <code style={{ fontSize: 12 }}>{r.targetId}</code>
        </Space>
      ),
    },
    {
      title: '调度', key: 'schedule', width: 190,
      render: (_, r) => (
        <span><ClockCircleOutlined style={{ marginRight: 4, color: '#888' }} />{scheduleText(r)}</span>
      ),
    },
    {
      title: '上次执行', dataIndex: 'lastRunAt', key: 'lastRunAt', width: 200,
      render: (v, r) => (
        <Space size={4}>
          {v ? dayjs(v).format('MM-DD HH:mm:ss') : '—'}
          {r.lastStatus === 'success' && <Tag color="success">成功</Tag>}
          {r.lastStatus === 'failure' && <Tag color="error">失败</Tag>}
        </Space>
      ),
    },
    {
      title: '启用', dataIndex: 'enabled', key: 'enabled', width: 80,
      render: (v, r) => <Switch size="small" checked={v} onChange={() => onToggle(r)} />,
    },
    {
      title: '操作', key: 'op', width: 130, fixed: 'right',
      render: (_, r) => (
        <Space size="small">
          <Tooltip title="编辑">
            <Button size="small" icon={<EditOutlined />} onClick={() => openEdit(r)} />
          </Tooltip>
          <Popconfirm title="确认删除该定时任务？" onConfirm={() => onDelete(r)}>
            <Button size="small" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div className="bf-page">
      <Card
        title="定时任务"
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={load} />
            <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新建任务</Button>
          </Space>
        }
      >
        <Table
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={data}
          pagination={{ pageSize: 20, showTotal: (t) => `共 ${t} 条` }}
        />
      </Card>

      <Modal
        title={editing ? '编辑定时任务' : '新建定时任务'}
        open={editOpen}
        onOk={onSave}
        onCancel={() => setEditOpen(false)}
        confirmLoading={saving}
        okText="保存"
        cancelText="取消"
        destroyOnClose
      >
        <Form form={form} layout="vertical" style={{ marginTop: 12 }}>
          <Form.Item name="name" label="名称">
            <Input placeholder="可选，便于识别" />
          </Form.Item>
          <Space size="large" style={{ display: 'flex' }}>
            <Form.Item name="targetType" label="目标类型" rules={[{ required: true }]} style={{ flex: 1 }}>
              <Radio.Group
                optionType="button"
                options={[{ value: 'chain', label: '规则链' }, { value: 'agent', label: 'Agent' }]}
              />
            </Form.Item>
            <Form.Item name="scheduleType" label="调度方式" rules={[{ required: true }]} style={{ flex: 1 }}>
              <Radio.Group
                optionType="button"
                options={[{ value: 'interval', label: '间隔' }, { value: 'cron', label: 'Cron' }, { value: 'once', label: '一次性' }]}
              />
            </Form.Item>
          </Space>
          <Form.Item name="targetId" label={targetType === 'agent' ? '目标 Agent' : '目标规则链（已发布）'} rules={[{ required: true, message: '请选择目标' }]}>
            <Select showSearch optionFilterProp="label" placeholder="选择目标" options={targetOptions} />
          </Form.Item>

          {scheduleType === 'cron' && (
            <Form.Item name="cronExpr" label="Cron 表达式（支持 6 段带秒）" rules={[{ required: true, message: '请输入 cron 表达式' }]}>
              <Input placeholder="0 */5 * * * *  (每5分钟)" style={{ fontFamily: 'monospace' }} />
            </Form.Item>
          )}
          {scheduleType === 'interval' && (
            <Form.Item name="intervalSec" label="间隔（秒）" rules={[{ required: true, message: '请输入间隔秒数' }]}>
              <InputNumber min={1} max={86400} style={{ width: '100%' }} />
            </Form.Item>
          )}
          {scheduleType === 'once' && (
            <Form.Item name="runAt" label="执行时间" rules={[{ required: true, message: '请选择执行时间' }]}>
              <DatePicker showTime style={{ width: '100%' }} />
            </Form.Item>
          )}

          <Form.Item name="payloadText" label={targetType === 'agent' ? '提示词（文本，可空）' : 'Payload（JSON，作为规则链输入 data）'}>
            <Input.TextArea rows={4} placeholder='{"temperature": 42}' style={{ fontFamily: 'monospace' }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
