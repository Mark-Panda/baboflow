package data

import (
	"context"
	"time"

	"baboflow/internal/biz"
	"baboflow/internal/data/po"

	"gorm.io/gorm"
)

// ---- BoardDataRepo（biz.BoardDataRepo）----

type boardRepo struct{ db *gorm.DB }

func NewBoardRepo(db *gorm.DB) biz.BoardDataRepo { return &boardRepo{db: db} }

func (r *boardRepo) ListBoards(ctx context.Context) ([]po.Board, error) {
	var list []po.Board
	err := r.db.WithContext(ctx).Where("deleted_at IS NULL").Order("id asc").Find(&list).Error
	return list, err
}

func (r *boardRepo) GetBoard(ctx context.Context, id int64) (*po.Board, error) {
	var b po.Board
	if err := r.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&b).Error; err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *boardRepo) CreateBoard(ctx context.Context, b *po.Board) error {
	return r.db.WithContext(ctx).Create(b).Error
}

func (r *boardRepo) UpdateBoard(ctx context.Context, b *po.Board) error {
	return r.db.WithContext(ctx).Save(b).Error
}

func (r *boardRepo) DeleteBoard(ctx context.Context, id int64) error {
	now := time.Now()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&po.Board{}).Where("id = ?", id).Update("deleted_at", now).Error; err != nil {
			return err
		}
		// 级联删除列 + 任务
		var colIDs []int64
		if err := tx.Model(&po.BoardColumn{}).Where("board_id = ?", id).Pluck("id", &colIDs).Error; err != nil {
			return err
		}
		if len(colIDs) > 0 {
			if err := tx.Where("column_id IN ?", colIDs).Delete(&po.Task{}).Error; err != nil {
				return err
			}
			if err := tx.Where("board_id = ?", id).Delete(&po.BoardColumn{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *boardRepo) ListColumns(ctx context.Context, boardID int64) ([]po.BoardColumn, error) {
	var list []po.BoardColumn
	err := r.db.WithContext(ctx).Where("board_id = ?", boardID).Order("sort asc, id asc").Find(&list).Error
	return list, err
}

func (r *boardRepo) GetColumn(ctx context.Context, id int64) (*po.BoardColumn, error) {
	var c po.BoardColumn
	if err := r.db.WithContext(ctx).First(&c, id).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *boardRepo) CreateColumn(ctx context.Context, c *po.BoardColumn) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *boardRepo) UpdateColumn(ctx context.Context, c *po.BoardColumn) error {
	return r.db.WithContext(ctx).Save(c).Error
}

func (r *boardRepo) DeleteColumn(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("column_id = ?", id).Delete(&po.Task{}).Error; err != nil {
			return err
		}
		return tx.Delete(&po.BoardColumn{}, id).Error
	})
}

func (r *boardRepo) ListTasksByBoard(ctx context.Context, boardID int64) ([]po.Task, error) {
	var list []po.Task
	err := r.db.WithContext(ctx).
		Joins("JOIN board_column ON board_column.id = task.column_id").
		Where("board_column.board_id = ?", boardID).
		Order("task.sort asc, task.id asc").
		Find(&list).Error
	return list, err
}

func (r *boardRepo) GetTask(ctx context.Context, id int64) (*po.Task, error) {
	var t po.Task
	if err := r.db.WithContext(ctx).First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *boardRepo) CreateTask(ctx context.Context, t *po.Task) error {
	return r.db.WithContext(ctx).Create(t).Error
}

func (r *boardRepo) UpdateTask(ctx context.Context, t *po.Task) error {
	return r.db.WithContext(ctx).Save(t).Error
}

func (r *boardRepo) DeleteTask(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&po.Task{}, id).Error
}
