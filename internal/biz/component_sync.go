package biz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"baboflow/internal/data/po"

	"github.com/rulego/rulego"
	"github.com/rulego/rulego/api/types"
	"gorm.io/datatypes"
)

// ComponentRepo 组件注册表镜像存储。
type ComponentRepo interface {
	ListAll(ctx context.Context) ([]po.ComponentMeta, error)
	Upsert(ctx context.Context, m *po.ComponentMeta) error
	MarkMissing(ctx context.Context, keepTypes []string) (int64, error)
	SearchKeyword(ctx context.Context, category, keyword string) ([]po.ComponentMeta, error)
}

// SyncResult 一次同步的统计。
type SyncResult struct {
	Added     int       `json:"added"`
	Updated   int       `json:"updated"`
	Removed   int       `json:"removed"`
	Skipped   int       `json:"skipped"`
	LastRunAt time.Time `json:"lastRunAt"`
}

// ComponentSync 把 RuleGo 注册表组件自动同步到 component_meta（零人工）。
// 触发时机：启动全量扫描 + rulegokit.Register 钩子 + 周期对账。
type ComponentSync struct {
	repo ComponentRepo
	last SyncResult
	mu   sync.Mutex
	// skillGen 可选：组件变更时同步生成/更新 SKILL（M6 接入，M1 先留钩子）
	onComponentChange func(ctx context.Context, m *po.ComponentMeta)
}

func NewComponentSync(repo ComponentRepo) *ComponentSync {
	return &ComponentSync{repo: repo}
}

// SetOnComponentChange 注册组件变更回调（用于 SKILL 自动生成）。
func (s *ComponentSync) SetOnComponentChange(fn func(ctx context.Context, m *po.ComponentMeta)) {
	s.onComponentChange = fn
}

// Last 返回最近一次同步结果。
func (s *ComponentSync) Last() SyncResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

// Run 全量扫描注册表并与 DB diff，幂等 upsert。
func (s *ComponentSync) Run(ctx context.Context) (SyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	forms := rulego.Registry.GetComponentForms()
	existing, err := s.repo.ListAll(ctx)
	if err != nil {
		return s.last, err
	}
	fpByType := make(map[string]string, len(existing))
	for _, e := range existing {
		fpByType[e.Type] = e.Fingerprint
	}

	res := SyncResult{LastRunAt: time.Now()}
	keepTypes := make([]string, 0, len(forms))
	for _, f := range forms.Values() {
		meta := formToMeta(&f)
		fp := fingerprint(meta.Type, meta.ConfigSchema, meta.Description)
		keepTypes = append(keepTypes, meta.Type)
		old, seen := fpByType[meta.Type]
		switch {
		case !seen:
			meta.Fingerprint = fp
			if err := s.repo.Upsert(ctx, meta); err == nil {
				res.Added++
				s.fireChange(ctx, meta)
			}
		case old != fp:
			meta.Fingerprint = fp
			if err := s.repo.Upsert(ctx, meta); err == nil {
				res.Updated++
				s.fireChange(ctx, meta)
			}
		default:
			res.Skipped++
		}
	}
	removed, err := s.repo.MarkMissing(ctx, keepTypes)
	if err == nil {
		res.Removed = int(removed)
	}
	s.last = res
	return res, nil
}

func (s *ComponentSync) fireChange(ctx context.Context, m *po.ComponentMeta) {
	if s.onComponentChange != nil {
		// 异步执行，避免阻塞同步主流程
		go s.onComponentChange(ctx, m)
	}
}

// formToMeta 把 RuleGo ComponentForm 映射为 component_meta。
func formToMeta(f *types.ComponentForm) *po.ComponentMeta {
	schema, _ := json.Marshal(f)
	example := exampleFromForm(f)
	exampleJSON, _ := json.Marshal(example)
	return &po.ComponentMeta{
		Type:         f.Type,
		Name:         firstNonEmpty(f.Label, f.Type),
		Category:     firstNonEmpty(f.Category, "common"),
		Description:  f.Desc,
		ConfigSchema: datatypes.JSON(schema),
		Example:      datatypes.JSON(exampleJSON),
	}
}

// exampleFromForm 从字段默认值生成最小配置示例。
func exampleFromForm(f *types.ComponentForm) map[string]interface{} {
	ex := map[string]interface{}{}
	for _, field := range f.Fields {
		if field.DefaultValue != nil && field.DefaultValue != "" {
			ex[field.Name] = field.DefaultValue
		}
	}
	return ex
}

func fingerprint(typ string, schema datatypes.JSON, desc string) string {
	h := sha256.Sum256(append([]byte(typ+"|"+desc+"|"), schema...))
	return hex.EncodeToString(h[:16])
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
