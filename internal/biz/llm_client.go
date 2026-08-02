package biz

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"baboflow/internal/conf"
	"baboflow/internal/data/po"

	"github.com/go-kratos/kratos/v2/log"
)

// llmClient 负责与 OpenAI 兼容接入点交互：测试连接、拉取模型列表。
type llmClient struct {
	http   *http.Client
	logger *log.Helper
}

// ErrInvalidBaseURL 表示接入点 baseUrl 未通过校验（客户端输入错误，应映射为 400）。
var ErrInvalidBaseURL = errors.New("baseUrl 非法")

func newLLMClient() *llmClient {
	return &llmClient{
		http:   &http.Client{Timeout: 20 * time.Second},
		logger: log.NewHelper(log.DefaultLogger),
	}
}

func normalizeBase(base string) string {
	return strings.TrimRight(strings.TrimSpace(base), "/")
}

// validateProviderBaseURL 校验接入点 baseUrl 的基本合法性，收敛 SSRF 面：
// 仅允许 http/https 绝对地址、必须是有效 host、不允许内嵌 userinfo（防 `http://@internal/`）。
// 说明：本平台 admin 可合法指向内网/回环的私有 LLM 网关，故不强制拒绝私网 IP。
func validateProviderBaseURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("%w: 不能为空", ErrInvalidBaseURL)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidBaseURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: 仅支持 http/https，当前 %q", ErrInvalidBaseURL, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("%w: 缺少主机", ErrInvalidBaseURL)
	}
	if u.User != nil {
		return fmt.Errorf("%w: 不允许包含用户信息", ErrInvalidBaseURL)
	}
	return nil
}

// FetchModels 调 {baseUrl}/models 拉取可用模型 id 列表。
func (c *llmClient) FetchModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	if err := validateProviderBaseURL(baseURL); err != nil {
		return nil, err
	}
	url := normalizeBase(baseURL) + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		// 上游响应体可能含内网信息，仅记录到服务端日志，不回显给客户端。
		c.logger.Warnf("FetchModels upstream %s returned %d: %s", normalizeBase(baseURL), resp.StatusCode, truncate(string(body), 200))
		return nil, fmt.Errorf("上游返回状态 %d", resp.StatusCode)
	}
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse models: %w", err)
	}
	ids := make([]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
}

// TestModel 用最小 chat 请求探测接入点+模型可用性，返回延迟毫秒。
func (c *llmClient) TestModel(ctx context.Context, baseURL, apiKey, model string) (int64, error) {
	if err := validateProviderBaseURL(baseURL); err != nil {
		return 0, err
	}
	url := normalizeBase(baseURL) + "/chat/completions"
	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": "ping"},
		},
		"max_tokens": 1,
		"stream":     false,
	}
	raw, _ := json.Marshal(payload)
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	latency := time.Since(start).Milliseconds()
	if resp.StatusCode != http.StatusOK {
		c.logger.Warnf("TestModel upstream %s returned %d: %s", normalizeBase(baseURL), resp.StatusCode, truncate(string(body), 200))
		return latency, fmt.Errorf("上游返回状态 %d", resp.StatusCode)
	}
	return latency, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ---- Usecase 侧封装 ----

// ProviderModels 拉取接入点可用模型（解密 apiKey）。
func (uc *LLMUsecase) ProviderModels(ctx context.Context, providerID int64) ([]string, error) {
	p, err := uc.repo.GetProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	key, err := conf.Decrypt(uc.secret, p.APIKeyEnc)
	if err != nil {
		return nil, err
	}
	return newLLMClient().FetchModels(ctx, p.BaseURL, key)
}

type TestResult struct {
	OK        bool   `json:"ok"`
	LatencyMs int64  `json:"latencyMs"`
	Message   string `json:"message"`
}

// TestProvider 用该接入点默认/首个模型测试连通性。
func (uc *LLMUsecase) TestProvider(ctx context.Context, providerID int64) (*TestResult, error) {
	p, err := uc.repo.GetProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	key, err := conf.Decrypt(uc.secret, p.APIKeyEnc)
	if err != nil {
		return nil, err
	}
	model, err := uc.pickTestModel(ctx, p)
	if err != nil {
		return &TestResult{OK: false, Message: err.Error()}, nil
	}
	lat, err := newLLMClient().TestModel(ctx, p.BaseURL, key, model)
	if err != nil {
		return &TestResult{OK: false, LatencyMs: lat, Message: err.Error()}, nil
	}
	return &TestResult{OK: true, LatencyMs: lat, Message: "连接成功"}, nil
}

// TestModel 测试单个模型。
func (uc *LLMUsecase) TestModel(ctx context.Context, modelID int64) (*TestResult, error) {
	m, err := uc.repo.GetModel(ctx, modelID)
	if err != nil {
		return nil, err
	}
	p, err := uc.repo.GetProvider(ctx, m.ProviderID)
	if err != nil {
		return nil, err
	}
	key, err := conf.Decrypt(uc.secret, p.APIKeyEnc)
	if err != nil {
		return nil, err
	}
	lat, err := newLLMClient().TestModel(ctx, p.BaseURL, key, m.Model)
	if err != nil {
		return &TestResult{OK: false, LatencyMs: lat, Message: err.Error()}, nil
	}
	return &TestResult{OK: true, LatencyMs: lat, Message: "连接成功"}, nil
}

func (uc *LLMUsecase) pickTestModel(ctx context.Context, p *po.LLMProvider) (string, error) {
	models, err := uc.repo.ListModels(ctx, p.ID)
	if err != nil || len(models) == 0 {
		return "", errors.New("该接入点下未登记模型")
	}
	for _, m := range models {
		if m.IsDefault {
			return m.Model, nil
		}
	}
	return models[0].Model, nil
}
