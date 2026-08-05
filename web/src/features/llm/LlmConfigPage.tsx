import { useCallback, useEffect, useState } from 'react';
import {
  App, Badge, Button, Card, Checkbox, Col, Empty, Form, Input,
  Modal, Popconfirm, Row, Select, Space, Switch, Table, Tag, Tooltip,
} from 'antd';
import {
  PlusOutlined, ThunderboltOutlined, CloudDownloadOutlined,
  EditOutlined, DeleteOutlined, StarOutlined, StarFilled,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';

import { llmApi, LLMModel, ModelInput, Provider, ProviderInput } from '@/api/llm';
import type { ProtoInt64 } from '@/api/http';

export default function LlmConfigPage() {
  const { message } = App.useApp();
  const [providers, setProviders] = useState<Provider[]>([]);
  const [loading, setLoading] = useState(false);
  const [activeId, setActiveId] = useState<ProtoInt64 | null>(null);
  const [models, setModels] = useState<LLMModel[]>([]);
  const [modelLoading, setModelLoading] = useState(false);

  const [providerOpen, setProviderOpen] = useState(false);
  const [editingProvider, setEditingProvider] = useState<Provider | null>(null);
  const [remoteOpen, setRemoteOpen] = useState(false);
  const [remoteModels, setRemoteModels] = useState<string[]>([]);
  const [remoteChecked, setRemoteChecked] = useState<string[]>([]);
  const [remoteLoading, setRemoteLoading] = useState(false);

  const [providerForm] = Form.useForm<ProviderInput>();

  const loadProviders = useCallback(async () => {
    setLoading(true);
    try {
      const res = await llmApi.listProviders();
      setProviders(res.list || []);
      if (!activeId && res.list?.length) setActiveId(res.list[0].id);
    } finally {
      setLoading(false);
    }
  }, [activeId]);

  const loadModels = useCallback(async (pid: ProtoInt64) => {
    setModelLoading(true);
    try {
      const res = await llmApi.listModels(pid);
      setModels(res.list || []);
    } finally {
      setModelLoading(false);
    }
  }, []);

  useEffect(() => { loadProviders(); }, []); // eslint-disable-line
  useEffect(() => { if (activeId) loadModels(activeId); }, [activeId, loadModels]);

  const active = providers.find((p) => p.id === activeId) || null;

  // ---- 接入点 CRUD ----
  const openCreate = () => {
    setEditingProvider(null);
    providerForm.resetFields();
    setProviderOpen(true);
  };
  const openEdit = (p: Provider) => {
    setEditingProvider(p);
    providerForm.setFieldsValue({ name: p.name, provider: p.provider, baseUrl: p.baseUrl, remark: p.remark });
    setProviderOpen(true);
  };
  const saveProvider = async () => {
    const v = await providerForm.validateFields();
    if (editingProvider) {
      await llmApi.updateProvider(editingProvider.id, v);
      message.success('接入点已更新');
    } else {
      if (!v.apiKey) { message.warning('请填写 API Key'); return; }
      await llmApi.createProvider(v);
      message.success('接入点已创建');
    }
    setProviderOpen(false);
    loadProviders();
  };
  const removeProvider = async (p: Provider) => {
    await llmApi.deleteProvider(p.id);
    message.success('已删除');
    if (activeId === p.id) setActiveId(null);
    loadProviders();
  };
  const testProvider = async (p: Provider) => {
    const r = await llmApi.testProvider(p.id);
    if (r.ok) message.success(`连通正常（${r.latencyMs ?? 0}ms）`);
    else message.error(r.message || '连接失败');
  };

  // ---- 拉取远程模型登记 ----
  const fetchRemote = async () => {
    if (!activeId) return;
    setRemoteLoading(true);
    try {
      const r = await llmApi.remoteModels(activeId);
      setRemoteModels(r.models || []);
      setRemoteChecked([]);
      setRemoteOpen(true);
    } catch {
      /* 提示已处理 */
    } finally {
      setRemoteLoading(false);
    }
  };
  const registerRemote = async () => {
    if (!activeId || !remoteChecked.length) return;
    const inputs: ModelInput[] = remoteChecked.map((m) => ({ model: m, alias: m }));
    await llmApi.createModels(activeId, inputs);
    message.success(`已登记 ${remoteChecked.length} 个模型`);
    setRemoteOpen(false);
    loadModels(activeId);
  };

  // ---- 模型操作 ----
  const setDefault = async (m: LLMModel) => {
    await llmApi.setDefaultModel(m.id);
    message.success(`已设 ${m.alias || m.model} 为默认`);
    if (activeId) loadModels(activeId);
  };
  const toggleEnabled = async (m: LLMModel, enabled: boolean) => {
    await llmApi.updateModel(m.id, { model: m.model, enabled });
    if (activeId) loadModels(activeId);
  };
  const removeModel = async (m: LLMModel) => {
    await llmApi.deleteModel(m.id);
    message.success('已删除模型');
    if (activeId) loadModels(activeId);
  };
  const testModel = async (m: LLMModel) => {
    const r = await llmApi.testModel(m.id);
    if (r.ok) message.success(`调用成功（${r.latencyMs ?? 0}ms）`);
    else message.error(r.message || '调用失败');
  };

  const modelCols: ColumnsType<LLMModel> = [
    {
      title: '模型', dataIndex: 'model', key: 'model',
      render: (v, m) => (
        <Space>
          <span style={{ fontFamily: 'monospace' }}>{v}</span>
          {m.isDefault && <Tag color="gold">默认</Tag>}
        </Space>
      ),
    },
    { title: '别名', dataIndex: 'alias', key: 'alias' },
    { title: '温度', dataIndex: 'temperature', key: 'temperature', width: 70 },
    { title: 'MaxTokens', dataIndex: 'maxTokens', key: 'maxTokens', width: 100 },
    {
      title: '启用', dataIndex: 'enabled', key: 'enabled', width: 70,
      render: (v, m) => <Switch size="small" checked={v} onChange={(e) => toggleEnabled(m, e)} />,
    },
    {
      title: '操作', key: 'op', width: 200,
      render: (_, m) => (
        <Space size="small">
          <Tooltip title={m.isDefault ? '默认模型' : '设为默认'}>
            <Button size="small" type="text" icon={m.isDefault ? <StarFilled style={{ color: '#faad14' }} /> : <StarOutlined />} onClick={() => setDefault(m)} />
          </Tooltip>
          <Tooltip title="测试调用">
            <Button size="small" type="text" icon={<ThunderboltOutlined />} onClick={() => testModel(m)} />
          </Tooltip>
          <Popconfirm title="删除该模型？" onConfirm={() => removeModel(m)}>
            <Button size="small" type="text" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div className="bf-page">
      <h2 style={{ marginTop: 0 }}>LLM 配置</h2>
      <Row gutter={16}>
        {/* 左：接入点 */}
        <Col xs={24} md={9} lg={7}>
          <Card
            title="接入点（baseUrl + apiKey）"
            extra={<Button size="small" type="primary" icon={<PlusOutlined />} onClick={openCreate}>新建</Button>}
            loading={loading}
          >
            {providers.length === 0 ? (
              <Empty description="暂无接入点，点击右上新建" />
            ) : (
              <Space direction="vertical" style={{ width: '100%' }} size={10}>
                {providers.map((p) => (
                  <Card
                    key={p.id}
                    size="small"
                    hoverable
                    onClick={() => setActiveId(p.id)}
                    style={{
                      cursor: 'pointer',
                      borderColor: p.id === activeId ? '#4f6ef7' : undefined,
                      boxShadow: p.id === activeId ? '0 0 0 2px rgba(79,110,247,0.15)' : undefined,
                    }}
                  >
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <div style={{ fontWeight: 600 }}>{p.name}</div>
                      <Space size={4} onClick={(e) => e.stopPropagation()}>
                        <Tooltip title="测试连接">
                          <Button size="small" type="text" icon={<ThunderboltOutlined />} onClick={() => testProvider(p)} />
                        </Tooltip>
                        <Button size="small" type="text" icon={<EditOutlined />} onClick={() => openEdit(p)} />
                        <Popconfirm title="删除接入点及其下未用模型？" onConfirm={() => removeProvider(p)}>
                          <Button size="small" type="text" danger icon={<DeleteOutlined />} />
                        </Popconfirm>
                      </Space>
                    </div>
                    <div style={{ fontSize: 12, color: '#888', marginTop: 4, fontFamily: 'monospace', wordBreak: 'break-all' }}>
                      {p.baseUrl}
                    </div>
                    <div style={{ fontSize: 12, color: '#aaa', marginTop: 4, display: 'flex', justifyContent: 'space-between' }}>
                      <span>Key: {p.apiKeyMasked}</span>
                      <Badge count={p.modelCount} showZero color="#4f6ef7" title="模型数" />
                    </div>
                  </Card>
                ))}
              </Space>
            )}
          </Card>
        </Col>

        {/* 右：模型 */}
        <Col xs={24} md={15} lg={17}>
          <Card
            title={active ? `模型 · ${active.name}` : '模型'}
            extra={
              active && (
                <Button icon={<CloudDownloadOutlined />} loading={remoteLoading} onClick={fetchRemote}>
                  从接入点拉取模型
                </Button>
              )
            }
          >
            {!active ? (
              <Empty description="请选择左侧接入点" />
            ) : (
              <Table
                rowKey="id"
                size="middle"
                loading={modelLoading}
                columns={modelCols}
                dataSource={models}
                pagination={false}
                locale={{ emptyText: <Empty description="暂无模型，点击右上拉取或等待登记" /> }}
              />
            )}
          </Card>
        </Col>
      </Row>

      {/* 接入点表单 */}
      <Modal
        title={editingProvider ? '编辑接入点' : '新建接入点'}
        open={providerOpen}
        onOk={saveProvider}
        onCancel={() => setProviderOpen(false)}
        okText="保存"
        cancelText="取消"
        destroyOnClose
      >
        <Form form={providerForm} layout="vertical" style={{ marginTop: 12 }}>
          <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input placeholder="如：OpenAI 主账号 / 公司网关" />
          </Form.Item>
          <Form.Item name="provider" label="类型" initialValue="openai">
            <Select
              options={[
                { value: 'openai', label: 'OpenAI 兼容' },
                { value: 'azure', label: 'Azure OpenAI' },
                { value: 'qwen', label: '通义千问' },
                { value: 'deepseek', label: 'DeepSeek' },
                { value: 'other', label: '其他' },
              ]}
            />
          </Form.Item>
          <Form.Item name="baseUrl" label="Base URL" rules={[{ required: true, message: '请输入 Base URL' }]}>
            <Input placeholder="https://api.openai.com/v1" />
          </Form.Item>
          <Form.Item
            name="apiKey"
            label={editingProvider ? 'API Key（留空不修改）' : 'API Key'}
            rules={editingProvider ? [] : [{ required: true, message: '请输入 API Key' }]}
          >
            <Input.Password placeholder={editingProvider ? '留空保持不变' : 'sk-...'} autoComplete="new-password" />
          </Form.Item>
          <Form.Item name="remark" label="备注">
            <Input.TextArea rows={2} placeholder="可选" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 远程模型勾选登记 */}
      <Modal
        title="从接入点拉取的模型"
        open={remoteOpen}
        onOk={registerRemote}
        onCancel={() => setRemoteOpen(false)}
        okText={`登记所选（${remoteChecked.length}）`}
        cancelText="取消"
        width={520}
      >
        <Checkbox.Group
          style={{ display: 'flex', flexDirection: 'column', gap: 8, maxHeight: 400, overflow: 'auto' }}
          value={remoteChecked}
          onChange={(v) => setRemoteChecked(v as string[])}
          options={remoteModels.map((m) => ({ label: m, value: m }))}
        />
        {remoteModels.length === 0 && <Empty description="该接入点未返回模型列表" />}
      </Modal>
    </div>
  );
}
