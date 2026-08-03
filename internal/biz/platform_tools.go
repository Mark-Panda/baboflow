package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"baboflow/internal/biz/rulegokit"
	"baboflow/internal/data/po"
)

// PlatformDeps 平台工具依赖（由 wire 注入，供 ExtraToolFactory 使用）。
type PlatformDeps struct {
	ComponentRepo ComponentRepo
	Chains        *RuleChainUsecase
	Skills        *SkillUsecase
}

// NewPlatformDeps 构造平台工具依赖（wire provider）。
func NewPlatformDeps(compRepo ComponentRepo, chains *RuleChainUsecase, skills *SkillUsecase) *PlatformDeps {
	return &PlatformDeps{ComponentRepo: compRepo, Chains: chains, Skills: skills}
}

// PlatformTools 把平台能力暴露为 Agent 工具：检索组件、校验/查询/创建规则链、创建 SKILL。
type PlatformTools struct {
	deps *PlatformDeps
}

func NewPlatformTools(deps *PlatformDeps) *PlatformTools {
	return &PlatformTools{deps: deps}
}

// Tools 返回全部平台工具。
func (p *PlatformTools) Tools() ([]tool.BaseTool, error) {
	var out []tool.BaseTool
	builders := []func() (tool.InvokableTool, error){
		p.searchComponentTool,
		p.validateChainTool,
		p.getChainTool,
		p.createChainTool,
		p.createSkillTool,
		p.listPublishedChainsTool,
	}
	for _, b := range builders {
		t, err := b()
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// ---- search_component ----

type searchComponentInput struct {
	Category string `json:"category,omitempty" jsonschema:"description=组件分类过滤(action/common/external/filter/flow/transform), 可空"`
	Keyword  string `json:"keyword" jsonschema:"description=检索关键词(组件类型/名称/描述),required"`
}

func (p *PlatformTools) searchComponentTool() (tool.InvokableTool, error) {
	return utils.InferTool("search_component",
		"检索可用的 RuleGo 规则链组件。返回组件 type/名称/分类/描述/配置 schema/DSL 示例, 用于挑选节点。",
		func(ctx context.Context, in searchComponentInput) (string, error) {
			list, err := p.deps.ComponentRepo.SearchKeyword(ctx, in.Category, in.Keyword)
			if err != nil {
				return "", err
			}
			if len(list) == 0 {
				return "未找到匹配组件", nil
			}
			if len(list) > 12 {
				list = list[:12]
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "找到 %d 个组件:\n\n", len(list))
			for _, c := range list {
				fmt.Fprintf(&sb, "## %s (%s)\n- 分类: %s\n- 描述: %s\n", c.Type, c.Name, c.Category, oneLine(c.Description))
				if len(c.ConfigSchema) > 0 && string(c.ConfigSchema) != "{}" {
					fmt.Fprintf(&sb, "- 配置 schema: %s\n", truncateRunes(string(c.ConfigSchema), 600))
				}
				if len(c.Example) > 0 && string(c.Example) != "{}" {
					fmt.Fprintf(&sb, "- DSL 示例: %s\n", truncateRunes(string(c.Example), 400))
				}
				sb.WriteString("\n")
			}
			return sb.String(), nil
		})
}

// ---- rulechain_validate ----

type validateChainInput struct {
	DSL string `json:"dsl" jsonschema:"description=规则链 DSL JSON 字符串,required"`
}

func (p *PlatformTools) validateChainTool() (tool.InvokableTool, error) {
	return utils.InferTool("rulechain_validate",
		"校验一段 RuleGo 规则链 DSL 是否合法(JSON 结构/组件存在/连接端点)。创建前先校验。",
		func(ctx context.Context, in validateChainInput) (string, error) {
			if err := rulegokit.Validate(json.RawMessage(in.DSL)); err != nil {
				return "校验失败: " + err.Error(), nil
			}
			return "校验通过: DSL 合法", nil
		})
}

// ---- rulechain_get ----

type getChainInput struct {
	ChainID string `json:"chainId" jsonschema:"description=规则链 id,required"`
}

func (p *PlatformTools) getChainTool() (tool.InvokableTool, error) {
	return utils.InferTool("rulechain_get",
		"按 id 查询规则链的完整 DSL/名称/描述/状态。用于读取已发布链以生成 SKILL 或参考。",
		func(ctx context.Context, in getChainInput) (string, error) {
			c, err := p.deps.Chains.Get(ctx, in.ChainID)
			if err != nil {
				return "", fmt.Errorf("规则链不存在: %w", err)
			}
			return fmt.Sprintf("规则链 %s (id=%s, 状态=%s, 版本=%d)\n描述: %s\nDSL:\n%s",
				c.Name, c.ID, c.Status, c.Version, c.Description, string(c.DSL)), nil
		})
}

// ---- rulechain_create ----

type createChainInput struct {
	Name        string `json:"name" jsonschema:"description=规则链名称,required"`
	Description string `json:"description,omitempty" jsonschema:"description=规则链描述"`
	DSL         string `json:"dsl" jsonschema:"description=规则链 DSL JSON 字符串,required"`
}

func (p *PlatformTools) createChainTool() (tool.InvokableTool, error) {
	return utils.InferTool("rulechain_create",
		"创建一条新规则链(草稿)。成功返回新链 id。务必先用 rulechain_validate 校验 DSL 合法。",
		func(ctx context.Context, in createChainInput) (string, error) {
			if strings.TrimSpace(in.Name) == "" {
				return "", fmt.Errorf("name 不能为空")
			}
			if err := rulegokit.Validate(json.RawMessage(in.DSL)); err != nil {
				return "", fmt.Errorf("DSL 校验失败, 请修正后重试: %w", err)
			}
			c, err := p.deps.Chains.Create(ctx, &ChainInput{
				Name: in.Name, Description: in.Description,
				DSL: json.RawMessage(in.DSL), Source: "agent",
			}, 0)
			if err != nil {
				return "", fmt.Errorf("创建失败: %w", err)
			}
			return fmt.Sprintf("创建成功: 规则链 id=%s 名称=%s (草稿, 需在界面调试后发布)", c.ID, c.Name), nil
		})
}

// ---- skill_create ----

type createSkillInput struct {
	Content string `json:"content" jsonschema:"description=完整 SKILL.md 文本(含 YAML frontmatter: name/description),required"`
}

func (p *PlatformTools) createSkillTool() (tool.InvokableTool, error) {
	return utils.InferTool("skill_create",
		"保存一段 SKILL.md 到平台。frontmatter 必须含 name 与 description。",
		func(ctx context.Context, in createSkillInput) (string, error) {
			view, err := p.deps.Skills.Upload(ctx, in.Content, "agent")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("SKILL 已保存: id=%d name=%s", view.ID, view.Name), nil
		})
}

// ---- list_published_chains ----

type listChainsInput struct {
	Keyword string `json:"keyword,omitempty" jsonschema:"description=名称关键词过滤, 可空"`
}

func (p *PlatformTools) listPublishedChainsTool() (tool.InvokableTool, error) {
	return utils.InferTool("list_published_chains",
		"列出平台已发布的规则链(id/名称/描述/版本), 供选择生成 SKILL 或编排。",
		func(ctx context.Context, in listChainsInput) (string, error) {
			list, _, err := p.deps.Chains.List(ctx, "published", in.Keyword, 1, 50)
			if err != nil {
				return "", err
			}
			if len(list) == 0 {
				return "暂无已发布规则链", nil
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "共 %d 条已发布规则链:\n\n", len(list))
			for i := range list {
				c := &list[i]
				fmt.Fprintf(&sb, "- id=%s 名称=%s 版本=%d 描述=%s\n", c.ID, c.Name, c.Version, oneLine(c.Description))
			}
			return sb.String(), nil
		})
}

var _ = po.ComponentMeta{}
