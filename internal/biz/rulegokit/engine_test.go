package rulegokit

import (
	"testing"
)

const validDSL = `{
  "ruleChain": {"id": "chain_test1", "name": "测试链", "root": true},
  "metadata": {
    "nodes": [
      {"id": "n1", "type": "jsTransform", "name": "转换", "configuration": {"jsScript": "return {'msg':msg,'metadata':metadata,'msgType':msgType};"}}
    ],
    "connections": []
  }
}`

func TestValidate_OK(t *testing.T) {
	if err := Validate([]byte(validDSL)); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidate_BadJSON(t *testing.T) {
	if err := Validate([]byte(`{not json`)); err == nil {
		t.Fatal("expected error for bad json")
	}
}

func TestValidate_MissingID(t *testing.T) {
	dsl := `{"ruleChain":{"root":true},"metadata":{"nodes":[],"connections":[]}}`
	if err := Validate([]byte(dsl)); err == nil {
		t.Fatal("expected error for missing ruleChain.id")
	}
}

func TestValidate_UnknownComponent(t *testing.T) {
	dsl := `{
	  "ruleChain": {"id": "c1", "root": true},
	  "metadata": {"nodes": [{"id":"n1","type":"no/such/component","configuration":{}}], "connections": []}
	}`
	if err := Validate([]byte(dsl)); err == nil {
		t.Fatal("expected error for unknown component")
	}
}

func TestValidate_DanglingConnection(t *testing.T) {
	dsl := `{
	  "ruleChain": {"id": "c1", "root": true},
	  "metadata": {
	    "nodes": [{"id":"n1","type":"jsTransform","configuration":{"jsScript":"return msg;"}}],
	    "connections": [{"fromId":"n1","toId":"n_ghost","type":"Success"}]
	  }
	}`
	if err := Validate([]byte(dsl)); err == nil {
		t.Fatal("expected error for dangling connection")
	}
}

func TestRunDSL(t *testing.T) {
	res, err := RunDSL("chain_test1", []byte(validDSL), "JSON", `{"a":1}`, nil)
	if err != nil {
		t.Fatalf("RunDSL error: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	if res.Err != nil {
		t.Fatalf("run err: %v", res.Err)
	}
	if res.Output == "" {
		t.Fatal("empty output")
	}
	t.Logf("output=%s traces=%d", res.Output, len(res.Traces))
}

// IN/OUT 配对：同一节点应合并为一条记录，含输入/输出/耗时/关系。
func TestRunDSL_PairsInOut(t *testing.T) {
	dsl := `{
	  "ruleChain": {"id": "chain_pair", "name": "配对", "root": true},
	  "metadata": {
	    "nodes": [
	      {"id": "t1", "type": "jsTransform", "name": "转换", "configuration": {"jsScript": "return {'msg':{'v':(msg.a||0)+1},'metadata':metadata,'msgType':msgType};"}},
	      {"id": "t2", "type": "jsTransform", "name": "转换2", "configuration": {"jsScript": "return {'msg':{'v':(msg.v||0)*10},'metadata':metadata,'msgType':msgType};"}}
	    ],
	    "connections": [{"fromId":"t1","toId":"t2","type":"Success"}]
	  }
	}`
	res, err := RunDSL("chain_pair", []byte(dsl), "JSON", `{"a":1}`, nil)
	if err != nil {
		t.Fatalf("RunDSL error: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("run err: %v", res.Err)
	}

	byID := map[string]NodeTrace{}
	for _, tr := range res.Traces {
		byID[tr.NodeID] = tr
	}
	t1, ok := byID["t1"]
	if !ok {
		t.Fatalf("expected trace for t1, got %+v", res.Traces)
	}
	if t1.In == "" || t1.Out == "" {
		t.Fatalf("t1 should have In and Out, got In=%q Out=%q", t1.In, t1.Out)
	}
	if t1.RelationType == "" {
		t.Fatalf("t1 should carry relation type, got %q", t1.RelationType)
	}
	if t1.Data != t1.Out {
		t.Fatalf("compat Data should equal Out, got Data=%q Out=%q", t1.Data, t1.Out)
	}
	// 同一节点只产生一条配对记录（t1 出现次数应为 1）。
	count := 0
	for _, tr := range res.Traces {
		if tr.NodeID == "t1" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 merged trace for t1, got %d", count)
	}
}

func TestRunDSL_TraceIncludesMessageAndMetadata(t *testing.T) {
	res, err := RunDSL("chain_trace_payload", []byte(validDSL), "JSON", `{"a":1}`, map[string]string{
		"traceId": "trace-001",
	})
	if err != nil {
		t.Fatalf("RunDSL error: %v", err)
	}
	if len(res.Traces) == 0 {
		t.Fatal("expected node trace")
	}
	trace := res.Traces[0]
	if trace.Input == nil || trace.Output == nil {
		t.Fatalf("trace should include structured input/output, got %+v", trace)
	}
	if trace.Input.Msg != `{"a":1}` {
		t.Fatalf("unexpected input msg: %q", trace.Input.Msg)
	}
	if trace.Input.Metadata["traceId"] != "trace-001" {
		t.Fatalf("unexpected input metadata: %+v", trace.Input.Metadata)
	}
}

func TestRun_NotLoaded(t *testing.T) {
	m := NewManager()
	if _, err := m.Run("nope", "JSON", `{}`, nil); err == nil {
		t.Fatal("expected error for unloaded chain")
	}
}

func TestManagerLoadRunUnload(t *testing.T) {
	m := NewManager()
	if err := m.Load("chain_test1", []byte(validDSL)); err != nil {
		t.Fatalf("load: %v", err)
	}
	res, err := m.Run("chain_test1", "JSON", `{"x":2}`, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("run err: %v", res.Err)
	}
	m.Unload("chain_test1")
	if _, ok := m.Get("chain_test1"); ok {
		t.Fatal("expected unloaded")
	}
}
