# Kratos Proto HTTP/gRPC Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 BaboFlow 的标准 JSON 业务接口迁移为由 Kratos proto 生成代码注册的 HTTP/gRPC 服务，同时保留 WebSocket、MCP SSE、OAuth、静态资源和二进制 I/O 旁路能力。

**Architecture:** 使用 `api/baboflow/v1` 下按领域拆分的 proto 作为唯一业务 HTTP/gRPC 契约。`internal/service` 实现生成的 server 接口，service 只处理 proto/上下文，biz 负责业务逻辑与转换，data 继续只依赖模型。`khttp.Server` 和 `kgrpc.Server` 分别注册生成服务，旁路端点使用标准 `http.Handler`。

**Tech Stack:** Go 1.25、Kratos v2.9.2、`google.api.http`、`protoc-gen-go`、`protoc-gen-go-grpc`、`protoc-gen-go-http`、Wire、TypeScript/axios、现有 GORM 和 Session 鉴权。

## Global Constraints

- 业务 JSON API 使用 Kratos 原生响应，不再返回 `{code,message,data}` 信封。
- HTTP 前缀保持 `/api/v1`，资源按设计文档重划为 `rule-chains`、`rule-chain-runs`、`audit-logs`、`cron-jobs` 等。
- 浏览器继续使用 axios + JSON + HttpOnly Cookie，不引入 protobuf.js 或 gRPC-Web。
- gRPC 首版监听内网配置地址，不默认对公网暴露。
- WebSocket、MCP SSE、飞书 OAuth 302、SPA 静态资源、Prometheus/健康检查、multipart/raw 下载不进入 proto。
- service 只能直接使用 proto 和业务接口；复杂校验、权限和转换放在 biz；data 不依赖 proto。
- 生成文件和 `third_party` 入库；所有生成命令必须可由 Makefile 重复执行。
- 本仓库已有 `conf.Config.GRPCAddr`、`config/default.yaml:17` 的 `grpcAddr: ":9000"` 和 `GRPC_ADDR` 加载逻辑；不要重复新增配置字段，只需接入 server。

---

## 文件地图

**新增**

- `api/baboflow/v1/common.proto`：分页、空响应、错误相关公共消息。
- `api/baboflow/v1/auth.proto`、`llm.proto`、`archery.proto`、`component.proto`、`rulechain.proto`、`agent.proto`、`skill.proto`、`mcp.proto`、`board.proto`、`audit.proto`、`cron.proto`：领域服务、请求、响应和资源消息。
- `third_party/google/api/annotations.proto`、`http.proto` 及其依赖：HTTP transcoding 选项。
- `Makefile`：插件安装、proto 生成、Wire、测试和构建入口。
- `internal/server/grpc.go`：gRPC Server 构造和领域服务注册。
- `internal/service/kratos_middleware.go`、`internal/service/kratos_sidecar.go`：Kratos middleware 与旁路端点适配。
- `internal/service/*_proto.go`：每个领域生成 server 接口的实现；原 Gin handler 逻辑迁移后删除对应 handler 文件或改名为 proto service。
- `internal/service/kratos_service_test.go`、`internal/server/grpc_test.go`：服务注册、鉴权、错误和 Cookie 行为测试。

**修改**

- `internal/server/http.go`：删除 Gin 业务路由，注册全部 `Register*HTTPServer`，保留旁路。
- `internal/server/provider.go`、`cmd/baboflow/wire.go`、`cmd/baboflow/wire_gen.go`：注入 HTTP/gRPC server 和 proto service。
- `internal/service/auth.go`、`middleware.go`、`ratelimit.go`：从 Gin context 改为 Kratos context/middleware；保留 Cookie 语义。
- `internal/service/{llm,archery,component,rulechain,agent,skill,mcp,board,audit,cron}.go`：迁移业务入口和 JSON/二进制边界。
- `internal/biz/*.go`：只在编译契约需要时改为 proto 入出参；不改变业务规则和 data 接口。
- `cmd/baboflow/main.go`：启动并优雅关闭 gRPC Server。
- `web/src/api/http.ts` 及全部 `web/src/api/*.ts`、相关页面调用点：移除信封解包，切换新资源路径和错误体。
- `go.mod`、`go.sum`：仅通过生成/编译实际需要的模块变更。

