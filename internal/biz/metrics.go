package biz

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics 平台 Prometheus 指标（规则链执行 / MCP 调用 / WS 连接 / Agent token）。
var (
	ChainExecTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "baboflow", Subsystem: "chain", Name: "executions_total",
		Help: "规则链执行次数",
	}, []string{"chainId", "trigger", "status"})

	ChainExecDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "baboflow", Subsystem: "chain", Name: "execution_duration_seconds",
		Help:    "规则链执行耗时",
		Buckets: prometheus.DefBuckets,
	}, []string{"chainId"})

	McpCallTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "baboflow", Subsystem: "mcp", Name: "calls_total",
		Help: "MCP 工具调用次数",
	}, []string{"tool", "status"})

	McpCallDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "baboflow", Subsystem: "mcp", Name: "call_duration_seconds",
		Help:    "MCP 工具调用耗时",
		Buckets: prometheus.DefBuckets,
	}, []string{"tool"})

	WsConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "baboflow", Subsystem: "ws", Name: "connections",
		Help: "当前 WebSocket 连接数",
	})

	AgentTokenTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "baboflow", Subsystem: "agent", Name: "tokens_total",
		Help: "Agent token 用量",
	}, []string{"agentKey", "kind"}) // kind: prompt/completion

	CronFireTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "baboflow", Subsystem: "cron", Name: "fires_total",
		Help: "定时任务触发次数",
	}, []string{"targetType", "status"})
)

var registerOnce sync.Once

// RegisterMetrics 幂等注册全部指标到默认注册表。
func RegisterMetrics() {
	registerOnce.Do(func() {
		prometheus.MustRegister(
			ChainExecTotal, ChainExecDuration,
			McpCallTotal, McpCallDuration,
			WsConnections, AgentTokenTotal, CronFireTotal,
		)
	})
}
