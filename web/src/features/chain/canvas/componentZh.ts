// RuleGo 组件 -> 中文显示映射（纯展示层）。
// 只用于 UI 显示；写入 DSL 的 `type` 与 `configuration` 的 key 始终用英文原值，保证引擎兼容。
export interface FieldZh {
  label: string;
  desc?: string;
}
export interface ComponentZh {
  name: string; // 组件中文名
  desc?: string; // 组件中文简述
  fields?: Record<string, FieldZh>; // 按字段名映射中文 label/desc
}

// 连接类型（relationType）-> 中文显示映射（纯展示层）。
// DSL connection.type 仍写英文原值；这里只影响画布连线标签、关系下拉、调试面板的显示。
const RELATION_ZH: Record<string, string> = {
  Success: "成功",
  Failure: "失败",
  True: "真",
  False: "假",
  Default: "默认",
};

// 连接类型中文名：命中映射返回中文，否则（switch 自定义分支等）返回原值。
export function relationZhName(relationType: string): string {
  return RELATION_ZH[relationType] ?? relationType;
}

const COMMON_FIELD_ZH: Record<string, FieldZh> = {
  delayMs: { label: "延迟时间", desc: "消息等待时间，单位为毫秒" },
  timeoutMs: { label: "超时时间", desc: "请求或执行的超时时间，单位为毫秒" },
  readTimeoutMs: { label: "读取超时", desc: "读取响应的超时时间，单位为毫秒" },
  maxParallelRequestsCount: { label: "最大并发数" },
  enabled: { label: "启用" },
  name: { label: "名称" },
  label: { label: "显示名称" },
  description: { label: "描述" },
  host: { label: "主机地址" },
  port: { label: "端口" },
  username: { label: "用户名" },
  password: { label: "密码" },
  driverName: { label: "数据库驱动", desc: "例如 mysql、postgres" },
  dsn: { label: "连接串 DSN" },
  sql: { label: "SQL 语句", desc: "支持 ${metadata.x} 占位符" },
  topic: { label: "主题" },
  clientId: { label: "客户端 ID" },
  broker: { label: "Broker 地址" },
  url: { label: "地址" },
  endpoint: { label: "端点地址" },
  requestMethod: { label: "请求方法" },
  headers: { label: "请求头" },
  body: { label: "请求体" },
  content: { label: "内容" },
  template: { label: "模板内容" },
  functionName: { label: "函数名" },
  command: { label: "命令" },
  args: { label: "命令参数" },
  key: { label: "键" },
  value: { label: "值" },
};

const OPTION_LABEL_ZH: Record<string, Record<string, string>> = {
  driverName: {
    mysql: "MySQL",
    postgres: "PostgreSQL",
    postgresql: "PostgreSQL",
    sqlite: "SQLite",
  },
  requestMethod: {
    GET: "GET（查询）",
    POST: "POST（创建/提交）",
    PUT: "PUT（整体更新）",
    PATCH: "PATCH（部分更新）",
    DELETE: "DELETE（删除）",
  },
};

