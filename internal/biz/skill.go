package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/datatypes"

	"baboflow/internal/biz/agentkit"
	"baboflow/internal/data/po"
)

// SkillDataRepo SKILL 持久化接口（data 层实现）。
type SkillDataRepo interface {
	List(ctx context.Context, source, keyword string) ([]po.Skill, error)
	GetByID(ctx context.Context, id int64) (*po.Skill, error)
	GetByName(ctx context.Context, name string) (*po.Skill, error)
	Create(ctx context.Context, s *po.Skill) error
	Update(ctx context.Context, s *po.Skill) error
	Delete(ctx context.Context, id int64) error
}

// SkillGenRunner 执行 Agent2 反生成：输入已发布链，返回 Agent 产出的文本。
// 由 service 层用 Agent 对话能力实现（避免 biz 循环依赖）。
type SkillGenRunner func(ctx context.Context, chainID, chainName string, dsl []byte) (string, error)

// SkillUsecase SKILL 管理 + Agent2 反生成 + 组件自动 SKILL。
type SkillUsecase struct {
	repo   SkillDataRepo
	chains *RuleChainUsecase
	genRunner SkillGenRunner
}

func NewSkillUsecase(repo SkillDataRepo, chains *RuleChainUsecase) *SkillUsecase {
	return &SkillUsecase{repo: repo, chains: chains}
}

// SetChains 注入规则链用例（生成/删除链级 SKILL 用）。
func (uc *SkillUsecase) SetChains(c *RuleChainUsecase) { uc.chains = c }

// SetGenRunner 注入 Agent2 反生成执行器。
func (uc *SkillUsecase) SetGenRunner(r SkillGenRunner) { uc.genRunner = r }

// ---- 视图 ----

