package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"baboflow/internal/biz/rulegokit/archeryclient"

	"github.com/rulego/rulego/api/types"
)

// ClientFactory 抽象"按 archery_instance ID 取其实例+所属连接的解密凭据并构造 HTTP 客户端"。
// 与 AgentRunner 同一 DI 模式，进程启动时经 SetArcheryClientFactory 注入
// （实现即 biz.ArcheryUsecase.NewClientForInstance），便于单测替换、避免 nodes 反向依赖 biz。
type ClientFactory func(ctx context.Context, instanceID int64) (*archeryclient.Client, error)

var (
	clientFactory   ClientFactory
	clientFactoryMu sync.RWMutex
)

// SetArcheryClientFactory 注入客户端工厂（在装配处构造 ArcheryUsecase 后调用）。
// 未注入时 archery 节点执行会返回明确错误，避免 nil 崩溃。
func SetArcheryClientFactory(f ClientFactory) {
	clientFactoryMu.Lock()
	defer clientFactoryMu.Unlock()
	clientFactory = f
}

// getClient 取注入的工厂并构造客户端；未注入返回错误。
func getClient(ctx context.Context, connectionID int64) (*archeryclient.Client, error) {
	clientFactoryMu.RLock()
	f := clientFactory
	clientFactoryMu.RUnlock()
	if f == nil {
		return nil, errors.New("archery 客户端工厂未初始化（SetArcheryClientFactory 未调用）")
	}
	if connectionID <= 0 {
		return nil, fmt.Errorf("archery 节点 connectionId 非法: %d", connectionID)
	}
	return f(ctx, connectionID)
}

// msgParam 读取一个入参：优先消息元数据，其次 JSON 消息体的同名字段，最后回退到节点默认值。
// MCP 暴露规则链时，runChainWithTrigger 把工具入参 JSON 作为消息 data，
// 因此 AI 传入的 sql/dbName 等会出现在消息体里，从而覆盖节点默认值。
func msgParam(msg types.RuleMsg, key, def string) string {
	if v := strings.TrimSpace(msg.Metadata.GetValue(key)); v != "" {
		return v
	}
	data := strings.TrimSpace(msg.GetData())
	if data != "" && json.Valid([]byte(data)) {
		var m map[string]any
		if json.Unmarshal([]byte(data), &m) == nil {
			if s, ok := m[key].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return def
}

// msgSQL 取查询 SQL：优先元数据/JSON 体的 sql 字段；都没有则用消息体原文（裸 SQL），再回退到节点默认。
func msgSQL(msg types.RuleMsg, def string) string {
	if v := strings.TrimSpace(msg.Metadata.GetValue("sql")); v != "" {
		return v
	}
	data := strings.TrimSpace(msg.GetData())
	if data != "" {
		if json.Valid([]byte(data)) {
			var m map[string]any
			if json.Unmarshal([]byte(data), &m) == nil {
				if s, ok := m["sql"].(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		} else {
			// 非 JSON：消息体本身就是 SQL。
			return data
		}
	}
	return def
}

// writeJSON 把结果序列化为 JSON 写回消息体。
func writeJSON(msg types.RuleMsg, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	msg.SetData(string(b))
	return nil
}
