package nodes

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rulego/rulego"
	"github.com/rulego/rulego/api/types"
	"github.com/rulego/rulego/engine"
	"github.com/rulego/rulego/utils/reflect"
)

// 组件表单应带中文类别与非空描述（描述会进入 component_meta 与组件 SKILL）。
func TestAgentNode_FormHasDescAndCategory(t *testing.T) {
	form := reflect.GetComponentForm(NewAgentNode())
	if form.Desc == "" {
		t.Fatal("expected non-empty Desc in component form")
	}
	if form.Category != "agent" {
		t.Fatalf("expected category=agent, got %q", form.Category)
	}
	if len(form.Fields) == 0 {
		t.Fatal("expected config fields in form")
	}
}

// testResult 一次同步执行的结果。
type testResult struct {
	Output string
	Err    error
}

// runDSLForTest 用独立临时引擎同步执行一段 DSL，返回最终输出与结束错误。
// rulegokit.RunDSL 无法在此复用（nodes 与 rulegokit 存在循环依赖），故内联一份最简实现。
func runDSLForTest(chainID string, dsl []byte, dataType, data string, metadata map[string]string) (testResult, error) {
	var out testResult
	eng, err := engine.NewRuleEngine(chainID, dsl)
	if err != nil {
		return out, err
	}
	defer eng.Stop(context.Background())

	dt := types.JSON
	if dataType != "" {
		dt = types.DataType(dataType)
	}
	md := types.NewMetadata()
	for k, v := range metadata {
		md.PutValue(k, v)
	}
	msg := types.NewMsg(0, "BABO_MSG", dt, md, data)
	eng.OnMsgAndWait(msg, types.WithOnEnd(func(_ types.RuleContext, m types.RuleMsg, err error, _ string) {
		out.Output = m.GetData()
		out.Err = err
	}))
	return out, nil
}

func TestAgentNode_Registered(t *testing.T) {
	if _, err := rulego.Registry.NewNode(NodeType); err != nil {
		t.Fatalf("agent node should be registered: %v", err)
	}
}

func TestAgentNode_InitRequiresAgentKey(t *testing.T) {
	n := &AgentNode{}
	if err := n.Init(types.Config{}, types.Configuration{}); err == nil {
		t.Fatal("expected error when agentKey missing")
	}
	if err := n.Init(types.Config{}, types.Configuration{"agentKey": "agent-general"}); err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}
	if n.config.AgentKey != "agent-general" {
		t.Fatalf("agentKey not decoded, got %q", n.config.AgentKey)
	}
}

// 端到端：注册一个假 runner，跑一条含 agent 节点的链，断言输出文本被写回。
func TestAgentNode_OnMsgRunsAndWritesBack(t *testing.T) {
	SetAgentRunner(func(_ context.Context, agentKey, prompt string) (string, error) {
		if agentKey != "agent-general" {
			return "", errors.New("unexpected agentKey " + agentKey)
		}
		return "AGENT_REPLY:" + prompt, nil
	})
	defer SetAgentRunner(nil)

	dsl := `{
	  "ruleChain": {"id": "chain_agent", "name": "agent链", "root": true},
	  "metadata": {
	    "nodes": [
	      {"id": "a1", "type": "agent", "name": "Agent", "configuration": {"agentKey": "agent-general"}}
	    ],
	    "connections": []
	  }
	}`
	res, err := runDSLForTest("chain_agent", []byte(dsl), "JSON", "你好", nil)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("run err: %v", res.Err)
	}
	if !strings.Contains(res.Output, "AGENT_REPLY:你好") {
		t.Fatalf("expected agent reply in output, got %q", res.Output)
	}
}

// 失败路径：runner 返回错误时节点应走 Failure（Err 非空）。
func TestAgentNode_OnMsgFailure(t *testing.T) {
	SetAgentRunner(func(_ context.Context, _, _ string) (string, error) {
		return "", errors.New("boom")
	})
	defer SetAgentRunner(nil)

	dsl := `{
	  "ruleChain": {"id": "chain_agent_fail", "root": true},
	  "metadata": {
	    "nodes": [{"id": "a1", "type": "agent", "configuration": {"agentKey": "agent-general"}}],
	    "connections": []
	  }
	}`
	res, err := runDSLForTest("chain_agent_fail", []byte(dsl), "JSON", "x", nil)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if res.Err == nil {
		t.Fatal("expected failure when runner errors")
	}
}
