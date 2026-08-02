# BaboFlow 前端详细设计

> 技术栈：React 18 · Vite · TypeScript · AntD 5 · ReactFlow 12 · Zustand · React Query · axios · react-router v6 · Monaco · @dnd-kit/core
> 实时：WebSocket（agent-chat / chain-debug 两个 channel）。认证：HttpOnly Cookie Session。
> 目录：`web/`

## 关键决策（锁定）
- 代码编辑器 **Monaco**；看板拖拽 **@dnd-kit/core**；密钥经 **.env** 注入。
- **规则链菜单 = 规则链列表**（创建/编辑/发布/撤销发布/删除）；**画布编辑器不是独立菜单**，从「新建 / 编辑」进入 `/chains/:id/edit`。
- **组件范围**：仅 RuleGo 内置组件 + 平台新增「Agent 组件」（规则链内调用某 Agent）。
- **for 循环/子流程**：子链容器式 —— for/subChain 为容器节点，双击进入独立子画布（可嵌套），类似 N8N 子工作流。
- **Agent 组织**：菜单=Agent 列表；除两个内置任务型 Agent（chain-builder / skill-generator）外，内置一个**通用助手 agent-general**（挂载全部 SKILL+MCP，可通用问答并调用任意能力）。点列表行进入 `/agents/:key/chat`，会话按 Agent 隔离。
- **日志**：规则链运行日志（`/runs`）与 Agent 会话日志（`/agents/logs`）分开，分别设计回放视图。

---

## 第一部分：信息架构与路由

### 1.1 目录结构
```
web/
├── index.html
├── vite.config.ts                # dev 代理 /api 与 /ws 到 Kratos(:8000)
├── package.json
└── src/
    ├── main.tsx                  # 入口: QueryClientProvider + ConfigProvider(zhCN) + Router
    ├── App.tsx
    ├── api/                      # 与后端 REST 对齐
    │   ├── http.ts               # axios 实例(带 cookie, 401 跳登录, 解信封)
    │   ├── auth.ts  llm.ts  chain.ts  component.ts  run.ts
    │   ├── skill.ts  mcp.ts  board.ts  agent.ts  cron.ts  dashboard.ts
    │   └── asset.ts              # 会话附件上传/下载(图片/文件)
    ├── ws/
    │   ├── client.ts             # WS 单例: 自动重连/订阅/分发
    │   └── types.ts              # agent-chat / chain-debug 消息类型
    ├── stores/                   # zustand
    │   ├── authStore.ts          # 当前用户
    │   ├── sessionStore.ts       # 当前 Agent 会话流式消息
    │   ├── canvasStore.ts        # 画布选中节点/调试事件
    │   └── llmStore.ts           # 默认 LLM 展示
    ├── routes/index.tsx          # 路由表 + RequireAuth 守卫
    ├── layouts/
    │   ├── MainLayout.tsx        # 侧边导航 + 顶栏
    │   └── AuthLayout.tsx        # 登录页居中卡片布局
    ├── features/
    │   ├── auth/LoginPage.tsx
    │   ├── dashboard/DashboardPage.tsx
    │   ├── chain/
    │   │   ├── ChainListPage.tsx
    │   │   └── canvas/
    │   │       ├── ChainEditorPage.tsx     # 编辑器外壳(顶栏工具条 + 三栏)
    │   │       ├── FlowCanvas.tsx          # ReactFlow 画布
    │   │       ├── ComponentPalette.tsx    # 左: 组件面板
    │   │       ├── NodeConfigPanel.tsx     # 右: 动态表单
    │   │       ├── DebugPanel.tsx          # 底: 调试控制台
    │   │       ├── nodes/RuleNode.tsx      # 自定义节点
    │   │       └── chainDsl.ts             # DSL<->ReactFlow 互转
    │   ├── run/RunLogPage.tsx  RunDetailDrawer.tsx
    │   ├── skill/SkillListPage.tsx  SkillUploadModal.tsx  SkillDetailDrawer.tsx
    │   ├── agent/AgentListPage.tsx  AgentChatPage.tsx  AgentLogPage.tsx
    │   ├── mcp/McpPage.tsx  McpExposurePage.tsx
    │   ├── board/BoardListPage.tsx  BoardPage.tsx  TaskCard.tsx  TaskEditModal.tsx
    │   ├── llm/LlmConfigPage.tsx
    │   └── cron/CronPage.tsx
    ├── components/
    │   ├── StatusTag.tsx  JsonEditor.tsx  CodeField.tsx
    │   └── PageHeader.tsx  EmptyBox.tsx
    └── styles/global.css
```

