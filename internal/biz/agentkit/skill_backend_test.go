package agentkit

import (
	"context"
	"errors"
	"testing"

	"baboflow/internal/data/po"
)

func TestParseSkillMarkdownWithFrontmatter(t *testing.T) {
	text := "---\nname: my-skill\ndescription: 测试技能\ncontext: fork\n---\n\n# 使用说明\n做一些事。\n"
	fm, content, err := ParseSkillMarkdown(text)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if fm["name"] != "my-skill" {
		t.Fatalf("expect name, got %v", fm["name"])
	}
	if fm["description"] != "测试技能" {
		t.Fatalf("expect description, got %v", fm["description"])
	}
	if fm["context"] != "fork" {
		t.Fatalf("expect context, got %v", fm["context"])
	}
	if content != "# 使用说明\n做一些事。\n" {
		t.Fatalf("expect markdown body, got %q", content)
	}
}

func TestParseSkillMarkdownNoFrontmatter(t *testing.T) {
	text := "# 纯 Markdown\n没有 frontmatter。\n"
	fm, content, err := ParseSkillMarkdown(text)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(fm) != 0 {
		t.Fatalf("expect empty frontmatter, got %v", fm)
	}
	if content != text {
		t.Fatalf("expect original content, got %q", content)
	}
}

func TestParseSkillMarkdownInvalidYAML(t *testing.T) {
	text := "---\n: : : bad\n---\nbody"
	_, _, err := ParseSkillMarkdown(text)
	if err == nil {
		t.Fatal("expect yaml error")
	}
}

type backendSkillRepo struct {
	skill *po.Skill
}

func (r *backendSkillRepo) ListByIDs(context.Context, []int64) ([]po.Skill, error) {
	return []po.Skill{*r.skill}, nil
}

func (r *backendSkillRepo) GetByName(context.Context, string) (*po.Skill, error) {
	return r.skill, nil
}

func TestSkillBackendPropagatesEnsureDirError(t *testing.T) {
	backend := NewSkillBackend(&backendSkillRepo{skill: &po.Skill{
		Name:     "package-skill",
		Content:  "---\nname: package-skill\n---\n",
		HasFiles: true,
	}}, []int64{1})
	backend.SetEnsureDir(func(context.Context, *po.Skill) (string, error) {
		return "", errors.New("workspace unavailable")
	})

	if _, err := backend.Get(context.Background(), "package-skill"); err == nil {
		t.Fatal("expected ensure directory error")
	}
}
