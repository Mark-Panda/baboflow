# BaboFlow 后端详细设计

> 技术栈：Kratos 最新(v2.x) · GORM 最新 · PostgreSQL(启用 pgvector) · aggo v2 · RuleGo 最新
> 认证：Session(Cookie)，单租户 admin。除 `/api/v1/auth/*` 外全部接口需会话。
> 历史 REST 设计曾使用统一响应信封。当前核心业务 API 已迁移到 Kratos proto HTTP/gRPC：成功响应为 protobuf JSON，失败响应为 Kratos 原生错误 JSON（按 HTTP status 与 `reason`/`message` 处理），不再使用旧业务信封。
> 命名：DB 用 snake_case；JSON 出入参用 camelCase；`jsonb` 字段在注释中给出结构。

---

## 第一部分：数据库表结构（DDL + 字段注释）

> PostgreSQL 14+，先 `CREATE EXTENSION IF NOT EXISTS vector;`。
> **向量维度可配置**：embedding 维度由 `.env` `EMBEDDING_DIM`（默认 1536）与所选 embedding 模型决定；建表/migration 时按该值生成 `vector(EMBEDDING_DIM)` 列。更换维度需重建向量列并重算 embedding（提供迁移命令）。

### 0. 公共约定
- 主键统一 `id BIGSERIAL PRIMARY KEY`（对外暴露字符串或 `bigint`）。
- 每张表含 `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`、`updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`。
- 软删除仅在业务表（rule_chain / skill / board）用 `deleted_at TIMESTAMPTZ NULL`。
- **多租户预留**：当前为单租户（仅 admin），但核心业务表（rule_chain / skill / board / task / mcp_server / cron_job / agent）**预留 `tenant_id BIGINT NOT NULL DEFAULT 0`** 字段并建索引；查询层统一经 `tenant scope`（当前恒为 0）。后续升级多租户时仅需启用租户中间件与登录体系，**无需改库结构**。

### 0b. audit_log —— 操作审计（安全加固）
```sql
CREATE TABLE audit_log (
  id          BIGSERIAL PRIMARY KEY,
  tenant_id   BIGINT       NOT NULL DEFAULT 0,
  user_id     BIGINT       NULL,                   -- 操作人(admin_user.id)
  action      VARCHAR(64)  NOT NULL,               -- chain.publish / chain.delete / llm.update / mcp.expose / task.trigger ...
  target_type VARCHAR(32)  NOT NULL,               -- rule_chain / agent / llm_provider / mcp_server ...
  target_id   VARCHAR(64)  NOT NULL DEFAULT '',
  detail      JSONB        NOT NULL DEFAULT '{}',  -- 变更摘要(脱敏后), 如 {name, version}
  ip          VARCHAR(64)  NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_action ON audit_log(action);
CREATE INDEX idx_audit_created ON audit_log(created_at DESC);
-- 写库时机: 敏感写操作(发布/删除/改密/LLM 变更/MCP 暴露/触发执行)统一在 biz 层记录
```

---