### 1.2 路由表
| 路径 | 页面 | 需求 | 说明 |
|---|---|---|---|
| `/login` | LoginPage | — | 未登录唯一入口 |
| `/` | DashboardPage | 总览 | 统计卡片 + 最近运行 |
| `/chains` | ChainListPage | 5 | **规则链菜单 = 列表**（创建/编辑/发布/撤销发布/删除） |
| `/chains/:id/edit` | ChainEditorPage | 4,5 | **画布编辑器**（从列表 新建/编辑 进入，含新建时 `:id=new`） |
| `/runs` | RunLogPage | 5 | **规则链运行日志**（含调试记录，详情逐节点回放） |
| `/skills` | SkillListPage | 3,6,10 | SKILL 管理 |
| `/agents` | AgentListPage | 1 | **Agent 菜单 = 列表**（内置 3 个 + 自定义） |
| `/agents/:key/chat` | AgentChatPage | 1,3,6 | 某 Agent 的对话（点列表进入） |
| `/agents/logs` | AgentLogPage | 1 | **Agent 会话日志**（会话→消息/工具调用回放，关联 Langfuse） |
| `/mcp` | McpPage | 11 | MCP 服务配置 |
| `/mcp/exposures` | McpExposurePage | 7 | 已暴露工具 |
| `/boards` | BoardListPage | 8 | 看板列表 |
| `/boards/:id` | BoardPage | 8 | 看板拖拽 |
| `/cron` | CronPage | 1 | 定时任务 |
| `/settings/llm` | LlmConfigPage | 9 | LLM 配置 |
| `/settings/audit` | AuditPage | 安全 | 审计日志查询（仅 admin） |

> 菜单可见项：总览 / **规则链** / **运行日志** / 看板 / **Agent** / **Agent 日志** / SKILL / MCP / 定时任务 / LLM 配置 / **审计日志**。
> 「画布编辑器」「Agent 对话」由列表跳入，不作为独立侧边菜单（编辑器为整页覆盖、对话带会话侧栏）。

### 1.3 主布局（MainLayout）
```
┌────────────────────────────────────────────────────────────────────┐
│ ◤ BaboFlow   规则链 · Agent · 看板          🤖默认LLM:GPT-4o │ ⚙Langfuse │ admin ▾ │ ← 顶栏
├──────────┬─────────────────────────────────────────────────────────┤
│ ▣ 总览    │                                                         │
│ ⛓ 规则链  │                                                         │
│ ▤ 运行日志│                  <Outlet />  (各页面)                    │
│ ✦ SKILL  │                                                         │
│ ➤ Agent  │                                                         │
│ ⚡ MCP    │                                                         │
│ ▦ 看板    │                                                         │
│ ⏱ 定时任务│                                                         │
│ ⚙ LLM配置 │                                                         │
└──────────┴─────────────────────────────────────────────────────────┘
```
- 侧边 `Menu`（AntD，受控选中态随路由）；顶栏右侧显示默认 LLM、Langfuse 外链、用户下拉（改密/登出）。

---

## 第二部分：核心页面界面布局（ASCII 线框）

### 2.1 登录页 `/login`
```
              ┌───────────────────────────┐
              │      ◤ BaboFlow           │
              │   Agent × RuleGo 平台     │
              │  ┌─────────────────────┐  │
              │  │ 用户名  [ admin   ] │  │
              │  │ 密码    [ ******* ] │  │
              │  │ ┌───────────────┐   │  │
              │  │ │    登 录      │   │  │
              │  │ └───────────────┘   │  │
              │  └─────────────────────┘  │
              └───────────────────────────┘
```

### 2.2 总览 `/`
```
┌ 统计卡片(4) ────────────────────────────────────────────────┐
│ [规则链 12]   [已发布 8]   [SKILL 15]   [今日运行 132/失败3] │
├ 快捷入口 ────────────────┬ 最近运行(表) ─────────────────────┤
│ [新建规则链] [与Agent对话] │ 链名称      触发  状态    时间     │
│ [配置LLM]   [上传SKILL]   │ HTTP接收入库 task  ✔成功  10:21   │
│                          │ 订单同步    cron  ✘失败  09:50   │
└──────────────────────────┴──────────────────────────────────┘
```

### 2.3 规则链列表 `/chains`（需求5）—— 菜单落地页
```
┌ 工具条 ────────────────────────────────────────────────────────┐
│ [+新建规则链] [💬与Agent生成]  状态[全部▾] 来源[全部▾] 关键字[__][搜索] │
├ 表格 ──────────────────────────────────────────────────────────┤
│ 名称        描述      来源   状态    版本 更新时间   操作           │
│ HTTP接收入库 ...      agent  ●已发布 v3  10-21  编辑|调试|撤销发布|更多▾ │
│ 订单同步     ...      manual ○草稿   v0  09-50  编辑|调试|发布    |更多▾ │
│   更多▾: 生成SKILL | 暴露MCP | 版本回滚 | 复制 | 删除              │
└─────────────────────────────────────────────────────────────────┘
```
- **新建** → 跳 `/chains/new/edit`（空骨架画布）；**编辑** → `/chains/:id/edit`。
- **发布**（草稿→已发布，生成版本快照）；**撤销发布**（已发布→草稿，卸载引擎，等价后端 offline）。
- 已发布链「编辑」保存后产生未发布变更标记（列表态显示 `●已发布 v3 *有改动`），再次发布后生效。

