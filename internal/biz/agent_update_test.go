package biz

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"gorm.io/datatypes"

	"baboflow/internal/biz/agentkit"
	"baboflow/internal/data/po"
)

// fakeAgentRepo 仅实现 Update 依赖的方法，捕获 UpdateAgent 落库结果。
type fakeAgentRepo struct {
	AgentDataRepo // 未实现的方法 panics（本测试不触达）
	agent         *po.Agent
	updated       *po.Agent
	subAgentIDs   []int64
}

type sessionDeleteRepo struct {
	AgentDataRepo
	session *po.AgentSession
	deleted bool
}

func (r *sessionDeleteRepo) GetSession(context.Context, string) (*po.AgentSession, error) {
	return r.session, nil
}

func (r *sessionDeleteRepo) DeleteSession(context.Context, string) error {
	r.deleted = true
	return nil
}

type failingSessionMemoryCleaner struct{}

func (failingSessionMemoryCleaner) DeleteSessionData(context.Context, string, string) error {
	return errors.New("memory cleanup failed")
}

func (r *fakeAgentRepo) GetAgentByKey(_ context.Context, key string) (*po.Agent, error) {
	if r.agent == nil || r.agent.Key != key {
		return nil, errors.New("not found")
	}
	return r.agent, nil
}

func (r *fakeAgentRepo) UpdateAgent(_ context.Context, a *po.Agent) error {
	cp := *a
	r.updated = &cp
	return nil
}

func (r *fakeAgentRepo) SetSubAgents(_ context.Context, _ int64, childIDs []int64) error {
	r.subAgentIDs = childIDs
	return nil
}

func newTestAgentUsecase(repo AgentDataRepo) *AgentUsecase {
	return NewAgentUsecase(repo, agentkit.NewManager(nil, nil, nil, nil, nil), nil, nil, nil)
}

func TestDeleteSessionKeepsBusinessDataWhenMemoryCleanupFails(t *testing.T) {
	repo := &sessionDeleteRepo{
		session: &po.AgentSession{ID: "session-1", UserID: ptrInt64(42)},
	}
	uc := newTestAgentUsecase(repo)
	uc.SetSessionMemoryCleaner(failingSessionMemoryCleaner{})

	if err := uc.DeleteSession(context.Background(), "session-1", 42); err == nil {
		t.Fatal("expected memory cleanup error")
	}
	if repo.deleted {
		t.Fatal("business session must remain when memory cleanup fails")
	}
}

func ptrInt64(v int64) *int64 { return &v }

func TestAssetMustBelongToCurrentSession(t *testing.T) {
	asset := &po.Asset{SessionID: "session-a"}
	if allowAssetForSession(asset, "session-b") {
		t.Fatal("asset from another session must not be accepted")
	}
	if !allowAssetForSession(asset, "session-a") {
		t.Fatal("asset from current session should be accepted")
	}
}

// 内置 Agent：仅技能/MCP/子Agent（及启用）可改，核心定义被锁定。
func TestUpdate_BuiltinLocksCoreFields(t *testing.T) {
	builtin := &po.Agent{
		ID:           1,
		Key:          "writer",
		Name:         "内置写作",
		Instruction:  "原始指令",
		IsBuiltin:    true,
		BuiltinTools: datatypes.JSON([]byte(`["read","grep"]`)),
		SkillIDs:     datatypes.JSON([]byte(`[]`)),
		McpIDs:       datatypes.JSON([]byte(`[]`)),
	}
	repo := &fakeAgentRepo{agent: builtin}
	uc := newTestAgentUsecase(repo)

	in := &AgentInput{
		Name:         "被篡改的名字",
		Instruction:  "被篡改的指令",
		BuiltinTools: []string{"bash"}, // 试图扩权
		SkillIDs:     []int64{7, 8},
		McpIDs:       []int64{3},
		SubAgentIDs:  []int64{11},
	}
	if err := uc.Update(context.Background(), "writer", in); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}

	u := repo.updated
	if u == nil {
		t.Fatal("UpdateAgent 未被调用")
	}
	if u.Name != "内置写作" {
		t.Errorf("内置 Name 被改动: %q", u.Name)
	}
	if u.Instruction != "原始指令" {
		t.Errorf("内置 Instruction 被改动: %q", u.Instruction)
	}
	if got := decodeStrings(u.BuiltinTools); len(got) != 2 || got[0] != "read" || got[1] != "grep" {
		t.Errorf("内置 BuiltinTools 被改动: %v", got)
	}
	if got := decodeIDs(u.SkillIDs); len(got) != 2 || got[0] != 7 || got[1] != 8 {
		t.Errorf("SkillIDs 应更新为 [7 8]，实际 %v", got)
	}
	if got := decodeIDs(u.McpIDs); len(got) != 1 || got[0] != 3 {
		t.Errorf("McpIDs 应更新为 [3]，实际 %v", got)
	}
	if len(repo.subAgentIDs) != 1 || repo.subAgentIDs[0] != 11 {
		t.Errorf("SubAgentIDs 应更新为 [11]，实际 %v", repo.subAgentIDs)
	}
}

// 非内置 Agent：全字段照常更新（不受内置锁影响）。
func TestUpdate_CustomAgentUpdatesAll(t *testing.T) {
	custom := &po.Agent{
		ID: 2, Key: "my-bot", Name: "旧名", Instruction: "旧指令",
		BuiltinTools: datatypes.JSON([]byte(`["read"]`)),
		SkillIDs:     datatypes.JSON([]byte(`[]`)),
		McpIDs:       datatypes.JSON([]byte(`[]`)),
	}
	repo := &fakeAgentRepo{agent: custom}
	uc := newTestAgentUsecase(repo)

	in := &AgentInput{
		Name:         "新名",
		Instruction:  "新指令",
		BuiltinTools: []string{"bash", "grep"},
		SkillIDs:     []int64{5},
	}
	if err := uc.Update(context.Background(), "my-bot", in); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}
	u := repo.updated
	if u.Name != "新名" || u.Instruction != "新指令" {
		t.Errorf("自定义 Agent 名称/指令未更新: %+v", u)
	}
	if got := decodeStrings(u.BuiltinTools); len(got) != 2 {
		t.Errorf("自定义 BuiltinTools 应更新，实际 %v", got)
	}
	if got := decodeIDs(u.SkillIDs); len(got) != 1 || got[0] != 5 {
		t.Errorf("自定义 SkillIDs 应更新为 [5]，实际 %v", got)
	}
}

func decodeIDs(j datatypes.JSON) []int64 {
	var out []int64
	_ = json.Unmarshal(j, &out)
	return out
}
