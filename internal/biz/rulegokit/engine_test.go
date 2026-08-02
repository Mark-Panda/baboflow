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
