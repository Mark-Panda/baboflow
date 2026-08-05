# Kratos Proto HTTP/gRPC 迁移设计

## 目标

将 BaboFlow 业务 HTTP 接口从 Gin Handler 迁移到 Kratos 标准 **proto 契约**（HTTP + gRPC），并补齐 `Makefile` / `protoc` 生成链路。浏览器继续使用 axios + JSON；gRPC 同步注册，首版可不对公网暴露。

非目标：不改业务语义与数据库模型；不引入 gRPC-Web；不把 WebSocket / MCP SSE / SPA 静态资源塞进 proto。

## 决策摘要

| 项 | 选择 |
|----|------|
| 范围 | 全量可迁移的标准 JSON REST |
| 响应格式 | Kratos 原生（去掉 `{code,message,data}` 信封） |
| 路径 | 按领域重划 API 面（非兼容旧 path） |
| 传输 | HTTP + gRPC 双端 |
| 前端 | 与后端同一次交付改造 |
| 架构 | 标准 Kratos 双协议注册 + 特殊端点旁路 |

## 现状问题

- 依赖 Kratos `http.Server`，但业务路由全部为 Gin，`srv.HandlePrefix("/", r)`。
- 仓库无 `api/**/*.proto`、无 Makefile / protoc 生成。
- 统一信封与 Gin 中间件（Session、限流）绑定，无法复用 Kratos middleware / 错误模型。

## 架构

```
Browser (axios JSON + Cookie)
        │
        ▼
khttp.Server
  ├─ Register*HTTPServer (proto 业务 API)
  ├─ Auth / RateLimit middleware
  └─ HandleFunc/HandlePrefix 旁路：
       /ws, /mcp/sse, /mcp/message,
       feishu OAuth, healthz/readyz/metrics,
       SPA static, multipart/binary I/O

gRPC clients (内网)
        │
        ▼
kgrpc.Server + Register*Server
  └─ Auth middleware (metadata Bearer)
```

分层对齐 Kratos：

- **service**：只接触 proto；校验、取操作者、调 biz、返回 proto。
- **biz**：业务逻辑；可直接使用 proto 消息做入出参（或短期 adapter，目标收敛到 proto）。
- **data**：仅 DB model / entity，不依赖 proto。

## API 领域划分

- Package：`baboflow.v1`，Go 路径：`baboflow/api/baboflow/v1`
- HTTP 前缀：`/api/v1`
- JSON：`protojson`，字段 camelCase

| Proto | Service | HTTP 资源前缀 |
|-------|---------|---------------|
| `auth.proto` | AuthService | `/api/v1/auth/*` |
| `llm.proto` | LLMService | `/api/v1/llm/providers`、`/api/v1/llm/models` |
| `archery.proto` | ArcheryService | `/api/v1/archery/connections` |
| `component.proto` | ComponentService | `/api/v1/components` |
| `rulechain.proto` | RuleChainService | `/api/v1/rule-chains`、`/api/v1/rule-chain-runs` |
| `agent.proto` | AgentService | `/api/v1/agents`、`/api/v1/agent-sessions`、`/api/v1/agent-assets` |
| `skill.proto` | SkillService | `/api/v1/skills` |
| `mcp.proto` | McpService | `/api/v1/mcp/servers`、`/api/v1/mcp/exposures` |
| `board.proto` | BoardService | `/api/v1/boards`、`/api/v1/board-columns`、`/api/v1/tasks` |
| `audit.proto` | AuditService | `/api/v1/audit-logs` |
| `cron.proto` | CronService | `/api/v1/cron-jobs` |

相对旧 API 的主要重划：

- `chains` → `rule-chains`；runs → `rule-chain-runs`
- Agent sessions / assets 独立资源前缀
- Board columns / tasks 与 boards 解耦命名
- `audit` → `audit-logs`；`cron` → `cron-jobs`

分页在 proto 中显式定义，字段保持 `list` / `total` / `page` / `pageSize`。

## 旁路端点（不进 proto）

| 端点 | 原因 |
|------|------|
| `GET /healthz`、`GET /readyz`、`GET /metrics` | 运维探针 |
| `GET /ws` | WebSocket |
| `ANY /mcp/sse`、`ANY /mcp/message` | MCP SSE + MCPAuth |
| 飞书 `login` / `callback` | 302 + Cookie |
| SPA `/assets/*` + index 回退 | 静态文件 |

