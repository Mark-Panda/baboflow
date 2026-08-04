# Agent 记忆模块问题修复实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 修复 Agent 记忆删除、历史回退、Provider 生命周期和优雅退出问题，确保记忆功能失败时对话仍可用。

**Architecture:** 业务会话仍以 `agent_message` 为主数据源；记忆库作为增强上下文和异步长期记忆存储。删除会话时只清理该会话的记忆消息、摘要和事件，保留用户级长期记忆。Provider 通过运行中引用保护，避免配置失效时关闭仍被请求使用的实例。

**Tech Stack:** Go、GORM、PostgreSQL、Cloudwego Eino ADK、aggo builtin memory、现有 Go testing。

## Global Constraints

- 不引入新的第三方依赖。
- 不修改用户级长期记忆的语义；删除单个会话不删除用户长期记忆。
- 记忆检索失败时必须能够回退到业务库历史消息。
- 所有异步记忆 worker 必须在应用 cleanup 阶段停止。
- 不提交 Git commit，除非用户后续明确要求。

---

### Task 1: 为记忆存储增加会话级清理能力

**Files:**
- Modify: `internal/memorystore/storage.go`
- Test: `internal/memorystore/storage_test.go`

**Interfaces:**
- Produces `DeleteSessionData(ctx context.Context, userID, sessionID string) error`。
- 方法删除 `aggo_mem_conversation_messages`、`aggo_mem_session_summaries` 和 `aggo_mem_user_memory_events` 中属于指定用户和会话的数据。
- 不删除 `aggo_mem_user_memories`。

- [ ] **Step 1: 编写清理行为测试**

使用 GORM SQLite 测试库或仓库已有测试数据库方式，创建三类记忆表，插入目标用户/会话、目标用户/其他会话、其他用户数据，调用 `DeleteSessionData`，断言只有目标会话数据被删除，用户长期记忆仍存在。

- [ ] **Step 2: 运行测试确认失败**

运行：

```bash
go test ./internal/memorystore -run TestPostgresStorageDeleteSessionData -count=1
```

预期：因方法尚未实现而失败。

- [ ] **Step 3: 实现清理方法**

在 `PostgresStorage` 上使用同一事务，分别按 `user_id` 和 `session_id` 删除消息、摘要；事件表没有 `session_id` 字段，因此本次不删除事件，避免错误删除用户级事件。若事件确实需要会话级清理，先通过模型和数据流补充会话归属字段后再处理。

- [ ] **Step 4: 运行测试确认通过**

运行：

```bash
go test ./internal/memorystore -run TestPostgresStorageDeleteSessionData -count=1
```

预期：PASS。

### Task 2: 将记忆清理接入删除会话流程

**Files:**
- Modify: `internal/biz/agent.go`
- Test: `internal/biz/agent_test.go` 或现有 Agent usecase 测试文件

**Interfaces:**
- 扩展 `AgentUsecase`，注入可选的会话记忆清理器。
- 删除会话时先校验会话归属，再删除业务会话和记忆会话数据。
- 记忆清理失败必须返回错误，避免向用户报告“删除成功”但残留会话记忆。

- [ ] **Step 1: 为记忆清理器定义最小接口和 fake**

定义仅包含：

```go
type SessionMemoryCleaner interface {
    DeleteSessionData(context.Context, string, string) error
}
```

测试 fake 记录收到的 `userID` 和 `sessionID`。

- [ ] **Step 2: 编写 DeleteSession 测试**

覆盖：

1. 正常删除时调用清理器；
2. 业务删除失败时不调用清理器；
3. 记忆清理失败时返回错误。

- [ ] **Step 3: 实现依赖注入和删除流程**

保持现有构造函数兼容，使用 setter 或可选依赖注入；生产组装时注入 `PostgresStorage`，测试未注入时保持原有行为。

- [ ] **Step 4: 运行 Agent 领域测试**

```bash
go test ./internal/biz -run 'Test.*Session' -count=1
```

预期：PASS。

### Task 3: 为 Chat 增加业务历史回退