### 2.4 规则链画布编辑器 `/chains/:id/edit`（需求4/5，核心）—— N8N 风格
```
┌ 编辑器顶栏 ───────────────────────────────────────────────────────────────────┐
│ ←返回 │ ●HTTP接收入库 (已发布v3 *) │ 链变量⚙ │ [保存草稿] [发布] [▶调试] [⋮]      │
├───────────┬────────────────────────────────────────────────┬──────────────────┤
│ 组件面板   │                                                │ 节点配置          │
│ 🔍搜索     │              ReactFlow 画布                   │  (选中节点时)     │
│ ▣ 端点     │                                                │ ┌──────────────┐ │
│  HTTP接入  │   ┌─────────┐       ┌─────────┐   ┌─────────┐ │ │ 名称 [JS过滤 ] │ │
│ ▣ 过滤     │   │ ◉ HTTP  │Success│ ◈ JS    │   │ ▶ 日志  │ │ │ 类型 filter/js│ │
│  JS过滤    │   │  接入   │──────▶│  过滤   │──▶│  输出   │ │ │ ──────────── │ │
│  条件分支  │   └─────────┘       └────┬────┘   └─────────┘ │ │ 脚本(Monaco) │ │
│ ▣ 转换     │                        │Failure              │ │ return msg.. │ │
│ ▣ 动作     │                        ▼                     │ └──────────────┘ │
│ ▣ 循环     │                  ┌─────────┐                  │ [运行此节点][删除]│
│  ┗forEach▹ │                  │ ⚠ 告警  │                  │                  │
│  ┗subChain▹│                  └─────────┘                  │                  │
│ ▣ Agent    │                                                │                  │
│  ┗调用Agent│   ┌──── forEach 容器(双击进入子画布) ────┐       │                  │
│ (拖入画布) │   │ ▷ 遍历 items        [进入 ▸]       │       │                  │
│           │   └───────────────────────────────────┘       │                  │
│           │   [小地图]                        [+ - ⛶]      │                  │
├───────────┴────────────────────────────────────────────────┴──────────────────┤
│ ▾ 调试控制台   输入[{\"t\":35}] [▶运行] │ s1接入→Success ✔  s2过滤→Success ✔  s3日志→Failure ✘ │
└──────────────────────────────────────────────────────────────────────────────┘
```
**美观与布局（参考 N8N）：**
- 节点为**圆角卡片**：左侧彩色类别条 + 类别图标 + 名称 + 类型小字；悬浮左侧入点、右侧出点（多个关系类型可多个出点：Success/Failure/True/False）。
- 类别配色固定：endpoint=紫、filter=金、transform=蓝、action=绿、loop=青、agent=品红。
- 连线用**贝塞尔曲线** + 关系类型徽标（Success 绿 / Failure 红 / 其余灰）；选中连线可改关系类型。
- 画布：点状网格背景、MiniMap、缩放/适应视图控件、对齐吸附、框选多选、撤销重做。
- **自动布局**：保存/打开时按 DAG 分层（dagre）排列，节点位置仍存 `additionalInfo.position` 以保持手工布局。

**画布交互规格（实际开发必须实现，静态 Demo 无法还原）：**
- **连线**：从出点拖出连线，悬停目标入点高亮吸附；仅允许 出点→入点；重复/自环连线拒绝并提示。落线后默认 `Success`，点连线徽标可在 Success/Failure/True/False… 间切换。
- **多选/框选**：空白处拖拽出选框；`Cmd/Ctrl+点击` 加选；多选后可整体拖动、对齐（左/顶/分布）、批量删除。
- **键盘**：`Delete` 删选中、`Cmd/Ctrl+Z/Shift+Z` 撤销重做、`Cmd/Ctrl+C/V` 复制粘贴节点、`Space+拖` 平移、滚轮缩放、`Cmd/Ctrl+0` 适应视图。
- **拖拽入图**：组件从面板拖入时显示落点预览；按当前缩放换算坐标。
- **调试易用性**（重点）：
  - **单节点调试**：节点卡片上有「▶」悬浮按钮，单独运行该节点（用上游缓存/手填输入）。
  - **断点**：节点可设断点，调试运行到该节点暂停，展示当前 msg 后再继续。
  - **运行中**：正在执行的节点呼吸高亮、已通过的节点打勾、失败节点红框并在节点上直接浮出错误气泡（hover 看堆栈）。
  - **输入构造**：调试控制台提供「从最近运行取输入 / 手填 JSON / 从看板任务取 payload」三种方式；支持多组输入用例保存复用。
  - **迭代对比**：同节点显示每次迭代的 in/out（forEach 容器可展开看每轮子执行）。

**for 循环 / 子流程（子链容器式）：**
- `forEach` / `subChain` 是**容器节点**，卡片更大、带「进入 ▸」入口与输入/输出锚点。
- **双击容器节点 → 进入其独立子画布**（面包屑 `链名 / forEach`），子画布是另一张 nodes/connections，DSL 中作为该节点的子 `ruleChain`（RuleGo 支持嵌套子链）。
- 支持多层嵌套（子链内再放 forEach），顶部面包屑可逐级返回。
- 容器输入锚点接收被遍历集合，输出锚点给出聚合结果；调试时容器可展开查看每次迭代的子执行。