## Task 1: 建立 proto 生成链路

**Files:**
- Create: `Makefile`
- Create: `api/baboflow/v1/common.proto`
- Create: `third_party/google/api/annotations.proto`
- Create: `third_party/google/api/http.proto` 及所需 google/api 依赖
- Modify: `go.mod`, `go.sum` only if `go mod tidy` proves a dependency is required

**Interfaces:**
- Produces `make init`, `make api`, `make generate`, `make build`, `make test`.
- Produces generated Go packages under `api/baboflow/v1/`.
- `common.proto` exports `PageRequest { int32 page = 1; int32 page_size = 2; }`, `PageResponse` fields `list`, `total`, `page`, `page_size`, and an explicit `Empty` message.

- [ ] **Step 1: Write the generation smoke test**

Create an `api-smoke` shell check in the Makefile that runs `make api`, verifies the message-only `common.proto` produces `api/baboflow/v1/common.pb.go`, and verifies a real service proto such as `auth.proto` produces `auth.pb.go`, `auth_grpc.pb.go`, and `auth_http.pb.go`. Generate a temporary annotated service twice with all three plugins and compare both outputs, then run `make api` a second time and verify all checked-in generated Go files are unchanged. The check must fail when `protoc` or any required plugin is missing.

- [ ] **Step 2: Implement the Makefile**

Use `protoc --proto_path=api --proto_path=third_party` with `paths=source_relative`; invoke `protoc-gen-go`, `protoc-gen-go-grpc`, and Kratos `protoc-gen-go-http`. `make init` must install the exact generator versions already resolved by this repository (`google.golang.org/protobuf@v1.36.10`, `github.com/go-kratos/kratos/v2@v2.9.2` plugin path) rather than floating versions. `make generate` must run `wire` after `make api`.

- [ ] **Step 3: Add the common proto and HTTP imports**

Define package `baboflow.v1`, `go_package = "baboflow/api/baboflow/v1;v1"`, and import `google/api/annotations.proto`. Keep JSON names camelCase through proto field names such as `page_size` rendered by protojson as `pageSize`.

- [ ] **Step 4: Run the generation smoke test**

Run:

```bash
make init
make api
test -f api/baboflow/v1/common.pb.go
test -f api/baboflow/v1/auth.pb.go
test -f api/baboflow/v1/auth_grpc.pb.go
test -f api/baboflow/v1/auth_http.pb.go
make api-smoke
go test ./api/baboflow/v1
```

Expected: generation succeeds, the generated package compiles, temporary plugin outputs are reproducible, and the second `make api` produces no content diff.

- [ ] **Step 5: Commit**

```bash
git add Makefile api third_party go.mod go.sum
git commit -m "chore(api): 建立 proto 生成链路"
```

## Task 2: 定义完整领域 proto 契约

**Files:**
- Create: `api/baboflow/v1/auth.proto`
- Create: `api/baboflow/v1/llm.proto`
- Create: `api/baboflow/v1/archery.proto`
- Create: `api/baboflow/v1/component.proto`
- Create: `api/baboflow/v1/rulechain.proto`
- Create: `api/baboflow/v1/agent.proto`
- Create: `api/baboflow/v1/skill.proto`
- Create: `api/baboflow/v1/mcp.proto`
- Create: `api/baboflow/v1/board.proto`
- Create: `api/baboflow/v1/audit.proto`
- Create: `api/baboflow/v1/cron.proto`
- Test: `api/baboflow/v1/proto_contract_test.go`

**Interfaces:**
- Each file exports one `XxxService` and generated `RegisterXxxHTTPServer`/`RegisterXxxServer`.
- Every HTTP method uses an explicit `google.api.http` binding under `/api/v1`.
- List methods use `PageRequest`; responses use `PageResponse`-compatible typed fields.

- [ ] **Step 1: Inventory existing handler contracts**

Before writing messages, copy the current request/query/response fields from `internal/service/auth.go`, `llm.go`, `archery.go`, `component.go`, `rulechain.go`, `agent.go`, `skill.go`, `mcp.go`, `board.go`, `audit.go`, and `cron.go`. Record each old path and method in the proto comment immediately above its new method. Do not invent fields that are not present in the current handler or frontend API type.

- [ ] **Step 2: Define the service methods and paths**

