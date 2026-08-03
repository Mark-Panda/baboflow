package data

import (
	"fmt"

	"baboflow/internal/conf"
	"baboflow/internal/data/po"

	"gorm.io/gorm"
)

// Migrate 建 vector 扩展 + AutoMigrate 全部表。
// 向量维度由 conf.EmbeddingDim 决定；pgvector-go 用 "vector" 类型，维度在迁移时按配置调整列。
func Migrate(db *gorm.DB, c *conf.Config) error {
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
		return fmt.Errorf("create vector extension: %w", err)
	}
	models := []interface{}{
		&po.AdminUser{},
		&po.Session{},
		&po.LLMProvider{},
		&po.LLMModel{},
		&po.ComponentMeta{},
		&po.Skill{},
		&po.Agent{},
		&po.AgentSubAgent{},
		&po.AgentSession{},
		&po.AgentMessage{},
		&po.Asset{},
		&po.AuditLog{},
		&po.RuleChain{},
		&po.RuleChainVersion{},
		&po.ChainRun{},
		&po.McpServer{},
		&po.McpExposure{},
		&po.Board{},
		&po.BoardColumn{},
		&po.Task{},
		&po.CronJob{},
		&po.ArcheryConnection{},
		&po.ArcheryInstance{},
	}
	if err := db.AutoMigrate(models...); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	// 调整向量列维度（默认 pgvector-go 不定维度；按配置设为 vector(dim)）。
	if c.EmbeddingDim > 0 {
		alter := []string{
			fmt.Sprintf("ALTER TABLE component_meta ALTER COLUMN embedding TYPE vector(%d)", c.EmbeddingDim),
			fmt.Sprintf("ALTER TABLE skill ALTER COLUMN embedding TYPE vector(%d)", c.EmbeddingDim),
		}
		for _, sql := range alter {
			// 忽略错误：若已有数据/维度已匹配可跳过；首次为空表可直接改。
			_ = db.Exec(sql).Error
		}
		// 向量索引（cosine），幂等创建。
		_ = db.Exec("CREATE INDEX IF NOT EXISTS idx_component_meta_embedding ON component_meta USING ivfflat (embedding vector_cosine_ops)").Error
		_ = db.Exec("CREATE INDEX IF NOT EXISTS idx_skill_embedding ON skill USING ivfflat (embedding vector_cosine_ops)").Error
	}
	return nil
}