### 2.5 规则链运行日志 `/runs`（需求5）—— 与 Agent 日志分离
```
┌ 筛选 ──────────────────────────────────────────────┐
│ 规则链[全部▾] 触发[全部▾] 状态[全部▾] 时间[范围] [查询] │
├ 表格 ──────────────────────────────────────────────┤
│ 链名称   触发  状态   开始时间        耗时   操作     │
│ HTTP接入 task ✔成功 10-21 12:00:01 120ms  回放|详情  │
│ 订单同步 cron ✘失败 09-50 00:00:00  3s    回放|详情  │
└─────────────────────────────────────────────────────┘
详情 = 全屏 Drawer / 独立视图：
┌ 运行 #100  HTTP接收入库  ✘失败  120ms ────────────────┐
│ 输入 {…}   输出 {…}   错误 node s3: connection refused │
│ ┌ 只读迷你画布(节点按结果着色) ────────┬ 节点时间线 ────┐│
│ │ 接入✔ → 过滤✔ → 日志✘               │ ▸s1 接入  in/out││
│ │         └Failure→ 告警(未执行,灰)     │ ▸s2 过滤  in/out││
│ │                                     │ ▾s3 日志  err…  ││
│ └─────────────────────────────────────┴───────────────┘│
└─────────────────────────────────────────────────────────┘
```
- 两种视图：**回放**（只读迷你画布，节点按 success绿/failure红/skipped灰 着色，点节点看该次 in/out）+ **详情**（输入/输出 JSON + 节点时间线列表）。
- 数据来源 `chain_run.node_trace`（RuleGo OnDebug 回调落库）。

### 2.6 SKILL 管理 `/skills`（需求3/6/10）
```
┌ 工具条 ────────────────────────────────────────────────┐
│ [+上传SKILL]   来源[全部▾]  关键字[____] [搜索]          │
├ 卡片网格 ──────────────────────────────────────────────┤
│ ┌──────────┐ ┌──────────┐ ┌──────────┐                 │
│ │✦ weather │ │✦http-ing │ │✦订单告警 │                 │
│ │ 上传     │ │ 链生成   │ │ 内置     │                 │
│ │ 查询天气 │ │ HTTP接入 │ │ ...      │                 │
│ │ [查看][删]│ │[查看][删]│ │[查看]   │                 │
│ └──────────┘ └──────────┘ └──────────┘                 │
└─────────────────────────────────────────────────────────┘
查看 Drawer：frontmatter(描述/模型) + markdown 渲染
```

### 2.7 Agent 列表 + 对话（需求1/3/6）

**Agent 列表 `/agents`（菜单落地页）**
```
┌ 工具条 ─────────────────────────────────────────────────────┐
│ [+新建 Agent]                                                │
├ 表格 ───────────────────────────────────────────────────────┤
│ 标识                  名称        类型   模型   SKILL MCP subAgent 操作 │
│ agent-general        通用助手     内置  GPT-4o 全部  全部   2   对话|配置|日志 │
│ agent-chain-builder  规则链生成器 内置  GPT-4o  3    1    0   对话|配置|日志 │
│ agent-skill-generator SKILL生成器 内置  GPT-4o  1    0    0   对话|配置|日志 │
│ my-ops-bot           运维助手     自定义 GLM-4  2    1    1   对话|配置|日志|删除 │
└──────────────────────────────────────────────────────────────┘
```
- 三个内置 Agent（is_builtin，不可删）：`agent-general`（通用助手，挂载全部 SKILL+MCP）、`agent-chain-builder`、`agent-skill-generator`。
- **配置抽屉**可设置：instruction、模型（接入点→模型级联）、SKILL 多选、MCP 多选、**subAgent 多选**（从其他 Agent 中选，配「何时委派」描述）。
- 点击「对话」→ `/agents/:key/chat`；「日志」→ `/agents/logs?agent=:key`。

**Agent 对话 `/agents/:key/chat`（按所选 Agent 隔离会话）**
```
┌ 会话侧栏(该 Agent 的会话)─┬ 聊天区 ────────────────────────────────┐
│ ← Agent列表   [+新会话]    │  通用助手 agent-general   [Langfuse↗] │
│ ▸ 生成HTTP链              │  ┌ 用户 ─────────────────────────┐    │
│ ▸ 订单告警                │  │ 看这张架构图, 生成对应规则链    │    │
│ ▸ …                      │  │ [🖼 arch.png] [📄 spec.pdf]    │    │ ← 附件缩略
│                          │  └────────────────────────────────┘    │
│                          │  ┌ Agent ───────────────────────┐    │
│                          │  │ 我先看下可用组件…              │    │
│                          │  │ ▶ search_component("http") ✔ │    │ ← 工具调用可展开
│                          │  │ ⬡ 委派 subAgent: chain-builder│    │ ← subAgent 执行(可展开其步骤)
│                          │  │   └ 已生成规则链 DSL ✔        │    │
│                          │  │ 已生成草稿, 需要发布吗?        │    │
│                          │  │ [在画布打开] [直接发布]         │    │
│                          │  └───────────────────────────────┘    │
│                          │  [📎] ┌──────────────────┐ [发送]      │
│                          │       │ 输入消息…(支持图片/文件)│       │
└──────────────────────────┴───────────────────────────────────────┘
```
- 会话按 `agentId` 过滤；切换 Agent 即切换会话列表与上下文。
- **附件**：输入框「📎」上传图片/文件（先 `POST /assets` 得 assetId，随消息发送）；图片显示缩略图、文件显示名称+大小；仅当所选模型 `capability.vision=true` 时图片走多模态，否则提示。
- **subAgent**：委派过程以「⬡ 委派 subAgent」卡片展示，可展开看子 Agent 的步骤与结论。
- 流式渲染 token，工具/subAgent 步骤可展开；`done` 显示 tokens 与 Langfuse 链接。

