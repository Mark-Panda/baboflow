// 字段控件与关系下拉映射（纯展示层，仿 componentZh.ts）。
// 只决定"这个字段用什么控件 / 去哪个接口取下拉"；写入 DSL 的 configuration key 始终用英文原值。
import type { ComponentFormField } from '@/api/component';

// 可用的关系资源（对应后端 list 接口；nodes 为前端本地资源，取自当前画布）。
export type RelationApi =
  | 'chains' // 已发布规则链
  | 'agents' // Agent
  | 'llmProviders' // LLM 接入点
  | 'llmModels' // LLM 模型（两级：依赖 provider）
  | 'mcpServers' // MCP 服务
  | 'archeryInstances' // Archery 实例（跨连接平铺）
  | 'skills' // 技能
  | 'nodes'; // 当前规则链内的节点（本链下拉，可手输跨链）

export interface RelationRef {
  api: RelationApi;
  valueKey: string; // 存进 configuration 的字段（如 agent 用 key、chain 用 id）
  labelKey: string; // 下拉显示字段
  // 附加请求参数（如 chains 只取已发布）
  params?: Record<string, unknown>;
  // 级联：llmModels 依赖上级 provider 字段
  dependsOn?: string;
  // nodes 资源：true 表示多选（输出 string[]）
  multiple?: boolean;
  // nodes 资源：true 表示允许手输跨链格式（chainId:nodeId）
  freeInput?: boolean;
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
  // Archery 查询节点：instanceId 引用一个已同步的 Archery 实例（存 id）
  archeryQuery: {
    instanceId: { api: 'archeryInstances', valueKey: 'id', labelKey: 'instanceName' },
  },
  // Archery schema 浏览节点：instanceId 同上
  archerySchema: {
    instanceId: { api: 'archeryInstances', valueKey: 'id', labelKey: 'instanceName' },
  },
  // ---- 节点引用类：指向当前规则链内的其它节点（本链下拉，部分允许手输跨链）----
  // 引用节点：targetId 指向同链某节点；高级用法可手输 chainId:nodeId 跨链
  ref: {
    targetId: { api: 'nodes', valueKey: 'id', labelKey: 'name', freeInput: true },
  },
  // 获取节点输出：nodeId 指向同链某节点
  fetchNodeOutput: {
    nodeId: { api: 'nodes', valueKey: 'id', labelKey: 'name' },
  },
  // 分组执行：nodeIds 指向同链多个节点（多选）
  groupAction: {
    nodeIds: { api: 'nodes', valueKey: 'id', labelKey: 'name', multiple: true },
  },
  // 组合过滤：nodeIds 指向同链多个过滤节点（多选）
  groupFilter: {
    nodeIds: { api: 'nodes', valueKey: 'id', labelKey: 'name', multiple: true },
  },
  // 遍历/条件循环：do 指向循环体节点（容器模式下一般由子画布决定，可手输 chain:{chainId}）
  for: {
    do: { api: 'nodes', valueKey: 'id', labelKey: 'name', freeInput: true },
  },
  while: {
    do: { api: 'nodes', valueKey: 'id', labelKey: 'name', freeInput: true },
  },
};

// 单选按钮候选：这些字段的静态枚举选项较少时，用 Radio.Group 替代 Select。
// value 为 true 表示"若 options 数量 <= RADIO_MAX_OPTIONS 则用单选"。
const RADIO_MAX_OPTIONS = 4;
const RADIO_FIELDS: Record<string, Record<string, boolean>> = {
  restApiCall: { requestMethod: true },
  dbClient: { driverName: true, opType: true },
  net: { protocol: true },
  mqttClient: { qos: true },
  cacheGet: { outputMode: true, whenKeyNotFound: true },
};

// 静态枚举选项：schema 未声明 component.options（RuleGo 仅在少数字段给了），
// 但 desc 明确了取值集合的字段，前端补一份本地 options（value 用写入 DSL 的原值）。
// label 即中文显示，无需再走 optionZh。选项数 <= RADIO_MAX_OPTIONS 且命中 RADIO_FIELDS 时用单选。
export interface StaticOption {
  label: string;
  value: string | number;
}
const STATIC_OPTIONS: Record<string, Record<string, StaticOption[]>> = {
  dbClient: {
    driverName: [
      { label: 'MySQL', value: 'mysql' },
      { label: 'PostgreSQL', value: 'postgres' },
      { label: 'SQLite', value: 'sqlite3' },
    ],
    opType: [
      { label: 'SELECT（查询）', value: 'SELECT' },
      { label: 'INSERT（新增）', value: 'INSERT' },
      { label: 'UPDATE（更新）', value: 'UPDATE' },
      { label: 'DELETE（删除）', value: 'DELETE' },
    ],
  },
  net: {
    protocol: [
      { label: 'TCP', value: 'tcp' },
      { label: 'UDP', value: 'udp' },
    ],
  },
  mqttClient: {
    qos: [
      { label: '0 - 最多一次', value: 0 },
      { label: '1 - 至少一次', value: 1 },
      { label: '2 - 恰好一次', value: 2 },
    ],
  },
  for: {
    mode: [
      { label: '0 - 忽略结果', value: 0 },
      { label: '1 - 合并为数组', value: 1 },
      { label: '2 - 替换消息体', value: 2 },
      { label: '3 - 异步执行', value: 3 },
    ],
  },
  while: {
    mode: [
      { label: '0 - 忽略结果', value: 0 },
      { label: '1 - 合并为数组', value: 1 },
      { label: '2 - 替换消息体', value: 2 },
    ],
  },
  cacheGet: {
    // 子字段 level（链级/全局），cacheGet/cacheSet/cacheDelete 的 struct 数组行内使用
    level: [
      { label: '链级', value: 'chain' },
      { label: '全局', value: 'global' },
    ],
    outputMode: [
      { label: '并入元数据', value: 0 },
      { label: '替换消息体', value: 1 },
    ],
    whenKeyNotFound: [
      { label: '忽略', value: 'ignore' },
      { label: '报错', value: 'error' },
      { label: '返回空', value: 'default' },
    ],
  },
  cacheSet: {
    level: [
      { label: '链级', value: 'chain' },
      { label: '全局', value: 'global' },
    ],
  },
  cacheDelete: {
    level: [
      { label: '链级', value: 'chain' },
      { label: '全局', value: 'global' },
    ],
  },
  groupAction: {
    matchRelationType: [
      { label: '成功（Success）', value: 'Success' },
      { label: '失败（Failure）', value: 'Failure' },
      { label: '真（True）', value: 'True' },
      { label: '假（False）', value: 'False' },
    ],
  },
};

