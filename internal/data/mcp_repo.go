package data

import (
	"context"

	"baboflow/internal/biz"
	"baboflow/internal/data/po"

	"gorm.io/gorm"
)

// ---- McpDataRepo（biz.McpDataRepo）----

type mcpRepo struct{ db *gorm.DB }

func NewMcpRepo(db *gorm.DB) biz.McpDataRepo { return &mcpRepo{db: db} }

func (r *mcpRepo) ListServers(ctx context.Context) ([]po.McpServer, error) {
	var list []po.McpServer
	err := r.db.WithContext(ctx).Order("id asc").Find(&list).Error
	return list, err
}

func (r *mcpRepo) GetServer(ctx context.Context, id int64) (*po.McpServer, error) {
	var s po.McpServer
	if err := r.db.WithContext(ctx).First(&s, id).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *mcpRepo) CreateServer(ctx context.Context, s *po.McpServer) error {
	return r.db.WithContext(ctx).Create(s).Error
}

func (r *mcpRepo) UpdateServer(ctx context.Context, s *po.McpServer) error {
	return r.db.WithContext(ctx).Save(s).Error
}

func (r *mcpRepo) DeleteServer(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&po.McpServer{}, id).Error
}

func (r *mcpRepo) ListExposures(ctx context.Context) ([]po.McpExposure, error) {
	var list []po.McpExposure
	err := r.db.WithContext(ctx).Order("id asc").Find(&list).Error
	return list, err
}

func (r *mcpRepo) ListEnabledExposures(ctx context.Context) ([]po.McpExposure, error) {
	var list []po.McpExposure
	err := r.db.WithContext(ctx).Where("enabled = ?", true).Order("id asc").Find(&list).Error
	return list, err
}

func (r *mcpRepo) GetExposure(ctx context.Context, id int64) (*po.McpExposure, error) {
	var e po.McpExposure
	if err := r.db.WithContext(ctx).First(&e, id).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *mcpRepo) GetExposureByTool(ctx context.Context, toolName string) (*po.McpExposure, error) {
	var e po.McpExposure
	if err := r.db.WithContext(ctx).Where("tool_name = ?", toolName).First(&e).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *mcpRepo) CreateExposure(ctx context.Context, e *po.McpExposure) error {
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *mcpRepo) UpdateExposure(ctx context.Context, e *po.McpExposure) error {
	return r.db.WithContext(ctx).Save(e).Error
}

func (r *mcpRepo) DeleteExposure(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&po.McpExposure{}, id).Error
}