二进制 I/O（标准 `http.Handler`，路径对齐新资源风格）：

| 能力 | 路径 |
|------|------|
| Agent 资产上传 | `POST /api/v1/agent-assets`（multipart） |
| Agent 资产读取 | `GET /api/v1/agent-assets/{assetId}` |
| Skill 包上传 | `POST /api/v1/skills:upload-package`（multipart） |
| Skill 包下载 | `GET /api/v1/skills/{id}/package` |
| Skill Markdown 上传 | 默认旁路 multipart；若改为 JSON body 则可进 proto |

进 proto 的类文件能力：规则链 Export/Import（JSON）。

Auth Login/Logout：proto 返回会话元数据；HTTP filter 负责 `Set-Cookie`（`baboflow_sid`）。

## 鉴权与错误

- HTTP：Cookie Session middleware；白名单含 Login、健康检查、metrics、飞书 OAuth、静态资源；MCP SSE 保留 Cookie 或 `MCP_AUTH_TOKEN` Bearer。
- gRPC：metadata `authorization: Bearer <token>`；与 AuthUsecase 共用校验。
- 限流：登录与触发类接口用 Kratos middleware 替代 Gin RateLimit。
- 错误：biz 使用 `kratos/errors`；HTTP/gRPC 统一 ErrorEncoder；前端按 status + `reason`/`message` 处理。

## 服务端改造要点

- 新增 `internal/server/grpc.go`；`wire` 同时提供 HTTP + gRPC。
- `internal/service/*` 从 `*Handler` 改为实现 proto `XxxServer`。
- 移除 Gin 业务路由与 `httputil` 业务信封；旁路改为标准库 `http.Handler`（可移除 gin 依赖）。
- 配置新增 `GRPCAddr`（如 `:9000`），写入 `config/default.yaml` 与 `.env.example`。

## 前端改造范围

- `web/src/api/http.ts`：去掉信封解包；成功用 `resp.data`；失败读 HTTP status + Kratos 错误体；保留 `withCredentials`。
- 重写全部 `web/src/api/*.ts` 路径与类型；修正硬编码旧 path 的页面/hooks。
- multipart 打旁路路径；WS 客户端不变。
- 不引入 protobuf.js / gRPC-Web。

## Makefile / 工具链

目录：

```
api/baboflow/v1/*.proto
third_party/google/api/...
Makefile
```

工具：`protoc-gen-go`、`protoc-gen-go-grpc`、`protoc-gen-go-http`、可选 `protoc-gen-openapi`、已有 `wire`。

| Target | 作用 |
|--------|------|
| `make init` | 安装 protoc 插件 |
| `make api` | 生成 `.pb.go` / `_grpc.pb.go` / `_http.pb.go`（及可选 openapi） |
| `make generate` | `go generate` + wire |
| `make all` | `api` + `generate` |
| `make build` | 编译 `cmd/baboflow` |
| `make test` | `go test ./...` |

生成约定：`paths=source_relative`；输出在 `api/baboflow/v1/`；`third_party` 与生成文件入库，保证 CI 可复现。

## 迁移完成标准

1. 业务 JSON API 全部由 proto HTTP 注册；Gin 业务路由清零。
2. gRPC Server 已注册全部业务 service。
3. `make api` / `make generate` / `make build` / `make test` 可用且通过。
4. 前端主路径联调通过：登录、各 CRUD、规则链编辑/运行、Agent/Skill/MCP/看板/Cron/审计；401/4xx/5xx 提示正常。
5. 旁路端点行为与迁移前等价（WS、MCP SSE、飞书 OAuth、静态、健康检查、二进制上传下载）。

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| 路径重划导致前端漏改 | 以 `web/src/api` 为唯一出口；全局搜旧 path |
| Cookie 与 Kratos filter 时序 | Login 响应头写 Cookie 的集成测试 |
| multipart 与 proto 混用 | 二进制明确旁路，文档列出路径 |
| 生成工具版本漂移 | `make init` 固定安装；生成物入库 |
| 一次改动面过大 | 实现计划按 proto service 分批提交，但同一交付内切完旧 Gin |

## 非范围（明确保留旁路）

WebSocket、MCP SSE 传输层、飞书 OAuth 重定向、SPA 托管、Prometheus/健康检查、Agent/Skill 的 multipart 与 raw 下载。
