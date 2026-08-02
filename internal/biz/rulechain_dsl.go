package biz

import (
	"encoding/json"
)

// skeletonDSL 生成最小合法 RuleGo 规则链骨架。
func skeletonDSL(id, name string) []byte {
	dsl := map[string]interface{}{
		"ruleChain": map[string]interface{}{
			"id":   id,
			"name": name,
			"root": true,
		},
		"metadata": map[string]interface{}{
			"nodes":       []interface{}{},
			"connections": []interface{}{},
		},
	}
	b, _ := json.Marshal(dsl)
	return b
}

// ensureChainID 强制 DSL 顶层 ruleChain.id 与链 id 一致。
func ensureChainID(dsl []byte, id string) []byte {
	var m map[string]interface{}
	if err := json.Unmarshal(dsl, &m); err != nil {
		return dsl
	}
	rc, ok := m["ruleChain"].(map[string]interface{})
	if !ok {
		rc = map[string]interface{}{}
		m["ruleChain"] = rc
	}
	rc["id"] = id
	if _, ok := rc["root"]; !ok {
		rc["root"] = true
	}
	b, err := json.Marshal(m)
	if err != nil {
		return dsl
	}
	return b
}

// withDebugMode 为所有节点开/关 debugMode，便于逐节点事件采集。
func withDebugMode(dsl []byte, on bool) []byte {
	var m map[string]interface{}
	if err := json.Unmarshal(dsl, &m); err != nil {
		return dsl
	}
	md, ok := m["metadata"].(map[string]interface{})
	if !ok {
		return dsl
	}
	nodes, ok := md["nodes"].([]interface{})
	if !ok {
		return dsl
	}
	for _, n := range nodes {
		if node, ok := n.(map[string]interface{}); ok {
			node["debugMode"] = on
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return dsl
	}
	return b
}
