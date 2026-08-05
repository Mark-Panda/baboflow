# baboflow

## API 与本地运行

- HTTP 默认由 `HTTP_ADDR=:8000` 监听，承载前端、proto HTTP、WebSocket、MCP 与 HTTP 旁路。
- gRPC 默认由 `GRPC_ADDR=127.0.0.1:9000` 监听。不要将 gRPC 直接暴露到公网；仅通过受控反向代理或专用网络访问。
- 核心业务 proto 位于 `api/baboflow/v1/`，修改后运行：

```bash
make api
make generate
```

proto HTTP 成功响应使用 protobuf JSON，失败使用 Kratos 原生 JSON 错误体（按 HTTP status 和 `reason`/`message` 处理），不使用旧 `{code,message,data}` 业务信封。`int64`/`uint64` 字段会以十进制字符串返回，客户端不能将其直接当作 JavaScript `number`。

`/healthz`、`/readyz`、`/metrics`、`/ws`、`/mcp/sse`、`/mcp/message`、飞书 OAuth、SPA 静态资源及 multipart/raw 文件端点保留为 HTTP 旁路。资产与技能包旁路为：

- `POST /api/v1/agent-assets`、`GET /api/v1/agent-assets/{assetId}`
- `POST /api/v1/skills/package`、`GET /api/v1/skills/{id}/package`

旁路文件端点需要 `baboflow_sid`；MCP 端点需要已登录会话或 `MCP_AUTH_TOKEN` Bearer。
