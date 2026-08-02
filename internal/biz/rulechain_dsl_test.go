package biz

import (
	"encoding/json"
	"testing"
)

func TestSkeletonDSL(t *testing.T) {
	b := skeletonDSL("chain_x1", "示例")
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("skeleton not json: %v", err)
	}
	rc, ok := m["ruleChain"].(map[string]interface{})
	if !ok {
		t.Fatal("missing ruleChain")
	}
	if rc["id"] != "chain_x1" {
		t.Fatalf("id mismatch: %v", rc["id"])
	}
	if rc["root"] != true {
		t.Fatal("root should be true")
	}
}

func TestEnsureChainID(t *testing.T) {
	in := []byte(`{"ruleChain":{"id":"old","name":"n"},"metadata":{"nodes":[],"connections":[]}}`)
	out := ensureChainID(in, "chain_new")
	var m map[string]interface{}
	_ = json.Unmarshal(out, &m)
	rc := m["ruleChain"].(map[string]interface{})
	if rc["id"] != "chain_new" {
		t.Fatalf("id not enforced: %v", rc["id"])
	}
	if rc["name"] != "n" {
		t.Fatal("name should be preserved")
	}
	if rc["root"] != true {
		t.Fatal("root should default true")
	}
}

func TestEnsureChainID_InvalidJSON(t *testing.T) {
	in := []byte(`{bad`)
	out := ensureChainID(in, "chain_new")
	if string(out) != string(in) {
		t.Fatal("invalid json should be returned as-is")
	}
}

func TestWithDebugMode(t *testing.T) {
	in := []byte(`{"ruleChain":{"id":"c"},"metadata":{"nodes":[{"id":"n1"},{"id":"n2","debugMode":false}],"connections":[]}}`)
	out := withDebugMode(in, true)
	var m map[string]interface{}
	_ = json.Unmarshal(out, &m)
	nodes := m["metadata"].(map[string]interface{})["nodes"].([]interface{})
	for _, n := range nodes {
		node := n.(map[string]interface{})
		if node["debugMode"] != true {
			t.Fatalf("node %v debugMode not set", node["id"])
		}
	}
}
