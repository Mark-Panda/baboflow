package agentkit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/adk/middlewares/skill"
	"gopkg.in/yaml.v3"
	"gorm.io/datatypes"

	"baboflow/internal/data/po"
)

// SkillRepo 供 agentkit 读取 SKILL 的最小接口（由 data 层实现）。
type SkillRepo interface {
	ListByIDs(ctx context.Context, ids []int64) ([]po.Skill, error)
	GetByName(ctx context.Context, name string) (*po.Skill, error)
}

// EnsureSkillDirFunc 确保含包技能已解压落盘，返回其目录（供 BaseDirectory）。
// 由上层（biz）注入，避免 agentkit 反向依赖落盘编排；纯文本技能不会被调用。
type EnsureSkillDirFunc func(ctx context.Context, s *po.Skill) (string, error)

// SkillBackend 用 DB 中的 skill 表实现 eino skill.Backend（List/Get）。
// 每个 Agent 只暴露其绑定的 skillIDs。
type SkillBackend struct {
	repo      SkillRepo
	ids       []int64
	ensureDir EnsureSkillDirFunc // 可空；含包技能 Get 时先确保目录在位
}

func NewSkillBackend(repo SkillRepo, ids []int64) *SkillBackend {
	return &SkillBackend{repo: repo, ids: ids}
}

// SetEnsureDir 注入含包技能的落盘回调（biz 提供）。
func (b *SkillBackend) SetEnsureDir(f EnsureSkillDirFunc) { b.ensureDir = f }

func (b *SkillBackend) List(ctx context.Context) ([]skill.FrontMatter, error) {
	if len(b.ids) == 0 {
		return nil, nil
	}
	skills, err := b.repo.ListByIDs(ctx, b.ids)
	if err != nil {
		return nil, err
	}
	out := make([]skill.FrontMatter, 0, len(skills))
	for _, s := range skills {
		out = append(out, toFrontMatter(&s))
	}
	return out, nil
}

func (b *SkillBackend) Get(ctx context.Context, name string) (skill.Skill, error) {
	s, err := b.repo.GetByName(ctx, name)
	if err != nil {
		return skill.Skill{}, fmt.Errorf("skill %q 不存在: %w", name, err)
	}
	baseDir := s.FilePath
	// 含包技能：先确保已解压落盘（磁盘丢失时从 DB 归档自愈），让模型能读包内附属文件。
	if s.HasFiles && b.ensureDir != nil {
		dir, derr := b.ensureDir(ctx, s)
		if derr != nil {
			return skill.Skill{}, fmt.Errorf("技能 %q 文件目录准备失败: %w", name, derr)
		}
		if dir != "" {
			baseDir = dir
		}
	}
	return skill.Skill{
		FrontMatter:   toFrontMatter(s),
		Content:       s.Content,
		BaseDirectory: baseDir,
	}, nil
}

// toFrontMatter 优先用 DB frontmatter，缺省时回退到 name/description 字段。
func toFrontMatter(s *po.Skill) skill.FrontMatter {
	fm := skill.FrontMatter{Name: s.Name, Description: s.Description}
	if len(s.Frontmatter) > 0 {
		var m map[string]any
		if err := json.Unmarshal(datatypes.JSON(s.Frontmatter), &m); err == nil {
			if v, ok := m["name"].(string); ok && v != "" {
				fm.Name = v
			}
			if v, ok := m["description"].(string); ok && v != "" {
				fm.Description = v
			}
			if v, ok := m["context"].(string); ok && v != "" {
				fm.Context = skill.ContextMode(v)
			}
			if v, ok := m["agent"].(string); ok {
				fm.Agent = v
			}
			if v, ok := m["model"].(string); ok {
				fm.Model = v
			}
		}
	}
	return fm
}

// ParseSkillMarkdown 解析 SKILL.md 文本（YAML frontmatter + markdown 正文），
// 供上传/生成时落库。frontmatter 缺省时返回空 map。
func ParseSkillMarkdown(text string) (frontmatter map[string]any, content string, err error) {
	frontmatter = map[string]any{}
	content = text
	if len(text) < 4 || text[:4] != "---\n" && text[:4] != "---\r" {
		return frontmatter, content, nil
	}
	rest := text[3:]
	// 跳过首个分隔行内容
	if idx := indexLineEnd(rest); idx >= 0 {
		rest = rest[idx:]
	}
	end := -1
	for i := 0; i+3 <= len(rest); i++ {
		if rest[i] == '-' && rest[i+1] == '-' && rest[i+2] == '-' && (i == 0 || rest[i-1] == '\n') {
			end = i
			break
		}
	}
	if end < 0 {
		return frontmatter, content, nil
	}
	yamlBody := rest[:end]
	body := rest[end+3:]
	if idx := indexLineEnd(body); idx >= 0 {
		body = body[idx:]
	}
	if err := yaml.Unmarshal([]byte(yamlBody), &frontmatter); err != nil {
		return nil, "", fmt.Errorf("解析 SKILL frontmatter 失败: %w", err)
	}
	return frontmatter, trimLeadingNewlines(body), nil
}

func indexLineEnd(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return i + 1
		}
	}
	return -1
}

func trimLeadingNewlines(s string) string {
	for len(s) > 0 && (s[0] == '\n' || s[0] == '\r') {
		s = s[1:]
	}
	return s
}