// 静态枚举查询：命中则该字段按下拉/单选渲染（优先级高于"默认文本框"，低于关系/JSON/KV）。
export function staticOptionsFor(
  ruleType: string,
  fieldName: string,
): StaticOption[] | undefined {
  return STATIC_OPTIONS[ruleType]?.[fieldName];
}

// groupAction.matchRelationType 允许自定义关系名：用可输可选的 AutoComplete。
export function isFreeInputSelect(ruleType: string, fieldName: string): boolean {
  return ruleType === 'groupAction' && fieldName === 'matchRelationType';
}

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
  // 键值对：headers / metadata / env / labels / mapping（字段→表达式） 等
  if (/headers|metadata|env|labels|attributes|mapping/.test(n)) return 'kv';
  // JSON 对象：明确的对象/数组型配置（排除 switch.cases 与 cache 的 struct 数组，各有专用编辑器）
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

// cache 系列 struct 数组字段：cacheGet/cacheDelete.keys、cacheSet.items。
// 这些字段带嵌套子字段 schema（field.fields），用专用 StructArrayField 行编辑器，而非 JSON 编辑器。
const STRUCT_ARRAY_FIELDS: Record<string, Record<string, boolean>> = {
  cacheGet: { keys: true },
  cacheDelete: { keys: true },
  cacheSet: { items: true },
};

// 该字段是否为（前端接管的）struct 数组行编辑器字段。
export function isStructArrayField(ruleType: string, fieldName: string): boolean {
  return !!STRUCT_ARRAY_FIELDS[ruleType]?.[fieldName];
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

// 需要隐藏的字段（RuleGo 的 deprecated 标签不会进 schema，这些字段已废弃、无 label/desc，
// 前端直接不渲染，避免出现无标签的裸输入框）。
const HIDDEN_FIELDS: Record<string, Record<string, boolean>> = {
  delay: { periodInSeconds: true, periodInSecondsPattern: true },
};

// 该字段是否在配置面板隐藏。
export function isHiddenField(ruleType: string, fieldName: string): boolean {
  return !!HIDDEN_FIELDS[ruleType]?.[fieldName];
}

// ---- 长表单分组 ----
// 字段数较多的组件按区分组渲染，基础区默认展开、高级区默认折叠。
export interface FieldGroup {
  title: string;
  fields: string[];
  defaultCollapsed?: boolean;
}
const FIELD_GROUPS: Record<string, FieldGroup[]> = {
  restApiCall: [
    {
      title: '基础',
      fields: ['restEndpointUrlPattern', 'requestMethod', 'withoutRequestBody', 'headers', 'body'],
    },
    {
      title: '超时与并发',
      fields: ['readTimeoutMs', 'maxParallelRequestsCount', 'insecureSkipVerify'],
    },
    {
      title: '代理',
      fields: ['enableProxy', 'useSystemProxyProperties', 'proxyScheme', 'proxyHost', 'proxyPort', 'proxyUser', 'proxyPassword'],
      defaultCollapsed: true,
    },
  ],
  sendEmail: [
    { title: 'SMTP 连接', fields: ['smtpHost', 'smtpPort', 'connectTimeout'] },
    { title: '凭据', fields: ['username', 'password'] },
    { title: '邮件内容', fields: ['from', 'to', 'cc', 'bcc', 'subject', 'body'] },
    { title: '高级', fields: ['enableTls'], defaultCollapsed: true },
  ],
  mqttClient: [
    { title: '连接', fields: ['server', 'clientId', 'maxReconnectInterval'] },
    { title: '凭据', fields: ['username', 'password'] },
    { title: '发布', fields: ['topic', 'qos', 'cleanSession'] },
    { title: 'TLS 证书', fields: ['caFile', 'certFile', 'certKeyFile'], defaultCollapsed: true },
  ],
  dbClient: [
    { title: '连接', fields: ['driverName', 'dsn', 'poolSize'] },
    { title: '查询', fields: ['opType', 'sql', 'params', 'getOne'] },
  ],
  net: [
    { title: '连接', fields: ['protocol', 'server'] },
    { title: '超时与心跳', fields: ['connectTimeout', 'heartbeatInterval'], defaultCollapsed: true },
  ],
};

// 分组查询：返回该组件的分组定义（未配置则 undefined，调用方维持平铺）。
export function fieldGroupsFor(ruleType: string): FieldGroup[] | undefined {
  return FIELD_GROUPS[ruleType];
}
