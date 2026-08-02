package data

import (
	"fmt"

	"baboflow/internal/conf"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// NewDB 建立 GORM 连接并执行 migration + 种子。
func NewDB(c *conf.Config) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(c.DatabaseDSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	if err := Migrate(db, c); err != nil {
		return nil, err
	}
	if err := Seed(db, c); err != nil {
		return nil, err
	}
	return db, nil
}
