package po

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gorm.io/datatypes"
)

// 回归：LLM 模型列表接口直接序列化 po.LLMModel，前端契约是 camelCase
// （id/providerId/model/alias/maxTokens/isDefault/...）。缺失 json tag 会导致
// 前端表格列「模型/别名」等显示为空。
func TestLLMModelJSONCasing(t *testing.T) {
	m := LLMModel{
		ID: 7, TenantID: 0, ProviderID: 1, Model: "kimi-for-coding", Alias: "K2 编码",
		Temperature: 0.7, MaxTokens: 4096, IsDefault: true,
		Capability: datatypes.JSON([]byte(`{"chat":true}`)), Enabled: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(raw)

	// 必须出现 camelCase 键
	for _, k := range []string{`"id"`, `"providerId"`, `"model"`, `"alias"`, `"maxTokens"`, `"isDefault"`, `"capability"`, `"enabled"`, `"createdAt"`, `"updatedAt"`} {
		if !strings.Contains(s, k) {
			t.Errorf("expected camelCase key %s in %s", k, s)
		}
	}
	// 不应出现 PascalCase 键（说明 json tag 生效）
	for _, k := range []string{`"ID"`, `"ProviderID"`, `"Model"`, `"Alias"`, `"MaxTokens"`, `"IsDefault"`} {
		if strings.Contains(s, k+":") {
			t.Errorf("unexpected PascalCase key %s in %s", k, s)
		}
	}
	// 值要正确解回
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back["model"] != "kimi-for-coding" || back["alias"] != "K2 编码" {
		t.Fatalf("values mismatch: %v", back)
	}
	if back["isDefault"] != true {
		t.Fatalf("isDefault mismatch: %v", back)
	}
}