type SkillView struct {
	ID          int64          `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Source      string         `json:"source"`
	ChainID     string         `json:"chainId"`
	Frontmatter datatypes.JSON `json:"frontmatter"`
	Content     string         `json:"content,omitempty"`
	CreatedAt   string         `json:"createdAt"`
}

func toSkillView(s *po.Skill, withContent bool) *SkillView {
	v := &SkillView{
		ID: s.ID, Name: s.Name, Description: s.Description, Source: s.Source,
		ChainID: s.ChainID, Frontmatter: s.Frontmatter,
		CreatedAt: s.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	if withContent {
		v.Content = s.Content
	}
	return v
}

// ---- CRUD ----

func (uc *SkillUsecase) List(ctx context.Context, source, keyword string) ([]SkillView, error) {
	list, err := uc.repo.List(ctx, source, keyword)
	if err != nil {
		return nil, err
	}
	out := make([]SkillView, 0, len(list))
	for i := range list {
		out = append(out, *toSkillView(&list[i], false))
	}
	return out, nil
}

func (uc *SkillUsecase) Get(ctx context.Context, id int64) (*SkillView, error) {
	s, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	return toSkillView(s, true), nil
}

// Upload 上传/保存 SKILL.md 文本：解析 frontmatter 入库（幂等：同名覆盖）。
func (uc *SkillUsecase) Upload(ctx context.Context, content, source string) (*SkillView, error) {
	fm, _, err := agentkit.ParseSkillMarkdown(content)
	if err != nil {
		return nil, err
	}
	name, _ := fm["name"].(string)
	desc, _ := fm["description"].(string)
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("SKILL.md 缺少 frontmatter name")
	}
	if source == "" {
		source = "upload"
	}
	fmJSON, _ := json.Marshal(fm)
	s := &po.Skill{
		Name: name, Description: desc, Source: source,
		Frontmatter: datatypes.JSON(fmJSON), Content: content,
	}
	if existing, err := uc.repo.GetByName(ctx, name); err == nil {
		s.ID = existing.ID
		s.ChainID = existing.ChainID
		if desc == "" {
			s.Description = existing.Description
		}
		if err := uc.repo.Update(ctx, s); err != nil {
			return nil, err
		}
	} else {
		if err := uc.repo.Create(ctx, s); err != nil {
			return nil, err
		}
	}
	return toSkillView(s, true), nil
}

func (uc *SkillUsecase) Delete(ctx context.Context, id int64) error {
	if _, err := uc.repo.GetByID(ctx, id); err != nil {
		return ErrNotFound
	}
	return uc.repo.Delete(ctx, id)
}

// ---- 组件自动 SKILL（零人工同步钩子，M1 已留 onComponentChange）----

// SyncComponentSkill 组件变更时模板化生成/更新对应 SKILL（source=component）。
// 供 ComponentSync 的 onComponentChange 回调调用。
func (uc *SkillUsecase) SyncComponentSkill(ctx context.Context, m *po.ComponentMeta) {
	name := ComponentSkillName(m.Type)
	content := renderComponentSkill(m)
	fm := map[string]any{"name": name, "description": m.Description}
	fmJSON, _ := json.Marshal(fm)
	s := &po.Skill{
		Name: name, Description: m.Description, Source: "component",
		Frontmatter: datatypes.JSON(fmJSON), Content: content,
	}
	if existing, err := uc.repo.GetByName(ctx, name); err == nil {
		s.ID = existing.ID
		_ = uc.repo.Update(ctx, s)
	} else {
		_ = uc.repo.Create(ctx, s)
	}
}

// ComponentSkillName 组件 → SKILL 名的稳定映射。
func ComponentSkillName(componentType string) string {
	return "component-" + strings.ReplaceAll(componentType, "/", "-")
}

// BackfillComponentSkills 启动时补齐缺失的组件 SKILL（零人工兜底）。
// 仅在对应 SKILL 不存在时生成，不覆盖用户手工修改。
func (uc *SkillUsecase) BackfillComponentSkills(ctx context.Context, comps []po.ComponentMeta) {
	for i := range comps {
		name := ComponentSkillName(comps[i].Type)
		if _, err := uc.repo.GetByName(ctx, name); err == nil {
			continue // 已存在（含手工改过），跳过
		}
		uc.SyncComponentSkill(ctx, &comps[i])
	}
}

// renderComponentSkill 模板化生成组件 SKILL.md（描述 + 配置 schema + 示例）。
func renderComponentSkill(m *po.ComponentMeta) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("name: " + ComponentSkillName(m.Type) + "\n")
	sb.WriteString("description: " + oneLine(m.Description) + "\n")
	sb.WriteString("---\n\n")
	sb.WriteString("# 组件 " + m.Type + "\n\n")
	sb.WriteString(m.Description + "\n\n")
	sb.WriteString("- 分类: " + m.Category + "\n\n")
	if len(m.ConfigSchema) > 0 && string(m.ConfigSchema) != "{}" && string(m.ConfigSchema) != "null" {
		sb.WriteString("## 配置项\n\n```json\n" + string(m.ConfigSchema) + "\n```\n\n")
	}
	if len(m.Example) > 0 && string(m.Example) != "{}" && string(m.Example) != "null" {
		sb.WriteString("## DSL 示例\n\n```json\n" + string(m.Example) + "\n```\n")
	}
	return sb.String()
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len([]rune(s)) > 120 {
		s = string([]rune(s)[:120])
	}
	return s
}

// ---- Agent2 反生成 ----

// GenerateFromChain 用 agent-skill-generator 从已发布链反生成 SKILL。
func (uc *SkillUsecase) GenerateFromChain(ctx context.Context, chainID string) (*SkillView, error) {
	if uc.genRunner == nil {
		return nil, errors.New("SKILL 生成器未就绪")
	}
	c, err := uc.chains.Get(ctx, chainID)
	if err != nil {
		return nil, ErrNotFound
	}
	if c.Status != "published" {
		return nil, errors.New("仅已发布规则链可生成 SKILL")
	}
	text, err := uc.genRunner(ctx, chainID, c.Name, c.DSL)
	if err != nil {
		return nil, err
	}
	md := extractSkillMarkdown(text)
	if md == "" {
		return nil, fmt.Errorf("Agent 未产出合法 SKILL.md；原始输出: %s", truncateRunes(text, 200))
	}
	view, err := uc.Upload(ctx, md, "chain")
	if err != nil {
		return nil, err
	}
	if s, err := uc.repo.GetByID(ctx, view.ID); err == nil {
		s.ChainID = chainID
		_ = uc.repo.Update(ctx, s)
		view.ChainID = chainID
	}
	return view, nil
}

// extractSkillMarkdown 从文本中抽取 ```markdown / ```yaml / ``` 包裹的 SKILL.md，或整段含 frontmatter 的文本。
func extractSkillMarkdown(text string) string {
	for _, fence := range []string{"```markdown", "```yaml", "```md", "```"} {
		if idx := strings.Index(text, fence); idx >= 0 {
			rest := text[idx+len(fence):]
			if end := strings.Index(rest, "```"); end >= 0 {
				block := strings.TrimSpace(rest[:end])
				if strings.HasPrefix(block, "---") {
					return block
				}
			}
		}
	}
	if strings.HasPrefix(strings.TrimSpace(text), "---") {
		return strings.TrimSpace(text)
	}
	return ""
}

func truncateRunes(s string, n int) string {
	if len([]rune(s)) > n {
		return string([]rune(s)[:n]) + "…"
	}
	return s
}