Use the approved resource names: `/api/v1/rule-chains`, `/api/v1/rule-chain-runs`, `/api/v1/agent-sessions`, `/api/v1/agent-assets`, `/api/v1/audit-logs`, and `/api/v1/cron-jobs`. Use `google.protobuf.FieldMask` for PUT update methods only where the existing handler supports partial updates; otherwise define the complete request message.

- [ ] **Step 3: Define special JSON operations**

Model validate/import/publish/offline/rollback/run/debug/expose/test/sync/toggle as explicit RPC methods with `POST` bindings and `body: "*"`. Model export as a JSON response. Do not model multipart upload, raw download, WebSocket, SSE, or OAuth redirect methods.

- [ ] **Step 4: Add contract tests**

Test that generated descriptors contain every service and that representative methods resolve to exact paths:

```go
func TestProtoHTTPPaths(t *testing.T) {
    if got := rulechain.File_rulechain_proto.Services().ByName("RuleChainService").
        Methods().ByName("List").Options().(*annotations.HttpRule).GetGet(); got.GetPattern() != "/api/v1/rule-chains" { t.Fatal(got) }
}
```

Add equivalent checks for auth login, Archery connections, agents, skills, boards, audit logs, and cron jobs.

- [ ] **Step 5: Generate and compile**

Run `make api && go test ./api/baboflow/v1`; expected: all generated service interfaces compile and path tests pass.

- [ ] **Step 6: Commit**

```bash
git add api/baboflow/v1
git commit -m "feat(api): 定义业务 proto 契约"
```

## Task 3: 迁移鉴权、错误和限流到 Kratos middleware

**Files:**
- Create: `internal/service/kratos_middleware.go`
- Modify: `internal/service/middleware.go`, `internal/service/ratelimit.go`, `internal/service/auth.go`
- Create: `internal/service/kratos_middleware_test.go`

**Interfaces:**
- `AuthMiddleware(auth *biz.AuthUsecase) middleware.Middleware` reads `baboflow_sid` from HTTP Cookie and `authorization` metadata from gRPC, validates the session through the existing usecase, and stores the existing user ID context key.
- `LoginRateLimitMiddleware` and `TriggerRateLimitMiddleware` implement the Kratos middleware signature and preserve current limits.
- `SetSessionCookie(ctx context.Context, header http.Header, sessionID string, maxAge int)` is transport-independent; it writes `Set-Cookie` with HttpOnly and the current secure policy.

- [ ] **Step 1: Add failing middleware tests**

Cover missing session → `Unauthorized`, invalid session → `Unauthorized`, valid Cookie → user ID in context, valid gRPC Bearer metadata → user ID in context, and login cookie attributes (`HttpOnly`, `Path=/`, max-age).

- [ ] **Step 2: Implement the context and Cookie adapter**

Move only transport-independent session lookup into the new middleware. Keep OAuth state Cookie and redirect behavior in the sidecar handler.

- [ ] **Step 3: Replace Gin-only rate limiting**

Wrap the existing token-bucket implementation with Kratos middleware; preserve login key `login:<client-ip>` and trigger key `trigger:<client-ip>` and their current rates/capacities.

- [ ] **Step 4: Run tests**

