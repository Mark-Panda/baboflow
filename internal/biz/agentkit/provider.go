// Package agentkit 封装 aggo/eino：按 DB 中的 Agent 配置构建可对话的 ReAct Agent。
package agentkit

import (
	"fmt"

	aggomodel "github.com/CoolBanHub/aggo/model"
	"github.com/cloudwego/eino/components/model"
	"github.com/google/wire"

	"baboflow/internal/conf"
	"baboflow/internal/data/po"
)

// ProviderSet agentkit 层依赖（Agent ReAct 运行时）。
var ProviderSet = wire.NewSet(
	NewModelFactory,
	NewMcpClientBuilder,
	NewManager,
	ProvideBuiltinTools,
	ProvideTracer,
)

// ProvideBuiltinTools 内置工具（沙箱目录 + bash 白名单来自配置）。
func ProvideBuiltinTools(c *conf.Config) *BuiltinTools {
	return NewBuiltinTools(c.Workspace, c.BashAllowlist)
}

// ProvideTracer Langfuse 追踪。未配置时 NewTracer 返回 nil Tracer（下游已 nil 守卫）；
// 返回的 func() 是清理钩子，由 wire 并入 injector 的 cleanup。
func ProvideTracer(c *conf.Config) (*Tracer, func(), error) {
	return NewTracer(c.LangfuseHost, c.LangfusePublicKey, c.LangfuseSecretKey)
}

// ModelFactory 由 llm_model + llm_provider 构造 OpenAI 兼容 ChatModel。
type ModelFactory struct {
	secret string
}

func NewModelFactory(c *conf.Config) *ModelFactory {
	return &ModelFactory{secret: c.Secret}
}

// Build 用 provider 的 baseUrl/解密 apiKey + model 的参数构造 ChatModel。
func (f *ModelFactory) Build(provider *po.LLMProvider, m *po.LLMModel) (model.AgenticModel, error) {
	apiKey, err := conf.Decrypt(f.secret, provider.APIKeyEnc)
	if err != nil {
		return nil, fmt.Errorf("解密 apiKey 失败: %w", err)
	}
	opts := []aggomodel.OptionFunc{
		aggomodel.WithModel(m.Model),
		aggomodel.WithBaseUrl(provider.BaseURL),
		aggomodel.WithAPIKey(apiKey),
	}
	if m.MaxTokens > 0 {
		opts = append(opts, aggomodel.WithMaxTokens(m.MaxTokens))
	}
	cm, err := aggomodel.NewChatModel(opts...)
	if err != nil {
		return nil, fmt.Errorf("构造 ChatModel 失败: %w", err)
	}
	return cm, nil
}
