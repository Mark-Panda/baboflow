## Context

系统已有 `archerySchema` 和 `archeryQuery` RuleGo 节点，Archery connection 凭据由后端管理，实例以 `archery_instance.id` 标识。当前节点主要按节点配置执行，尚未提供统一的 MCP 查询入口，也没有实例列表资源。

本变更需要跨越 Archery 节点、connection 数据模型、规则链 DSL 和 MCP exposure。MCP 暴露模型是一条已发布规则链对应一个工具，因此采用一个 `action` 分流工具。

## Goals / Non-Goals

**Goals:**

- 创建一条支持 5 种 `action` 的 Archery 查询规则链。
- 支持动态传入 `instanceId`，并校验实例归属和访问权限。
- 支持实例发现、数据库、表、字段结构和只读数据查询。
- 为实例发现提供租户级默认 Archery connection。
- 保持结构化 JSON 输出和现有 SQL 只读限制。

**Non-Goals:**

- 不开放 INSERT、UPDATE、DELETE、DDL 或多语句执行。
- 不新增 Archery 平台本身的权限体系。
- 不在本次变更中实现五个独立 MCP 工具。
- 不允许 MCP 调用方通过入参切换实例列表使用的默认 connection。

## Decisions

### 1. 使用一个 action 分流 MCP 工具

MCP exposure 当前以一条规则链注册一个工具。规则链通过 `action` 使用 `switch` 分流，避免修改 MCP 注册模型，同时保持后续扩展动作的能力。MCP input schema 明确描述每个动作的字段要求。

### 2. 动态 instanceId 的解析顺序

节点读取顺序固定为 `metadata.instanceId` → `msg.instanceId` → 节点默认值。这样兼容已有节点配置，并允许 MCP 通过消息体传入实例。实例 ID 必须经过租户归属校验，再交给 Archery client factory 创建客户端。

### 3. 实例列表使用租户默认 connection

`listInstances` 不依赖 `instanceId`，使用租户明确设置的默认 Archery connection 同步并返回实例。connection 增加默认标记，并保证同一租户最多一个默认 connection。未配置默认 connection 时返回业务错误，不按创建时间隐式选择。

### 4. 扩展 archerySchema 资源

在 `archerySchema` 增加 `instances` resource。该 resource 通过默认 connection 获取实例，其余 resources 仍通过动态 `instanceId` 获取客户端。数据库、表和字段查询复用现有 Archery client 方法。

### 5. 规则链输入输出

MCP 和调试输入使用 JSON 消息体，输出统一为 JSON。缺少字段、未知 action、实例不存在或 Archery 调用失败均转换为明确失败结果，不调用不满足条件的 Archery 分支。

### 6. 默认 connection 管理

在 connection 管理接口增加设置/取消默认 connection 的能力。设置时按租户清除其他默认标记并设置目标 connection，使用事务保证唯一性。默认 connection 的凭据继续使用现有加密存储。

## Risks / Trade-offs

- [一个工具需要 AI 正确填写 action] → 在 MCP input schema 和描述中列出五种 action 及必填字段，并在规则链入口校验。
- [默认 connection 未配置] → 实例列表直接返回明确错误，并在管理界面提示配置。
- [外部 instanceId 越权] → 在 client factory 前校验租户和实例归属，禁止直接信任消息字段。
- [查询结果过大] → 复用 `archeryQuery` 既有行数上限和只读校验。
- [默认标记迁移兼容] → 新字段使用 false 默认值，已有数据不改变；部署后由管理员显式设置默认 connection。

## Migration Plan

1. 增加默认 connection 字段并执行自动迁移。
2. 发布后在每个租户的 Archery connection 中设置一个默认项。
3. 创建并调试规则链的五种 action。
4. 发布规则链并配置 MCP exposure。
5. 若需回滚，停用 MCP exposure；保留旧 Archery 节点和 connection 数据，不影响已有规则链。

## Open Questions

- MCP 工具名称和规则链名称由产品侧最终确认。
- 是否需要在前端 connection 管理页增加默认标记展示和操作。
