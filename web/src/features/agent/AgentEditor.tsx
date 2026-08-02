import { useEffect, useState } from 'react';
import {
  App,
  Alert,
  Button,
  Checkbox,
  Drawer,
  Form,
  Input,
  Popconfirm,
  Select,
  Space,
  Switch,
} from 'antd';
import { DeleteOutlined } from '@ant-design/icons';

import * as api from '@/api/agent';
import { llmApi } from '@/api/llm';
import { mcpApi } from '@/api/mcp';
import { skillApi } from '@/api/skill';

// 内置工具全集（与后端 builtin_tools.go 一致），用于勾选子集。
const ALL_BUILTIN_TOOLS = [
  { value: 'bash', label: 'bash（执行命令）' },
  { value: 'read', label: 'read（读文件）' },
  { value: 'write', label: 'write（写文件）' },
  { value: 'edit', label: 'edit（改文件）' },
  { value: 'grep', label: 'grep（搜内容）' },
];

interface Option {
  label: string;
  value: number | string;
}

export interface AgentEditorProps {
  // null=新建；否则编辑该 Agent（含内置，内置可改不可删、key 只读）
  agent: api.Agent | null;
  open: boolean;
  onClose: (saved: boolean) => void;
}

// Agent 编辑抽屉：新建/编辑/删除 Agent。
export default function AgentEditor({ agent, open, onClose }: AgentEditorProps) {
  const { message } = App.useApp();
  const [form] = Form.useForm();
  const isEdit = !!agent;
  // 内置 Agent：仅技能/MCP/子Agent（及启用）可编辑，核心定义锁定（后端同样强制）。
  const lockCore = isEdit && !!agent?.isBuiltin;
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);

  // 关联资源选项
  const [providers, setProviders] = useState<Option[]>([]);
  const [models, setModels] = useState<Option[]>([]);
  const [skills, setSkills] = useState<Option[]>([]);
  const [mcps, setMcps] = useState<Option[]>([]);
  const [agentOpts, setAgentOpts] = useState<Option[]>([]);
  const providerId = Form.useWatch('llmProviderId', form);

  // 打开时初始化表单 + 拉取静态选项
  useEffect(() => {
    if (!open) return;
    form.resetFields();
    form.setFieldsValue({
      key: agent?.key ?? '',
      name: agent?.name ?? '',
      instruction: agent?.instruction ?? '',
      llmProviderId: undefined,
      llmModelId: agent?.llmModelId ?? undefined,
      builtinTools: agent?.builtinTools?.length
        ? agent.builtinTools
        : ALL_BUILTIN_TOOLS.map((t) => t.value),
      skillIds: agent?.skillIds ?? [],
      mcpIds: agent?.mcpIds ?? [],
      subAgentIds: agent?.subAgentIds ?? [],
      enabled: agent?.enabled ?? true,
    });

    llmApi.listProviders().then((r) =>
      setProviders(r.list.map((p) => ({ label: p.name, value: p.id }))),
    ).catch(() => {});
    skillApi.list({}).then((r) =>
      setSkills(r.list.map((s) => ({ label: s.name, value: s.id }))),
    ).catch(() => {});
    mcpApi.listServers().then((r) =>
      setMcps(r.list.map((s) => ({ label: s.name, value: s.id }))),
    ).catch(() => {});
    api.listAgents().then((r) =>
      setAgentOpts(
        r.list
          .filter((a) => a.id !== agent?.id)
          .map((a) => ({ label: a.name, value: a.id })),
      ),
    ).catch(() => {});

    // 编辑且已选模型时，反查其所属 provider 以回填级联。
    if (agent?.llmModelId) {
      llmApi.listProviders().then(async (r) => {
        for (const p of r.list) {
          const ms = await llmApi.listModels(p.id);
          if (ms.list.some((m) => m.id === agent.llmModelId)) {
            form.setFieldsValue({ llmProviderId: p.id });
            break;
          }
        }
      }).catch(() => {});
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, agent]);

  // provider 变化时拉取其模型
  useEffect(() => {
    if (!providerId) {
      setModels([]);
      return;
    }
    llmApi.listModels(providerId).then((r) =>
      setModels(
        r.list.map((m) => ({
          label: (m.alias || m.model) + (m.isDefault ? '（默认）' : ''),
          value: m.id,
        })),
      ),
    ).catch(() => {});
  }, [providerId]);

  const onSave = async () => {
    const v = await form.validateFields();
    const input: api.AgentInput = {
      name: v.name,
      instruction: v.instruction,
      llmModelId: v.llmModelId ?? undefined,
      builtinTools: v.builtinTools ?? [],
      skillIds: v.skillIds ?? [],
      mcpIds: v.mcpIds ?? [],
      subAgentIds: v.subAgentIds ?? [],
      enabled: v.enabled,
    };
    setSaving(true);
    try {
      if (isEdit) {
        await api.updateAgent(agent!.key, input);
        message.success('已保存 Agent');
      } else {
        await api.createAgent(v.key, input);
        message.success('已创建 Agent');
      }
      onClose(true);
    } catch {
      /* 拦截器已提示 */
    } finally {
      setSaving(false);
    }
  };

  const onDelete = async () => {
    setDeleting(true);
    try {
      await api.deleteAgent(agent!.key);
      message.success('已删除 Agent');
      onClose(true);
    } catch {
      /* 拦截器已提示（内置不可删由后端拦截） */
    } finally {
      setDeleting(false);
    }
  };

  return (
    <Drawer
      title={isEdit ? `编辑 Agent：${agent!.name}` : '新建 Agent'}
      width={560}
      open={open}
      onClose={() => onClose(false)}
      destroyOnClose
      footer={
        <Space style={{ width: '100%', justifyContent: 'space-between' }}>
          <span>
            {isEdit && !agent!.isBuiltin && (
              <Popconfirm title="确定删除该 Agent？" onConfirm={onDelete} okText="删除" okButtonProps={{ danger: true }}>
                <Button danger icon={<DeleteOutlined />} loading={deleting}>
                  删除
                </Button>
              </Popconfirm>
            )}
          </span>
          <Space>
            <Button onClick={() => onClose(false)}>取消</Button>
            <Button type="primary" loading={saving} onClick={onSave}>
              {isEdit ? '保存' : '创建'}
            </Button>
          </Space>
        </Space>
      }
    >
      <Form form={form} layout="vertical">
        {lockCore && (
          <Alert
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
            message="内置 Agent 仅可调整技能、MCP 服务、子 Agent 与启用状态；名称、系统提示、模型、内置工具不可修改。"
          />
        )}
        {!isEdit && (
          <Form.Item
            name="key"
            label="标识 key"
            tooltip="Agent 的稳定标识，创建后不可改；规则链 Agent 节点、定时任务按 key 引用"
            rules={[
              { required: true, message: '请输入 key' },
              { pattern: /^[a-z][a-z0-9-]*$/, message: '小写字母开头，仅小写字母/数字/中划线' },
            ]}
          >
            <Input placeholder="如 my-assistant" />
          </Form.Item>
        )}
        <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
          <Input placeholder="如：客服助手" disabled={lockCore} />
        </Form.Item>
        <Form.Item
          name="instruction"
          label="系统提示（Instruction）"
          tooltip="定义 Agent 的角色、能力与边界"
        >
          <Input.TextArea rows={5} placeholder="你是…，你可以…，回答时应…" disabled={lockCore} />
        </Form.Item>

        <Form.Item label="LLM 模型（留空用系统默认）">
          <Space.Compact block>
            <Form.Item name="llmProviderId" noStyle>
              <Select
                style={{ width: '45%' }}
                allowClear
                showSearch
                optionFilterProp="label"
                placeholder="接入点"
                options={providers}
                disabled={lockCore}
                onChange={() => form.setFieldsValue({ llmModelId: undefined })}
              />
            </Form.Item>
            <Form.Item name="llmModelId" noStyle>
              <Select
                style={{ width: '55%' }}
                allowClear
                showSearch
                optionFilterProp="label"
                placeholder="模型（可选）"
                options={models}
                disabled={lockCore || !providerId}
              />
            </Form.Item>
          </Space.Compact>
        </Form.Item>

        <Form.Item name="builtinTools" label="内置工具">
          <Checkbox.Group options={ALL_BUILTIN_TOOLS} disabled={lockCore} />
        </Form.Item>

        <Form.Item name="skillIds" label="技能（Skills）">
          <Select mode="multiple" allowClear showSearch optionFilterProp="label" placeholder="挂载技能" options={skills} />
        </Form.Item>

        <Form.Item name="mcpIds" label="MCP 服务">
          <Select mode="multiple" allowClear showSearch optionFilterProp="label" placeholder="挂载 MCP 服务" options={mcps} />
        </Form.Item>

        <Form.Item name="subAgentIds" label="子 Agent（作为工具调用）">
          <Select mode="multiple" allowClear showSearch optionFilterProp="label" placeholder="可被本 Agent 调用的子 Agent" options={agentOpts} />
        </Form.Item>

        <Form.Item name="enabled" label="启用" valuePropName="checked">
          <Switch />
        </Form.Item>
      </Form>
    </Drawer>
  );
}
