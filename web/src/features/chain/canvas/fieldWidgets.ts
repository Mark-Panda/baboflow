// 字段控件与关系下拉映射（纯展示层，仿 componentZh.ts）。
// 只决定"这个字段用什么控件 / 去哪个接口取下拉"；写入 DSL 的 configuration key 始终用英文原值。
import type { ComponentFormField } from '@/api/component';

// 可用的关系资源（对应后端 list 接口）。
export type RelationApi =
  | 'chains' // 已发布规则链
  | 'agents' // Agent
  | 'llmProviders' // LLM 接入点
  | 'llmModels' // LLM 模型（两级：依赖 provider）
  | 'mcpServers' // MCP 服务
  | 'skills'; // 技能

export interface RelationRef {
  api: RelationApi;
  valueKey: string; // 存进 configuration 的字段（如 agent 用 key、chain 用 id）
  labelKey: string; // 下拉显示字段
  // 附加请求参数（如 chains 只取已发布）
  params?: Record<string, unknown>;
  // 级联：llmModels 依赖上级 provider 字段
  dependsOn?: string;
}

// 控件 hint。undefined 表示走 NodeConfigPanel 现有的默认分派。
export type WidgetHint = 'relation' | 'radio' | 'json' | 'kv';

// 关系映射表：ruleType -> fieldName -> 关系资源。
// 只列出"引用其它平台资源"的字段；普通字段不进来。
const RELATION_MAP: Record<string, Record<string, RelationRef>> = {
  // 子规则链节点：targetId 引用一条已发布规则链
  flow: {
    targetId: {
      api: 'chains',
      valueKey: 'id',
      labelKey: 'name',
      params: { status: 'published', pageSize: 200 },
    },
  },
  // 自定义 Agent 节点：agentKey 引用一个已配置 Agent（存 key）
  agent: {
    agentKey: { api: 'agents', valueKey: 'key', labelKey: 'name' },
    llmModelId: {
      api: 'llmModels',
      valueKey: 'id',
      labelKey: 'alias',
      dependsOn: 'llmProviderId',
    },
  },
};

// 单选按钮候选：这些字段的静态枚举选项较少时，用 Radio.Group 替代 Select。
// value 为 true 表示"若 options 数量 <= RADIO_MAX_OPTIONS 则用单选"。
const RADIO_MAX_OPTIONS = 4;
const RADIO_FIELDS: Record<string, Record<string, boolean>> = {
  restApiCall: { requestMethod: true },
  dbClient: { driverName: true },
};

// 关系映射查询：命中则该字段渲染为关系下拉。
export function relationFor(
  ruleType: string,
  fieldName: string,
): RelationRef | undefined {
  return RELATION_MAP[ruleType]?.[fieldName];
}

// 是否用单选按钮（基于 curated 名单 + 选项数量，调用方传入已解析的 options）。
export function useRadio(
  ruleType: string,
  fieldName: string,
  optionCount: number,
): boolean {
  if (!RADIO_FIELDS[ruleType]?.[fieldName]) return false;
  return optionCount > 0 && optionCount <= RADIO_MAX_OPTIONS;
}

// 控件 hint：relation 优先；其次判断 JSON 对象 / 键值对。
export function widgetFor(
  ruleType: string,
  field: ComponentFormField,
): WidgetHint | undefined {
  if (relationFor(ruleType, field.name)) return 'relation';
  if (useRadio(ruleType, field.name, optionCountOf(field))) return 'radio';
  const n = field.name.toLowerCase();
  // 键值对：headers / metadata / env / labels 等
  if (/headers|metadata|env|labels|attributes/.test(n)) return 'kv';
  // JSON 对象：明确的对象/数组型配置（排除 switch.cases，已有专用编辑器）
  if (
    field.type === 'object' ||
    field.type === 'array' ||
    field.type === 'map' ||
    /json|config$|payload/.test(n)
  ) {
    return 'json';
  }
  return undefined;
}

// 解析字段的静态选项数量（component.options 或 validate oneof=）。
export function optionCountOf(field: ComponentFormField): number {
  const fromComponent = field.component?.options;
  if (Array.isArray(fromComponent) && fromComponent.length > 0) {
    return fromComponent.length;
  }
  const m = (field.validate ?? '').match(/oneof=(.+)$/);
  return m ? m[1].trim().split(/\s+/).filter(Boolean).length : 0;
}
