## Why

当前 Archery 能力只能通过规则链节点被动使用，AI 无法通过统一 MCP 工具发现可用实例并逐级查询数据库、表结构及表数据。需要将这些只读能力组合成一条可暴露的规则链，并支持外部动态传入 `instanceId`，避免将查询范围固定在单个实例。

## What Changes

- 新增一条通过 `action` 分流的 Archery 查询规则链。
- 支持查看默认 Archery connection 下可访问的实例。
- 支持按 `instanceId` 查询数据库、表、表结构和只读表数据。
- 扩展 `archerySchema` 支持实例列表资源。
- 让 `archerySchema` 和 `archeryQuery` 从 `metadata.instanceId` 或 `msg.instanceId` 动态解析实例。
- 增加租户级默认 Archery connection 配置，用于实例发现。
- 将规则链暴露为一个带结构化输入 schema 的 MCP 工具。
- 保留现有查询安全边界，仅允许只读 `SELECT`。

## Capabilities

### New Capabilities

- `archery-mcp-query`: 通过 MCP 暴露 Archery 实例发现、数据库查询、表查询、表结构查询和只读数据查询能力。

### Modified Capabilities

无。

## Impact

- 规则链 Archery 节点及其参数解析、资源分支和 DSL 配置。
- Archery connection 数据模型、默认 connection 管理接口及权限校验。
- MCP 规则链暴露的输入 schema 和执行入口。
- 相关 Go 单元测试、规则链 DSL 校验测试和 Archery 节点测试。
