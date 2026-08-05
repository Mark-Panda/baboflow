package data

import (
	"baboflow/internal/conf"
	"baboflow/internal/data/po"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Seed 首次启动种子：默认 admin、内置 3 Agent。幂等（已存在则跳过）。
func Seed(db *gorm.DB, c *conf.Config) error {
	if err := seedAdmin(db, c); err != nil {
		return err
	}
	if err := seedBuiltinAgents(db); err != nil {
		return err
	}
	return seedArcheryMCPChain(db)
}

func seedArcheryMCPChain(db *gorm.DB) error {
	const chainID = "chain-archery-mcp-query"
	dsl := datatypes.JSON([]byte(`{
		"ruleChain":{"id":"chain-archery-mcp-query","name":"Archery MCP 查询","root":true},
		"metadata":{"firstNodeIndex":0,"nodes":[
			{"id":"route","type":"switch","name":"按 action 分流","configuration":{"cases":[
				{"case":"msg.action == \"listInstances\"","then":"listInstances"},
				{"case":"msg.action == \"listDatabases\"","then":"listDatabases"},
				{"case":"msg.action == \"listTables\"","then":"listTables"},
				{"case":"msg.action == \"describeTable\"","then":"describeTable"},
				{"case":"msg.action == \"query\"","then":"query"}
			]}},
			{"id":"node_instances","type":"archerySchema","name":"查看实例","configuration":{"resource":"instances"}},
			{"id":"node_databases","type":"archerySchema","name":"查看数据库","configuration":{"resource":"databases"}},
			{"id":"node_tables","type":"archerySchema","name":"查看表","configuration":{"resource":"tables"}},
			{"id":"node_columns","type":"archerySchema","name":"查看表结构","configuration":{"resource":"columns"}},
			{"id":"node_query","type":"archeryQuery","name":"查询表数据","configuration":{"limitNum":100}},
			{"id":"failure","type":"jsTransform","name":"不支持的 action","configuration":{"jsScript":"throw new Error('不支持的 Archery action');"}}
		],"connections":[
			{"fromId":"route","toId":"node_instances","type":"listInstances"},
			{"fromId":"route","toId":"node_databases","type":"listDatabases"},
			{"fromId":"route","toId":"node_tables","type":"listTables"},
			{"fromId":"route","toId":"node_columns","type":"describeTable"},
			{"fromId":"route","toId":"node_query","type":"query"},
			{"fromId":"route","toId":"failure","type":"Default"}
		]}
	}`))
	inputSchema := datatypes.JSON([]byte(`{
		"type":"object",
		"properties":{
			"action":{"type":"string","enum":["listInstances","listDatabases","listTables","describeTable","query"]},
			"instanceId":{"type":"integer","description":"Archery 实例 ID，listInstances 不需要"},
			"dbName":{"type":"string"},
			"schemaName":{"type":"string","description":"Schema 名；PostgreSQL 未传时默认 public"},
			"tableName":{"type":"string"},
			"sql":{"type":"string","description":"只读 SELECT SQL"}
		},
		"allOf":[
			{"if":{"properties":{"action":{"const":"listDatabases"}}},"then":{"required":["instanceId"]}},
			{"if":{"properties":{"action":{"const":"listTables"}}},"then":{"required":["instanceId","dbName"]}},
			{"if":{"properties":{"action":{"const":"describeTable"}}},"then":{"required":["instanceId","dbName","tableName"]}},
			{"if":{"properties":{"action":{"const":"query"}}},"then":{"required":["instanceId","sql"]}}
		],
		"required":["action"]
	}`))
	var chain po.RuleChain
	err := db.Where("id = ?", chainID).First(&chain).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			return err
		}
		chain = po.RuleChain{
			ID: chainID, Name: "Archery MCP 查询",
			Description: "通过 action 查询 Archery 实例、数据库、表、表结构和只读表数据",
			InputSchema: inputSchema, DSL: dsl, Status: "published", Source: "manual",
		}
		if err := db.Create(&chain).Error; err != nil {
			return err
		}
	} else if err := db.Model(&po.RuleChain{}).Where("id = ?", chainID).
		Updates(map[string]any{"status": "published", "input_schema": inputSchema}).Error; err != nil {
		return err
	}
	var exposure po.McpExposure
	err = db.Where("tool_name = ?", "archery_mcp_query").Limit(1).Find(&exposure).Error
	if err != nil {
		return err
	}
	if exposure.ID == 0 {
		return db.Create(&po.McpExposure{
			ChainID: chainID, ToolName: "archery_mcp_query",
			Description: "通过 Archery API 查询实例、数据库、表、表结构和只读表数据",
			InputSchema: inputSchema, Enabled: true,
		}).Error
	}
	return db.Model(&po.McpExposure{}).Where("id = ?", exposure.ID).
		Updates(map[string]any{"chain_id": chainID, "input_schema": inputSchema}).Error
}

