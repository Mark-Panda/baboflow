# M0 Spike 验证记录

> 目的：在设计落地前验证 aggo v2 / RuleGo 最新版的 5 个框架能力假设，避免大规模返工。
> 环境：go1.25 / `github.com/CoolBanHub/aggo v0.3.2` / `github.com/rulego/rulego v0.36.0`（eino 由 aggo 传递依赖 `cloudwego/eino v0.9.5`）。
> 结论：**5 项假设全部成立，设计无需改动架构，仅需按下列真实 API 微调实现。** 详见每节末尾「对设计的影响」。

---

## 假设 1：aggo SKILL 能否从 DB/字符串动态注册（而非仅读文件目录）？✅ 成立

**证据**（`eino@v0.9.5/adk/middlewares/skill/skill.go`）：
```go
type Backend interface {
    List(ctx context.Context) ([]FrontMatter, error)
    Get(ctx context.Context, name string) (Skill, error)
}
type Skill struct { FrontMatter; Content string; BaseDirectory string }
type FrontMatter struct { Name, Description, Context, Agent, Model string }
```
- skill 中间件 `New(ctx, &skill.Config{Backend: ...})` 接受**自定义 Backend**。
- 默认实现是 `NewBackendFromFilesystem`（读目录），但接口只有两个方法，**我们可以实现 `dbBackend`**（从 `skill` 表读 `frontmatter + content`）。

**对设计的影响**：`skill.file_path` **不必强制落盘**——可由 DB 直接供 skill。仍保留落盘作为缓存/导出。后端需新增 `internal/biz/agentkit/skill_backend.go` 实现 `skill.Backend`。

## 假设 2：subAgent / Agent-as-Tool？✅ 成立

**证据**（`eino@v0.9.5/adk/agent_tool.go`）：
```go
func NewAgentTool(_ context.Context, agent Agent, options ...AgentToolOption) tool.BaseTool
func NewTypedAgentTool[M MessageType](_ context.Context, agent TypedAgent[M], options ...AgentToolOption) tool.BaseTool
// 选项: WithFullChatHistoryAsInput() / WithAgentInputSchema(...)
```
- 直接把一个 `adk.Agent` 包装成 `tool.BaseTool` 挂到父 Agent 的 `Tools` 里。
- 默认**隔离上下文**（除非显式 `WithFullChatHistoryAsInput()`），符合我们 subAgent 隔离执行的设计。

**对设计的影响**：`agentkit/manager.go` 按 `agent_sub_agent` 表，为父 Agent 的每个子 Agent 调 `adk.NewTypedAgentTool(subAgent)` 注入即可。aggo 的 `AgentBuilder.WithTools(...)` 透传 `[]tool.BaseTool`，直接可用。

## 假设 3：RuleGo 组件 config_schema 能否自动拿到？✅ 成立（超出预期）

**证据**（实测 `internal/spike` 输出）：`rulego.Registry.GetComponentForms()` 返回 `types.ComponentFormList`，36 个内置组件，每个含**字段级结构化 schema**：
```json
{"type":"jsFilter","category":"filter","fields":[{"name":"jsScript","type":"string","label":"Filter Script","desc":"...","defaultValue":"return msg.temperature > 50;","rules":[{"required":true,"message":"..."}]}],"relationTypes":["True","False","Failure"],"componentKind":"nc"}
```
- 字段含 `name/label/desc/type/defaultValue/rules(校验)`；组件含 `category/desc/relationTypes/icon`。
- **无需手工为每个组件维护 schema**——直接从注册表反射生成。

**对设计的影响**：`componentsync` 直接调 `rulego.Registry.GetComponentForms()` 落库到 `component_meta.config_schema`；前端动态表单字段类型映射：`type=string+json name(jsScript/json)→Monaco`、`select→下拉`、`number/bool→对应控件`。这大幅降低实现成本。

## 假设 4：for 循环 / 子链 DSL + OnDebug？✅ 成立（需调整组件选型）

