import { useMemo, useState } from 'react';
import { Select, Space, Spin } from 'antd';
import { useQuery } from '@tanstack/react-query';

import { chainApi } from '@/api/chain';
import { listAgents } from '@/api/agent';
import { llmApi } from '@/api/llm';
import { mcpApi } from '@/api/mcp';
import { skillApi } from '@/api/skill';
import { archeryApi } from '@/api/archery';
import type { RelationRef } from './fieldWidgets';
import NodeSelect, { type NodeOption } from './NodeSelect';

export interface RelationSelectProps {
  relation: RelationRef;
  value?: unknown;
  onChange?: (v: unknown) => void;
  placeholder?: string;
  // nodes 资源：当前规则链可引用节点 + 排除自身
  nodes?: NodeOption[];
  excludeId?: string;
}

interface Option {
  label: string;
  value: unknown;
}

// 关系下拉：按 relation.api 调对应 list 接口，showSearch 过滤。
// 统一渲染为受控 Select；LLM 模型走两级级联（先 provider 后 model）；
// nodes 走前端本地节点选择器（本链下拉，可手输跨链）。
export default function RelationSelect({
  relation,
  value,
  onChange,
  placeholder,
  nodes,
  excludeId,
}: RelationSelectProps) {
  if (relation.api === 'nodes') {
    return (
      <NodeSelect
        nodes={nodes ?? []}
        excludeId={excludeId}
        multiple={relation.multiple}
        freeInput={relation.freeInput}
        value={value}
        onChange={onChange}
        placeholder={placeholder}
      />
    );
  }
  if (relation.api === 'llmModels') {
    return (
      <LlmModelCascade
        relation={relation}
        value={value}
        onChange={onChange}
        placeholder={placeholder}
      />
    );
  }
  return (
    <FlatRelationSelect
      relation={relation}
      value={value}
      onChange={onChange}
      placeholder={placeholder}
    />
  );
}

function FlatRelationSelect({
  relation,
  value,
  onChange,
  placeholder,
}: RelationSelectProps) {
  const { options, isLoading } = useRelationOptions(relation);
  return (
    <Select
      showSearch
      allowClear
      optionFilterProp="label"
      loading={isLoading}
      options={options}
      value={value as string | number | undefined}
      onChange={(v) => onChange?.(v)}
      placeholder={placeholder ?? '请选择'}
      notFoundContent={isLoading ? <Spin size="small" /> : undefined}
    />
  );
}

// 拉取并归一化各资源的下拉选项。
function useRelationOptions(relation: RelationRef): {
  options: Option[];
  isLoading: boolean;
} {
  const query = useQuery({
    queryKey: ['relation-options', relation.api, relation.params ?? {}],
    staleTime: 60 * 1000,
    queryFn: async (): Promise<Option[]> => {
      switch (relation.api) {
        case 'chains': {
          const res = await chainApi.list({
            status: 'published',
            pageSize: 200,
            ...(relation.params ?? {}),
          });
          return res.list.map((c) => ({
            label: `${c.name}（v${c.version}）`,
            value: c.id,
          }));
        }
        case 'agents': {
          const res = await listAgents();
          return res.list.map((a) => ({
            label: a.name + (a.enabled ? '' : '（已停用）'),
            value: a.key,
          }));
        }
        case 'llmProviders': {
          const res = await llmApi.listProviders();
          return res.list.map((p) => ({ label: p.name, value: p.id }));
        }
        case 'mcpServers': {
          const res = await mcpApi.listServers();
          return res.list.map((s) => ({ label: s.name, value: s.id }));
        }
        case 'archeryInstances': {
          // 跨连接平铺所有已同步实例，标签带连接名以便区分同名实例。
          const conns = (await archeryApi.listConnections()).list;
          const grouped = await Promise.all(
            conns.map(async (c) => ({
              conn: c,
              instances: (await archeryApi.listInstances(c.id)).list,
            })),
          );
          return grouped.flatMap(({ conn, instances }) =>
            instances.map((i) => ({
              label: `${conn.name} / ${i.instanceName}${i.dbType ? `（${i.dbType}）` : ''}`,
              value: i.id,
            })),
          );
        }
        case 'skills': {
          const res = await skillApi.list({});
          return res.list.map((s) => ({ label: s.name, value: s.id }));
        }
        default:
          return [];
      }
    },
  });
  return { options: query.data ?? [], isLoading: query.isLoading };
}

// LLM 模型两级级联：先选接入点，再选其下模型。最终只把 modelId 写回表单。
function LlmModelCascade({
  value,
  onChange,
  placeholder,
}: RelationSelectProps) {
  const [providerId, setProviderId] = useState<number | undefined>(undefined);

  const providers = useQuery({
    queryKey: ['relation-options', 'llmProviders'],
    staleTime: 60 * 1000,
    queryFn: async () =>
      (await llmApi.listProviders()).list.map((p) => ({
        label: p.name,
        value: p.id,
      })),
  });

  const models = useQuery({
    queryKey: ['relation-options', 'llmModels', providerId],
    enabled: providerId != null,
    staleTime: 60 * 1000,
    queryFn: async () =>
      (await llmApi.listModels(providerId!)).list.map((m) => ({
        label: m.alias || m.model,
        value: m.id,
      })),
  });

  // 若已有 value 但尚未选 provider，尝试定位其所属 provider（用于回显）。
  const allProviders = providers.data ?? [];
  useMemo(() => {
    if (providerId != null || value == null || allProviders.length === 0) return;
    // 回显时无法仅由 modelId 反推 provider（需逐个查询），此处不强行反查，
    // 保留用户手动选择 provider 后即正常。若后端后续提供平铺模型接口可优化。
  }, [providerId, value, allProviders.length]);

  return (
    <Space.Compact block>
      <Select
        style={{ width: '45%' }}
        showSearch
        optionFilterProp="label"
        placeholder="接入点"
        loading={providers.isLoading}
        options={allProviders}
        value={providerId}
        onChange={(pid) => {
          setProviderId(pid);
          onChange?.(undefined); // 切换接入点时清空已选模型
        }}
      />
      <Select
        style={{ width: '55%' }}
        showSearch
        allowClear
        optionFilterProp="label"
        placeholder={placeholder ?? '选择模型'}
        loading={models.isLoading}
        options={models.data ?? []}
        value={value as number | undefined}
        onChange={(v) => onChange?.(v)}
        disabled={providerId == null}
      />
    </Space.Compact>
  );
}