### 1. admin_user —— 管理员（单租户 Session 登录）
```sql
CREATE TABLE admin_user (
  id            BIGSERIAL PRIMARY KEY,
  username      VARCHAR(64)  NOT NULL UNIQUE,        -- 登录名(默认 admin)
  password_hash VARCHAR(255) NOT NULL,               -- bcrypt 哈希, 不明文
  display_name  VARCHAR(64)  NOT NULL DEFAULT '管理员',
  last_login_at TIMESTAMPTZ NULL,                    -- 最近登录时间
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 2. session —— 登录会话
```sql
CREATE TABLE session (
  id          VARCHAR(64) PRIMARY KEY,             -- 随机会话ID(放 Cookie: baboflow_sid)
  user_id     BIGINT NOT NULL REFERENCES admin_user(id) ON DELETE CASCADE,
  ip          VARCHAR(64)  NOT NULL DEFAULT '',     -- 登录 IP
  user_agent  VARCHAR(255) NOT NULL DEFAULT '',     -- UA
  expires_at  TIMESTAMPTZ NOT NULL,                 -- 过期时间(滑动续期)
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_session_user ON session(user_id);
```

### 3. llm_provider —— LLM 接入点（一组 baseUrl + apiKey，下挂多个 model）需求9
```sql
CREATE TABLE llm_provider (
  id           BIGSERIAL PRIMARY KEY,
  name         VARCHAR(64)  NOT NULL,               -- 接入点名, 如 "OpenAI官方" / "公司网关"
  provider     VARCHAR(32)  NOT NULL DEFAULT 'openai', -- openai/azure/glm/deepseek/兼容OpenAI
  base_url     VARCHAR(255) NOT NULL,               -- 接口地址, 如 https://api.openai.com/v1
  api_key_enc  TEXT         NOT NULL,               -- apiKey, AES-GCM 加密(密钥取 .env BABO_SECRET)
  extra        JSONB        NOT NULL DEFAULT '{}',  -- 其余参数 {organization, apiVersion...}
  remark       VARCHAR(255) NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 一个接入点下的具体模型(可多个), Agent/任务按 model 选择
CREATE TABLE llm_model (
  id           BIGSERIAL PRIMARY KEY,
  provider_id  BIGINT       NOT NULL REFERENCES llm_provider(id) ON DELETE CASCADE,
  model        VARCHAR(128) NOT NULL,               -- 模型名, 如 gpt-4o / gpt-4o-mini / glm-4
  alias        VARCHAR(64)  NOT NULL DEFAULT '',    -- 显示别名
  temperature  NUMERIC(3,2) NOT NULL DEFAULT 0.7,   -- 采样温度
  max_tokens   INT          NOT NULL DEFAULT 4096,  -- 最大输出 token
  is_default   BOOLEAN      NOT NULL DEFAULT false, -- 该接入点下默认模型
  capability   JSONB        NOT NULL DEFAULT '{}',  -- 能力标记 {chat:true, embedding:false, vision:false}
  enabled      BOOLEAN      NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(provider_id, model)
);
-- 说明: Agent.llm_config_id 改指 llm_model.id(见第8节); embedding 模型也在此登记(capability.embedding=true)
```

### 4. rule_chain —— 规则链（需求5）
```sql
CREATE TABLE rule_chain (
  id            VARCHAR(64) PRIMARY KEY,            -- 规则链ID(RuleGo 引擎以此为主键, 如 chain_xxx 或 uuid)
  name          VARCHAR(128) NOT NULL,              -- 名称
  description   VARCHAR(512) NOT NULL DEFAULT '',   -- 描述
  dsl           JSONB        NOT NULL,              -- 完整 RuleGo DSL: {ruleChain:{...}, metadata:{nodes:[], connections:[]}}
  status        VARCHAR(16)  NOT NULL DEFAULT 'draft', -- draft草稿/published已发布/archived归档
  version       INT          NOT NULL DEFAULT 0,    -- 当前已发布版本号(0=从未发布)
  debug_mode    BOOLEAN      NOT NULL DEFAULT false,-- 链级调试开关(写入 dsl.ruleChain.debugMode)
  source        VARCHAR(16)  NOT NULL DEFAULT 'manual', -- manual手工/agent由Agent1生成
  created_by    BIGINT       NULL,                  -- 创建人(admin_user.id)
  deleted_at    TIMESTAMPTZ  NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_rule_chain_status ON rule_chain(status);
```

### 5. rule_chain_version —— 规则链发布快照（支持回滚）
```sql
CREATE TABLE rule_chain_version (
  id           BIGSERIAL PRIMARY KEY,
  chain_id     VARCHAR(64) NOT NULL REFERENCES rule_chain(id) ON DELETE CASCADE,
  version      INT         NOT NULL,               -- 版本号(自增)
  dsl          JSONB       NOT NULL,               -- 发布时的 DSL 快照
  published_by BIGINT      NULL,                   -- 发布人
  published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(chain_id, version)
);
```

### 6. component_meta —— RuleGo 组件注册表镜像（供画布动态表单 & Agent1 检索）需求2/3
```sql
CREATE TABLE component_meta (
  id            BIGSERIAL PRIMARY KEY,
  type          VARCHAR(128) NOT NULL UNIQUE,      -- 组件类型, 如 filter/js, action/log, endpoint/http
  name          VARCHAR(128) NOT NULL,             -- 显示名
  category      VARCHAR(32)  NOT NULL,             -- 分类: filter/transform/action/endpoint/external/ai...
  description   TEXT         NOT NULL DEFAULT '',  -- 功能描述(供 Agent 语义检索)
  config_schema JSONB        NOT NULL DEFAULT '{}',-- 配置表单 schema(动态渲染 NodeConfigPanel): {fields:[{name,label,type,required,default,options}]}
  example       JSONB        NOT NULL DEFAULT '{}',-- 配置示例
  fingerprint   VARCHAR(64)  NOT NULL DEFAULT '',  -- hash(type+schema+description), 同步幂等用
  embedding     vector(1536) NULL,                 -- description 的向量(语义检索)
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_component_meta_category ON component_meta(category);
CREATE INDEX idx_component_meta_embedding ON component_meta USING ivfflat (embedding vector_cosine_ops);
-- 由「组件自动同步」维护: 启动扫描 + 注册钩子 upsert; fingerprint 未变则跳过(幂等)
```

### 7. skill —— SKILL 文件（需求3/6/10）
```sql
CREATE TABLE skill (
  id           BIGSERIAL PRIMARY KEY,
  name         VARCHAR(128) NOT NULL UNIQUE,       -- SKILL 名(对应 SKILL.md frontmatter name)
  description  VARCHAR(512) NOT NULL DEFAULT '',   -- 描述
  source       VARCHAR(16)  NOT NULL DEFAULT 'upload', -- upload上传/chain由规则链反生成/builtin内置/component组件自动同步
  chain_id     VARCHAR(64)  NULL,                  -- 若 source=chain, 来源规则链
  frontmatter  JSONB        NOT NULL DEFAULT '{}', -- YAML frontmatter 解析结果 {name,description,context,agent,model}
  content      TEXT         NOT NULL,              -- SKILL.md 完整 markdown 正文
  file_path    VARCHAR(255) NOT NULL DEFAULT '',   -- 落盘路径(供 aggo 加载)
  embedding    vector(1536) NULL,                  -- 描述向量(语义检索)
  deleted_at   TIMESTAMPTZ  NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_skill_embedding ON skill USING ivfflat (embedding vector_cosine_ops);
```

### 8. agent —— Agent 定义（需求1/3/6）
```sql
CREATE TABLE agent (
  id             BIGSERIAL PRIMARY KEY,
  key            VARCHAR(64)  NOT NULL UNIQUE,     -- 标识: agent-chain-builder / agent-skill-generator / 自定义
  name           VARCHAR(128) NOT NULL,
  instruction    TEXT         NOT NULL DEFAULT '', -- 系统提示词(ReAct 引导)
  llm_model_id   BIGINT       NULL REFERENCES llm_model(id), -- 使用的模型(关联接入点取 baseUrl/apiKey)
  memory_backend VARCHAR(16)  NOT NULL DEFAULT 'builtin', -- builtin/mem0/memu
  skill_ids      JSONB        NOT NULL DEFAULT '[]', -- 挂载的 skill id 数组
  mcp_ids        JSONB        NOT NULL DEFAULT '[]', -- 挂载的 mcp_server id 数组
  builtin_tools  JSONB        NOT NULL DEFAULT '["bash","read","write","edit","grep"]', -- 启用的内置工具
  is_builtin     BOOLEAN      NOT NULL DEFAULT false, -- 内置 Agent(不可删)
  enabled        BOOLEAN      NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 9. agent_session —— Agent 会话（需求1 日志）
```sql
CREATE TABLE agent_session (
  id          BIGSERIAL PRIMARY KEY,
  agent_id    BIGINT      NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
  user_id     BIGINT      NULL,
  title       VARCHAR(255) NOT NULL DEFAULT '',    -- 会话标题(首条消息生成)
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_agent_session_agent ON agent_session(agent_id);
```

### 10. agent_message —— 会话消息（含工具调用与多模态附件）
```sql
CREATE TABLE agent_message (
  id          BIGSERIAL PRIMARY KEY,
  session_id  BIGINT      NOT NULL REFERENCES agent_session(id) ON DELETE CASCADE,
  role        VARCHAR(16) NOT NULL,                -- user/assistant/tool/system
  content     TEXT        NOT NULL DEFAULT '',     -- 文本内容
  attachments JSONB       NOT NULL DEFAULT '[]',   -- 附件 [{assetId,name,mimeType,size,url}] 图片/文件(模型支持时)
  tool_calls  JSONB       NOT NULL DEFAULT '[]',   -- 工具调用记录 [{name,args,result,durationMs}]
  sub_agent   VARCHAR(64) NOT NULL DEFAULT '',     -- 若由 subAgent 产生, 记录其 agent key
  tokens      INT         NOT NULL DEFAULT 0,      -- 消耗 token
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_agent_message_session ON agent_message(session_id);
```

### 10b. asset —— 会话上传的文件/图片（多模态输入）
```sql
CREATE TABLE asset (
  id          BIGSERIAL PRIMARY KEY,
  session_id  BIGINT      NULL,                    -- 来源会话(可空)
  name        VARCHAR(255) NOT NULL,               -- 原始文件名
  mime_type   VARCHAR(128) NOT NULL,               -- image/png, application/pdf, text/csv...
  size        BIGINT       NOT NULL DEFAULT 0,     -- 字节
  storage_path VARCHAR(512) NOT NULL,              -- 本地/对象存储路径
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- 图片类在模型 capability.vision=true 时以 base64/URL 注入多模态消息; 文本类可抽取内容入 prompt
```

### 10c. agent_sub_agent —— Agent 层级（subAgent 委派）
```sql
CREATE TABLE agent_sub_agent (
  id          BIGSERIAL PRIMARY KEY,
  parent_id   BIGINT NOT NULL REFERENCES agent(id) ON DELETE CASCADE, -- 父 Agent
  child_id    BIGINT NOT NULL REFERENCES agent(id) ON DELETE CASCADE, -- 委派的 subAgent
  description VARCHAR(512) NOT NULL DEFAULT '',     -- 何时委派给该 subAgent(供父 Agent 路由)
  UNIQUE(parent_id, child_id)
);
-- 父 Agent 运行时把 subAgent 注册为一个工具: 调用即在隔离上下文运行子 Agent 并回传结果
```

### 11. mcp_server —— MCP 服务配置（需求11）
```sql
CREATE TABLE mcp_server (
  id         BIGSERIAL PRIMARY KEY,
  name       VARCHAR(128) NOT NULL,
  transport  VARCHAR(16)  NOT NULL DEFAULT 'sse',  -- stdio/sse/streamable-http
  endpoint   VARCHAR(255) NOT NULL DEFAULT '',     -- sse/http 模式: 服务地址
  command    VARCHAR(255) NOT NULL DEFAULT '',     -- stdio 模式: 启动命令
  args       JSONB        NOT NULL DEFAULT '[]',   -- stdio 命令参数
  env        JSONB        NOT NULL DEFAULT '{}',   -- 环境变量
  status     VARCHAR(16)  NOT NULL DEFAULT 'disabled', -- enabled/disabled/error
  last_check_at TIMESTAMPTZ NULL,                  -- 最近连通性检测
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 12. mcp_exposure —— 已发布规则链 → MCP 工具 映射（需求7）
```sql
CREATE TABLE mcp_exposure (
  id          BIGSERIAL PRIMARY KEY,
  chain_id    VARCHAR(64)  NOT NULL REFERENCES rule_chain(id) ON DELETE CASCADE,
  tool_name   VARCHAR(128) NOT NULL UNIQUE,        -- 对外 MCP 工具名
  description VARCHAR(512) NOT NULL DEFAULT '',    -- 工具描述
  input_schema JSONB      NOT NULL DEFAULT '{}',   -- 入参 JSON Schema(由规则链输入推断/手填)
  enabled     BOOLEAN      NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 13. board / board_column / task —— 看板（需求8）
```sql
CREATE TABLE board (
  id          BIGSERIAL PRIMARY KEY,
  name        VARCHAR(128) NOT NULL,
  description VARCHAR(512) NOT NULL DEFAULT '',
  deleted_at  TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE board_column (
  id       BIGSERIAL PRIMARY KEY,
  board_id BIGINT      NOT NULL REFERENCES board(id) ON DELETE CASCADE,
  name     VARCHAR(64) NOT NULL,                   -- 列名, 如 待办/进行中/已完成
  sort     INT         NOT NULL DEFAULT 0,         -- 列顺序
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE task (
  id           BIGSERIAL PRIMARY KEY,
  column_id    BIGINT      NOT NULL REFERENCES board_column(id) ON DELETE CASCADE,
  title        VARCHAR(255) NOT NULL,
  payload      JSONB       NOT NULL DEFAULT '{}',  -- 任务输入(作为规则链 msg.data)
  status       VARCHAR(16) NOT NULL DEFAULT 'pending', -- pending/running/success/failure
  assigned_chain_id VARCHAR(64) NULL,              -- 分配的已发布规则链
  run_id       BIGINT      NULL,                   -- 最近一次执行 chain_run.id
  result       JSONB       NOT NULL DEFAULT '{}',  -- 执行结果回写
  retry_max    INT         NOT NULL DEFAULT 0,     -- 失败最大重试次数(0=不重试)
  retry_count  INT         NOT NULL DEFAULT 0,     -- 已重试次数
  timeout_sec  INT         NOT NULL DEFAULT 300,   -- 单次执行超时(秒)
  sort         INT         NOT NULL DEFAULT 0,     -- 列内排序
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_task_column ON task(column_id);
```

### 14. chain_run —— 规则链运行/调试记录（需求5 日志）
```sql
CREATE TABLE chain_run (
  id          BIGSERIAL PRIMARY KEY,
  chain_id    VARCHAR(64) NOT NULL,                -- 规则链 id
  task_id     BIGINT      NULL,                    -- 触发来源任务(可空)
  trigger     VARCHAR(16) NOT NULL DEFAULT 'manual', -- manual手工/task看板/mcp/cron
  status      VARCHAR(16) NOT NULL DEFAULT 'running', -- running/success/failure
  input       JSONB       NOT NULL DEFAULT '{}',   -- 输入 msg {dataType, data, metadata}
  output      JSONB       NOT NULL DEFAULT '{}',   -- 最终输出
  error       TEXT        NOT NULL DEFAULT '',     -- 失败原因
  node_trace  JSONB       NOT NULL DEFAULT '[]',   -- 逐节点调试事件 [{nodeId,nodeName,in,out,relationType,err,ts}]
  started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at TIMESTAMPTZ NULL
);
CREATE INDEX idx_chain_run_chain ON chain_run(chain_id);
CREATE INDEX idx_chain_run_started ON chain_run(started_at DESC);
```

### 15. cron_job —— 定时任务（需求1）
```sql
CREATE TABLE cron_job (
  id         BIGSERIAL PRIMARY KEY,
  target_type VARCHAR(16) NOT NULL,                -- agent / chain
  target_id  VARCHAR(64)  NOT NULL,                -- agent.key 或 rule_chain.id
  schedule_type VARCHAR(16) NOT NULL DEFAULT 'cron', -- once/interval/cron
  cron_expr  VARCHAR(64)  NOT NULL DEFAULT '',     -- cron 表达式(schedule_type=cron)
  interval_sec INT        NOT NULL DEFAULT 0,      -- 间隔秒(interval)
  run_at     TIMESTAMPTZ  NULL,                    -- 一次性执行时间(once)
  payload    JSONB        NOT NULL DEFAULT '{}',   -- 触发输入
  enabled    BOOLEAN      NOT NULL DEFAULT true,
  last_run_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 表关系一览
```
admin_user 1─N session
llm_provider 1─N llm_model 1─N agent
rule_chain 1─N rule_chain_version / mcp_exposure / chain_run
rule_chain 1─N skill(source=chain)
board 1─N board_column 1─N task ──(assigned_chain_id)→ rule_chain ──(run_id)→ chain_run
agent 1─N agent_session 1─N agent_message ──(attachments)→ asset
agent N─N agent (经 agent_sub_agent 表达 父Agent─subAgent 委派)
agent ──(skill_ids)→ skill   agent ──(mcp_ids)→ mcp_server
```

---

## 第二部分：后端分层与目录结构

### 2.0 Kratos 分层（api → service → biz → data）
```
baboflow/
├── api/baboflow/v1/            # proto: auth, llm, rulechain, component, skill, agent, mcp, board, runlog, cron, dashboard, asset
├── cmd/baboflow/main.go        # wire 注入、HTTP+gRPC+WS server、启动跑 migration + 组件同步
├── configs/config.yaml         # Kratos 配置(读 .env)
├── internal/
│   ├── conf/                   # config: db, mcp, rulego, secret
│   ├── server/                 # http.go / grpc.go / websocket.go
│   ├── service/                # 薄层, DTO 转换, 调 biz
│   ├── biz/                    # 领域逻辑 + repo 接口
│   │   ├── auth.go llm.go rulechain.go component.go skill.go agent.go mcp.go board.go runlog.go cron.go asset.go
│   │   ├── agentkit/           # aggo 封装(见 第四部分)
│   │   │   ├── provider.go     # LLM 接入点→ChatModel(经 model→provider 取 baseUrl/解密 apiKey)
│   │   │   ├── manager.go      # 构建/缓存 ChatModelAgent(ReAct), 注入 tools/skills/memory/subAgent
│   │   │   ├── builtin_tools.go# ★ 内置工具: bash / read / write / edit / grep
│   │   │   └── tools.go        # search_component / rulechain_* / skill_* / mcp 工具
│   │   ├── rulegokit/          # RuleGo 封装
│   │   │   ├── engine.go       # 引擎池 Publish/Offline/Run/Debug(OnDebug→WS)
│   │   │   ├── register.go     # rulegokit.Register 包装(注册组件 + 触发同步)
│   │   │   ├── agent_node.go   # baboflow/agent 组件(规则链内调用 Agent)
│   │   │   └── mcp.go          # 规则链→MCP 工具暴露
│   │   └── componentsync/      # ★ 组件知识库/SKILL 自动同步
│   │       ├── sync.go         # Run(): 启动全量/周期对账 diff + 单飞 + upsert
│   │       ├── fingerprint.go  # hash(type+schema+description) 幂等判断
│   │       ├── skillgen.go     # 模板化生成组件 SKILL.md(+可选 LLM 润色)
│   │       └── overview.go     # 重建 baboflow-components-overview 总览 SKILL
│   └── data/                   # GORM 实现 repo 接口、po、pgvector 检索、migration
│       ├── migrate.go          # AutoMigrate + CREATE EXTENSION vector + 种子(内置3 Agent)
│       └── ...
├── web/                        # 前端(见前端设计文档)
├── Dockerfile  docker-compose.yml  .env.example
└── docs/
```

---

## 第三部分：历史接口与业务设计（REST + WebSocket）

> 本部分保留路由、数据库与业务语义设计作为迁移参考，其中旧 REST Req/Resp 示例不代表当前线上响应结构。当前核心业务 API 前缀为 `/api/v1`，除 Auth 外均需 Cookie `baboflow_sid`。
> 迁移后，成功响应直接序列化各 proto response message 为 protobuf JSON，不再使用 `{code,message,data}` 信封；分页结构以各 service 定义的 typed response 与 `page` 字段为准，不再统一解包 `data.list`。
> proto HTTP 错误使用实际 HTTP status 与 Kratos 原生错误体，调用方读取 `message`/`reason`。HTTP 旁路端点及其错误格式遵循“第六部分：proto HTTP/gRPC 迁移实施说明”的现行约定。

### A. Auth 认证
**POST `/api/v1/auth/login`** — 登录
```json
// Req
{ "username":"admin", "password":"xxxx" }
```
登录成功返回 `LoginReply` 的 proto JSON message，并设置 `baboflow_sid` Cookie（`HttpOnly`、`SameSite=Lax`）；认证失败返回 HTTP 401 与 Kratos `message`/`reason`。
**POST `/api/v1/auth/logout`** — 登出（销毁会话）。Req 空；成功响应为 `LogoutReply` 的 proto JSON message。
**GET `/api/v1/auth/me`** — 当前登录信息。成功响应为 `MeReply` 的 proto JSON message；未登录返回 HTTP 401。
**PUT `/api/v1/auth/password`** — 修改密码。Req `{oldPassword,newPassword}`。

### B. LLM 配置（需求9）—— 两层：接入点(provider) + 模型(model)

#### B.1 接入点 llm_provider（一组 baseUrl + apiKey）
**GET `/api/v1/llm/providers`** — 接入点列表（apiKey 仅回掩码，附带各 provider 的 model 数）。Resp `data.list[]`：
```json
{ "id":1, "name":"OpenAI官方", "provider":"openai", "baseUrl":"https://api.openai.com/v1",
  "apiKeyMasked":"sk-****abcd", "extra":{}, "remark":"", "modelCount":3 }
```
**POST `/api/v1/llm/providers`** — 新建接入点。Req：
```json
{ "name":"OpenAI官方", "provider":"openai", "baseUrl":"https://api.openai.com/v1",
  "apiKey":"sk-...", "extra":{}, "remark":"" }
```
**PUT `/api/v1/llm/providers/{id}`** — 更新（apiKey 留空表示不修改）。
**DELETE `/api/v1/llm/providers/{id}`** — 删除（级联删其下 model；被 Agent 引用时 409）。
**POST `/api/v1/llm/providers/{id}/test`** — 测试接入点连通性（用默认 model 探测）。
```json
// Resp data: { "ok":true, "latencyMs":320, "message":"连接成功" }
```

**GET `/api/v1/llm/providers/{id}/models/remote`** — **从接入点拉取可用模型列表**（调 `{baseUrl}/models`），用于勾选登记。
```json
// Resp data: { "models":["gpt-4o","gpt-4o-mini","gpt-3.5-turbo"] }
// 网关不支持 /models 时返回 400, 前端改为手填 model
```

#### B.2 模型 llm_model（某接入点下的具体模型）
**GET `/api/v1/llm/providers/{id}/models`** — 该接入点下已登记模型列表。
```json
// data.list[]: { "id":11, "providerId":1, "model":"gpt-4o", "alias":"GPT4o",
//   "temperature":0.7, "maxTokens":4096, "isDefault":true, "capability":{"chat":true}, "enabled":true }
```
**POST `/api/v1/llm/providers/{id}/models`** — 登记模型（可批量）。Req：
```json
{ "models":[ {"model":"gpt-4o","alias":"GPT4o","isDefault":true,"capability":{"chat":true}},
             {"model":"text-embedding-3-large","capability":{"embedding":true}} ] }
```
**PUT `/api/v1/llm/models/{modelId}`** — 更新模型（别名/温度/maxTokens/默认/能力/启停）。
**DELETE `/api/v1/llm/models/{modelId}`** — 删除（被 Agent 引用时 409）。
**POST `/api/v1/llm/models/{modelId}/default`** — 设为该接入点默认模型。
**POST `/api/v1/llm/models/{modelId}/test`** — 测试该模型可用性。

> 说明：`agent.llm_model_id` 引用具体模型；运行时经 `llm_model → llm_provider` 取 baseUrl/apiKey 构造 ChatModel。embedding 检索（component_meta/skill 向量）选用 `capability.embedding=true` 的模型。

### C. RuleChain 规则链（需求5）
**GET `/api/v1/chains?status=&keyword=&page=&pageSize=`** — 列表（不含 dsl 大字段）。
```json
// data.list[]
{ "id":"chain_1", "name":"HTTP接收入库", "description":"...", "status":"published",
  "version":3, "source":"agent", "debugMode":false, "updatedAt":"..." }
```
**POST `/api/v1/chains`** — 新建（可传空骨架或完整 dsl）。
```json
// Req: { "name":"...", "description":"...", "dsl":{ruleChain:{...},metadata:{nodes:[],connections:[]}} }
// 若不传 dsl, 后端生成最小骨架(root chain + 空 nodes)
// Resp data: { "id":"chain_x", ... }
```
**GET `/api/v1/chains/{id}`** — 详情（含完整 dsl）。
**PUT `/api/v1/chains/{id}`** — 更新名称/描述/dsl（保存草稿；已发布链保存后 status 回到 draft 或产生未发布变更标记）。
```json
// Req: { "name":"...", "description":"...", "dsl":{...}, "debugMode":false }
// 后端用 RuleGo 校验 DSL 合法性; 非法返回 400 + 具体节点错误
```
**DELETE `/api/v1/chains/{id}`** — 删除（published 需先下线/归档，否则 409）。
**POST `/api/v1/chains/{id}/publish`** — 发布：生成 version 快照、RuleGo 引擎热加载、status→published。
```json
// Resp data: { "version":4, "publishedAt":"..." }
```
**POST `/api/v1/chains/{id}/offline`** — 下线（status→draft，卸载引擎）。
**GET `/api/v1/chains/{id}/versions`** — 版本列表。
**POST `/api/v1/chains/{id}/rollback`** — 回滚到指定版本。Req `{version:2}`（生成新版本快照，非覆盖）。
**GET `/api/v1/chains/{id}/export`** — 导出链 JSON（name/description/dsl/version），下载备份/迁移。
**POST `/api/v1/chains/import`** — 导入链 JSON 为 draft；组件缺失/校验失败返回 400 + 明细。
**POST `/api/v1/chains/{id}/run`** — 同步执行一次（已发布链）。
```json
// Req: { "dataType":"JSON", "data":"{\"temperature\":35}", "metadata":{} }
// Resp data: { "runId":100, "status":"success", "output":{...}, "nodeTrace":[...] }
```
**POST `/api/v1/chains/{id}/skill`** — 调 Agent2 从该已发布链生成 SKILL（见 F）。
**POST `/api/v1/chains/{id}/expose`** — 暴露为 MCP 工具（见 G）。

### D. Component 组件（需求2/3，供画布 & Agent1）
> 仅含 RuleGo 内置组件 + 平台 Agent 组件（`baboflow/agent`）。
**GET `/api/v1/components?category=&keyword=`** — 组件清单。`category`：endpoint/filter/transform/action/loop/ai 等；Agent 组件 `category=ai`、`type=baboflow/agent`。
```json
// data.list[]
{ "type":"filter/js", "name":"JS脚本过滤", "category":"filter",
  "description":"用 JS 表达式对消息过滤",
  "configSchema":{ "fields":[
      {"name":"jsScript","label":"脚本","type":"code","required":true,"default":"return msg.temperature>50;"}
  ]},
  "example":{ "jsScript":"return true;" } }
// forEach / subChain 组件 category=loop, configSchema 标记 "container":true(前端据此建子画布)
// Agent 组件示例:
{ "type":"baboflow/agent", "name":"调用 Agent", "category":"ai",
  "description":"在规则链中调用指定 Agent 执行并返回结果",
  "configSchema":{ "container":false, "fields":[
      {"name":"agentKey","label":"Agent","type":"select","required":true,"options":"{{agents}}"},
      {"name":"prompt","label":"提示词","type":"code","required":false} ] } }
```
**GET `/api/v1/components/search?q=&topK=`** — pgvector 语义检索（Agent1 内部也用此逻辑）。
**GET `/api/v1/components/sync`** — 组件知识库/SKILL 同步状态。Resp `data:{added,updated,removed,skipped,lastRunAt}`。
**POST `/api/v1/components/sync`** — 手动触发一次同步（正常由启动扫描/注册钩子/周期对账自动完成，此为运维兜底）。

### E. RunLog 运行日志（需求5）
**GET `/api/v1/runs?chainId=&status=&page=&pageSize=`** — 列表。
```json
// data.list[] { "id":100, "chainId":"chain_1", "trigger":"task", "status":"failure",
//               "startedAt":"...", "finishedAt":"...", "error":"node s2: ..." }
```
**GET `/api/v1/runs/{id}`** — 详情（含 input/output/nodeTrace 回放）。
```json
// data.nodeTrace[]: { "nodeId":"s1", "nodeName":"过滤", "in":{...}, "out":{...},
//                     "relationType":"Success", "err":"", "ts":"..." }
```

### F. Skill 技能（需求3/6/10）
**GET `/api/v1/skills?source=&keyword=`** — 列表。
**GET `/api/v1/skills/{id}`** — 详情（frontmatter + content markdown）。
**POST `/api/v1/skills/upload`** — 上传 SKILL.md（multipart 或 JSON）。
```json
// Req(JSON): { "content":"---\nname: weather-skill\ndescription: ...\n---\n# 用法..." }
// 后端解析 frontmatter 入库 + 落盘 + 生成 embedding
```
**DELETE `/api/v1/skills/{id}`** — 删除。
**POST `/api/v1/chains/{id}/skill`** — Agent2 反生成 SKILL（异步，WS 推送结果；或同步返回）。
```json
// Resp data: { "skillId":5, "name":"http-ingest-skill", "content":"---\nname:..." }
```

### G. MCP（需求7/11）
**GET `/api/v1/mcp/servers`** — MCP 服务列表。
**POST `/api/v1/mcp/servers`** — 新增。Req：
```json
{ "name":"本地工具", "transport":"stdio", "command":"npx", "args":["-y","@mcp/fs"], "env":{}, "endpoint":"" }
```
**PUT `/api/v1/mcp/servers/{id}`** / **DELETE** / **POST `/{id}/toggle`**（启停）/ **POST `/{id}/test`**（连通性，列出可用工具）。
**GET `/api/v1/mcp/exposures`** — 已暴露的规则链工具列表。
**POST `/api/v1/chains/{id}/expose`** — 暴露规则链为 MCP 工具。
```json
// Req: { "toolName":"ingest_data", "description":"...", "inputSchema":{...} }
// 后端经 RuleGo MCP-server-endpoint 注册; Resp data: { "id":1, "toolName":"ingest_data", "mcpEndpoint":"/mcp" }
```
**DELETE `/api/v1/mcp/exposures/{id}`** — 取消暴露。

### H. Board 看板（需求8）
**GET `/api/v1/boards`** / **POST** / **PUT `/{id}`** / **DELETE `/{id}`**。
**GET `/api/v1/boards/{id}`** — 看板详情（列 + 任务，一次性返回）。
```json
// data: { "id":1, "name":"数据管道", "columns":[
//   { "id":11, "name":"待办", "sort":0, "tasks":[
//       {"id":101,"title":"同步订单","status":"pending","assignedChainId":"chain_1","sort":0} ]} ]}
```
**POST `/api/v1/boards/{id}/columns`** / **PUT / DELETE `/api/v1/columns/{cid}`**。
**POST `/api/v1/columns/{cid}/tasks`** — 新建任务。Req `{title,payload,assignedChainId?}`。
**PUT `/api/v1/tasks/{id}`** — 编辑任务（含 `assignedChainId` 分配规则链）。
**POST `/api/v1/tasks/{id}/move`** — 状态流转（拖拽）。Req `{toColumnId, toSort}`。
**POST `/api/v1/tasks/{id}/trigger`** — 用分配的链执行任务（带超时/重试，入执行器队列）。
```json
// Resp data: { "runId":100, "status":"success", "result":{...} }  // 结果回写 task.result
```
**POST `/api/v1/tasks/{id}/retry`** — 失败任务重跑；可选 `{fromNodeId:"s3"}` 从指定节点续跑（用上次中间结果）。
**DELETE `/api/v1/tasks/{id}`**。

### I. Agent（需求1/3/6）
> 内置三个 Agent：`agent-general`(通用助手)、`agent-chain-builder`、`agent-skill-generator`（is_builtin）。
**GET `/api/v1/agents`** — Agent 列表。
**GET `/api/v1/agents/{id}`** — 详情（instruction/skillIds/mcpIds/llmModelId）。
**PUT `/api/v1/agents/{id}`** — 更新配置。
**GET `/api/v1/agents/{id}/sessions`** — 该 Agent 的会话列表（对话页侧栏）。
**GET `/api/v1/sessions?agentId=&keyword=&page=&pageSize=`** — 会话检索（Agent 日志页左侧，可按 Agent 过滤）。
**GET `/api/v1/sessions/{sid}/messages`** — 会话消息（含 toolCalls、attachments、subAgent 标记，日志页右侧回放）。
**DELETE `/api/v1/sessions/{sid}`** — 删除会话。
**POST `/api/v1/assets`** — 上传会话附件（multipart，图片/文件）。Resp `data:{assetId,name,mimeType,size,url}`。发送消息时引用 assetId。
**GET `/api/v1/assets/{id}`** — 下载/预览附件（图片直接返回二进制）。
> Agent 对话走 WebSocket（见下）。

### J. Cron 定时任务（需求1）
**GET / POST `/api/v1/cron`**，**PUT / DELETE `/api/v1/cron/{id}`**，**POST `/{id}/toggle`**。
Req 示例 `{ "targetType":"chain", "targetId":"chain_1", "scheduleType":"cron", "cronExpr":"0 */5 * * * *", "payload":{...} }`。

### K. Dashboard 总览
**GET `/api/v1/dashboard`** — 统计：`{chainCount, publishedCount, skillCount, mcpCount, todayRunCount, failRunCount, agentCount, recentRuns:[...]}`。

### L. 运维 / 审计
**GET `/api/v1/audit?action=&userId=&page=&pageSize=`** — 审计日志查询（仅 admin）。`data.list[]:{id,userId,action,targetType,targetId,detail,ip,createdAt}`。
**GET `/healthz` / `/readyz`** — 存活 / 就绪（db 可达）。
**GET `/metrics`** — Prometheus 指标（规则链执行、Agent token、MCP 延迟、WS 连接数）。

---

## 第三部分：WebSocket 协议

**连接**：`GET /ws?sid=<baboflow_sid>`（或读 Cookie）。单连接多 channel，消息统一 JSON 信封：
```json
{ "channel":"agent-chat", "type":"delta", "seq":12, "payload":{...} }
```

### 1) channel `agent-chat`（Agent 对话，需求1/3/6）
- 客户端发送（可携带附件 assetIds，模型 vision=true 时图片以多模态注入）：
```json
{ "channel":"agent-chat", "type":"send",
  "payload":{ "agentId":1, "sessionId":0, "message":"看这张图帮我分析", "assetIds":[7,8] } }
```
- 服务端推送序列：
```
type=session   payload:{sessionId:55}                       # 新会话时
type=delta     payload:{text:"我先看一下…"}                  # LLM 流式 token
type=tool      payload:{name:"search_component", args:{...}, status:"start"}
type=tool      payload:{name:"search_component", status:"end", result:{...}}
type=subagent  payload:{agentKey:"agent-chain-builder", status:"start", task:"生成规则链DSL"}  # 委派给 subAgent
type=subagent  payload:{agentKey:"agent-chain-builder", status:"end", summary:"已生成草稿"}     # subAgent 完成
type=delta     payload:{text:"已生成规则链, 是否发布?"}
type=done      payload:{messageId:9, tokens:1200}
type=error     payload:{message:"..."}
```

### 2) channel `chain-debug`（规则链调试，需求5）
- 客户端发起调试：
```json
{ "channel":"chain-debug", "type":"start", "payload":{ "chainId":"chain_1", "input":{ "dataType":"JSON", "data":"{\"t\":35}", "metadata":{} } } }
```
- 服务端推送（来自 RuleGo `config.OnDebug` 回调）：
```
type=run       payload:{runId:100, status:"start"}
type=node      payload:{nodeId:"s1", nodeName:"过滤", phase:"in",  msg:{...}}
type=node      payload:{nodeId:"s1", nodeName:"过滤", phase:"out", msg:{...}, relationType:"Success"}
type=node      payload:{nodeId:"s2", nodeName:"日志", phase:"out", relationType:"Failure", err:"..."}
type=run       payload:{runId:100, status:"failure", output:{...}, error:"..."}
```
- 运行结束写 `chain_run`（含 node_trace），前端画布按 nodeId 高亮、`/runs` 可回放。

---

## 第四部分：aggo / RuleGo 集成点（`internal/biz`）

### agentkit（aggo 封装）
- `Provider.Build(llmModelID)` → 经 `llm_model → llm_provider` 取 baseUrl/解密 apiKey，构造 OpenAI 兼容 ChatModel（model/temperature/maxTokens）。
- `Manager.Get(agentKey)` → 缓存的 `ChatModelAgent`（ReAct）：
  - Tools：① **内置工具**（见下 builtin）② MCP 工具（按 `mcp_ids` 经 eino-ext mcp 拉取）③ `search_component`（向量检索 component_meta）④ `rulechain_*`（创建/校验/发布规则链）⑤ `skill_*` 工具
  - Skills：按 `skill_ids` 加载 SKILL.md（Eino skill middleware）
  - Memory：`memory_backend`
  - SubAgent：按 `agent_sub_agent` 把子 Agent 注册为工具（Agent-as-Tool）

#### 内置工具（builtin_tools.go）—— 所有 Agent 默认挂载
> 让 Agent 具备本地执行与文件操作能力，实现 bash / read / write / edit / grep 五个工具（Eino Tool，schema 见下）。**安全受控**：默认沙箱在工作区目录 `BABO_WORKSPACE`（默认 `./workspace`，容器内 `/app/workspace`），路径越界拒绝；bash 有命令白名单/超时。

| 工具 | 入参 | 说明 |
|---|---|---|
| `bash` | `{command, timeoutSec?}` | 在工作区执行 shell；默认超时 30s、输出截断 100KB；危险命令(rm -rf / 等)拦截；返回 stdout/stderr/exitCode |
| `read` | `{path, offset?, limit?}` | 读文件文本（行号），支持大文件分页；二进制返回提示 |
| `write` | `{path, content}` | 写文件（覆盖/新建），自动建父目录；写前备份到 `.bak` |
| `edit` | `{path, oldString, newString}` | 精确字符串替换（须唯一匹配，否则报错），适合局部修改 |
| `grep` | `{pattern, path?, glob?, ignoreCase?}` | 在工作区内按正则搜索文件内容，返回 `file:line:content` 匹配列表 |

- 工具调用/结果记入 `agent_message.tool_calls` 并经 WS `tool` 事件实时推送（前端可展开）。
- 文件类工具作用域 = 该 Agent 会话工作区（`workspace/<sessionId>/`），便于隔离与清理；可用于让 Agent 生成规则链 DSL 文件、读写 SKILL.md、检索项目文件等。
- **Agent1 `agent-chain-builder`** instruction：「理解需求→调 search_component 查可用组件→与用户确认细节→产出合法 RuleGo DSL→调 rulechain_validate/create」
- **Agent2 `agent-skill-generator`** instruction：读规则链 DSL→生成 SKILL.md（name/description/何时用/输入输出 schema/示例）
- **通用助手 `agent-general`**：内置第三个 Agent，挂载全部 SKILL + MCP 工具，处理通用问答/编排，可调用 `rulechain_*` 与 `skill_*` 工具。三者均为内置（is_builtin），Agent 对话页 `/agents/:key/chat` 按所选 Agent 隔离会话。
- **SubAgent（多 Agent 委派）**：`agent_sub_agent` 表描述父→子委派关系。运行时 Manager 把每个 subAgent 注册为父 Agent 的一个工具（Eino ADK 支持把 Agent 作为 Tool），父 Agent ReAct 决策何时委派；子 Agent 在**隔离上下文**执行（不共享父历史），结果回传父 Agent。委派过程经 WS `subagent` 事件推给前端展示。典型：`agent-general` 作为父，把「生成规则链」委派给 `agent-chain-builder`。
- **多模态输入**：发送消息时附 `assetIds`；`capability.vision=true` 的模型，图片以 base64/URL 构造多模态 content；文档类（pdf/csv/txt）先抽取文本随 prompt 注入。附件存 `asset` 表 + 本地/对象存储。

### rulegokit（RuleGo 封装）
- **组件范围**：仅注册 ① RuleGo 内置组件 ② 平台新增的 **Agent 组件**（`baboflow/agent`，在规则链节点内调用指定 Agent 执行，配置 `{agentKey, prompt}`）。`ScanComponents()` 只落这两类到 `component_meta`。
- `Engine` 池：`map[chainID]*rulego.RuleEngine` + RWMutex；`Publish` 时 `rulego.New(id, dsl)` 热加载；`Offline`（撤销发布）时卸载。
- `Run(chainID, msg)` 同步执行收集结果。
- `Debug`：`config.OnDebug = func(...)` 把逐节点事件写入 `chain_run.node_trace` 并经 WS 推送。
- **Agent 组件注册**：实现 RuleGo `types.Node`，`OnMsg` 时经 agentkit 调目标 Agent（传入 msg.data 作为 prompt），结果作为新 msg 传出（relationType=Success/Failure）。
- `ExposeMCP(chainID, toolName, schema)`：经 MCP-server-endpoint 注册为工具，服务对外提供 `/mcp` 端点。

### 组件知识库 / SKILL 自动同步（零人工）
> 目标：后端新增/升级一个 RuleGo 组件（代码注册即生效）后，component_meta 知识库与其 embedding、以及对应 SKILL **自动写入/更新**，无需人工操作。

**触发时机（三路兜底）：**
1. **启动全量扫描**：服务启动时 `ComponentSync.Run()` 遍历 RuleGo 注册表 `rulego.Registry.GetComponents()`，与 `component_meta` 比对 diff。
2. **注册钩子**：自定义组件统一经 `rulegokit.Register(name, factory, meta)` 包装注册（内部调 RuleGo 注册 + 触发 upsert），新增组件在代码 `init()` 里调用即自动入库；后续可在运行时热注册。
3. **周期对账**：轻量定时任务（如每 10 min）重扫注册表，防漏网。

**同步流水线（每组件）：**
```
detect(新增/变更/删除)
  ├─ 生成指纹 hash(type + configSchema + description) → 无变化则跳过(幂等)
  ├─ upsert component_meta (type/name/category/description/configSchema/example)
  ├─ 若 description/schema 变化 → 用 embedding 模型重算 embedding(向量列)
  ├─ 自动生成/更新该组件的 SKILL 片段(SKILL.md, source=builtin/component):
  │     name=组件type, description=组件用途+何时使用+配置项说明+示例DSL
  │     写入 skill 表 + 落盘 file_path, 并刷新其 embedding
  └─ 删除：注册表中已不存在 → 标记 component_meta 失效(或软删对应 SKILL)
```
- **embedding 模型**：取 `llm_model` 中 `capability.embedding=true` 的模型；未配置时退化为「仅结构入库、向量置空」，语义检索降级为关键字检索（不影响启动）。
- **SKILL 生成**：默认用**模板化生成**（不依赖 LLM，确定性、离线可用）：按组件 type/category/configSchema 渲染成固定结构的 SKILL.md（用途/何时用/配置项表/最小可用 DSL 示例）。可选开关 `component_skill_llm=true` 时改用默认 chat 模型润色描述，失败回退模板。
- **Agent 组件特殊**：除通用组件 SKILL 外，`baboflow/agent` 的 SKILL 还会列出当前可用 Agent 清单（动态拼装）。
- **组件聚合知识**：额外维护一份「组件总览 SKILL」`baboflow-components-overview`（按 category 分组的组件目录 + 选用指引），供 Agent1 快速判断「该用哪些组件」；每次同步后重建。
- **并发/幂等**：同步用单飞（singleflight）+ DB upsert（`ON CONFLICT(type) DO UPDATE`）；hash 不变不写库不重算 embedding，避免重复消耗。

**对外可观测（可选接口）：**
- `GET /api/v1/components/sync` — 查看最近同步结果 `{added, updated, removed, skipped, lastRunAt}`。
- `POST /api/v1/components/sync` — 手动触发一次同步（运维兜底，正常无需调用）。

### 安全
- apiKey：AES-GCM，密钥 `BABO_SECRET`（32字节）经 **.env** 注入（Viper 读取；`.env` 提供 `.env.example`，不入版本库），入库为 `api_key_enc`，接口仅回掩码。
- 密码：bcrypt。Session：随机 128-bit ID，HttpOnly Cookie，滑动过期（默认 7 天）。
- 所有外部输入（DSL、SKILL 内容、看板 payload、WS 消息）经校验；RuleGo JS 脚本节点在受控引擎内执行。
- `.env` 关键项：`BABO_SECRET`、`DATABASE_DSN`、`HTTP_ADDR`、`GRPC_ADDR`（默认 `127.0.0.1:9000`；仅在受控反向代理或专用网络需要时显式覆盖）、`COMPONENT_SKILL_LLM`(组件 SKILL 是否用 LLM 润色, 默认 false 走模板)、`BABO_WORKSPACE`(Agent 内置工具沙箱目录, 默认 ./workspace)、`EXECUTOR_WORKERS`(规则链执行并发, 默认 8)、`BASH_ALLOWLIST`(bash 工具命令白名单, 逗号分隔, 空=黑名单模式)、`EMBEDDING_DIM`(向量维度, 默认 1536)、`EMBEDDING_MODEL_ID`(生成向量所用 llm_model.id, 可配置)、`ADMIN_INIT_PASSWORD`(首次启动 admin 初始密码, 登录后强制改密)。

---

## 第四部分 c：已定的实现决策（实施约束）
1. **API 形态（混合）**：核心业务域（auth/llm/rulechain/component/skill/agent/mcp/board/runlog/cron/dashboard/audit）用 **proto 定义 + Kratos 生成 HTTP+gRPC**；**WebSocket、文件上传(/assets)、前端静态托管、/metrics、/healthz** 用原生 `kratos http` 手写路由（不走 proto）。
2. **embedding 可配置**：向量维度 `EMBEDDING_DIM` + 生成向量的模型 `EMBEDDING_MODEL_ID` 均可配置；component_meta/skill 的 embedding 统一由该模型生成。换维度→重建向量列+重算（迁移命令）；未配置则向量置空、语义检索降级关键字。
3. **首次启动种子**（`data/migrate.go`）：建 vector 扩展 + AutoMigrate；若无 admin 则用 `ADMIN_INIT_PASSWORD` 建默认 admin（标记须改密）；种子内置 3 Agent（agent-general/chain-builder/skill-generator）；随后跑一次组件同步（componentsync）。
4. **会话工作区随会话删**：删除会话（`DELETE /sessions/{sid}`）时级联删除 `workspace/<sessionId>/`；孤儿目录由启动时清扫兜底。
5. **会话附件**：图片仅在所选模型 `capability.vision=true` 时走多模态，文档类抽取文本注入；均先落 `asset` 表。

---

## 第四部分 b：健壮性与安全加固（补强）

### 1. 规则链治理（导入 / 导出 / 校验）
- **导出**：`GET /api/v1/chains/{id}/export` → 下载链 JSON（含 name/description/dsl/version），便于迁移/备份/分享。
- **导入**：`POST /api/v1/chains/import`（body 为链 JSON）→ 校验通过后新建 draft；`id` 冲突时生成新 id；引用的组件 type 不存在时返回 400 + 缺失清单。
- **校验**：保存/发布统一走 `rulegokit.Validate(dsl)` —— 结构合法 + 组件 type 在注册表存在 + 必填配置齐全 + 连接关系无悬空节点/环（除容器内子链）。发布前强制校验，失败返回具体节点错误。
- **发布守卫**：发布必须基于当前 draft 通过校验；版本号单调递增，回滚生成新版本快照而非覆盖，保证可追溯。

### 2. 任务执行健壮性（看板 / 触发）
- **超时**：每次执行带 `timeout_sec`（task 可配，默认 300s），RuleGo `OnMsgWithOptions` 超时取消，`chain_run.status=timeout`。
- **重试**：失败按 `retry_max/retry_count` 指数退避自动重试（如 5s/15s/45s）；超过上限置 `failure`。
- **并发限制**：全局执行器为有界 worker 池（`EXECUTOR_WORKERS`，默认 8），同一 `chain_id` 可配并发上限，防止单链打满；超出排队。
- **人工介入**：失败任务支持「重跑」「跳过节点重跑」（从指定节点续跑，用上次中间结果）。
- **隔离**：任务执行与 WS 调试执行共用引擎池但独立 context，互不强杀。

### 3. MCP 稳定性与凭证
- **超时/重试/熔断**：调用外部 MCP 工具统一包裹 `timeout(默认30s) + 重试(2次) + 熔断(连续失败 N 次打开, 半开探测)`，避免单个 MCP 服务拖垮 Agent。
- **stdio 守护**：stdio 模式 MCP server 以子进程托管，崩溃自动重启（退避）、健康检查写 `status/last_check_at`。
- **凭证脱敏**：`mcp_server.env` 中的敏感值（含 KEY/TOKEN/SECRET 字样）AES-GCM 加密存储、接口仅回掩码；日志/审计中一律脱敏。

### 4. 安全与审计
- **限流**：登录接口（防爆破，IP+账号双维度）、调试/运行触发接口、LLM test 接口加令牌桶限流（Kratos ratelimit 中间件）。
- **bash 工具管控**：在沙箱基础上增加命令白名单模式（可选 `BASH_ALLOWLIST`）+ 资源限额（超时/输出截断/禁网络可选）；默认禁 `rm -rf /`、`sudo`、`>(/dev/tcp)` 等。
- **审计**：敏感写操作（登录、改密、LLM 增删改、规则链 发布/删除/导入、MCP 暴露/取消、任务触发、SKILL 删除）写 `audit_log`；提供 `GET /api/v1/audit?action=&user=&page=` 查询（仅 admin）。
- **密钥轮换**：`BABO_SECRET` 变更时提供 `api_key_enc` 重加密迁移命令。

### 5. 可观测性补强
- **指标**：暴露 `/metrics`（Prometheus）：规则链执行次数/耗时/失败率、Agent token 用量、MCP 调用延迟、WS 连接数。
- **健康检查**：`/healthz`(存活) `/readyz`(就绪: db 可达)。
- **结构化日志**：Kratos log 统一 JSON，关键操作带 trace_id 串联。

> 注：以上补强均为单租户下的服务端能力；多租户字段已预留（见 0 公共约定），升级时启用租户中间件即可。

---

## 第五部分：部署（Docker / docker-compose）

### 5.1 后端 Dockerfile（多阶段）
```dockerfile
# ---- 构建 ----
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 生成 wire / 编译(Kratos)
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/baboflow ./cmd/baboflow

# ---- 前端构建 ----
FROM node:20-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web ./
RUN npm run build   # 产物 dist/

# ---- 运行 ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/baboflow /app/baboflow
COPY --from=build /src/configs /app/configs
COPY --from=web /web/dist /app/web/dist     # 由后端静态托管
EXPOSE 8000 9000
ENTRYPOINT ["/app/baboflow", "-conf", "/app/configs"]
```

### 5.2 docker-compose.yml
```yaml
services:
  db:
    image: pgvector/pgvector:pg16          # 自带 vector 扩展
    environment:
      POSTGRES_USER: babo
      POSTGRES_PASSWORD: babo
      POSTGRES_DB: baboflow
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./docker/db-init:/docker-entrypoint-initdb.d:ro
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U babo -d baboflow"]
      interval: 5s
      timeout: 3s
      retries: 12
    ports: ["0.0.0.0:5433:5432"]

  baboflow:
    build: .
    environment:
      DATABASE_DSN: host=db user=babo password=babo dbname=baboflow port=5432 sslmode=disable
      BABO_SECRET: ${BABO_SECRET:-please-change-me-32bytes-secret!!}
      HTTP_ADDR: ":8000"
      MCP_AUTH_TOKEN: ${MCP_AUTH_TOKEN:-}
      # 未设置 GRPC_ADDR：默认 127.0.0.1:9000，compose 不发布 gRPC 端口。
      # 如需内网 gRPC，仅可通过受控网络配置显式覆盖 GRPC_ADDR。
      BABO_WORKSPACE: /app/workspace      # Agent 内置工具(bash/read/write/edit/grep)沙箱
      ADMIN_INIT_PASSWORD: ${ADMIN_INIT_PASSWORD:-admin123}
    volumes:
      - workspace:/app/workspace           # 持久化 Agent 工作区
    depends_on:
      db:
        condition: service_healthy
    ports:
      - "8001:8000"   # HTTP/WS + 前端静态资源 + /mcp
volumes:
  pgdata:
  workspace:
```
- 单容器内嵌前端 `dist`（Kratos 静态托管）+ 自动执行 DB migration（启动时 `CREATE EXTENSION IF NOT EXISTS vector` + AutoMigrate）。
- `docker compose up -d` 一键拉起：db(pgvector) + baboflow；浏览器访问 `http://localhost:8001`。
- `.env.example` 提供 `BABO_SECRET` 等默认值；生产通过环境变量覆盖。

---

## 第六部分：proto HTTP/gRPC 迁移实施说明（以当前实现为准）

### 服务监听与暴露

- HTTP 由 `HTTP_ADDR` 配置，默认 `:8000`，承载 proto HTTP、WebSocket、MCP、OAuth、静态资源与上传下载旁路。
- gRPC 由 `GRPC_ADDR` 配置，默认 `127.0.0.1:9000`。它只供本机进程、受控反向代理或专用网络调用；生产环境不要将 `:9000` 直接映射到公网，应保持 loopback 绑定或在防火墙、网络策略中拒绝公网入站。
- HTTP 与 gRPC 复用同一组 proto service 实现、会话认证与限流策略；HTTP 使用 `baboflow_sid` Cookie，gRPC 使用 metadata `authorization: Bearer <session-id>`。

### 生成与资源目录

核心业务契约位于 `api/baboflow/v1/`：`auth.proto`、`archery.proto`、`llm.proto`、`component.proto`、`rulechain.proto`、`agent.proto`、`skill.proto`、`mcp.proto`、`board.proto`、`audit.proto`、`cron.proto` 与公共定义 `common.proto`。每个业务 proto 声明 `google.api.http` 绑定，并生成同目录的 `*.pb.go`、`*_grpc.pb.go`、`*_http.pb.go`。

修改 proto 后依次执行：

```bash
make api
make generate
```

`make api` 生成 protobuf、gRPC 与 Kratos HTTP 代码；`make generate` 在此基础上执行 Wire 依赖注入生成。

### HTTP 响应与旁路边界

- proto HTTP 成功响应为 protobuf JSON；`int64`/`uint64` 在 JSON 中以十进制字符串表示，前端应保留字符串以避免 JavaScript 大整数精度丢失。错误由 Kratos 原生 ErrorEncoder 输出 JSON，调用方按 HTTP status 和 `reason`/`message` 处理，不能再解包旧 `{code,message,data}` 业务信封。
- 以下端点不走 proto：`/healthz`、`/readyz`、`/metrics`、`/ws`、`/mcp/sse`、`/mcp/message`、`/api/v1/auth/feishu/login`、`/api/v1/auth/feishu/callback`、`/assets/*` 与 SPA 回退。
- multipart/raw 下载仍是 HTTP 旁路：`POST /api/v1/agent-assets`、`GET /api/v1/agent-assets/{assetId}`、`POST /api/v1/skills/package`、`GET /api/v1/skills/{id}/package`。其中资产与技能包端点要求会话；MCP 端点要求会话或 `MCP_AUTH_TOKEN` Bearer。
