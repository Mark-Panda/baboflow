package agentkit

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// StreamEvent 一次 agent 运行中推给前端的事件（WS 帧）。
type StreamEvent struct {
	Type     string `json:"type"`               // text/tool_call/tool_result/done/error
	Delta    string `json:"delta,omitempty"`    // text 增量
	ToolName string `json:"toolName,omitempty"` // tool_call/tool_result 工具名
	CallID   string `json:"callId,omitempty"`   // tool_call/tool_result 调用标识
	ToolArgs string `json:"toolArgs,omitempty"` // tool_call 入参(JSON)
	ToolOut  string `json:"toolOut,omitempty"`  // tool_result 摘要
	Agent    string `json:"agent,omitempty"`    // 产出事件的 agent（subAgent 区分）
	Err      string `json:"err,omitempty"`      // error
}

// RunCallbacks 运行期回调。onEvent 用于 WS 实时推送。
type RunCallbacks struct {
	OnEvent func(ev *StreamEvent)
}

// RunResult 汇总一次运行的最终文本与全部工具调用记录。
type RunResult struct {
	Text      string        `json:"text"`
	ToolCalls []ToolCallRec `json:"toolCalls"`
}

// ToolCallRec 一条工具调用记录（入库 agent_message.tool_calls）。
type ToolCallRec struct {
	Name       string        `json:"name"`
	Input      string        `json:"input"`
	Output     string        `json:"output"`
	Status     string        `json:"status"` // ok/error
	QuestionID string        `json:"questionId,omitempty"`
	Question   *UserQuestion `json:"question,omitempty"`
}

type UserQuestion struct {
	Question   string   `json:"question"`
	Options    []string `json:"options,omitempty"`
	Multiple   bool     `json:"multiple,omitempty"`
	AllowOther bool     `json:"allowOther,omitempty"`
}

// Input 构造一次用户输入（支持文本 + 图片多模态）。
type Input struct {
	Text   string
	Images []ImageInput
}

// ImageInput 一张图片（URL 或 base64）。
type ImageInput struct {
	URL        string
	Base64Data string
	MIMEType   string
}

// Run 执行 agent，逐事件回调并汇总结果。
// history 为会话历史（不含本次输入），用于多轮对话。
func Run(ctx context.Context, ag adk.TypedAgent[*schema.AgenticMessage], history []*schema.AgenticMessage, in *Input, cb *RunCallbacks, userID, sessionID string) (*RunResult, error) {
	return run(ctx, ag, history, in, cb, userID, sessionID, true)
}

// RunWithMemoryHistory 控制是否由记忆 Provider 注入会话历史。
// 关闭时用于会话摘要模式，避免把已摘要的业务历史再次传入模型。
func RunWithMemoryHistory(ctx context.Context, ag adk.TypedAgent[*schema.AgenticMessage], history []*schema.AgenticMessage, in *Input, cb *RunCallbacks, userID, sessionID string, useMemoryHistory bool) (*RunResult, error) {
	return run(ctx, ag, history, in, cb, userID, sessionID, useMemoryHistory)
}

func run(ctx context.Context, ag adk.TypedAgent[*schema.AgenticMessage], history []*schema.AgenticMessage, in *Input, cb *RunCallbacks, userID, sessionID string, useMemoryHistory bool) (*RunResult, error) {
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{Agent: ag})

	msgs := make([]*schema.AgenticMessage, 0, len(history)+1)
	if useMemoryHistory {
		msgs = append(msgs, history...)
	}
	msgs = append(msgs, buildUserMessage(in))

	var opts []adk.AgentRunOption
	opts = append(opts, adk.WithSessionValues(map[string]any{
		"userID":           userID,
		"sessionID":        sessionID,
		"businessHistory":  history,
		"useMemoryHistory": useMemoryHistory,
	}))

	iter := runner.Run(ctx, msgs, opts...)
	res := &RunResult{}
	var textSb strings.Builder

	emit := func(ev *StreamEvent) {
		if cb != nil && cb.OnEvent != nil {
			cb.OnEvent(ev)
		}
	}

	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			emit(&StreamEvent{Type: "error", Err: event.Err.Error()})
			return res, event.Err
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		mv := event.Output.MessageOutput
		msg, err := mv.GetMessage()
		if err != nil {
			continue
		}
		if msg == nil {
			continue
		}
		collectMessage(msg, event.AgentName, &textSb, res, emit)
		// ask_user 是一次可暂停的交互：问题帧发出后必须结束当前运行，
		// 先持久化本轮 assistant 消息，用户回答才能在下一轮带着上下文继续。
		if lastToolNeedsUserInput(res) {
			res.Text = textSb.String()
			emit(&StreamEvent{Type: "done"})
			return res, nil
		}
	}

	res.Text = textSb.String()
	emit(&StreamEvent{Type: "done"})
	return res, nil
}