### 2.7b Agent 会话日志 `/agents/logs`（需求1）—— 与规则链日志分离
```
┌ 筛选 ────────────────────────────────────────────────────────┐
│ Agent[全部▾]  时间[范围]  关键字[__]  [查询]                   │
├ 会话列表(左) ────────────┬ 会话回放(右) ───────────────────────┤
│ ▸通用助手 生成HTTP链      │  session#55  agent-general  12条消息 │
│  10-21 12:00  12条  ✔    │  ┌ user: 帮我做HTTP接收入库链      │
│ ▸通用助手 订单告警        │  ┌ asst: 我先看可用组件…           │
│  10-20 18:22   8条  ✔    │  │  ▶ search_component ✔ (展开)   │
│ ▸规则链生成器 …           │  │  tokens:1200  Langfuse↗        │
│                          │  └ …(逐条 user/assistant/tool)      │
└──────────────────────────┴────────────────────────────────────┘
```
- 左侧会话列表（按 Agent 过滤、显示消息数/时间/Langfuse 状态）；右侧只读回放该会话的 user/assistant/tool 序列。
- 数据来自 `agent_session` + `agent_message`（含 toolCalls、tokens、langfuse_trace_id）。

### 2.8 MCP 配置 `/mcp`（需求11）+ 已暴露 `/mcp/exposures`（需求7）
```
/mcp:           表格[name/transport/endpoint|command/状态/最近检测] 操作:[编辑][启停][测试][删除]
/mcp/exposures: 表格[工具名 描述 来源链 MCP端点 状态] 操作:[禁用][取消暴露]
```

### 2.9 看板 `/boards/:id`（需求8）
```
┌ 看板顶栏 ───────────────────────────────────────────────────────┐
│ 数据管道   [+新建列]  [+新建任务]                                 │
├ 拖拽列(横向滚动) ────────────────────────────────────────────────┤
│ ┌待办───┐   ┌进行中─┐   ┌已完成─┐                                │
│ │┌────┐ │   │┌────┐ │   │┌────┐ │                                │
│ ││订单 │ │   ││库存 │ │   ││日志 │ │                                │
│ ││同步 │ │   ││同步 │ │   ││清洗 │ │                                │
│ ││⛓链: │ │   ││⛓链: │ │   ││✔成功│ │  卡片: 标题+分配的链+状态     │
│ ││HTTP │ │   ││⏳执行 │ │   │└────┘ │  点击→编辑(分配规则链/触发)   │
│ │└────┘ │   │└────┘ │   │        │                                │
│ │[+任务] │   │[+任务] │   │[+任务] │                                │
│ └───────┘   └───────┘   └───────┘                                │
└──────────────────────────────────────────────────────────────────┘
```

### 2.10 LLM 配置 `/settings/llm`（需求9）—— 接入点 + 模型 两级
```
┌ 左: 接入点列表 ────────────┬ 右: 选中接入点下的模型 ──────────────────────┐
│ [+新增接入点]              │  OpenAI官方  https://api.openai.com/v1        │
│ ┌───────────────────────┐ │  sk-****abcd   [编辑] [测试连接] [删除]        │
│ │●OpenAI官方   (3 模型)  │ │ ──────────────────────────────────────────  │
│ │ openai  api.openai.com │ │ 模型  [+登记模型] [从接入点拉取]             │
│ └───────────────────────┘ │ ┌──────────────────────────────────────────┐ │
│ ┌───────────────────────┐ │ │ gpt-4o        alias:GPT4o  ★默认 chat  ✔ │ │
│ │ 公司网关     (5 模型)  │ │ │ gpt-4o-mini   alias:-           chat  ✔ │ │
│ │ openai  gw.internal   │ │ │ text-embed-3  alias:-     embedding  ✔ │ │
│ └───────────────────────┘ │ └──────────────────────────────────────────┘ │
│ ┌───────────────────────┐ │  每行操作: [编辑][设为默认][测试][删除]        │
│ │ GLM          (2 模型)  │ │                                            │
│ └───────────────────────┘ │                                            │
└──────────────────────────┴────────────────────────────────────────────┘

接入点编辑 Modal: 名称 / Provider▾ / BaseURL / ApiKey(密码框) / extra / 备注   [测试连接]
模型登记 Modal:  [从接入点拉取] 列出 /models 供勾选  或  手填 model
                 每个模型可设: 别名 / 温度 / 最大Token / 能力(chat/embedding/vision) / 是否默认
```
- 数据结构对应后端 `llm_provider` + `llm_model`；接入点存 baseUrl+apiKey，模型挂在接入点下。
- 「从接入点拉取」调 `GET /llm/providers/{id}/models/remote` 列出可用模型勾选登记；网关不支持时手填。
- Agent 配置页选择模型时，按「接入点 → 模型」级联选择（`llm_model_id`）。

### 2.11 审计日志 `/settings/audit`（安全，仅 admin）
```
┌ 筛选 ────────────────────────────────────────────────────┐
│ 操作[全部▾: chain.publish/chain.delete/llm.update/mcp.expose…]  时间[范围] [查询] │
├ 表格 ────────────────────────────────────────────────────┤
│ 时间            操作           目标          详情        IP     │
│ 10-21 12:00:01 chain.publish HTTP接收入库   {version:4}  10.0.0.2 │
│ 10-21 11:40:22 mcp.expose   ingest_data     {chain:HTTP} 10.0.0.2 │
│ 10-20 09:12:03 llm.update   OpenAI官方      {model:gpt-4o} 10.0.0.3 │
└───────────────────────────────────────────────────────────┘
```