**组件真相**（`components/common`）：
- **遍历用 `for`（ForNode，type=`for`）**，配置：
  ```json
  {"range":"msg.items | metadata.list | 1..5 | 表达式", "do":"{nodeId} 或 chain:{chainId}", "mode":0}
  ```
  `do` 指向**同链节点或子规则链**；`mode`：0忽略结果/1合并数组/2每次替换msg/3异步。⚠️ `iterator`（IteratorNode）已**弃用**，勿用。
- **子规则链用 `flow`（ChainNode，type=`flow`）**，配置 `{"targetId":"子链ID"}`，经 `ctx.TellFlow` 执行另一链。
- 实测 `for` DSL 可正常 `rulego.New(...)` 创建引擎。

**OnDebug**（`api/types/config.go`）：
```go
var OnDebug func(ruleChainId, flowType, nodeId string, msg RuleMsg, relationType string, err error)
// 仅当节点 debugMode=true 时回调; Config.OnDebug 亦可按引擎设置
```
- 调试事件通过全局/引擎级 `OnDebug` 回调拿到，含 `flowType`（节点进出）、`relationType`（Success/Failure/True/False）。子链经 `TellFlow` 会带 `WithOnEnd` 回调。

**对设计的影响**：**子链容器节点类型用 `flow`（ChainNode，targetId 指向子链）**；**forEach 用 `for`（do 指向子节点/子链）**。前端的「双击进入子画布」对应编辑 `targetId` 指向的子规则链 DSL（独立 chain），而非内嵌子图。需在画布保存时把容器节点的子图序列化为独立子链并在主链以 `flow`/`for` 引用。OnDebug 可直接对接 chain-debug WS 推送。

## 假设 5：规则链 → MCP 暴露？✅ 成立（应用层实现）

**证据**（`api/types/mcp_provider.go`）：
```go
type MCPToolDefinition struct { Name, Description, InputSchema ... }
type MCPToolProvider interface {
    ListToolDefinitions() ([]MCPToolDefinition, error)
    CallTool(ctx, toolName, args) (string, error)
}
const MCPToolProviderKey = "mcp_tool_provider"
```
- RuleGo 库层定义了 `MCPToolProvider` 接口，由**应用层实现**：把已发布规则链包装为 MCP 工具（`CallTool` 内部 `rulego.Get(chainID).OnMsg(...)`），再经 mark3labs/mcp-go 之类的 server 暴露 `/mcp`。

**对设计的影响**：`rulegokit/mcp.go` 实现 `MCPToolProvider`，从 `mcp_exposure` 表读启用中的工具，`CallTool` 执行对应规则链并返回 output；MCP server 端点用 mark3labs/mcp-go（需引入该依赖，aggo 侧 MCP 客户端同理）。输入 schema 从 `mcp_exposure.input_schema` 提供。

---

## 汇总：对实现的具体调整
| 项 | 原设计 | Spike 后调整 |
|---|---|---|
| SKILL 存储 | 必须落盘 | **DB 后端即可**（实现 `skill.Backend`），落盘仅作缓存 |
| subAgent | 未确定 API | `adk.NewTypedAgentTool(sub)` 挂父 Agent Tools，默认隔离上下文 |
| 组件 schema | 可能需手工维护 | **直接 `GetComponentForms()`**，零手工 |
| forEach 组件 | 泛指 forEach | **`for`（ForNode）**；iterator 已弃用勿用 |
| 子链容器 | 内嵌子图 | **`flow`（ChainNode targetId→独立子链）**；子画布编辑独立子链 DSL |
| 调试 | OnDebug 回调 | 全局/引擎 `OnDebug(ruleChainId,flowType,nodeId,msg,relationType,err)`，节点需 debugMode=true |
| MCP 暴露 | rulego-server 能力 | **应用层实现 `MCPToolProvider`** + 引入 mark3labs/mcp-go |

## 版本锁定（go.mod）
- `github.com/CoolBanHub/aggo v0.3.2`
- `github.com/rulego/rulego v0.36.0`
- 传递：`github.com/cloudwego/eino v0.9.5`
- 待引入：Kratos 最新 v2、GORM 最新、`github.com/mark3labs/mcp-go`（MCP server/client）
