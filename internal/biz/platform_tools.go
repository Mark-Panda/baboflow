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
	return p.build([]func() (tool.InvokableTool, error){
		p.searchComponentTool,
		p.validateChainTool,
		p.getChainTool,
		p.createChainTool,
		p.listPublishedChainsTool,
		p.createSkillTool,
	})
}

// ToolsForAgent 返回指定 Agent 可用的平台工具。
//   - agent-chain-builder：在画布内生成/编辑规则链，用 apply_chain_dsl 把 DSL 回传画布，
//     不暴露 rulechain_create（落库）与 skill_create，避免绕开画布直接写库。
//   - agent-skill-generator：SKILL 反生成结果由外层统一保存，不暴露 skill_create，避免重复写入。
func (p *PlatformTools) ToolsForAgent(agentKey string) ([]tool.BaseTool, error) {
	switch agentKey {
	case "agent-chain-builder":
		return p.build([]func() (tool.InvokableTool, error){
			p.searchComponentTool,
			p.validateChainTool,
			p.askUserTool,
			p.applyChainTool,
		})
	case "agent-skill-generator":
		// 保持与原逻辑一致：全部平台工具，仅去掉 skill_create（反生成结果由外层统一保存）。
		return p.build([]func() (tool.InvokableTool, error){
			p.searchComponentTool,
			p.validateChainTool,
			p.getChainTool,
			p.createChainTool,
			p.listPublishedChainsTool,
		})
	default:
		return p.Tools()
	}
}

func (p *PlatformTools) build(builders []func() (tool.InvokableTool, error)) ([]tool.BaseTool, error) {
	var out []tool.BaseTool
	for _, b := range builders {
		t, err := b()
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// ---- ask_user ----

const AskUserToolName = "ask_user"

type askUserInput struct {
	Question   string   `json:"question" jsonschema:"description=需要用户确认的问题,required"`
	Options    []string `json:"options,omitempty" jsonschema:"description=可选答案列表"`
	Multiple   bool     `json:"multiple,omitempty" jsonschema:"description=是否允许多选"`
	AllowOther bool     `json:"allowOther,omitempty" jsonschema:"description=是否允许用户填写其他答案"`
}

func (p *PlatformTools) askUserTool() (tool.InvokableTool, error) {
	return utils.InferTool(AskUserToolName,
		"向用户提出一个需要澄清的问题。调用后必须停止本轮生成，不要调用 apply_chain_dsl；等待用户回答后再继续。",
		func(ctx context.Context, in askUserInput) (string, error) {
			if strings.TrimSpace(in.Question) == "" {
				return "", fmt.Errorf("question 不能为空")
			}
			if len(in.Options) == 0 && !in.AllowOther {
				return "", fmt.Errorf("options 不能为空，或必须允许用户填写其他答案")
			}
			payload := map[string]any{
				"question":   strings.TrimSpace(in.Question),
				"options":    in.Options,
				"multiple":   in.Multiple,
				"allowOther": in.AllowOther,
			}
			data, err := json.Marshal(payload)
			if err != nil {
				return "", err
			}
			return string(data), nil
		})
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

// ---- apply_chain_dsl ----

// ApplyChainToolName 是把规则链 DSL 回传画布的专用工具名。
// 完整 DSL 由该工具的调用入参(tool_call 帧)携带给前端，而非 tool_result(会被截断)。
const ApplyChainToolName = "apply_chain_dsl"
const ApplyChainSuccessMarker = `{"ok":true,"message":"已应用到画布"}`

type applyChainInput struct {
	DSL string `json:"dsl" jsonschema:"description=要应用到画布的完整规则链 DSL JSON 字符串,required"`
}

func (p *PlatformTools) applyChainTool() (tool.InvokableTool, error) {
	return utils.InferTool(ApplyChainToolName,
		"把最终确定的完整规则链 DSL 应用到用户当前画布(不落库)。调用前必须先用 rulechain_validate 校验通过。入参 dsl 必须是完整 DSL。",
		func(ctx context.Context, in applyChainInput) (string, error) {
			if err := rulegokit.Validate(json.RawMessage(in.DSL)); err != nil {
				return "", fmt.Errorf("DSL 校验失败, 请修正后重新调用 apply_chain_dsl: %w", err)
			}
			// 不落库。完整 DSL 由 tool_call 入参携带给前端，这里只回简短确认。
			return ApplyChainSuccessMarker, nil
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
