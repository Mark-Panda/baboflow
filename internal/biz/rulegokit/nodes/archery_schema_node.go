package nodes

import (
	"context"
	"errors"
	"strings"

	"baboflow/internal/biz/rulegokit/archeryclient"

	"github.com/rulego/rulego"
	"github.com/rulego/rulego/api/types"
	"github.com/rulego/rulego/utils/maps"
)

// ArcherySchemaNodeType 是 archery schema 浏览节点在 DSL / 注册表中的类型标识。
const ArcherySchemaNodeType = "archerySchema"

// schema 浏览的 resource 取值（对应 psql 风格的 \l \dn \dt \d）。
const (
	schemaResDatabases = "databases"
	schemaResSchemas   = "schemas"
	schemaResTables    = "tables"
	schemaResColumns   = "columns"
)

// ArcherySchemaNodeConfiguration archery schema 浏览节点配置。
type ArcherySchemaNodeConfiguration struct {
	// InstanceID 引用的 archery_instance 的 ID（实例=某连接下一个可查询数据源）。
	InstanceID int64 `json:"instanceId" label:"Archery实例" desc:"要浏览的 Archery 实例（在连接管理中同步）" required:"true"`
	// Resource 浏览对象：databases/schemas/tables/columns。
	Resource string `json:"resource" label:"浏览对象" desc:"databases=库 schemas=schema tables=表 columns=字段" required:"true" component:"{\"type\":\"select\",\"options\":[{\"label\":\"databases（库）\",\"value\":\"databases\"},{\"label\":\"schemas（schema）\",\"value\":\"schemas\"},{\"label\":\"tables（表）\",\"value\":\"tables\"},{\"label\":\"columns（字段）\",\"value\":\"columns\"}]}"`
	// DBName 目标库；schemas/tables/columns 需要。留空取消息 dbName。
	DBName string `json:"dbName" label:"数据库" desc:"schemas/tables/columns 必填；留空由上游消息 dbName 指定"`
	// SchemaName 目标 schema；tables/columns 可用。留空取消息 schemaName。
	SchemaName string `json:"schemaName" label:"Schema" desc:"tables/columns 可用；留空由上游消息 schemaName 指定"`
	// TableName 目标表；仅 columns 需要。留空取消息 tableName。
	TableName string `json:"tableName" label:"表名" desc:"仅 columns 需要；留空由上游消息 tableName 指定"`
}

// ArcherySchemaNode 在规则链中浏览 Archery 平台某实例的库/schema/表/字段元数据，
// 结果以 JSON 写回消息，供 AI 先探 schema 再构造查询。
type ArcherySchemaNode struct {
	config ArcherySchemaNodeConfiguration
}

// NewArcherySchemaNode 创建节点原型（注册进 Registry 用）。
func NewArcherySchemaNode() *ArcherySchemaNode { return &ArcherySchemaNode{} }

// Type 返回节点类型标识。
func (n *ArcherySchemaNode) Type() string { return ArcherySchemaNodeType }

// Category 让节点在组件面板归入 "external" 类别。
func (n *ArcherySchemaNode) Category() string { return "external" }

// Desc 提供组件描述：写入 component_meta.Description，进而成为组件 SKILL 与面板说明。
func (n *ArcherySchemaNode) Desc() string {
	return "浏览 Archery（SQL 审核/查询平台）某实例的元数据：列出数据库（databases）、schema（schemas）、表（tables）或字段（columns，对应 \\l \\dn \\dt \\d）。库/schema/表名可来自节点配置或上游消息，结果以 JSON 写回消息。适用于让 AI 先探明库表结构，再用 archeryQuery 节点构造准确查询。"
}

// New 为每条规则链创建新实例（原型模式），保证链间数据隔离。
func (n *ArcherySchemaNode) New() types.Node { return &ArcherySchemaNode{} }

// Init 解析并校验节点配置。
func (n *ArcherySchemaNode) Init(_ types.Config, configuration types.Configuration) error {
	if err := maps.Map2Struct(configuration, &n.config); err != nil {
		return err
	}
	if n.config.InstanceID <= 0 {
		return errors.New("archerySchema 节点缺少必填配置 instanceId")
	}
	switch strings.TrimSpace(n.config.Resource) {
	case schemaResDatabases, schemaResSchemas, schemaResTables, schemaResColumns:
	default:
		return errors.New("archerySchema 节点 resource 必须是 databases/schemas/tables/columns 之一")
	}
	return nil
}

// schemaOut 是写回消息的结构化浏览结果。
type schemaOut struct {
	Resource string   `json:"resource"`
	DBName   string   `json:"dbName,omitempty"`
	Schema   string   `json:"schemaName,omitempty"`
	Table    string   `json:"tableName,omitempty"`
	Items    []string `json:"items"`
	Count    int      `json:"count"`
}

// OnMsg 浏览元数据：成功把结果 JSON 写回消息并走 Success；失败走 Failure。
func (n *ArcherySchemaNode) OnMsg(ctx types.RuleContext, msg types.RuleMsg) {
	resource := strings.TrimSpace(n.config.Resource)
	db := msgParam(msg, "dbName", strings.TrimSpace(n.config.DBName))
	schema := msgParam(msg, "schemaName", strings.TrimSpace(n.config.SchemaName))
	table := msgParam(msg, "tableName", strings.TrimSpace(n.config.TableName))

	var rt archeryclient.ResourceType
	switch resource {
	case schemaResDatabases:
		rt = archeryclient.ResDatabase
	case schemaResSchemas:
		rt = archeryclient.ResSchema
	case schemaResTables:
		rt = archeryclient.ResTable
	case schemaResColumns:
		rt = archeryclient.ResColumn
		if table == "" {
			ctx.TellFailure(msg, errors.New("archerySchema 浏览 columns 需要 tableName（配置或由消息提供）"))
			return
		}
	}

	cli, err := getClient(context.Background(), n.config.InstanceID)
	if err != nil {
		ctx.TellFailure(msg, err)
		return
	}
	items, err := cli.Resource(rt, db, schema, table)
	if err != nil {
		ctx.TellFailure(msg, err)
		return
	}
	if items == nil {
		items = []string{}
	}
	out := schemaOut{
		Resource: resource, DBName: db, Schema: schema, Table: table,
		Items: items, Count: len(items),
	}
	if err := writeJSON(msg, out); err != nil {
		ctx.TellFailure(msg, err)
		return
	}
	ctx.TellSuccess(msg)
}

// Destroy 释放资源（本节点无持有资源）。
func (n *ArcherySchemaNode) Destroy() {}

// 进程启动即把 archery schema 浏览节点注册进全局注册表。
func init() {
	_ = rulego.Registry.Register(NewArcherySchemaNode())
}
