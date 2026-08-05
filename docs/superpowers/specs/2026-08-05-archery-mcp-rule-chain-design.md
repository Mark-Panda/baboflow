# Archery MCP 查询规则链设计

## 目标

创建一条可被 MCP 暴露、供 AI 调用的规则链，通过外部传入的 Archery `instanceId` 支持：

1. 查看当前用户可访问的 Archery 实例；
2. 查询指定实例下的数据库；
3. 查询指定数据库下的表；
4. 查询指定表的字段结构；
5. 执行指定表的只读 `SELECT` 查询。

规则链只暴露为一个 MCP 工具，通过 `action` 字段分流。`instanceId` 支持从外部请求动态传入，节点配置中的值仅作为兼容兜底。

## 输入契约

规则链 `msg` 使用以下 JSON 结构：

```json
{
  "action": "listInstances | listDatabases | listTables | describeTable | query",
  "instanceId": 12,
  "dbName": "业务库",
  "schemaName": "public",
  "tableName": "users",
  "sql": "SELECT id, name FROM users LIMIT 20"
}
```

字段要求：

- `action=listInstances`：无需 `instanceId`；
- `action=listDatabases`：需要 `instanceId`；
- `action=listTables`：需要 `instanceId`、`dbName`；
- `action=describeTable`：需要 `instanceId`、`dbName`、`tableName`；
- `action=query`：需要 `instanceId`、`sql`，可选 `dbName`、`schemaName`；
- 查询能力由现有 `archeryQuery` 节点限制为只读 `SELECT`。

`instanceId` 读取优先级为 `metadata.instanceId` → `msg.instanceId` → 节点默认值。实例列表动作例外，不要求该字段，使用当前租户明确配置的默认 Archery connection。未配置默认 connection 时返回明确错误，不隐式选择连接。

系统需要在 Archery connection 管理中增加租户级默认 connection 设置，同一租户最多只能有一个默认 connection。实例列表先同步该 connection 的实例，再返回当前用户可访问的实例。

## 规则链结构

使用一个 `switch` 节点按 `msg.action` 分流：

```text
switch(action)
├── listInstances  → archerySchema(resource=instances,
│                                   connection=default)
├── listDatabases  → archerySchema(resource=databases, instanceId=${msg.instanceId})
├── listTables     → archerySchema(resource=tables,
│                                   instanceId=${msg.instanceId},
│                                   dbName=${msg.dbName})
├── describeTable  → archerySchema(resource=columns,
│                                   instanceId=${msg.instanceId},
│                                   dbName=${msg.dbName},
│                                   tableName=${msg.tableName})
├── query          → archeryQuery(instanceId=${msg.instanceId},
│                                 dbName=${msg.dbName},
│                                 schemaName=${msg.schemaName},
│                                 sql=${msg.sql})
└── Failure        → 统一错误输出
```

`archerySchema` 和 `archeryQuery` 的 `instanceId` 由外部消息动态解析，节点默认配置只用于兼容旧链。`listInstances` 使用租户默认 connection 获取实例列表。节点成功后将查询结果写回消息，供 MCP 调用方直接读取；未知 action、缺少必要字段或 Archery 返回错误时走 Failure。

## MCP 调用流程

```text
AI → MCP 暴露的规则链工具 → chain.run(msg)
  → switch action
  → Archery Instance/Schema/Query
  → 结构化 JSON 结果
  → MCP 返回给 AI
```

规则链创建后先在画布中调试五种 action，再发布并配置一个 MCP exposure。MCP 工具的输入 schema 应与上述输入契约一致，并说明不同 `action` 的字段要求。

## 安全与边界

- `instanceId` 虽由外部输入，但必须校验当前用户/租户对该实例的访问权限，并确认其属于已配置的 Archery connection；
- 默认 connection 只用于实例发现，不允许通过 MCP 入参切换到其他 connection；
- `query` 仅允许 `SELECT`，不开放写操作；
- `archeryQuery` 保留现有行数上限；
- 不在规则链中拼接或执行 AI 生成的非 `SELECT` 语句；
- Archery 认证仍由已配置的 Archery connection 管理。

## 验证标准

- 五种合法 `action` 均能路由到正确节点；
- `listInstances` 能返回当前 Archery 账号可访问的实例；
- 其他四种 action 能使用动态 `instanceId`；
- 租户未配置默认 connection 时，`listInstances` 返回明确错误；
- 缺少 `dbName`、`tableName` 或 `sql` 时返回明确失败结果；
- 未知 `action` 不会调用 Archery；
- `archeryQuery` 的非 `SELECT` 请求被拒绝；
- 规则链可通过 MCP exposure 被 AI 调用并返回结构化结果；
- 补充规则链 DSL 校验和后端执行测试。