Run `go test ./internal/service -run 'Test(KratosMiddleware|SessionCookie|RateLimit)'`; expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service
git commit -m "refactor(auth): 迁移 Kratos 鉴权与限流中间件"
```

## Task 4: 实现 proto service 与 biz 适配层

**Files:**
- Modify/Create: `internal/service/*_proto.go` for all eleven domains
- Modify: corresponding `internal/biz/*.go` only where generated request/response types are required
- Test: `internal/service/kratos_service_test.go`

**Interfaces:**
- Each service constructor accepts the existing usecase/repository dependency and returns a concrete implementation of the generated `XxxServer`.
- Each method validates primitive IDs, required names, pagination bounds, and body presence before calling biz.
- Each method returns generated proto responses and `errors.BadRequest`, `errors.NotFound`, `errors.Unauthorized`, `errors.Conflict`, or `errors.InternalServer` with stable reasons.

- [ ] **Step 1: Migrate Auth and Archery first**

Implement `AuthService` and `ArcheryService` against existing usecases. Preserve password/session behavior, Archery connection fields, instance sync, and connection test semantics. Add tests for login, current user, connection list, and invalid connection ID.

- [ ] **Step 2: Migrate LLM, Component, and Cron**

Implement CRUD/list/test/sync/toggle methods and map existing page response fields to typed proto messages. Test one success and one validation failure per service.

- [ ] **Step 3: Migrate RuleChain and Run operations**

Implement list/get/create/update/delete/import/export/validate/publish/offline/versions/rollback/run/debug and run list/detail. Preserve trigger rate-limit middleware on run/debug.

- [ ] **Step 4: Migrate Agent, Skill, MCP, Board, and Audit**

Implement all structured JSON methods. Keep Agent asset upload/read and Skill package upload/download as sidecar handlers; proto methods must not attempt multipart parsing.

- [ ] **Step 5: Run service tests**

Run `go test ./internal/service ./internal/biz`; expected: all existing tests pass and new service tests cover generated interfaces.

- [ ] **Step 6: Commit**

```bash
git add internal/service internal/biz
git commit -m "refactor(service): 迁移业务接口到 proto service"
```

## Task 5: Replace HTTP routing and add gRPC server

**Files:**
- Create: `internal/server/grpc.go`
- Modify: `internal/server/http.go`, `internal/server/provider.go`
- Modify: `cmd/baboflow/wire.go`, `cmd/baboflow/wire_gen.go`, `cmd/baboflow/main.go`
- Create: `internal/server/grpc_test.go`

**Interfaces:**
- `NewHTTPServer(...) *khttp.Server` registers every generated HTTP server with auth/limit middleware and registers only sidecar handlers through `HandleFunc`/`HandlePrefix`.
- `NewGRPCServer(c *conf.Config, services ...) *kgrpc.Server` registers every generated gRPC service and uses the metadata auth middleware.
- `App` owns both servers and starts/stops both with the existing Kratos lifecycle.

- [ ] **Step 1: Add server registration test**

Construct servers with test doubles and assert HTTP routes exist for one method in every domain and gRPC reflection/registration contains every service. Assert `/ws`, `/mcp/sse`, `/healthz`, and multipart paths remain available.

- [ ] **Step 2: Register proto HTTP services**

Delete the Gin `api.Group` business route block and call all `RegisterXxxHTTPServer` functions. Keep the same `khttp.Server` address and timeout.

- [ ] **Step 3: Rebuild sidecars with standard handlers**

Preserve exact behavior for readiness, metrics, WebSocket, MCP SSE/message, Feishu redirects, SPA fallback, Agent asset upload/read, and Skill package upload/download. Apply Session/MCP auth explicitly where the removed Gin group previously supplied it.

- [ ] **Step 4: Add gRPC construction and lifecycle**

Use `conf.GRPCAddr` already loaded from `GRPC_ADDR`/`config/default.yaml`; register all generated gRPC services and close the server during shutdown.

- [ ] **Step 5: Regenerate Wire and run server tests**

Run `make generate`, then `go test ./internal/server ./cmd/baboflow`; expected: both servers compile and registration tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/server cmd/baboflow
git commit -m "feat(server): 注册 proto HTTP 与 gRPC 服务"
```

## Task 6: Remove the legacy Gin business path

**Files:**
- Delete or rewrite: `internal/server/httputil/resp.go`, `internal/server/httputil/resp_test.go`
- Modify: `go.mod`, `go.sum`
- Modify: all remaining `internal/service/*.go` imports and tests

**Interfaces:**
- No business route may use `gin.Context`, `httputil.OK`, `httputil.Fail`, or the old envelope.
- Gin may remain only if a verified sidecar dependency still requires it; otherwise remove it from `go.mod`.

- [ ] **Step 1: Add a repository guard**

Run `rg 'gin\\.Context|httputil\\.(OK|Fail|OKPage)|api\\.Group|r\\.(GET|POST|PUT|DELETE)' internal/service internal/server` and make the check return no business-route matches. Keep explicit allowlisted sidecar handlers documented in `internal/server/http.go`.

- [ ] **Step 2: Remove legacy response helpers**

Delete only helpers made unused by the migration; retain tests for any still-used standalone response behavior.

- [ ] **Step 3: Tidy and test**

Run `go mod tidy`, `gofmt -w` on changed Go files, then `go test ./...`; expected: no Gin business handler or stale envelope type remains.

- [ ] **Step 4: Commit**

```bash
git add internal/server internal/service go.mod go.sum
git commit -m "refactor(http): 移除 Gin 业务路由与响应信封"
```

## Task 7: Migrate frontend API clients and call sites

**Files:**
- Modify: `web/src/api/http.ts`
- Modify: `web/src/api/auth.ts`, `llm.ts`, `archery.ts`, `component.ts`, `chain.ts`, `run.ts`, `agent.ts`, `skill.ts`, `mcp.ts`, `board.ts`, `audit.ts`, `cron.ts`
- Modify: every page/hook importing old resource paths or envelope types
- Test: existing frontend test/build configuration

**Interfaces:**
- Axios success interceptor returns `resp.data` directly.
- Error interceptor reads HTTP status and Kratos error JSON (`message`, `reason`, or `code`) and redirects status 401 to `/login`.
- API methods use the approved new paths; uploads/downloads use sidecar paths and preserve `responseType`/multipart headers.

- [ ] **Step 1: Update the HTTP adapter**

Remove `Envelope`-based success detection. Keep `withCredentials: true`, timeout, and 401 redirect. Add a typed `KratosErrorBody` and use the first non-empty `message`, `reason`, then Axios message.

- [ ] **Step 2: Update each domain client**

Replace old `/chains`, `/runs`, `/audit`, and `/cron` paths and all other paths according to the proto descriptors. Keep TypeScript field names camelCase and map only where the backend proto JSON contract requires it.

- [ ] **Step 3: Update upload/download callers**

Use `FormData` for Agent assets and Skill packages, and keep raw asset/package response handling. Do not alter WebSocket frame format or `/ws` URL.

- [ ] **Step 4: Build the frontend**

Run from `web/`:

```bash
pnpm exec tsc --noEmit
pnpm build
```

Expected: no stale `Envelope` assumptions, old resource paths, or type errors.

- [ ] **Step 5: Commit**

```bash
git add web/src
git commit -m "refactor(web): 适配 proto API 与原生错误响应"
```

## Task 8: End-to-end verification and documentation

**Files:**
- Modify: `docs/backend-design.md`, `docs/deployment.md`, `.env.example`, `README.md`
- Create: `internal/server/http_integration_test.go`
- Create: `docs/superpowers/plans/2026-08-05-kratos-proto-http-migration-verification.md` only if the verification output is too large for the test report

**Interfaces:**
- Document `HTTP_ADDR`, `GRPC_ADDR`, generated API commands, new paths, native error response, and sidecar endpoints.

- [ ] **Step 1: Add HTTP integration checks**

With an in-memory/test DB and service doubles, assert login sets `baboflow_sid`, an authenticated proto route succeeds, unauthenticated protected routes return 401, invalid JSON returns 400, and sidecars still return expected content types.

- [ ] **Step 2: Run the complete verification matrix**

Run:

```bash
make api
make api-smoke
make generate
make build
make test
cd web && pnpm exec tsc --noEmit && pnpm build
```

Expected: all commands exit 0; generated files are unchanged after a second `make api`; no old business route appears in the repository guard.

- [ ] **Step 3: Update operations docs**

Document that HTTP remains on `:8000`, gRPC defaults to `:9000`, gRPC should be firewalled from public traffic, and MCP/WebSocket/OAuth/static/multipart paths are HTTP sidecars.

- [ ] **Step 4: Commit**

```bash
git add docs .env.example README.md internal/server/http_integration_test.go
git commit -m "docs: 补充 proto API 与双协议部署说明"
```

## Self-Review

- Spec coverage: Tasks 1–2 cover proto and generation; Task 3 covers Cookie/metadata auth and limits; Tasks 4–6 cover service, server registration, sidecars, and Gin removal; Task 7 covers all frontend clients and uploads; Task 8 covers acceptance and deployment documentation.
- Placeholder scan: no `TBD`, `TODO`, or unspecified “appropriate handling” step is used; each task names files, interfaces, commands, and expected results.
- Type consistency: `PageRequest`, generated `XxxServer`, `AuthMiddleware`, `NewHTTPServer`, and `NewGRPCServer` are introduced before their consumers; existing `GRPCAddr` is treated as already present.
