## 1. Archery connection 默认配置

- [x] 1.1 为 `ArcheryConnection` 增加租户级默认标记，并补充迁移/初始化兼容逻辑
- [x] 1.2 在 biz/data/service 层实现设置和取消默认 connection，保证同一租户最多一个默认项
- [x] 1.3 为默认 connection 的设置、取消、未配置和跨租户场景编写失败测试并运行

## 2. Archery 节点能力

- [x] 2.1 为 `archerySchema` 增加 `instances` resource，并使用租户默认 connection 查询实例
- [x] 2.2 实现 `metadata.instanceId`、`msg.instanceId`、节点默认值的动态解析和实例归属校验
- [x] 2.3 在节点测试中覆盖五种资源/动作、参数缺失、未知 action 和非法实例
- [x] 2.4 确认 `archeryQuery` 保持只读 SELECT、多语句和写操作拒绝，并补充回归测试

## 3. 规则链与 MCP 暴露

- [x] 3.1 创建五种 action 的规则链 DSL，包含实例列表、数据库、表、表结构、查询和失败分支
- [x] 3.2 为规则链配置 MCP input schema，声明 action 与各自必填字段
- [x] 3.3 补充 DSL 校验和 MCP 执行测试，确认未知 action 或缺参不会调用 Archery

## 4. 验证与交付

- [x] 4.1 运行相关 Go 单元测试并修复失败项
- [x] 4.2 运行 `go build ./...` 和全量 `go test ./...`
- [x] 4.3 检查修改文件 lint，并逐条核对 spec 场景覆盖
