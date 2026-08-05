import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  App, Button, Card, Drawer, Dropdown, Empty, Form, Input, InputNumber, Modal,
  Popconfirm, Select, Space, Tag, Tooltip,
} from 'antd';
import {
  PlusOutlined, ReloadOutlined, ArrowLeftOutlined, PlayCircleOutlined,
  DeleteOutlined, MoreOutlined, SwapOutlined, ThunderboltOutlined,
} from '@ant-design/icons';
import { useNavigate, useParams } from 'react-router-dom';

import { boardApi, BoardDetail, BoardTask, BoardColumn, TaskInput } from '@/api/board';
import { chainApi, ChainListItem } from '@/api/chain';
import type { ProtoInt64 } from '@/api/http';
import StatusTag from '@/components/StatusTag';

// 任务卡片
function TaskCard({
  task, columns, onMove, onTrigger, onEdit, onDelete, triggering,
}: {
  task: BoardTask;
  columns: BoardColumn[];
  onMove: (task: BoardTask, toColumnId: ProtoInt64) => void;
  onTrigger: (task: BoardTask) => void;
  onEdit: (task: BoardTask) => void;
  onDelete: (task: BoardTask) => void;
  triggering: boolean;
}) {
  const hasChain = !!task.assignedChainId;
  const moveItems = columns
    .filter((c) => c.id !== task.columnId)
    .map((c) => ({ key: String(c.id), label: `移到「${c.name}」`, icon: <SwapOutlined /> }));

  return (
    <Card
      size="small"
      style={{ marginBottom: 8, borderLeft: `3px solid ${task.status === 'success' ? '#52c41a' : task.status === 'failure' ? '#ff4d4f' : task.status === 'running' ? '#1677ff' : '#d9d9d9'}` }}
      styles={{ body: { padding: '10px 12px' } }}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 8 }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontWeight: 600, marginBottom: 4, wordBreak: 'break-all' }}>{task.title}</div>
          <Space size={4} wrap>
            <StatusTag value={task.status} />
            {hasChain && (
              <Tooltip title={`规则链 ${task.assignedChainId}`}>
                <Tag color="purple" style={{ fontSize: 11 }}>链</Tag>
              </Tooltip>
            )}
          </Space>
        </div>
        <Dropdown
          menu={{
            items: [
              { key: 'run', icon: <PlayCircleOutlined />, label: '触发执行', disabled: !hasChain, onClick: () => onTrigger(task) },
              ...moveItems.map((m) => ({ ...m, onClick: () => onMove(task, m.key) })),
              { type: 'divider' },
              { key: 'edit', label: '编辑', onClick: () => onEdit(task) },
              { key: 'del', icon: <DeleteOutlined />, label: '删除', danger: true, onClick: () => onDelete(task) },
            ],
          }}
          trigger={['click']}
        >
          <Button size="small" type="text" icon={<MoreOutlined />} loading={triggering} />
        </Dropdown>
      </div>
      {task.status === 'failure' && task.result?.error && (
        <div style={{ marginTop: 6, fontSize: 12, color: '#ff4d4f', wordBreak: 'break-all' }}>
          {task.result.error}
        </div>
      )}
    </Card>
  );
}

