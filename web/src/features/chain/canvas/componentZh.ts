// RuleGo 组件 -> 中文显示映射（纯展示层）。
// 只用于 UI 显示；写入 DSL 的 `type` 与 `configuration` 的 key 始终用英文原值，保证引擎兼容。
export interface FieldZh {
  label: string;
  desc?: string;
}
export interface ComponentZh {
  name: string;   // 组件中文名
  desc?: string;  // 组件中文简述
  fields?: Record<string, FieldZh>; // 按字段名映射中文 label/desc
}

// 覆盖当前注册表 36 个组件；未命中时回退英文原名。
export const COMPONENT_ZH: Record<string, ComponentZh> = {
  // ---- action ----
  delay: { name: '延迟', desc: '延迟投递消息（delayMs 支持表达式）' },
  exec: { name: '执行命令', desc: '执行本地系统命令（受安全配置约束）' },
  fetchNodeOutput: { name: '获取节点输出', desc: '读取指定节点缓存的输出' },
  functions: { name: '自定义函数', desc: '按名称调用已注册的自定义函数' },
  log: {
    name: '日志', desc: '用 JavaScript 格式化并记录消息',
    fields: { jsScript: { label: '日志脚本', desc: 'JS 脚本，须 return 一个字符串；可用 msg/metadata/msgType' } },
  },

  // ---- common（流程控制）----
  break: { name: '跳出循环', desc: '跳出 for 循环（置 _break 标记）' },
  comment: { name: '注释', desc: '画布注释节点，不参与执行' },
  end: { name: '结束', desc: '结束节点，触发规则链完成' },
  for: {
    name: '遍历循环', desc: '对集合做 range 遍历（容器，可含子节点）',
    fields: {
      range: { label: '遍历范围', desc: '如 msg.list，逐项迭代' },
      do: { label: '循环体', desc: '每次迭代执行的子节点' },
    },
  },
  fork: { name: '并行分支', desc: '并行网关，把消息广播到各分支' },
  groupAction: { name: '分组执行', desc: '将多个节点编组并异步执行' },
  inclusive: { name: '包容网关', desc: '评估所有条件，命中即放行' },
  join: { name: '汇聚', desc: '等待 fork 的所有分支完成后汇合' },
  ref: { name: '引用节点', desc: '引用并执行同链中的另一个节点' },
  switch: {
    name: '条件分支', desc: '排他条件路由，命中第一个为真的分支',
    fields: { cases: { label: '分支条件', desc: '逐条评估，命中即路由' } },
  },
  while: { name: '条件循环', desc: '条件为真时反复执行循环体' },

  // ---- external ----
  cacheDelete: { name: '缓存删除', desc: '从链/全局缓存删除数据' },
  cacheGet: { name: '缓存读取', desc: '从链/全局缓存读取数据' },
  cacheSet: { name: '缓存写入', desc: '向链/全局缓存写入数据' },
  dbClient: {
    name: '数据库', desc: 'SQL 数据库客户端（MySQL/PostgreSQL 等）',
    fields: {
      sql: { label: 'SQL 语句', desc: '支持 ${metadata.x} 占位符' },
      driverName: { label: '驱动', desc: 'mysql / postgres 等' },
      dsn: { label: '连接串 DSN' },
    },
  },
  mqttClient: { name: 'MQTT 发布', desc: '向 MQTT Broker 发布消息' },
  net: { name: '网络通信', desc: 'TCP/UDP 等网络协议通信' },
  restApiCall: {
    name: 'HTTP 请求', desc: '调用外部 HTTP API',
    fields: {
      restEndpointUrlPattern: { label: '请求地址', desc: '支持 ${metadata.x} 变量' },
      requestMethod: { label: '请求方法', desc: 'GET/POST/PUT/DELETE…' },
      headers: { label: '请求头' },
      body: { label: '请求体', desc: '可用 ${} 引用消息/元数据' },
      readTimeoutMs: { label: '超时(毫秒)' },
      maxParallelRequestsCount: { label: '最大并发数' },
    },
  },
  sendEmail: { name: '发送邮件', desc: '经 SMTP（支持 TLS）发送邮件' },
  ssh: { name: 'SSH 命令', desc: 'SSH 远程执行命令' },

  // ---- filter ----
  exprFilter: {
    name: '表达式过滤', desc: '用 expr-lang 表达式过滤消息',
    fields: { expr: { label: '过滤表达式', desc: 'expr 语法，结果为真则放行，如 msg.temperature > 50' } },
  },
  fieldFilter: { name: '字段过滤', desc: '按字段存在性/取值过滤消息' },
  groupFilter: { name: '组合过滤', desc: '多个过滤节点编组联合评估' },
  jsFilter: {
    name: 'JS 过滤', desc: '用 JavaScript 表达式过滤消息',
    fields: { jsScript: { label: '过滤脚本', desc: 'JS 脚本，return true 放行 / false 拦截；可用 msg/metadata/msgType' } },
  },
  jsSwitch: {
    name: 'JS 路由', desc: '用 JavaScript 决定消息路由到哪个分支',
    fields: { jsScript: { label: '路由脚本', desc: 'JS 脚本，return 分支关系名数组，如 return ["High","Low"]' } },
  },
  msgTypeSwitch: { name: '消息类型路由', desc: '按消息类型路由到匹配连接' },

  // ---- flow ----
  flow: { name: '子规则链', desc: '按 targetId 执行一条子规则链（容器）' },

  // ---- transform ----
  exprTransform: {
    name: '表达式转换', desc: '用 expr-lang 转换消息',
    fields: { expr: { label: '转换表达式', desc: 'expr 语法，返回新消息体' } },
  },
  jsTransform: {
    name: 'JS 转换', desc: '用 JavaScript 转换消息体/元数据/类型',
    fields: {
      jsScript: {
        label: '转换脚本',
        desc: "须 return {'msg':msg,'metadata':metadata,'msgType':msgType}；可用 msg/metadata/msgType/dataType",
      },
    },
  },
  metadataTransform: { name: '元数据转换', desc: '用 expr-lang 转换消息元数据' },
  'text/template': {
    name: '模板渲染', desc: '用 Go text/template 渲染消息',
    fields: { template: { label: '模板内容', desc: 'Go template 语法，如 {{ .msg.id }}' } },
  },
};

export function componentZhName(type: string): string {
  return COMPONENT_ZH[type]?.name ?? type;
}
export function componentZhDesc(type: string): string | undefined {
  return COMPONENT_ZH[type]?.desc;
}
export function fieldZh(type: string, fieldName: string): FieldZh | undefined {
  return COMPONENT_ZH[type]?.fields?.[fieldName];
}