func lastToolNeedsUserInput(res *RunResult) bool {
	if res == nil || len(res.ToolCalls) == 0 {
		return false
	}
	last := res.ToolCalls[len(res.ToolCalls)-1]
	return last.Name == "ask_user" && last.Question != nil
}

// buildUserMessage 把输入组装成多模态 user 消息。
func buildUserMessage(in *Input) *schema.AgenticMessage {
	blocks := []*schema.ContentBlock{}
	if len(in.Images) > 0 {
		for _, im := range in.Images {
			blocks = append(blocks, &schema.ContentBlock{
				Type: schema.ContentBlockTypeUserInputImage,
				UserInputImage: &schema.UserInputImage{
					URL:        im.URL,
					Base64Data: im.Base64Data,
					MIMEType:   im.MIMEType,
				},
			})
		}
	}
	blocks = append(blocks, &schema.ContentBlock{
		Type:          schema.ContentBlockTypeUserInputText,
		UserInputText: &schema.UserInputText{Text: in.Text},
	})
	return &schema.AgenticMessage{
		Role:          schema.AgenticRoleTypeUser,
		ContentBlocks: blocks,
	}
}

// collectMessage 从一条 agentic 消息中提取文本/工具调用/工具结果。
func collectMessage(msg *schema.AgenticMessage, agentName string, textSb *strings.Builder, res *RunResult, emit func(*StreamEvent)) {
	for _, blk := range msg.ContentBlocks {
		switch blk.Type {
		case schema.ContentBlockTypeAssistantGenText:
			if blk.AssistantGenText != nil && blk.AssistantGenText.Text != "" {
				textSb.WriteString(blk.AssistantGenText.Text)
				emit(&StreamEvent{Type: "text", Delta: blk.AssistantGenText.Text, Agent: agentName})
			}
		case schema.ContentBlockTypeFunctionToolCall:
			if blk.FunctionToolCall != nil {
				fc := blk.FunctionToolCall
				rec := ToolCallRec{
					Name: fc.Name, Input: fc.Arguments, Status: "ok",
				}
				if fc.Name == "ask_user" {
					var question UserQuestion
					if json.Unmarshal([]byte(fc.Arguments), &question) == nil && question.Question != "" {
						rec.QuestionID = fc.CallID
						rec.Question = &question
					}
				}
				res.ToolCalls = append(res.ToolCalls, rec)
				emit(&StreamEvent{Type: "tool_call", ToolName: fc.Name, CallID: fc.CallID, ToolArgs: fc.Arguments, Agent: agentName})
			}
		case schema.ContentBlockTypeFunctionToolResult:
			if blk.FunctionToolResult != nil {
				fr := blk.FunctionToolResult
				out := toolResultText(fr)
				// 更新对应调用记录的输出（按 name 从尾部匹配最近一次未填输出的）
				for i := len(res.ToolCalls) - 1; i >= 0; i-- {
					if res.ToolCalls[i].Name == fr.Name && res.ToolCalls[i].Output == "" {
						res.ToolCalls[i].Output = out
						break
					}
				}
				emit(&StreamEvent{Type: "tool_result", ToolName: fr.Name, CallID: fr.CallID, ToolOut: summarize(out, 400), Agent: agentName})
			}
		}
	}
}

// toolResultText 把工具结果内容块拼成纯文本。
func toolResultText(fr *schema.FunctionToolResult) string {
	var sb strings.Builder
	for _, c := range fr.Content {
		if c != nil && c.Text != nil {
			sb.WriteString(c.Text.Text)
		}
	}
	return sb.String()
}

func summarize(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
