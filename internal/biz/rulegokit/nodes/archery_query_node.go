package nodes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/rulego/rulego"
	"github.com/rulego/rulego/api/types"
	"github.com/rulego/rulego/utils/maps"
)

// ArcheryQueryNodeType 是 archery 查询节点在 DSL / 注册表中的类型标识。
const ArcheryQueryNodeType = "archeryQuery"

// ArcheryQueryNodeConfiguration archery 查询节点配置。
type ArcheryQueryNodeConfiguration struct {
	// InstanceID 引用的 archery_instance 的 ID（实例=某连接下一个可查询数据源）。
	InstanceID int64 `json:"instanceId" label:"Archery实例" desc:"要查询的 Archery 实例（在连接管理中同步）" required:"true"`
	// DBName 目标数据库；留空则取消息元数据/JSON 体的 dbName。
	DBName string `json:"dbName" label:"数据库" desc:"目标数据库名；留空由上游消息 dbName 指定"`
	// SchemaName 目标 schema；留空则取消息 schemaName（Archery 默认按实例默认 schema）。
	SchemaName string `json:"schemaName" label:"Schema" desc:"目标 schema；留空由上游消息 schemaName 指定"`
	// SQL 查询语句；留空则用消息体（或消息 sql 字段）作为 SQL。
	SQL string `json:"sql" label:"SQL" desc:"只读 SELECT；留空使用消息体/消息 sql 字段作为查询"`
	// LimitNum 返回行数上限（对应 Archery limit_num），默认 100。
	LimitNum int `json:"limitNum" label:"行数上限" desc:"默认 100"`
	// TimeoutMs 预留：单次查询超时（毫秒）。0 表示用客户端默认。
	TimeoutMs int `json:"timeoutMs" label:"超时(毫秒)" desc:"0 表示不限制"`
}

// ArcheryQueryNode 在规则链中对 Archery 平台的一个库执行只读 SELECT，
// 并把结构化结果（列/行/耗时/告警）以 JSON 写回消息。
type ArcheryQueryNode struct {
	config ArcheryQueryNodeConfiguration
}

// NewArcheryQueryNode 创建节点原型（注册进 Registry 用）。
func NewArcheryQueryNode() *ArcheryQueryNode { return &ArcheryQueryNode{} }

// Type 返回节点类型标识。
func (n *ArcheryQueryNode) Type() string { return ArcheryQueryNodeType }

// Category 让节点在组件面板归入 "external" 类别。
func (n *ArcheryQueryNode) Category() string { return "external" }

// Desc 提供组件描述：写入 component_meta.Description，进而成为组件 SKILL 与面板说明。
func (n *ArcheryQueryNode) Desc() string {
	return "对 Archery（SQL 审核/查询平台）暴露的数据库执行只读 SELECT 查询：按连接+库/schema 运行 SQL，把列、行、行数、耗时、告警以 JSON 写回消息。SQL 可来自节点配置或上游消息（sql 字段/消息体），适用于在规则链或 MCP 工具中查询业务库数据。仅支持 SELECT。"
}

// New 为每条规则链创建新实例（原型模式），保证链间数据隔离。
func (n *ArcheryQueryNode) New() types.Node { return &ArcheryQueryNode{} }

// Init 解析并校验节点配置。
func (n *ArcheryQueryNode) Init(_ types.Config, configuration types.Configuration) error {
	if err := maps.Map2Struct(configuration, &n.config); err != nil {
		return err
	}
	if n.config.InstanceID <= 0 {
		return errors.New("archeryQuery 节点缺少必填配置 instanceId")
	}
	if n.config.LimitNum <= 0 {
		n.config.LimitNum = 100
	}
	return nil
}

// queryOut 是写回消息的结构化查询结果。
type queryOut struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	RowCount  int      `json:"rowCount"`
	QueryTime float64  `json:"queryTime"`
	Warning   string   `json:"warning,omitempty"`
	Masked    bool     `json:"masked"`
}

// OnMsg 执行查询：成功把结果 JSON 写回消息并走 Success；失败走 Failure。
func (n *ArcheryQueryNode) OnMsg(ctx types.RuleContext, msg types.RuleMsg) {
	sql := msgSQL(msg, strings.TrimSpace(n.config.SQL))
	if sql == "" {
		ctx.TellFailure(msg, errors.New("archeryQuery 节点缺少 SQL（配置 sql 或由消息提供）"))
		return
	}
	db := msgParam(msg, "dbName", strings.TrimSpace(n.config.DBName))
	schema := msgParam(msg, "schemaName", strings.TrimSpace(n.config.SchemaName))

	cli, err := getClient(context.Background(), n.config.InstanceID)
	if err != nil {
		ctx.TellFailure(msg, err)
		return
	}
	res, err := cli.Query(db, schema, sql, n.config.LimitNum)
	if err != nil {
		ctx.TellFailure(msg, fmt.Errorf("archery 查询失败: %w", err))
		return
	}
	if res.Error != nil && *res.Error != "" {
		ctx.TellFailure(msg, fmt.Errorf("archery 查询错误: %s", *res.Error))
		return
	}
	out := queryOut{
		Columns:   res.ColumnList,
		Rows:      res.Rows,
		RowCount:  len(res.Rows),
		QueryTime: res.QueryTime,
		Masked:    res.IsMasked,
	}
	if res.Warning != nil {
		out.Warning = *res.Warning
	}
	if out.Rows == nil {
		out.Rows = [][]any{}
	}
	if err := writeJSON(msg, out); err != nil {
		ctx.TellFailure(msg, err)
		return
	}
	ctx.TellSuccess(msg)
}

// Destroy 释放资源（本节点无持有资源）。
func (n *ArcheryQueryNode) Destroy() {}

// 进程启动即把 archery 查询节点注册进全局注册表，确保 Validate()、RestorePublished、
// component_sync 能发现它（同 agent 节点）。该 init 在任何 DI 之前执行。
func init() {
	_ = rulego.Registry.Register(NewArcheryQueryNode())
}
