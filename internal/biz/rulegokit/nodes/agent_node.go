// Package nodes 存放 BaboFlow 自定义的 RuleGo 节点。
package nodes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/rulego/rulego"
	"github.com/rulego/rulego/api/types"
	"github.com/rulego/rulego/utils/maps"
)

// NodeType 是 Agent 节点在 DSL / 注册表中的类型标识。
const NodeType = "agent"

// AgentRunner 抽象"以一段文本输入运行一次 Agent，返回最终文本"。
// 与 cron 的 AgentRunner 同一模式，便于单测替换与 DI 注入。
type AgentRunner func(ctx context.Context, agentKey, prompt string) (string, error)

// runner 为节点实际调用的执行器，进程启动时经 SetAgentRunner 注入（wire）。
// 未注入时节点执行会返回明确错误，避免 nil 崩溃。
var runner AgentRunner

// SetAgentRunner 注入 Agent 执行器（在 wire_gen 构造 agentManager 后调用）。
func SetAgentRunner(r AgentRunner) { runner = r }

// AgentNodeConfiguration Agent 节点配置。
type AgentNodeConfiguration struct {
	// AgentKey 引用的已配置 Agent 的稳定 key（与 cron targetId 一致）。
	AgentKey string `json:"agentKey" label:"Agent" desc:"要调用的已配置 Agent 的 key" required:"true"`
	// Prompt 输入模板；留空则用消息体作为 Agent 输入。
	Prompt string `json:"prompt" label:"输入模板" desc:"可选；留空使用消息体作为输入"`
	// TimeoutMs 预留：执行超时（毫秒）。0 表示不限制。
	TimeoutMs int `json:"timeoutMs" label:"超时(毫秒)" desc:"0 表示不限制"`
}

// AgentNode 在规则链中调用一个已配置的 BaboFlow Agent。
type AgentNode struct {
	config AgentNodeConfiguration
}

// NewAgentNode 创建节点原型（注册进 Registry 用）。
func NewAgentNode() *AgentNode { return &AgentNode{} }

// Type 返回节点类型标识。
func (n *AgentNode) Type() string { return NodeType }

// Category 让节点在组件面板归入 "agent" 类别（前端已有该类别色/图标）。
func (n *AgentNode) Category() string { return "agent" }

// Desc 提供组件描述：会写入 component_meta.Description，进而成为组件 SKILL 的
// description 与组件面板/列表中的说明文字（rulego 仅对实现 DescGetter 的节点设置 Desc）。
func (n *AgentNode) Desc() string {
	return "调用一个已配置的 Agent：把节点输入（或输入模板）交给指定 Agent 执行，并将结果文本写回消息。适用于在规则链中嵌入 LLM 推理、工具调用等智能处理。"
}

// New 为每条规则链创建新实例（原型模式），保证链间数据隔离。
func (n *AgentNode) New() types.Node { return &AgentNode{} }

// Init 解析并校验节点配置。
func (n *AgentNode) Init(_ types.Config, configuration types.Configuration) error {
	if err := maps.Map2Struct(configuration, &n.config); err != nil {
		return err
	}
	if strings.TrimSpace(n.config.AgentKey) == "" {
		return errors.New("agent 节点缺少必填配置 agentKey")
	}
	return nil
}

// OnMsg 运行 Agent：成功把结果文本写回消息并走 Success；失败走 Failure。
func (n *AgentNode) OnMsg(ctx types.RuleContext, msg types.RuleMsg) {
	if runner == nil {
		ctx.TellFailure(msg, errors.New("agent 执行器未初始化（runner 未注入）"))
		return
	}
	input := strings.TrimSpace(n.config.Prompt)
	if input == "" {
		input = msg.GetData()
	}
	text, err := runner(context.Background(), n.config.AgentKey, input)
	if err != nil {
		ctx.TellFailure(msg, fmt.Errorf("agent %q 执行失败: %w", n.config.AgentKey, err))
		return
	}
	msg.SetData(text)
	ctx.TellSuccess(msg)
}

// Destroy 释放资源（本节点无持有资源）。
func (n *AgentNode) Destroy() {}

// 进程启动即把 Agent 节点注册进全局注册表，确保：
// 1. Validate() 的 NewNode("agent") 校验通过；
// 2. RestorePublished 加载含 agent 节点的已发布链成功；
// 3. component_sync 的 GetComponentForms() 能发现它并同步到组件面板。
// 该 init 在任何 DI 之前执行，因此与 wire_gen.go 的再生成无关。
func init() {
	_ = rulego.Registry.Register(NewAgentNode())
}