export default function BoardPage() {
  const { id } = useParams<{ id: string }>();
  const boardId = id ?? '';
  const navigate = useNavigate();
  const { message } = App.useApp();

  const [board, setBoard] = useState<BoardDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [triggering, setTriggering] = useState<ProtoInt64 | null>(null);
  const [publishedChains, setPublishedChains] = useState<ChainListItem[]>([]);

  // 新建/编辑任务
  const [taskOpen, setTaskOpen] = useState(false);
  const [taskColumn, setTaskColumn] = useState<ProtoInt64 | null>(null);
  const [editingTask, setEditingTask] = useState<BoardTask | null>(null);
  const [savingTask, setSavingTask] = useState(false);
  const [taskForm] = Form.useForm<TaskInput>();

  // 任务结果
  const [resultTask, setResultTask] = useState<BoardTask | null>(null);

  // 新建列
  const [colOpen, setColOpen] = useState(false);
  const [colForm] = Form.useForm<{ name: string }>();

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await boardApi.get(boardId);
      setBoard(res);
    } catch {
      /* 拦截器已提示 */
    } finally {
      setLoading(false);
    }
  }, [boardId]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    chainApi.list({ status: 'published', pageSize: 200 })
      .then((r) => setPublishedChains(r.list || []))
      .catch(() => { /* 拦截器已提示 */ });
  }, []);

  const openCreateTask = (columnId: ProtoInt64) => {
    setEditingTask(null);
    setTaskColumn(columnId);
    taskForm.resetFields();
    taskForm.setFieldsValue({ timeoutSec: 300, retryMax: 0 } as never);
    setTaskOpen(true);
  };

  const openEditTask = (task: BoardTask) => {
    setEditingTask(task);
    setTaskColumn(task.columnId);
    taskForm.setFieldsValue({
      title: task.title,
      payload: task.payload,
      assignedChainId: task.assignedChainId || undefined,
      retryMax: task.retryMax,
      timeoutSec: task.timeoutSec,
    });
    setTaskOpen(true);
  };

  const onSaveTask = async () => {
    const v = await taskForm.validateFields();
    if (v.payload && v.payload.trim()) {
      try { JSON.parse(v.payload); } catch { message.error('Payload 需为合法 JSON'); return; }
    }
    setSavingTask(true);
    try {
      if (editingTask) {
        await boardApi.updateTask(editingTask.id, v);
        message.success('已更新');
      } else if (taskColumn != null) {
        await boardApi.createTask(taskColumn, v);
        message.success('已创建任务');
      }
      setTaskOpen(false);
      load();
    } catch {
      /* 拦截器已提示 */
    } finally {
      setSavingTask(false);
    }
  };

  const onMove = async (task: BoardTask, toColumnId: ProtoInt64) => {
    await boardApi.moveTask(task.id, toColumnId, '0');
    load();
  };

  const onTrigger = async (task: BoardTask) => {
    setTriggering(task.id);
    try {
      const res = await boardApi.triggerTask(task.id);
      if (res.status === 'success') {
        message.success(`「${task.title}」执行成功`);
      } else {
        message.warning(`「${task.title}」执行失败`);
      }
      setResultTask(res);
      load();
    } catch {
      /* 拦截器已提示 */
    } finally {
      setTriggering(null);
    }
  };

  const onDeleteTask = async (task: BoardTask) => {
    await boardApi.removeTask(task.id);
    message.success('已删除');
    load();
  };

  const onAddColumn = async () => {
    const v = await colForm.validateFields();
    await boardApi.createColumn(boardId, { name: v.name });
    message.success('已添加列');
    setColOpen(false);
    colForm.resetFields();
    load();
  };

  const onDeleteColumn = async (c: BoardColumn) => {
    await boardApi.removeColumn(c.id);
    message.success('已删除列');
    load();
  };

  const chainOptions = useMemo(
    () => publishedChains.map((c) => ({ value: c.id, label: `${c.name} (v${c.version})` })),
    [publishedChains],
  );

  if (!board) {
    return <div className="bf-page"><Card loading={loading} /></div>;
  }

  return (
    <div className="bf-page" style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <Card
        size="small"
        title={
          <Space>
            <Button size="small" type="text" icon={<ArrowLeftOutlined />} onClick={() => navigate('/boards')} />
            <span>{board.name}</span>
            {board.description && <span style={{ color: '#999', fontWeight: 400, fontSize: 13 }}>{board.description}</span>}
          </Space>
        }
        extra={
          <Space>
            <Button size="small" icon={<ReloadOutlined />} onClick={load} />
            <Button size="small" icon={<PlusOutlined />} onClick={() => setColOpen(true)}>加列</Button>
          </Space>
        }
        styles={{ body: { flex: 1, overflow: 'auto' } }}
        style={{ flex: 1, display: 'flex', flexDirection: 'column' }}
      >
        <div style={{ display: 'flex', gap: 12, alignItems: 'flex-start', minHeight: 400 }}>
          {(board.columns || []).map((col) => (
            <div
              key={col.id}
              style={{
                flex: '0 0 280px', background: '#f0f2f5', borderRadius: 8,
                padding: '8px 8px 4px', minHeight: 200,
              }}
            >
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '0 4px 8px' }}>
                <Space size={6}>
                  <span style={{ fontWeight: 600 }}>{col.name}</span>
                  <Tag style={{ marginInlineEnd: 0 }}>{(col.tasks || []).length}</Tag>
                </Space>
                <Space size={0}>
                  <Tooltip title="新建任务">
                    <Button size="small" type="text" icon={<PlusOutlined />} onClick={() => openCreateTask(col.id)} />
                  </Tooltip>
                  <Popconfirm title="删除该列及其任务？" onConfirm={() => onDeleteColumn(col)}>
                    <Button size="small" type="text" danger icon={<DeleteOutlined />} />
                  </Popconfirm>
                </Space>
              </div>
              {(col.tasks || []).length === 0 ? (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="空" style={{ margin: '16px 0' }} />
              ) : (
                (col.tasks || []).map((t) => (
                  <TaskCard
                    key={t.id}
                    task={t}
                    columns={board.columns}
                    triggering={triggering === t.id}
                    onMove={onMove}
                    onTrigger={onTrigger}
                    onEdit={openEditTask}
                    onDelete={onDeleteTask}
                  />
                ))
              )}
            </div>
          ))}
        </div>
      </Card>

      {/* 新建/编辑任务 */}
      <Modal
        title={editingTask ? '编辑任务' : '新建任务'}
        open={taskOpen}
        onOk={onSaveTask}
        onCancel={() => setTaskOpen(false)}
        confirmLoading={savingTask}
        okText="保存"
        cancelText="取消"
        destroyOnClose
      >
        <Form form={taskForm} layout="vertical" style={{ marginTop: 12 }}>
          <Form.Item name="title" label="标题" rules={[{ required: true, message: '请输入标题' }]}>
            <Input placeholder="任务标题" />
          </Form.Item>
          <Form.Item name="assignedChainId" label="分配规则链（触发时执行）">
            <Select allowClear showSearch optionFilterProp="label" placeholder="选择已发布规则链" options={chainOptions} />
          </Form.Item>
          <Form.Item name="payload" label="Payload（JSON，作为规则链输入 data）">
            <Input.TextArea rows={4} placeholder='{"temperature": 42}' style={{ fontFamily: 'monospace' }} />
          </Form.Item>
          <Space size="large">
            <Form.Item name="retryMax" label="重试次数">
              <InputNumber min={0} max={10} />
            </Form.Item>
            <Form.Item name="timeoutSec" label="超时（秒）">
              <InputNumber min={1} max={3600} />
            </Form.Item>
          </Space>
        </Form>
      </Modal>

      {/* 新建列 */}
      <Modal
        title="新建列"
        open={colOpen}
        onOk={onAddColumn}
        onCancel={() => setColOpen(false)}
        okText="添加"
        cancelText="取消"
        destroyOnClose
      >
        <Form form={colForm} layout="vertical" style={{ marginTop: 12 }}>
          <Form.Item name="name" label="列名" rules={[{ required: true, message: '请输入列名' }]}>
            <Input placeholder="如 待审核" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 任务执行结果 */}
      <Drawer
        title={resultTask ? `执行结果 · ${resultTask.title}` : '执行结果'}
        width={560}
        open={!!resultTask}
        onClose={() => setResultTask(null)}
      >
        {resultTask && (
          <>
            <Space style={{ marginBottom: 12 }}>
              <StatusTag value={resultTask.status} />
              <Tag icon={<ThunderboltOutlined />} color="purple">{resultTask.assignedChainId}</Tag>
            </Space>
            <pre
              style={{
                background: '#0d1117', color: '#e6edf3', padding: 16, borderRadius: 8,
                overflow: 'auto', fontSize: 13, maxHeight: '60vh',
              }}
            >
              {resultTask.result?.output ?? resultTask.result?.error ?? '（无输出）'}
            </pre>
          </>
        )}
      </Drawer>
    </div>
  );
}
