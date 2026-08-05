# 部署（Docker / docker-compose）

一键拉起 **pgvector(db) + langfuse + baboflow**。服务内部 HTTP 默认监听 `:8000`；当前 `docker-compose.yml` 将它映射到宿主机 `http://localhost:8001`。

## 快速开始

```bash
cp .env.example .env        # 可选：compose 主要从环境变量读取，默认值已内置
docker compose up -d --build
```

- 前端 + 后端 + WS + MCP 同端口：容器内 `:8000`，当前 compose 宿主机 `http://localhost:8001`
- 首次启动自动：建 `vector` 扩展 → AutoMigrate 全表 → 种子 admin/内置 Agent → 组件同步 + 组件 SKILL 兜底
- 默认账号 `admin` / `ADMIN_INIT_PASSWORD`（默认 `admin123`，登录后强制改密）
- HTTP 服务默认监听 `HTTP_ADDR=:8000`；当前 compose 将容器 `8000` 映射到宿主机 `8001`，因此 compose 环境请访问 `http://localhost:8001`。

## 服务

| 服务 | 镜像 | 端口 | 说明 |
|------|------|------|------|
| db | pgvector/pgvector:pg16 | 宿主 5433 → 容器 5432 | PostgreSQL + vector；`docker/db-init` 首次自动建 `langfuse` 库 |
| langfuse | langfuse/langfuse:2 | 3001 | LLM 观测（可选）；Web 在容器 3000 |
| baboflow | 本地 build | 8000 | HTTP/WS/静态托管/MCP SSE |
| baboflow gRPC | 同 baboflow 进程 | `127.0.0.1:9000` | proto gRPC；默认不发布宿主机端口 |

> db 宿主端口用 **5433** 而非 5432，避免与本机已有 PostgreSQL 冲突；容器间通信仍走内部 `db:5432`，无需改 `DATABASE_DSN`。

## 环境变量

全部经环境变量注入（见 `.env.example`）。compose 中关键项：

- `BABO_SECRET` — apiKey AES-GCM 密钥（32 字节，**生产务必改**）
- `ADMIN_INIT_PASSWORD` — admin 初始密码
- `MCP_AUTH_TOKEN` — `/mcp` SSE 端点的 Bearer 令牌（外部 MCP 客户端无 Cookie 时用它鉴权；留空则仅接受已登录会话）。**该端点会执行已发布规则链，已默认鉴权**；对外提供 MCP 服务时请设强随机令牌。
- `LANGFUSE_PUBLIC_KEY` / `LANGFUSE_SECRET_KEY` — 在 Langfuse Web 建项目后填入，留空则关闭上报
- `HTTP_ADDR` — HTTP 监听地址，默认 `:8000`。
- `GRPC_ADDR` — gRPC 监听地址，默认 `127.0.0.1:9000`。仅在受控反向代理或专用网络需要时覆盖；不要直接暴露到公网，须保持防火墙/网络策略隔离。

## proto API 生成与协议边界

核心业务接口的 proto 位于 `api/baboflow/v1/`，生成文件与源 proto 同目录。修改契约后执行：

```bash
make api       # 生成 protobuf、gRPC、Kratos HTTP 绑定
make generate  # 生成 Wire 注入代码
```

业务 proto HTTP 端点使用 protobuf JSON；错误使用 Kratos 原生 JSON 错误体，客户端按 HTTP status 与 `reason`/`message` 处理，不再依赖旧 `{code,message,data}` 信封。所有 `int64`/`uint64` 字段使用十进制字符串，客户端应避免转换为 JavaScript `number`。

以下能力保留 HTTP 旁路：`/healthz`、`/readyz`、`/metrics`、`/ws`、`/mcp/sse`、`/mcp/message`、飞书 OAuth、`/assets/*`、SPA 静态资源与回退，以及 multipart/raw 文件端点：

- `POST /api/v1/agent-assets`、`GET /api/v1/agent-assets/{assetId}`
- `POST /api/v1/skills/package`、`GET /api/v1/skills/{id}/package`

资产与技能包端点必须携带 `baboflow_sid`；MCP 端点必须携带已登录会话或 `MCP_AUTH_TOKEN` Bearer。

## 运维端点

- `GET /healthz` 存活
- `GET /readyz` 就绪（db 可达）
- `GET /metrics` Prometheus 指标（规则链执行/耗时、MCP 调用、WS 连接、定时任务触发）

## 数据持久化

- `pgdata` volume：PostgreSQL 数据（含用户、规则链、Agent、SKILL 等全部业务表）。**SKILL 技能包的 zip 归档也作为权威源存于 skill 表 `package bytea` 列**，随此卷持久化，重建不丢。
- `workspace` volume：Agent 内置工具（bash/read/write/edit/grep）沙箱。**含包 SKILL 在运行时按需解压落盘到 `workspace/skills/<name>/`**，供模型经 eino `BaseDirectory` 读包内附属文件（references/scripts 等）；磁盘目录是可重建缓存——即便丢失（如卷被清），只要 DB 记录仍在，读取技能时会自动从 DB 归档重新解压（自愈）。

> 因此 `docker compose down` / `up -d --build` 均不影响技能包；仅 `down -v` 同时清空两卷才会删除。

## 常用命令

```bash
docker compose logs -f baboflow   # 看日志
docker compose down               # 停止（保留数据卷）
docker compose down -v            # 停止并清空数据（⚠️ 删库）
```
