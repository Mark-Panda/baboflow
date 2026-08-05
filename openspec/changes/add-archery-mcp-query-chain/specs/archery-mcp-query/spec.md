## ADDED Requirements

### Requirement: MCP 查询工具按 action 分流

系统 SHALL 将 Archery 查询规则链暴露为一个 MCP 工具，并根据 `action` 分流到对应的 Archery 能力。

#### Scenario: 查询可访问实例

- **WHEN** MCP 调用传入 `{"action":"listInstances"}`
- **THEN** 系统使用租户默认 Archery connection 返回当前账号可访问的实例列表

#### Scenario: 查询实例下的数据库

- **WHEN** MCP 调用传入 `{"action":"listDatabases","instanceId":12}`
- **THEN** 系统校验实例权限后返回实例下的数据库列表

#### Scenario: 查询数据库下的表

- **WHEN** MCP 调用传入 `{"action":"listTables","instanceId":12,"dbName":"app"}`
- **THEN** 系统返回指定数据库下的表列表

#### Scenario: 查询表结构

- **WHEN** MCP 调用传入 `{"action":"describeTable","instanceId":12,"dbName":"app","tableName":"users"}`
- **THEN** 系统返回指定表的字段结构

#### Scenario: 查询表数据

- **WHEN** MCP 调用传入 `{"action":"query","instanceId":12,"dbName":"app","sql":"SELECT id FROM users LIMIT 20"}`
- **THEN** 系统执行只读查询并返回结构化数据

### Requirement: 动态实例参数

Archery schema 和 query 节点 SHALL 支持从运行时消息读取 `instanceId`，读取优先级 SHALL 为 `metadata.instanceId`、`msg.instanceId`、节点默认值。

#### Scenario: 使用消息体实例

- **WHEN** 消息体包含有效的 `instanceId` 且 metadata 未提供该字段
- **THEN** 节点使用消息体实例执行查询

#### Scenario: metadata 覆盖消息体

- **WHEN** metadata 和消息体都包含 `instanceId`
- **THEN** 节点使用 metadata 中的实例

#### Scenario: 实例不属于当前租户

- **WHEN** 运行时传入的实例不属于当前租户或不存在
- **THEN** 系统拒绝执行 Archery 请求并返回明确错误

### Requirement: 默认 Archery connection

系统 SHALL 支持为租户设置唯一的默认 Archery connection，实例列表 SHALL 使用该 connection。

#### Scenario: 设置默认 connection

- **WHEN** 管理员将一个 Archery connection 设置为默认
- **THEN** 系统清除该租户其他 connection 的默认标记并设置目标 connection

#### Scenario: 未配置默认 connection

- **WHEN** 调用 `listInstances` 且租户没有默认 connection
- **THEN** 系统返回明确错误且不隐式选择其他 connection

### Requirement: 输入校验和只读安全

规则链 SHALL 在调用 Archery 前校验 action 和必填字段，并 SHALL 拒绝非只读 SQL。

#### Scenario: 缺少必填字段

- **WHEN** `listTables` 缺少 `dbName`、`describeTable` 缺少 `tableName` 或 `query` 缺少 `sql`
- **THEN** 系统返回参数错误且不调用 Archery

#### Scenario: 未知 action

- **WHEN** MCP 传入不支持的 `action`
- **THEN** 系统返回参数错误且不调用 Archery

#### Scenario: 拒绝写操作

- **WHEN** `query` 传入 INSERT、UPDATE、DELETE、DDL 或多语句 SQL
- **THEN** 系统拒绝执行并返回只读限制错误