**Files:**
- Modify: `internal/biz/agent.go`
- Modify: `internal/biz/agentkit/runner.go`（仅在需要统一历史转换时）
- Test: `internal/biz/agent_test.go`

**Interfaces:**
- 在 `Chat` 中读取当前会话最近消息，并转换为 `[]*schema.AgenticMessage`。
- 记忆启用时让记忆中间件继续工作；不把业务历史和记忆历史无条件重复拼接。
- 当记忆未启用或 Provider 检索失败时，业务历史仍作为 Agent 输入。

- [ ] **Step 1: 编写历史转换测试**

验证业务库中的 user/assistant 消息可以按时间升序转换为 Eino `AgenticMessage`，工具调用字段不会被误当成普通用户文本。

- [ ] **Step 2: 编写 Chat 回退测试**

使用 fake Agent 或可记录输入消息的 runner，模拟记忆未启用，断言第二轮 Chat 收到上一轮业务历史。

- [ ] **Step 3: 实现最小回退逻辑**

在 `Chat` 获取会话后读取业务消息，转换后传给 `agentkit.Run`。将读取失败作为 Chat 错误返回；不要静默运行一个缺少上下文的请求。

- [ ] **Step 4: 运行测试**

```bash
go test ./internal/biz -run 'Test.*Chat|Test.*History' -count=1
```

预期：PASS。

### Task 4: 修复 Provider 失效和运行中请求的生命周期

**Files:**
- Modify: `internal/biz/agentkit/manager.go`
- Modify: `internal/biz/agent.go`
- Test: `internal/biz/agentkit/manager_test.go`

**Interfaces:**
- Provider 不能仅由缓存 Agent 数量决定关闭时机。
- 为一次 Agent 运行提供 acquire/release 运行引用，运行结束后再允许关闭旧 Provider。
- `Invalidate` 只标记旧 Provider 待关闭；没有运行引用时立即关闭。
- `SetMemoryDB` 配置变更时使旧 Agent/Provider 失效。

- [ ] **Step 1: 编写并发失效测试**

创建一个阻塞中的 fake Provider，启动 Agent 运行后调用 `Invalidate`，断言 Provider 在运行结束前没有调用 `Close`，运行结束后才关闭。

- [ ] **Step 2: 编写配置更新测试**

先用配置 A 创建 Provider，再调用 `SetMemoryDB` 注入配置 B，断言下次 `Get` 不复用旧 Provider。

- [ ] **Step 3: 实现运行引用和延迟关闭**

在 Manager 内维护 provider 的 cache reference 与 active run reference；为 AgentUsecase 或 `agentkit.Run` 增加生命周期包装，确保所有路径（成功、错误、context cancel）都执行 release。

- [ ] **Step 4: 运行竞态测试**

```bash
go test -race ./internal/biz/agentkit -run 'TestManager|Test.*Memory' -count=1
```

预期：PASS，且无 race。

### Task 5: 接入优雅退出并补齐验证

**Files:**
- Modify: `cmd/baboflow/wire_gen.go`
- Modify: `cmd/baboflow/main.go`（仅当 cleanup 入口需要调整）
- Modify: `internal/biz/agentkit/memory_integration_test.go`

- [ ] **Step 1: 将 `agentManager.Close` 接入 cleanup**

确保 tracer、Agent Manager、数据库资源的关闭顺序明确，Provider worker 在共享数据库关闭前停止。

- [ ] **Step 2: 增加集成测试**

覆盖：

1. Provider `Memorize` 后可检索；
2. 记忆 Provider 关闭后 worker 停止；
3. 记忆未启用时 Chat 仍使用业务历史；
4. 删除会话后不会召回该会话摘要和消息。

- [ ] **Step 3: 执行完整验证**

```bash
go test ./internal/memorystore ./internal/biz ./internal/biz/agentkit ./internal/conf
go test -race ./internal/biz/agentkit
go vet ./internal/memorystore ./internal/biz ./internal/biz/agentkit ./cmd/baboflow
```

预期：全部通过，无新增 vet 或 race 问题。