// 覆盖当前注册表 36 个组件；未命中时回退英文原名。
export const COMPONENT_ZH: Record<string, ComponentZh> = {
  // ---- action ----
  delay: {
    name: "延迟",
    desc: "延迟投递消息（delayMs 支持表达式）",
    fields: {
      delayMs: { label: "延迟时间(毫秒)", desc: "支持数字或 ${metadata.x} 表达式" },
      maxPendingMsgs: { label: "最大排队消息数", desc: "延迟队列允许的最大排队消息数，默认 1000" },
      overwrite: { label: "覆盖模式", desc: "开启后仅保留一条排队消息（新覆盖旧），否则全部排队" },
    },
  },
  exec: {
    name: "执行命令",
    desc: "执行本地系统命令（受安全配置约束）",
    fields: {
      cmd: { label: "命令", desc: "要执行的命令，支持 ${metadata.x}、${msg.x} 占位符" },
      args: { label: "命令参数", desc: "参数列表，逐个支持占位符" },
      log: { label: "记录输出", desc: "开启后把命令标准输出写入调试日志" },
      replaceData: { label: "替换消息体", desc: "开启后用命令输出替换消息体" },
    },
  },
  fetchNodeOutput: {
    name: "获取节点输出",
    desc: "读取指定节点缓存的输出",
    fields: {
      nodeId: { label: "目标节点", desc: "选择要读取输出的本链节点" },
    },
  },
  functions: {
    name: "自定义函数",
    desc: "按名称调用已注册的自定义函数",
    fields: {
      functionName: { label: "函数名", desc: "已注册的函数名，支持 ${metadata.x}、${msg.x} 占位符" },
      param: { label: "函数入参", desc: "函数输入参数，支持占位符；留空用消息体" },
    },
  },
  log: {
    name: "日志",
    desc: "用 JavaScript 格式化并记录消息",
    fields: {
      jsScript: {
        label: "日志脚本",
        desc: "JS 脚本，须 return 一个字符串；可用 msg/metadata/msgType",
      },
    },
  },

  // ---- common（流程控制）----
  break: { name: "跳出循环", desc: "跳出 for 循环（置 _break 标记）" },
  comment: { name: "注释", desc: "画布注释节点，不参与执行" },
  end: { name: "结束", desc: "结束节点，触发规则链完成" },
  for: {
    name: "遍历循环",
    desc: "对集合做 range 遍历（容器，可含子节点）",
    fields: {
      range: { label: "遍历范围", desc: "如 msg.list、metadata.list、1..5 或表达式；留空遍历消息体" },
      do: { label: "循环体节点", desc: "每次迭代执行的节点；容器模式下由子画布决定，一般无需手填；跨链可手输 chain:{chainId}" },
      mode: { label: "结果处理", desc: "每次迭代结果的处理方式" },
    },
  },
  fork: { name: "并行分支", desc: "并行网关，把消息广播到各分支" },
  groupAction: {
    name: "分组执行",
    desc: "将多个节点编组并异步执行",
    fields: {
      matchRelationType: { label: "匹配关系", desc: "判定完成所依据的连接类型，默认 Success" },
      matchNum: { label: "匹配数量", desc: "需命中匹配关系的节点数；0 表示全部" },
      nodeIds: { label: "组内节点", desc: "编组执行的节点（可多选）" },
      timeout: { label: "超时(秒)", desc: "执行超时时间，0 表示不限制" },
      mergeToMap: { label: "合并为对象", desc: "开启后把各节点输出合并为 {节点ID: 结果} 对象" },
    },
  },
  inclusive: { name: "包容网关", desc: "评估所有条件，命中即放行" },
  join: {
    name: "汇聚",
    desc: "等待 fork 的所有分支完成后汇合",
    fields: {
      timeout: { label: "超时(秒)", desc: "等待所有分支的超时时间，0 表示不限制" },
      mergeToMap: { label: "合并为对象", desc: "开启后把各分支输出合并为 {分支名: 结果} 对象，否则用最后一条消息" },
    },
  },
  ref: {
    name: "引用节点",
    desc: "引用并执行同链中的另一个节点",
    fields: {
      targetId: { label: "目标节点", desc: "选择本链节点；跨链可手输 chainId:nodeId" },
      tellChain: { label: "执行整链", desc: "开启后从目标节点起执行整条链，否则只执行目标节点" },
    },
  },
  switch: {
    name: "条件分支",
    desc: "排他条件路由，命中第一个为真的分支",
    fields: { cases: { label: "分支条件", desc: "逐条评估，命中即路由" } },
  },
  while: {
    name: "条件循环",
    desc: "条件为真时反复执行循环体",
    fields: {
      condition: { label: "循环条件", desc: "每次迭代前求值，为真则继续，如 ${msg.count} < 10" },
      do: { label: "循环体节点", desc: "每次迭代执行的节点；跨链可手输 chain:{chainId}" },
      mode: { label: "结果处理", desc: "每次迭代结果的处理方式" },
    },
  },

  // ---- external ----
  cacheDelete: {
    name: "缓存删除",
    desc: "从链/全局缓存删除数据",
    fields: {
      keys: { label: "缓存键", desc: "要删除的缓存键列表（级别 + 键）" },
    },
  },
  cacheGet: {
    name: "缓存读取",
    desc: "从链/全局缓存读取数据",
    fields: {
      keys: { label: "缓存键", desc: "要读取的缓存键列表（级别 + 键）" },
      outputMode: { label: "输出方式", desc: "读取结果写入消息的方式" },
      whenKeyNotFound: { label: "键不存在时", desc: "缓存键未命中时的处理" },
    },
  },
  cacheSet: {
    name: "缓存写入",
    desc: "向链/全局缓存写入数据",
    fields: {
      items: { label: "缓存项", desc: "要写入的缓存项（级别 + 键 + 值 + TTL）" },
    },
  },
  dbClient: {
    name: "数据库",
    desc: "SQL 数据库客户端（MySQL/PostgreSQL 等）",
    fields: {
      driverName: { label: "数据库驱动", desc: "mysql / postgres / sqlite3 等" },
      dsn: { label: "连接串 DSN", desc: "如 user:password@tcp(host:port)/dbname" },
      poolSize: { label: "连接池大小", desc: "数据库连接池大小" },
      opType: { label: "操作类型", desc: "SELECT / INSERT / UPDATE / DELETE" },
      sql: { label: "SQL 语句", desc: "支持 ${metadata.x}、${msg.x} 占位符" },
      params: { label: "SQL 参数", desc: "参数列表，支持 ${metadata.x} 占位符" },
      getOne: { label: "只取一条", desc: "开启仅返回首条记录，否则返回全部" },
    },
  },
  mqttClient: {
    name: "MQTT 发布",
    desc: "向 MQTT Broker 发布消息",
    fields: {
      server: { label: "Broker 地址", desc: "格式 tcp://host:port" },
      username: { label: "用户名" },
      password: { label: "密码" },
      topic: { label: "发布主题", desc: "支持 ${metadata.x}、${msg.x} 占位符" },
      maxReconnectInterval: { label: "最大重连间隔(毫秒)" },
      qos: { label: "QoS 等级", desc: "消息投递质量等级" },
      cleanSession: { label: "清理会话", desc: "开启后清除之前的会话状态" },
      clientId: { label: "客户端 ID", desc: "MQTT 客户端唯一标识" },
      caFile: { label: "CA 证书", desc: "CA 证书文件路径" },
      certFile: { label: "客户端证书", desc: "TLS 客户端证书文件路径" },
      certKeyFile: { label: "客户端私钥", desc: "TLS 客户端私钥文件路径" },
    },
  },
  net: {
    name: "网络通信",
    desc: "TCP/UDP 等网络协议通信",
    fields: {
      protocol: { label: "协议", desc: "tcp 或 udp，默认 tcp" },
      server: { label: "服务器地址", desc: "格式 host:port" },
      connectTimeout: { label: "连接超时(秒)" },
      heartbeatInterval: { label: "心跳间隔(秒)" },
    },
  },
  restApiCall: {
    name: "HTTP 请求",
    desc: "调用外部 HTTP API",
    fields: {
      restEndpointUrlPattern: {
        label: "请求地址",
        desc: "支持 ${metadata.x} 变量",
      },
      requestMethod: { label: "请求方法", desc: "GET/POST/PUT/DELETE…" },
      headers: { label: "请求头" },
      body: { label: "请求体", desc: "可用 ${} 引用消息/元数据" },
      readTimeoutMs: { label: "超时(毫秒)" },
      maxParallelRequestsCount: { label: "最大并发数" },
    },
  },
  sendEmail: {
    name: "发送邮件",
    desc: "经 SMTP（支持 TLS）发送邮件",
    fields: {
      smtpHost: { label: "SMTP 主机", desc: "SMTP 服务器地址" },
      smtpPort: { label: "SMTP 端口", desc: "默认 25" },
      username: { label: "用户名", desc: "SMTP 认证用户名" },
      password: { label: "密码", desc: "SMTP 认证密码" },
      enableTls: { label: "启用 TLS", desc: "开启 TLS 加密" },
      connectTimeout: { label: "连接超时(秒)" },
      from: { label: "发件人", desc: "发件人邮箱地址" },
      to: { label: "收件人", desc: "收件人邮箱，多个用逗号分隔" },
      cc: { label: "抄送", desc: "抄送邮箱，多个用逗号分隔" },
      bcc: { label: "密送", desc: "密送邮箱，多个用逗号分隔" },
      subject: { label: "主题", desc: "支持 ${metadata.x}、${msg.x} 占位符" },
      body: { label: "正文", desc: "支持 ${metadata.x}、${msg.x} 占位符" },
    },
  },
  ssh: {
    name: "SSH 命令",
    desc: "SSH 远程执行命令",
    fields: {
      host: { label: "主机地址", desc: "SSH 服务器主机" },
      port: { label: "端口", desc: "默认 22" },
      username: { label: "用户名" },
      password: { label: "密码" },
      cmd: { label: "命令", desc: "要执行的 Shell 命令，支持 ${metadata.x}、${msg.x} 占位符" },
    },
  },

  // ---- filter ----
  exprFilter: {
    name: "表达式过滤",
    desc: "用 expr-lang 表达式过滤消息",
    fields: {
      expr: {
        label: "过滤表达式",
        desc: "expr 语法，结果为真则放行，如 msg.temperature > 50",
      },
    },
  },
  fieldFilter: {
    name: "字段过滤",
    desc: "按字段存在性/取值过滤消息",
    fields: {
      checkAllKeys: { label: "全部满足", desc: "开启需所有字段都存在，否则任一存在即放行" },
      dataNames: { label: "消息体字段", desc: "逗号分隔的字段名，检查消息体（JSON）" },
      metadataNames: { label: "元数据字段", desc: "逗号分隔的字段名，检查消息元数据" },
    },
  },
  groupFilter: {
    name: "组合过滤",
    desc: "多个过滤节点编组联合评估",
    fields: {
      allMatches: { label: "全部通过", desc: "开启为 AND（全部通过），否则 OR（任一通过）" },
      nodeIds: { label: "组内过滤节点", desc: "参与联合评估的过滤节点（可多选）" },
      timeout: { label: "超时(秒)", desc: "执行超时时间，0 表示不限制" },
    },
  },
  jsFilter: {
    name: "JS 过滤",
    desc: "用 JavaScript 表达式过滤消息",
    fields: {
      jsScript: {
        label: "过滤脚本",
        desc: "JS 脚本，return true 放行 / false 拦截；可用 msg/metadata/msgType",
      },
    },
  },
  jsSwitch: {
    name: "JS 路由",
    desc: "用 JavaScript 决定消息路由到哪个分支",
    fields: {
      jsScript: {
        label: "路由脚本",
        desc: 'JS 脚本，return 分支关系名数组，如 return ["High","Low"]',
      },
    },
  },
  msgTypeSwitch: { name: "消息类型路由", desc: "按消息类型路由到匹配连接" },

  // ---- flow ----
  flow: {
    name: "子规则链",
    desc: "按 targetId 执行一条子规则链（容器）",
    fields: {
      targetId: { label: "目标规则链", desc: "选择一条已发布的子规则链" },
      extend: { label: "逐条转发", desc: "开启后直接转发子链每条输出，否则合并为单一结果" },
    },
  },

  // ---- agent ----
  agent: {
    name: "Agent",
    desc: "调用一个已配置的 Agent，把结果文本写入消息",
    fields: {
      agentKey: { label: "Agent", desc: "从已配置的 Agent 中选择" },
      prompt: {
        label: "输入模板",
        desc: "可选；留空则用上游消息体作为 Agent 输入",
      },
      timeoutMs: { label: "超时(毫秒)", desc: "0 表示不限制" },
    },
  },

  // ---- transform ----
  exprTransform: {
    name: "表达式转换",
    desc: "用 expr-lang 转换消息",
    fields: {
      expr: { label: "转换表达式", desc: "expr 语法，返回新消息体；优先于下方映射" },
      mapping: { label: "字段映射", desc: "字段名 → 表达式，如 name → upper(msg.name)；expr 为空时生效" },
    },
  },
  jsTransform: {
    name: "JS 转换",
    desc: "用 JavaScript 转换消息体/元数据/类型",
    fields: {
      jsScript: {
        label: "转换脚本",
        desc: "须 return {'msg':msg,'metadata':metadata,'msgType':msgType}；可用 msg/metadata/msgType/dataType",
      },
    },
  },
  metadataTransform: {
    name: "元数据转换",
    desc: "用 expr-lang 转换消息元数据",
    fields: {
      mapping: { label: "字段映射", desc: "元数据键 → 表达式，如 temperature → msg.temperature" },
      isNew: { label: "新建元数据", desc: "开启后重建元数据结构，否则在现有键上更新" },
    },
  },
  "text/template": {
    name: "模板渲染",
    desc: "用 Go text/template 渲染消息",
    fields: {
      template: {
        label: "模板内容",
        desc: "Go template 语法，如 {{ .msg.id }}",
      },
    },
  },
};

export function componentZhName(type: string): string {
  return COMPONENT_ZH[type]?.name ?? type;
}
export function componentZhDesc(type: string): string | undefined {
  return COMPONENT_ZH[type]?.desc;
}
export function fieldZh(type: string, fieldName: string): FieldZh | undefined {
  return COMPONENT_ZH[type]?.fields?.[fieldName] ?? COMMON_FIELD_ZH[fieldName];
}

export function optionZh(
  fieldName: string,
  value: unknown,
): string | undefined {
  return OPTION_LABEL_ZH[fieldName]?.[String(value)];
}