### 2.12 规则链治理交互（列表页补充）
- 列表「更多▾」内提供 **导出**（下载链 JSON）；工具条提供 **导入**（上传链 JSON，校验失败弹缺失组件清单）。
- 发布/撤销发布/删除/触发等敏感操作均弹确认（后端写审计）。
- 看板失败任务卡片提供「重跑」「从节点续跑」入口。

---

## 第三部分：Demo 样例代码（关键实现骨架）

> 以下是可指导编码的最小骨架，非完整实现。

### 3.1 axios 实例（`api/http.ts`）
```ts
import axios from 'axios';
import { message } from 'antd';

const http = axios.create({ baseURL: '/api/v1', withCredentials: true, timeout: 30000 });

http.interceptors.response.use(
  (res) => {
    const env = res.data; // 统一信封 {code,message,data}
    if (env.code === 0) return env.data;
    message.error(env.message || '请求失败');
    return Promise.reject(env);
  },
  (err) => {
    if (err.response?.status === 401 && location.pathname !== '/login') {
      location.href = '/login';
    } else {
      message.error(err.response?.data?.message || err.message);
    }
    return Promise.reject(err);
  }
);
export default http;
```

### 3.2 WebSocket 客户端（`ws/client.ts`）
```ts
type Handler = (msg: any) => void;
class WsClient {
  private ws?: WebSocket;
  private handlers = new Map<string, Set<Handler>>();
  private retry = 0;

  connect() {
    this.ws = new WebSocket(`${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws`);
    this.ws.onmessage = (e) => {
      const env = JSON.parse(e.data); // {channel,type,payload}
      this.handlers.get(env.channel)?.forEach((h) => h(env));
    };
    this.ws.onopen = () => (this.retry = 0);
    this.ws.onclose = () => setTimeout(() => this.connect(), Math.min(1000 * 2 ** this.retry++, 15000));
  }
  subscribe(channel: string, h: Handler) {
    if (!this.handlers.has(channel)) this.handlers.set(channel, new Set());
    this.handlers.get(channel)!.add(h);
    return () => this.handlers.get(channel)!.delete(h);
  }
  send(channel: string, type: string, payload: any) {
    this.ws?.send(JSON.stringify({ channel, type, payload }));
  }
}
export const ws = new WsClient();
```

### 3.3 DSL ↔ ReactFlow 互转（`chain/canvas/chainDsl.ts`）— 画布核心
```ts
import type { Node, Edge } from '@xyflow/react';

// RuleGo DSL -> ReactFlow
export function dslToFlow(dsl: any): { nodes: Node[]; edges: Edge[] } {
  const nodes: Node[] = (dsl?.metadata?.nodes ?? []).map((n: any, i: number) => ({
    id: n.id,
    type: 'rule',
    position: n.additionalInfo?.position ?? { x: 120 + i * 220, y: 120 },
    data: { ruleType: n.type, name: n.name, configuration: n.configuration ?? {}, debugMode: n.debugMode },
  }));
  const edges: Edge[] = (dsl?.metadata?.connections ?? []).map((c: any) => ({
    id: `${c.fromId}->${c.toId}:${c.type}`,
    source: c.fromId,
    target: c.toId,
    label: c.type,                       // Success / Failure / True / False ...
    data: { relationType: c.type },
    className: `edge-${String(c.type).toLowerCase()}`,
  }));
  return { nodes, edges };
}

// ReactFlow -> RuleGo DSL
export function flowToDsl(chainMeta: any, nodes: Node[], edges: Edge[]) {
  return {
    ruleChain: { ...chainMeta, root: true },
    metadata: {
      nodes: nodes.map((n) => ({
        id: n.id,
        type: n.data.ruleType,
        name: n.data.name,
        debugMode: !!n.data.debugMode,
        configuration: n.data.configuration ?? {},
        additionalInfo: { position: n.position },   // 记住画布坐标
      })),
      connections: edges.map((e) => ({
        fromId: e.source,
        toId: e.target,
        type: e.data?.relationType ?? 'Success',
      })),
    },
  };
}
```

### 3.4 ReactFlow 画布（`chain/canvas/FlowCanvas.tsx`）
```tsx
import ReactFlow, { Background, Controls, MiniMap, addEdge, useEdgesState, useNodesState } from '@xyflow/react';
import { useCallback } from 'react';
import RuleNode from './nodes/RuleNode';
import ContainerNode from './nodes/ContainerNode';   // forEach / subChain 容器
import { useCanvasStore } from '@/stores/canvasStore';

const nodeTypes = { rule: RuleNode, container: ContainerNode };

export default function FlowCanvas() {
  const [nodes, setNodes, onNodesChange] = useNodesState([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState([]);
  const setSelected = useCanvasStore((s) => s.setSelectedNode);
  const debugNodeId = useCanvasStore((s) => s.debugNodeId);   // 调试高亮
  const enterSubChain = useCanvasStore((s) => s.enterSubChain); // 进入容器子画布

  const onConnect = useCallback((c: any) =>
    setEdges((eds) => addEdge({ ...c, label: 'Success', data: { relationType: 'Success' } }, eds)), []);

  const onDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    const comp = JSON.parse(e.dataTransfer.getData('application/rulego-component'));
    // 计算画布坐标; 容器类(forEach/subChain) type='container' 并初始化空子链 subChain:{nodes:[],connections:[]}
  }, []);

  return (
    <div style={{ width: '100%', height: '100%' }} onDrop={onDrop} onDragOver={(e) => e.preventDefault()}>
      <ReactFlow
        nodes={nodes.map((n) => ({ ...n, className: n.id === debugNodeId ? 'node-debugging' : '' }))}
        edges={edges}
        nodeTypes={nodeTypes}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        onNodeClick={(_, node) => setSelected(node)}
        onNodeDoubleClick={(_, node) => node.type === 'container' && enterSubChain(node.id)} // 双击进入子画布
        fitView
      >
        <Background gap={16} />
        <MiniMap pannable />
        <Controls />
      </ReactFlow>
    </div>
  );
}
```
> 编辑器外壳 `ChainEditorPage` 维护**子画布栈**：`[{nodes,edges}(根), {chainId:nodeId,nodes,edges}(forEach子链)…]`，面包屑切换；保存时把子链递归嵌回容器节点 DSL。自动布局用 dagre 分层（保存/打开时可选「整理布局」）。

### 3.5 自定义节点（`chain/canvas/nodes/RuleNode.tsx`）
```tsx
import { Handle, Position } from '@xyflow/react';
import { Tag } from 'antd';

const CATEGORY_COLOR: Record<string, string> = {
  endpoint: 'purple', filter: 'gold', transform: 'geekblue',
  action: 'green', loop: 'cyan', ai: 'magenta',
};

export default function RuleNode({ data, selected }: any) {
  const category = (data.ruleType || '').split('/')[0];
  return (
    <div className={`rule-node ${selected ? 'selected' : ''}`}>
      <Handle type="target" position={Position.Left} />
      <Tag color={CATEGORY_COLOR[category] ?? 'default'}>{category}</Tag>
      <div className="rule-node__name">{data.name}</div>
      <div className="rule-node__type">{data.ruleType}</div>
      <Handle type="source" position={Position.Right} />
    </div>
  );
}
```

### 3.6 组件面板（`chain/canvas/ComponentPalette.tsx`）
```tsx
import { useQuery } from '@tanstack/react-query';
import { Collapse, Input } from 'antd';
import { listComponents } from '@/api/component';

export default function ComponentPalette() {
  const { data } = useQuery({ queryKey: ['components'], queryFn: listComponents });
  const groups = groupBy(data?.list ?? [], 'category'); // 按分类分组
  return (
    <div className="palette">
      <Input.Search placeholder="搜索组件" style={{ marginBottom: 8 }} />
      <Collapse
        items={Object.entries(groups).map(([category, items]) => ({
          key: category,
          label: category,
          children: items.map((c: any) => (
            <div
              key={c.type}
              className="palette-item"
              draggable
              onDragStart={(e) => e.dataTransfer.setData('application/rulego-component', JSON.stringify(c))}
              title={c.description}
            >
              {c.name}
            </div>
          )),
        }))}
      />
    </div>
  );
}
```

### 3.7 节点动态表单（`chain/canvas/NodeConfigPanel.tsx`）
```tsx
import { Form, Input, InputNumber, Select, Switch, Button } from 'antd';
import CodeField from '@/components/CodeField';

// 按选中节点的 configSchema.fields 动态渲染
export default function NodeConfigPanel({ node, schema, onChange, onDelete }: any) {
  if (!node) return <div className="config-empty">选中画布中的节点进行配置</div>;
  return (
    <Form layout="vertical" initialValues={node.data.configuration}
          onValuesChange={(_, all) => onChange(node.id, all)}>
      <Form.Item label="节点名称"><Input value={node.data.name} /></Form.Item>
      <Form.Item label="类型"><Input value={node.data.ruleType} disabled /></Form.Item>
      {(schema?.fields ?? []).map((f: any) => (
        <Form.Item key={f.name} label={f.label} name={f.name} rules={[{ required: f.required }]}>
          {f.type === 'code'   ? <CodeField lang="javascript" /> :
           f.type === 'select' ? <Select options={f.options} /> :
           f.type === 'number' ? <InputNumber style={{ width: '100%' }} /> :
           f.type === 'bool'   ? <Switch /> :
           <Input />}
        </Form.Item>
      ))}
      <Button danger block onClick={() => onDelete(node.id)}>删除节点</Button>
    </Form>
  );
}
```

### 3.8 Agent 聊天（`agent/AgentChatPage.tsx`，流式）
```tsx
import { useEffect, useRef, useState } from 'react';
import { ws } from '@/ws/client';

export default function AgentChatPage({ agentId }: { agentId: number }) {
  const [messages, setMessages] = useState<any[]>([]);
  const [input, setInput] = useState('');
  const sessionRef = useRef<number>(0);

  useEffect(() => ws.subscribe('agent-chat', (env) => {
    const { type, payload } = env;
    if (type === 'session') sessionRef.current = payload.sessionId;
    if (type === 'delta')   appendToken(payload.text);          // 追加到最后一条 assistant
    if (type === 'tool')    appendToolStep(payload);            // 渲染工具调用步骤
    if (type === 'done')    finalize(payload);
  }), []);

  const send = () => {
    pushUser(input);
    ws.send('agent-chat', 'send', { agentId, sessionId: sessionRef.current, message: input });
    setInput('');
  };
  // ...渲染 messages: user / assistant(markdown) / 可展开的 tool 步骤
  return <div className="chat">{/* 布局见 2.7 */}</div>;
}
```

### 3.9 规则链调试（`chain/canvas/DebugPanel.tsx`）
```tsx
import { useEffect, useState } from 'react';
import { ws } from '@/ws/client';
import { useCanvasStore } from '@/stores/canvasStore';

export default function DebugPanel({ chainId }: { chainId: string }) {
  const [events, setEvents] = useState<any[]>([]);
  const [input, setInput] = useState('{"t":35}');
  const setDebugNode = useCanvasStore((s) => s.setDebugNodeId);

  useEffect(() => ws.subscribe('chain-debug', (env) => {
    if (env.type === 'node') {
      setEvents((e) => [...e, env.payload]);
      setDebugNode(env.payload.nodeId);            // 画布高亮当前节点
    }
    if (env.type === 'run' && env.payload.status !== 'start') setDebugNode(undefined);
  }), []);

  const run = () => ws.send('chain-debug', 'start',
    { chainId, input: { dataType: 'JSON', data: input, metadata: {} } });

  // 渲染: 输入框 + ▶运行 + 逐节点事件流(成功绿/失败红)
  return <div className="debug-panel">{/* 布局见 2.4 底部 */}</div>;
}
```

### 3.10 看板列拖拽（`board/BoardPage.tsx`）
```tsx
// 用 @dnd-kit/core 实现列内/跨列拖拽; 落点调 POST /tasks/{id}/move
import { DndContext, closestCorners } from '@dnd-kit/core';
// onDragEnd(e): const to = e.over; await moveTask(taskId, { toColumnId, toSort });
// 任务卡片 TaskCard 显示 标题 + assignedChain(标签) + status(图标); 点击弹 TaskEditModal 分配规则链/触发执行
```

### 3.11 路由守卫（`routes/index.tsx`）
```tsx
import { createBrowserRouter, Navigate } from 'react-router-dom';
import { useAuthStore } from '@/stores/authStore';

function RequireAuth({ children }: { children: JSX.Element }) {
  const user = useAuthStore((s) => s.user);
  return user ? children : <Navigate to="/login" replace />;
}

export const router = createBrowserRouter([
  { path: '/login', element: <LoginPage /> },
  {
    path: '/', element: <RequireAuth><MainLayout /></RequireAuth>,
    children: [
      { index: true, element: <DashboardPage /> },
      { path: 'chains', element: <ChainListPage /> },
      { path: 'chains/:id/edit', element: <ChainEditorPage /> },
      { path: 'runs', element: <RunLogPage /> },
      { path: 'skills', element: <SkillListPage /> },
      { path: 'agents', element: <AgentListPage /> },
      { path: 'agents/:key/chat', element: <AgentChatPage /> },
      { path: 'agents/logs', element: <AgentLogPage /> },
      { path: 'mcp', element: <McpPage /> },
      { path: 'mcp/exposures', element: <McpExposurePage /> },
      { path: 'boards', element: <BoardListPage /> },
      { path: 'boards/:id', element: <BoardPage /> },
      { path: 'cron', element: <CronPage /> },
      { path: 'settings/llm', element: <LlmConfigPage /> },
    ],
  },
]);
```

### 3.12 package.json 关键依赖
```json
{
  "dependencies": {
    "react": "^18.3.1", "react-dom": "^18.3.1", "react-router-dom": "^6.26.0",
    "antd": "^5.21.0", "@ant-design/icons": "^5.5.0",
    "@xyflow/react": "^12.3.0",
    "@tanstack/react-query": "^5.59.0", "zustand": "^4.5.5",
    "axios": "^1.7.7", "@dnd-kit/core": "^6.1.0",
    "monaco-editor": "^0.52.0", "react-markdown": "^9.0.1", "dayjs": "^1.11.13"
  },
  "devDependencies": {
    "vite": "^5.4.0", "@vitejs/plugin-react": "^4.3.0",
    "typescript": "^5.6.0", "sass": "^1.79.0"
  }
}
```
> `vite.config.ts` 需 `server.proxy`：`/api` → `http://localhost:8000`，`/ws` → `ws://localhost:8000`（`ws:true`）。

---

## 第四部分：状态与数据流约定
- **服务端状态**（列表/详情）：React Query，`queryKey` 按域（`['chains']`/`['chain',id]`/`['components']`…），mutation 后 `invalidateQueries`。
- **客户端状态**：zustand —— `authStore`(用户)、`sessionStore`(Agent 流式消息)、`canvasStore`(选中节点/调试高亮)、`llmStore`(默认 LLM)。
- **认证**：登录成功写 `authStore.user`；`RequireAuth` 守卫；axios 401 自动跳 `/login`。
- **WS**：`ws/client.ts` 应用启动即连，组件按需 `subscribe(channel)`，卸载自动取消订阅。

## 已确认决策（锁定）
1. 代码编辑器：**Monaco**（节点 JS 脚本 / JSON / DSL 编辑统一用 `monaco-editor`，经 `CodeField.tsx` 封装按需加载）。
2. 看板拖拽库：**@dnd-kit/core**（列内/跨列拖拽）。
3. apiKey 加密密钥 `BABO_SECRET`：通过 **.env** 注入（后端 Viper 读取 `.env`，`.env` 不入库、不进版本库，提供 `.env.example`）。
