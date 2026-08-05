package data

import (
	"encoding/json"
	"strings"
	"testing"

	"gorm.io/datatypes"

	"baboflow/internal/biz/rulegokit"
	_ "baboflow/internal/biz/rulegokit/nodes"
	"baboflow/internal/conf"
	"baboflow/internal/data/po"
)

// 验证规则链生成器(agent-chain-builder)的 seed：
// 首次创建即写入新指令与精简工具集；对已有库再次执行时修正旧指令/工具集。
func TestSeed_ChainBuilderAgent(t *testing.T) {
	db := newTestDB(t, &po.AdminUser{}, &po.Agent{}, &po.RuleChain{}, &po.McpExposure{})
	cfg := &conf.Config{AdminInitPassword: "x"}

	if err := Seed(db, cfg); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var a po.Agent
	if err := db.Where(`"key" = ?`, "agent-chain-builder").First(&a).Error; err != nil {
		t.Fatalf("query agent: %v", err)
	}
	if !strings.Contains(a.Instruction, "apply_chain_dsl") {
		t.Fatalf("instruction 未包含 apply_chain_dsl: %q", a.Instruction)
	}
	if !strings.Contains(a.Instruction, "不要调用 rulechain_create") {
		t.Fatalf("instruction 未明确禁止 rulechain_create: %q", a.Instruction)
	}
	if got := strings.Join(decodeStrs(a.BuiltinTools), ","); strings.Contains(got, "write") || strings.Contains(got, "bash") {
		t.Fatalf("内置工具集未精简: %v", got)
	}

	// 模拟历史库：把指令/工具集改回旧版，再跑 Seed 应被修正回新版。
	oldInstr := "你是规则链生成器。用 ReAct 模式：... rulechain_validate/rulechain_create。"
	if err := db.Model(&po.Agent{}).Where(`"key" = ?`, "agent-chain-builder").
		Updates(map[string]any{
			"instruction":   oldInstr,
			"builtin_tools": datatypes.JSON([]byte(`["bash","read","write","edit","grep"]`)),
		}).Error; err != nil {
		t.Fatalf("downgrade: %v", err)
	}
	if err := Seed(db, cfg); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	var a2 po.Agent
	if err := db.Where(`"key" = ?`, "agent-chain-builder").First(&a2).Error; err != nil {
		t.Fatalf("requery: %v", err)
	}
	if !strings.Contains(a2.Instruction, "apply_chain_dsl") {
		t.Fatalf("re-seed 未修正指令: %q", a2.Instruction)
	}
	if got := strings.Join(decodeStrs(a2.BuiltinTools), ","); strings.Contains(got, "write") {
		t.Fatalf("re-seed 未修正工具集: %v", got)
	}
	var chain po.RuleChain
	if err := db.Where("id = ?", "chain-archery-mcp-query").First(&chain).Error; err != nil {
		t.Fatalf("query Archery MCP chain: %v", err)
	}
	if !json.Valid(chain.DSL) || !json.Valid(chain.InputSchema) {
		t.Fatal("Archery MCP chain DSL/input schema must be valid JSON")
	}
	if !strings.Contains(string(chain.DSL), `"resource":"instances"`) {
		t.Fatal("Archery MCP chain missing instances action")
	}
	if err := rulegokit.Validate(chain.DSL); err != nil {
		t.Fatalf("Archery MCP chain DSL should validate: %v", err)
	}
	if chain.Status != "published" {
		t.Fatalf("Archery MCP chain status = %q, want published", chain.Status)
	}
	var exposure po.McpExposure
	if err := db.Where("tool_name = ?", "archery_mcp_query").First(&exposure).Error; err != nil {
		t.Fatalf("query Archery MCP exposure: %v", err)
	}
	if exposure.ChainID != chain.ID || !exposure.Enabled {
		t.Fatalf("unexpected Archery MCP exposure: %+v", exposure)
	}
	var exposureCount int64
	if err := db.Model(&po.McpExposure{}).Where("tool_name = ?", "archery_mcp_query").Count(&exposureCount).Error; err != nil {
		t.Fatalf("count Archery MCP exposures: %v", err)
	}
	if exposureCount != 1 {
		t.Fatalf("Archery MCP exposure count = %d, want 1", exposureCount)
	}
}

func decodeStrs(j datatypes.JSON) []string {
	var out []string
	_ = json.Unmarshal(j, &out)
	return out
}
