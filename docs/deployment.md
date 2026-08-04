# 部署（Docker / docker-compose）

一键拉起 **pgvector(db) + langfuse + baboflow**，浏览器访问 `http://localhost:8000`。

## 快速开始

```bash
cp .env.example .env        # 可选：compose 主要从环境变量读取，默认值已内置
docker compose up -d --build
```

- 前端 + 后端 + WS + MCP 同端口：`http://localhost:8000`
- 首次启动自动：建 `vector` 扩展 → AutoMigrate 全表 → 种子 admin/内置 Agent → 组件同步 + 组件 SKILL 兜底
- 默认账号 `admin` / `ADMIN_INIT_PASSWORD`（默认 `admin123`，登录后强制改密）

## 服务

| 服务 | 镜像 | 端口 | 说明 |
|------|------|------|------|
| db | pgvector/pgvector:pg16 | 宿主 5433 → 容器 5432 | PostgreSQL + vector；`docker/db-init` 首次自动建 `langfuse` 库 |
| langfuse | langfuse/langfuse:2 | 3001 | LLM 观测（可选）；Web 在容器 3000 |
| baboflow | 本地 build | 8000 | HTTP/WS/静态托管/MCP SSE |

> db 宿主端口用 **5433** 而非 5432，避免与本机已有 PostgreSQL 冲突；容器间通信仍走内部 `db:5432`，无需改 `DATABASE_DSN`。

## 环境变量

全部经环境变量注入（见 `.env.example`）。compose 中关键项：

- `BABO_SECRET` — apiKey AES-GCM 密钥（32 字节，**生产务必改**）
- `ADMIN_INIT_PASSWORD` — admin 初始密码
- `MCP_AUTH_TOKEN` — `/mcp` SSE 端点的 Bearer 令牌（外部 MCP 客户端无 Cookie 时用它鉴权；留空则仅接受已登录会话）。**该端点会执行已发布规则链，已默认鉴权**；对外提供 MCP 服务时请设强随机令牌。
- `LANGFUSE_PUBLIC_KEY` / `LANGFUSE_SECRET_KEY` — 在 Langfuse Web 建项目后填入，留空则关闭上报

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
