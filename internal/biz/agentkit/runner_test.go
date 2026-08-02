package agentkit

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// mockAgent 按脚本发射事件，用于测试 runner 的事件聚合。
type mockAgent struct {
	events []*adk.TypedAgentEvent[*schema.AgenticMessage]
}

func (m *mockAgent) Name(ctx context.Context) string        { return "mock" }
func (m *mockAgent) Description(ctx context.Context) string { return "mock" }

func (m *mockAgent) Run(ctx context.Context, input *adk.TypedAgentInput[*schema.AgenticMessage], options ...adk.AgentRunOption) *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.TypedAgentEvent[*schema.AgenticMessage]]()
	go func() {
		defer gen.Close()
		for _, ev := range m.events {
			gen.Send(ev)
		}
	}()
	return iter
}

func textEvent(text string) *adk.TypedAgentEvent[*schema.AgenticMessage] {
	return &adk.TypedAgentEvent[*schema.AgenticMessage]{
		AgentName: "mock",
		Output: &adk.TypedAgentOutput[*schema.AgenticMessage]{
			MessageOutput: &adk.TypedMessageVariant[*schema.AgenticMessage]{
				Message: &schema.AgenticMessage{
					Role: schema.AgenticRoleTypeAssistant,
					ContentBlocks: []*schema.ContentBlock{
						schema.NewContentBlock(&schema.AssistantGenText{Text: text}),
					},
				},
			},
		},
	}
}

func toolCallEvent(name, args, callID string) *adk.TypedAgentEvent[*schema.AgenticMessage] {
	return &adk.TypedAgentEvent[*schema.AgenticMessage]{
		AgentName: "mock",
		Output: &adk.TypedAgentOutput[*schema.AgenticMessage]{
			MessageOutput: &adk.TypedMessageVariant[*schema.AgenticMessage]{
				Message: &schema.AgenticMessage{
					Role: schema.AgenticRoleTypeAssistant,
					ContentBlocks: []*schema.ContentBlock{
						schema.NewContentBlock(&schema.FunctionToolCall{CallID: callID, Name: name, Arguments: args}),
					},
				},
			},
		},
	}
}

func toolResultEvent(name, callID, out string) *adk.TypedAgentEvent[*schema.AgenticMessage] {
	return &adk.TypedAgentEvent[*schema.AgenticMessage]{
		AgentName: "mock",
		Output: &adk.TypedAgentOutput[*schema.AgenticMessage]{
			MessageOutput: &adk.TypedMessageVariant[*schema.AgenticMessage]{
				Message: &schema.AgenticMessage{
					Role: schema.AgenticRoleTypeAssistant,
					ContentBlocks: []*schema.ContentBlock{
						schema.NewContentBlock(&schema.FunctionToolResult{
							CallID: callID, Name: name,
							Content: []*schema.FunctionToolResultContentBlock{
								{Type: schema.FunctionToolResultContentBlockTypeText, Text: &schema.UserInputText{Text: out}},
							},
						}),
					},
				},
			},
		},
	}
}

// runner 聚合文本增量 + 工具调用 + 工具结果，并按序触发回调。
func TestRunAggregatesTextAndTools(t *testing.T) {
	ag := &mockAgent{events: []*adk.TypedAgentEvent[*schema.AgenticMessage]{
		textEvent("我来"),
		toolCallEvent("bash", `{"command":"ls"}`, "c1"),
		toolResultEvent("bash", "c1", "file1\nfile2"),
		textEvent("处理完成"),
	}}

	var streamed []string
	cb := &RunCallbacks{OnEvent: func(ev *StreamEvent) {
		streamed = append(streamed, ev.Type)
	}}

	res, err := Run(context.Background(), ag, nil, &Input{Text: "hi"}, cb, nil, "1", "sess")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if res.Text != "我来处理完成" {
		t.Fatalf("expect concatenated text, got %q", res.Text)
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].Name != "bash" {
		t.Fatalf("expect 1 bash tool call, got %+v", res.ToolCalls)
	}
	if res.ToolCalls[0].Output != "file1\nfile2" {
		t.Fatalf("expect tool output mapped to call, got %q", res.ToolCalls[0].Output)
	}
	// 回调序：text, tool_call, tool_result, text, done
	want := []string{"text", "tool_call", "tool_result", "text", "done"}
	if len(streamed) != len(want) {
		t.Fatalf("expect %d events, got %v", len(want), streamed)
	}
	for i := range want {
		if streamed[i] != want[i] {
			t.Fatalf("event %d expect %s, got %v", i, want[i], streamed)
		}
	}
}

// agent 运行出错时应传播 error 并发 error 事件。
func TestRunPropagatesError(t *testing.T) {
	errAgent := &mockAgent{events: []*adk.TypedAgentEvent[*schema.AgenticMessage]{
		{Err: context.DeadlineExceeded},
	}}
	var gotErr bool
	cb := &RunCallbacks{OnEvent: func(ev *StreamEvent) {
		if ev.Type == "error" {
			gotErr = true
		}
	}}
	_, err := Run(context.Background(), errAgent, nil, &Input{Text: "x"}, cb, nil, "1", "s")
	if err == nil {
		t.Fatal("expect error")
	}
	if !gotErr {
		t.Fatal("expect error event emitted")
	}
}

// 多模态输入：图片应编码进 user 消息块。
func TestBuildUserMessageMultimodal(t *testing.T) {
	msg := buildUserMessage(&Input{
		Text:   "看图",
		Images: []ImageInput{{Base64Data: "abc", MIMEType: "image/png"}},
	})
	if msg.Role != schema.AgenticRoleTypeUser {
		t.Fatalf("expect user role, got %s", msg.Role)
	}
	if len(msg.ContentBlocks) != 2 {
		t.Fatalf("expect 2 blocks (image+text), got %d", len(msg.ContentBlocks))
	}
	if msg.ContentBlocks[0].Type != schema.ContentBlockTypeUserInputImage {
		t.Fatalf("expect first block image, got %s", msg.ContentBlocks[0].Type)
	}
	if msg.ContentBlocks[1].Type != schema.ContentBlockTypeUserInputText {
		t.Fatalf("expect second block text, got %s", msg.ContentBlocks[1].Type)
	}
}
