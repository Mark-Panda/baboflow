package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"baboflow/internal/biz"
	"baboflow/internal/biz/agentkit"
)

func TestWsInboundAcceptsProtoInt64AssetIDs(t *testing.T) {
	var inbound wsInbound
	if err := json.Unmarshal([]byte(`{"action":"input","channel":"agent-chat","assetIds":["9007199254740993","9"]}`), &inbound); err != nil {
		t.Fatalf("unmarshal string assetIds: %v", err)
	}
	if got := fmt.Sprint(inbound.AssetIDs); got != "[9007199254740993 9]" {
		t.Fatalf("assetIds = %s, want string IDs", got)
	}
}

func TestWsConnectionContextSurvivesRequestCancellation(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.WithValue(context.Background(), "request-key", "request-value"))
	defer cancelRequest()

	connCtx, cancelConn := newWsConnContext(requestCtx)
	defer cancelConn()
	cancelRequest()

	select {
	case <-connCtx.Done():
		t.Fatal("websocket work context should not be canceled with HTTP request")
	default:
	}
	if got := connCtx.Value("request-key"); got != "request-value" {
		t.Fatalf("expected request context values to be preserved, got %v", got)
	}
}

func TestWsHubApplyChainDslWaitsForSuccessfulToolResult(t *testing.T) {
	hub := NewWsHub(nil, nil)
	sessionID := "session-1"
	args := `{"dsl":"{\"metadata\":{\"nodes\":[],\"connections\":[]}}"}`

	runID := "run-1"
	if frame := hub.toWsFrame(sessionID, runID, &agentkit.StreamEvent{
		Type:     "tool_call",
		ToolName: biz.ApplyChainToolName,
		ToolArgs: args,
	}); frame != nil {
		t.Fatalf("apply_chain_dsl tool_call should not reach client before result: %+v", frame)
	}

	failed := hub.toWsFrame(sessionID, runID, &agentkit.StreamEvent{
		Type:     "tool_result",
		ToolName: biz.ApplyChainToolName,
		ToolOut:  "DSL 校验失败",
	})
	if failed == nil || failed.Type != "tool_call" {
		t.Fatalf("failed apply should emit error tool frame, got %+v", failed)
	}

	if frame := hub.toWsFrame(sessionID, runID, &agentkit.StreamEvent{
		Type:     "tool_call",
		ToolName: biz.ApplyChainToolName,
		ToolArgs: args,
	}); frame != nil {
		t.Fatalf("second apply_chain_dsl tool_call should wait for result: %+v", frame)
	}
	success := hub.toWsFrame(sessionID, runID, &agentkit.StreamEvent{
		Type:     "tool_result",
		ToolName: biz.ApplyChainToolName,
		ToolOut:  biz.ApplyChainSuccessMarker,
	})
	if success == nil || success.Type != "chain_dsl" {
		t.Fatalf("successful apply should emit chain_dsl frame, got %+v", success)
	}
	if data, ok := success.Data.(map[string]any); !ok || data["dsl"] != `{"metadata":{"nodes":[],"connections":[]}}` {
		t.Fatalf("chain_dsl frame should contain original DSL, got %+v", success.Data)
	}
}

func TestWsHubApplyChainDslPendingIsolatedByRun(t *testing.T) {
	hub := NewWsHub(nil, nil)
	first := `{"dsl":"first"}`
	second := `{"dsl":"second"}`

	if hub.toWsFrame("session-1", "run-1", &agentkit.StreamEvent{
		Type: "tool_call", ToolName: biz.ApplyChainToolName, ToolArgs: first,
	}) != nil {
		t.Fatal("first tool call should be buffered")
	}
	if hub.toWsFrame("session-1", "run-2", &agentkit.StreamEvent{
		Type: "tool_call", ToolName: biz.ApplyChainToolName, ToolArgs: second,
	}) != nil {
		t.Fatal("second tool call should be buffered")
	}

	frame := hub.toWsFrame("session-1", "run-2", &agentkit.StreamEvent{
		Type: "tool_result", ToolName: biz.ApplyChainToolName, ToolOut: biz.ApplyChainSuccessMarker,
	})
	data := frame.Data.(map[string]any)
	if data["dsl"] != "second" {
		t.Fatalf("run-2 should apply its own DSL, got %v", data["dsl"])
	}
}

func TestWsHubAskUserEmitsQuestionFrame(t *testing.T) {
	hub := NewWsHub(nil, nil)
	frame := hub.toWsFrame("session-1", "run-1", &agentkit.StreamEvent{
		Type:     "tool_call",
		ToolName: biz.AskUserToolName,
		CallID:   "question-1",
		ToolArgs: `{"question":"选择失败处理方式","options":["重试","返回错误"],"allowOther":true}`,
	})
	if frame == nil || frame.Type != "question" {
		t.Fatalf("ask_user should emit question frame, got %+v", frame)
	}
	data := frame.Data.(map[string]any)
	if data["questionId"] != "question-1" || data["question"] != "选择失败处理方式" {
		t.Fatalf("question frame payload mismatch: %+v", data)
	}
}