func seedAdmin(db *gorm.DB, c *conf.Config) error {
	var count int64
	if err := db.Model(&po.AdminUser{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(c.AdminInitPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	admin := po.AdminUser{
		Username:      "admin",
		PasswordHash:  string(hash),
		DisplayName:   "管理员",
		MustChangePwd: true, // 初始密码登录后强制改密
	}
	return db.Create(&admin).Error
}

func seedBuiltinAgents(db *gorm.DB) error {
	builtins := []po.Agent{
		{
			Key:          "agent-general",
			Name:         "通用助手",
			Instruction:  "你是 BaboFlow 通用助手，可回答平台相关问题、编排规则链与 SKILL、调用各类工具完成任务。",
			IsBuiltin:    true,
			SkillIDs:     datatypes.JSON([]byte("[]")),
			McpIDs:       datatypes.JSON([]byte("[]")),
			BuiltinTools: datatypes.JSON([]byte(`["bash","read","write","edit","grep"]`)),
		},
		{
			Key:  "agent-chain-builder",
			Name: "规则链生成器",
			Instruction: "你是规则链生成器，在用户的当前画布上增量生成/编辑规则链。工作方式(ReAct)：\n" +
				"1) 用户消息里若有 <current_canvas_dsl>...</current_canvas_dsl>，那是当前画布已有的完整 DSL；在其基础上增删改，保留无关节点与连线，输出完整新 DSL。为空表示空画布，从零生成。\n" +
				"2) 需要选组件时调用 search_component 检索(返回 type/配置 schema/DSL 示例)，不要臆造组件 type。\n" +
				"3) 信息不足时必须调用 ask_user 提出结构化问题；调用后立即结束本轮，不要继续调用 apply_chain_dsl，等待用户回答。\n" +
				"4) 只有用户回答后才能继续；需求明确后产出 DSL，并先调用 rulechain_validate 校验；失败就修正后重校，直到通过。\n" +
				"5) 校验通过后调用 apply_chain_dsl 把完整 DSL 应用到画布(不要调用 rulechain_create，本工具不落库)。apply_chain_dsl 的 dsl 入参必须是完整 DSL。\n" +
				"6) 容器节点(for/flow)的子链放 subChain；连线 type 用组件支持的 relationType(如 Success/Failure)。\n" +
				"节点 id 用可读短名并保证唯一；连线三元组(fromId,toId,type)不重复。",
			IsBuiltin:    true,
			SkillIDs:     datatypes.JSON([]byte("[]")),
			McpIDs:       datatypes.JSON([]byte("[]")),
			BuiltinTools: datatypes.JSON([]byte(`["read","grep"]`)),
		},
		{
			Key:          "agent-skill-generator",
			Name:         "SKILL 生成器",
			Instruction:  "你是 SKILL 生成器。读取已发布的规则链 DSL，反向生成标准 SKILL.md（name/description/何时使用/输入输出 schema/示例）。只输出完整 SKILL.md，不要调用 skill_create 或其他保存工具，系统会在校验后统一保存。",
			IsBuiltin:    true,
			SkillIDs:     datatypes.JSON([]byte("[]")),
			McpIDs:       datatypes.JSON([]byte("[]")),
			BuiltinTools: datatypes.JSON([]byte(`["read","write","edit","grep"]`)),
		},
	}
	for _, a := range builtins {
		var count int64
		if err := db.Model(&po.Agent{}).Where(`"key" = ?`, a.Key).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err := db.Create(&a).Error; err != nil {
				return err
			}
		} else if a.Key == "agent-skill-generator" {
			// 修正历史种子中的旧指令，避免 Agent 先调用 skill_create 导致
			// 外层反生成接口报错但数据库已有半成品 SKILL。
			if err := db.Model(&po.Agent{}).Where(`"key" = ?`, a.Key).
				Update("instruction", a.Instruction).Error; err != nil {
				return err
			}
		} else if a.Key == "agent-chain-builder" {
			// 修正历史种子：旧版规则链生成器用 rulechain_create 落库新建，
			// 新版改为 apply_chain_dsl 把 DSL 回传当前画布，需同步指令与内置工具集。
			// Updates 触发 UpdatedAt 变更，AgentManager 缓存随之失效重建。
			if err := db.Model(&po.Agent{}).Where(`"key" = ?`, a.Key).
				Updates(map[string]any{"instruction": a.Instruction, "builtin_tools": a.BuiltinTools}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
