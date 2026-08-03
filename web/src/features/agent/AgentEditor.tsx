import { useEffect, useMemo, useState } from 'react';
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

// 技能选项：携带来源类型（source）以支持按类型分组筛选。
interface SkillOption extends Option {
  source: string;
}

// 技能来源 → 中文（与 SKILL 页一致）。
const SKILL_SOURCE_LABEL: Record<string, string> = {
  component: '系统组件',
  upload: '业务组件',
  chain: '规则链',
  agent: 'Agent',
  builtin: '内置',
};
const skillSourceLabel = (s: string) => SKILL_SOURCE_LABEL[s] ?? s;

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
  const [skills, setSkills] = useState<SkillOption[]>([]);
  const [mcps, setMcps] = useState<Option[]>([]);
  const [agentOpts, setAgentOpts] = useState<Option[]>([]);
  // 技能来源筛选（多选）：选中若干类型后，技能下拉只展示这些类型；空=全部。
  const [skillSourceFilter, setSkillSourceFilter] = useState<string[]>([]);
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
      setSkills(r.list.map((s) => ({ label: s.name, value: s.id, source: s.source }))),
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

  // 当前数据里实际存在的技能来源类型（按固定顺序），供筛选器使用。
  const presentSkillSources = useMemo(() => {
    const set = new Set(skills.map((s) => s.source));
    const known = Object.keys(SKILL_SOURCE_LABEL).filter((k) => set.has(k));
    const extra = [...set].filter((s) => !(s in SKILL_SOURCE_LABEL));
    return [...known, ...extra];
  }, [skills]);

  // 选中类型筛选后，仅保留这些类型的技能；空筛选=全部。
  const visibleSkills = useMemo(
    () =>
      skillSourceFilter.length
        ? skills.filter((s) => skillSourceFilter.includes(s.source))
        : skills,
    [skills, skillSourceFilter],
  );

  // 技能下拉选项：按来源类型分组（Select.OptGroup），组内即技能。
  const groupedSkillOptions = useMemo(() => {
    const bySource = new Map<string, SkillOption[]>();
    for (const s of visibleSkills) {
      const arr = bySource.get(s.source) ?? [];
      arr.push(s);
      bySource.set(s.source, arr);
    }
    const order = [
      ...Object.keys(SKILL_SOURCE_LABEL).filter((k) => bySource.has(k)),
      ...[...bySource.keys()].filter((k) => !(k in SKILL_SOURCE_LABEL)),
    ];
    return order.map((src) => ({
      label: skillSourceLabel(src),
      options: (bySource.get(src) ?? []).map((s) => ({ label: s.label, value: s.value })),
    }));
  }, [visibleSkills]);

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
          <Select
            mode="multiple"
            allowClear
            showSearch
            optionFilterProp="label"
            placeholder="挂载技能"
            options={groupedSkillOptions}
            popupMatchSelectWidth={false}
            popupRender={(menu) => (
              <>
                <div style={{ padding: '6px 8px', borderBottom: '1px solid #f0f0f0' }}>
                  <Select
                    mode="multiple"
                    allowClear
                    size="small"
                    maxTagCount={2}
                    placeholder="按类型筛选（默认全部）"
                    style={{ width: '100%' }}
                    value={skillSourceFilter}
                    onChange={setSkillSourceFilter}
                    options={presentSkillSources.map((s) => ({ label: skillSourceLabel(s), value: s }))}
                  />
                </div>
                {menu}
              </>
            )}
          />
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
