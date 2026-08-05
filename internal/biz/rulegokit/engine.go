// Package rulegokit 封装 RuleGo 引擎池、DSL 校验、运行与调试。
package rulegokit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rulego/rulego"
	"github.com/rulego/rulego/api/types"
	"github.com/rulego/rulego/engine"
)

// Manager 管理已发布规则链的 RuleGo 引擎池。
type Manager struct {
	mu      sync.RWMutex
	engines map[string]types.RuleEngine
}

func NewManager() *Manager {
	return &Manager{engines: make(map[string]types.RuleEngine)}
}

// Validate 校验 DSL 合法性与组件存在性。返回 nil 表示通过。
func Validate(dsl []byte) error {
	var doc struct {
		RuleChain struct {
			ID   string `json:"id"`
			Root bool   `json:"root"`
		} `json:"ruleChain"`
		Metadata struct {
			Nodes []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"nodes"`
			Connections []struct {
				FromID string `json:"fromId"`
				ToID   string `json:"toId"`
			} `json:"connections"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(dsl, &doc); err != nil {
		return fmt.Errorf("DSL 不是合法 JSON: %w", err)
	}
	if doc.RuleChain.ID == "" {
		return errors.New("ruleChain.id 不能为空")
	}
	// 组件存在性校验
	for _, n := range doc.Metadata.Nodes {
		if n.Type == "" {
			return fmt.Errorf("节点 %s 缺少 type", n.ID)
		}
		if _, err := rulego.Registry.NewNode(n.Type); err != nil {
			return fmt.Errorf("节点 %s 引用了不存在的组件类型 %q", n.ID, n.Type)
		}
	}
	// 连接端点校验（无悬空）
	nodeIDs := map[string]bool{}
	for _, n := range doc.Metadata.Nodes {
		nodeIDs[n.ID] = true
	}
	for _, c := range doc.Metadata.Connections {
		if !nodeIDs[c.FromID] {
			return fmt.Errorf("连接 fromId %q 对应的节点不存在", c.FromID)
		}
		if !nodeIDs[c.ToID] {
			return fmt.Errorf("连接 toId %q 对应的节点不存在", c.ToID)
		}
	}
	// 引擎级解析校验（结构/必填配置）。用独立引擎，不进全局池，校验后即停。
	eng, err := engine.NewRuleEngine("validate_"+doc.RuleChain.ID, dsl)
	if err != nil {
		return fmt.Errorf("DSL 解析失败: %w", err)
	}
	eng.Stop(context.Background())
	return nil
}

// Load 加载（或热更新）一个引擎。用独立引擎实例，不进全局池，避免临时调试引擎冲突。
func (m *Manager) Load(chainID string, dsl []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if eng, ok := m.engines[chainID]; ok {
		return eng.ReloadSelf(dsl)
	}
	eng, err := engine.NewRuleEngine(chainID, dsl)
	if err != nil {
		return err
	}
	m.engines[chainID] = eng
	return nil
}

// Unload 卸载引擎（撤销发布）。
func (m *Manager) Unload(chainID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if eng, ok := m.engines[chainID]; ok {
		eng.Stop(context.Background())
		delete(m.engines, chainID)
	}
}

// StopAll 停止并清空引擎池（优雅停机时调用）。
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, eng := range m.engines {
		eng.Stop(context.Background())
		delete(m.engines, id)
	}
}

// Get 获取引擎。
func (m *Manager) Get(chainID string) (types.RuleEngine, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	eng, ok := m.engines[chainID]
	return eng, ok
}

// NodeTrace 逐节点调试事件。
// 一次节点执行 = 一条记录：In 为流入消息、Out 为流出消息、DurationMs 为耗时。
// 同一节点被多次触发（循环/并行）时按触发顺序产生多条。
type NodeTrace struct {
	NodeID       string        `json:"nodeId"`
	FlowType     string        `json:"flowType"`
	RelationType string        `json:"relationType"`
	Data         string        `json:"data"` // 兼容旧字段：等同 Out（无 Out 时为 In）
	In           string        `json:"in,omitempty"`
	Out          string        `json:"out,omitempty"`
	Input        *TraceMessage `json:"input,omitempty"`
	Output       *TraceMessage `json:"output,omitempty"`
	DurationMs   int64         `json:"durationMs,omitempty"`
	Err          string        `json:"err,omitempty"`
}

// TraceMessage 是节点执行时的完整消息快照，包含 msg、metadata 及消息类型。
type TraceMessage struct {
	Msg      string            `json:"msg"`
	Metadata map[string]string `json:"metadata"`
	Type     string            `json:"type"`
	DataType string            `json:"dataType"`
}

// RunResult 一次同步执行的结果。
type RunResult struct {
	Output string      `json:"output"`
	Traces []NodeTrace `json:"traces"`
	Err    error       `json:"-"`
}

// Run 同步执行已加载的规则链，收集 OnDebug 逐节点事件与最终输出。
func (m *Manager) Run(chainID, dataType, data string, metadata map[string]string) (*RunResult, error) {
	eng, ok := m.Get(chainID)
	if !ok {
		return nil, fmt.Errorf("规则链 %s 未加载（未发布）", chainID)
	}
	return runOnEngine(eng, chainID, dataType, data, metadata), nil
}

// RunDSL 直接对一段 DSL 同步执行（用于调试草稿，无需发布）。每次创建独立临时引擎。
func RunDSL(chainID string, dsl []byte, dataType, data string, metadata map[string]string) (*RunResult, error) {
	eng, err := engine.NewRuleEngine(chainID, dsl)
	if err != nil {
		return nil, err
	}
	defer eng.Stop(context.Background())
	return runOnEngine(eng, chainID, dataType, data, metadata), nil
}

func runOnEngine(eng types.RuleEngine, chainID, dataType, data string, metadata map[string]string) *RunResult {
	res := &RunResult{Traces: []NodeTrace{}}
	var mu sync.Mutex
	// 每个节点一条记录的索引（nodeID -> traces 下标），用于 IN/OUT 配对；
	// 节点再次被触发（循环/并行）时改指向下一条新记录。
	open := map[string]int{}
	// 节点 IN 时间戳（nodeID -> 起始时间），配对 OUT 时计算耗时。
	starts := map[string]time.Time{}

	dt := types.JSON
	if dataType != "" {
		dt = types.DataType(dataType)
	}
	md := types.NewMetadata()
	for k, v := range metadata {
		md.PutValue(k, v)
	}
	msg := types.NewMsg(0, "BABO_MSG", dt, md, data)

	eng.OnMsgAndWait(msg,
		types.WithDebugMode(true),
		types.WithOnNodeDebug(func(rcID, flowType, nodeID string, m types.RuleMsg, relationType string, err error) {
			mu.Lock()
			defer mu.Unlock()
			data := m.GetData()
			snapshot := traceMessage(m)
			if flowType == types.In {
				// 新的一次节点执行：追加一条记录，记录输入与起始时间。
				open[nodeID] = len(res.Traces)
				starts[nodeID] = time.Now()
				res.Traces = append(res.Traces, NodeTrace{
					NodeID:   nodeID,
					FlowType: flowType,
					In:       data,
					Data:     data,
					Input:    snapshot,
				})
				return
			}
			// OUT：配对到该节点最近一次 IN 记录，补输出/耗时/关系/错误。
			idx, ok := open[nodeID]
			if !ok {
				// 没有配对的 IN（异常情况）：单独成一条。
				tr := NodeTrace{
					NodeID: nodeID, FlowType: flowType, RelationType: relationType,
					Out: data, Data: data, Output: snapshot,
				}
				if err != nil {
					tr.Err = err.Error()
				}
				res.Traces = append(res.Traces, tr)
				return
			}
			tr := &res.Traces[idx]
			tr.FlowType = flowType
			tr.RelationType = relationType
			tr.Out = data
			tr.Data = data
			tr.Output = snapshot
			if st, has := starts[nodeID]; has {
				tr.DurationMs = time.Since(st).Milliseconds()
			}
			if err != nil {
				tr.Err = err.Error()
			}
		}),
		types.WithOnEnd(func(ctx types.RuleContext, m types.RuleMsg, err error, relationType string) {
			mu.Lock()
			defer mu.Unlock()
			res.Output = m.GetData()
			if err != nil {
				res.Err = err
			}
		}),
	)
	return res
}

func traceMessage(m types.RuleMsg) *TraceMessage {
	metadata := map[string]string{}
	if md := m.GetMetadata(); md != nil {
		metadata = md.Values()
	}
	return &TraceMessage{
		Msg:      m.GetData(),
		Metadata: metadata,
		Type:     m.GetType(),
		DataType: string(m.GetDataType()),
	}
}
