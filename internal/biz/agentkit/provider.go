// Package agentkit 封装 aggo/eino：按 DB 中的 Agent 配置构建可对话的 ReAct Agent。
package agentkit

import (
	"fmt"

	aggomodel "github.com/CoolBanHub/aggo/model"
	"github.com/cloudwego/eino/components/model"

	"baboflow/internal/conf"
	"baboflow/internal/data/po"
)

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
