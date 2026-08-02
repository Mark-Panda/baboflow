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
	return seedBuiltinAgents(db)
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
			Key:       "agent-general",
			Name:      "通用助手",
			Instruction: "你是 BaboFlow 通用助手，可回答平台相关问题、编排规则链与 SKILL、调用各类工具完成任务。",
			IsBuiltin: true,
			SkillIDs:  datatypes.JSON([]byte("[]")),
			McpIDs:    datatypes.JSON([]byte("[]")),
			BuiltinTools: datatypes.JSON([]byte(`["bash","read","write","edit","grep"]`)),
		},
		{
			Key:       "agent-chain-builder",
			Name:      "规则链生成器",
			Instruction: "你是规则链生成器。用 ReAct 模式：1) 理解用户需求 2) 调用 search_component 检索可用 RuleGo 组件 3) 与用户逐步确认细节 4) 产出合法 RuleGo 规则链 DSL 并调用 rulechain_validate/rulechain_create。",
			IsBuiltin: true,
			SkillIDs:  datatypes.JSON([]byte("[]")),
			McpIDs:    datatypes.JSON([]byte("[]")),
			BuiltinTools: datatypes.JSON([]byte(`["read","write","edit","grep"]`)),
		},
		{
			Key:       "agent-skill-generator",
			Name:      "SKILL 生成器",
			Instruction: "你是 SKILL 生成器。读取已发布的规则链 DSL，反向生成标准 SKILL.md（name/description/何时使用/输入输出 schema/示例），并调用 skill_create 保存。",
			IsBuiltin: true,
			SkillIDs:  datatypes.JSON([]byte("[]")),
			McpIDs:    datatypes.JSON([]byte("[]")),
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
		}
	}
	return nil
}
