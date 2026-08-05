# Kratos Proto HTTP/gRPC 迁移验证报告

## Task 6 收尾验证

验证日期：2026-08-05

- SPA/API fallback 404 已移除旧 `{code,message}` 信封，改为 `message` 与 `error` 错误体。
- `Makefile guard` 通过 `scripts/guard_legacy_http.py` 扫描 `internal/server` 与 `internal/service` 的生产 Go 源码，不依赖 Gin receiver 名称。
- `guard` 覆盖 `.GET/.POST/.PUT/.DELETE/.PATCH/.Any/.Handle/.Group/.Static/...` 调用形态，仅允许 `internal/server/http.go` 中按文件、方法和路径精确列出的旁路路由。
- 已移除 `skills/:id/files` 与 `skills/:id/file` 的重复 Gin JSON 路由和对应 Gin handler；这两个接口仅由 proto HTTP 注册。
- `guard` 同时按函数白名单检查 `gin.Context`，并检查旧 `httputil`、`response/envelope` 类型及 `gin.H{"code": ...}` 信封。
- 未创建 git commit。

## 测试结果

- `make guard`（第一次）：通过，输出 `guard: repository boundaries verified`。
- 临时写入 `internal/server/zz_guard_receiver_probe.go`，内容包含 `engine.GET("/api/v1/legacy", handler)` 形态：`make guard` 按预期失败并报告未批准路由；临时文件已通过 `trap` 删除。
- `gofmt -w internal/server/http.go internal/service/skill.go`：通过。
- `make guard`（第二次）：通过，输出 `guard: repository boundaries verified`。
- `go test ./internal/server ./internal/service`：通过。
- `go test ./...`：通过。

## Concerns

无。
